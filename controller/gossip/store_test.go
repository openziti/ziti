/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package gossip

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// --- mock mesh ---

type mockMesh struct {
	peers    []string
	sent     []sentMessage
	mu       sync.Mutex
	handlers map[int32]channel.TypedReceiveHandler
}

type sentMessage struct {
	peerId string
	msg    *channel.Message
}

func newMockMesh(peers ...string) *mockMesh {
	return &mockMesh{
		peers:    peers,
		handlers: make(map[int32]channel.TypedReceiveHandler),
	}
}

func (m *mockMesh) PeerIds() []string {
	return m.peers
}

func (m *mockMesh) PeerIdForChannel(channel.Channel) string {
	return ""
}

func (m *mockMesh) Send(peerId string, msg *channel.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMessage{peerId: peerId, msg: msg})
	return nil
}

func (m *mockMesh) Broadcast(msg *channel.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.peers {
		m.sent = append(m.sent, sentMessage{peerId: p, msg: msg})
	}
}

func (m *mockMesh) getSent() []sentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]sentMessage, len(m.sent))
	copy(result, m.sent)
	return result
}

func (m *mockMesh) clearSent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = nil
}

// --- mock listener ---

type changeEvent struct {
	key      string
	value    string
	version  uint64
	owner    string
	isCreate bool
	origin   ChangeOrigin
}

type removeEvent struct {
	key    string
	owner  string
	origin ChangeOrigin
}

type mockListener struct {
	changes []changeEvent
	removes []removeEvent
	mu      sync.Mutex
}

func (l *mockListener) EntryChanged(key string, value string, version uint64, owner string, isCreate bool, origin ChangeOrigin) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.changes = append(l.changes, changeEvent{key, value, version, owner, isCreate, origin})
}

func (l *mockListener) EntryRemoved(key string, owner string, origin ChangeOrigin) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.removes = append(l.removes, removeEvent{key, owner, origin})
}

func (l *mockListener) getChanges() []changeEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]changeEvent, len(l.changes))
	copy(result, l.changes)
	return result
}

func (l *mockListener) getRemoves() []removeEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]removeEvent, len(l.removes))
	copy(result, l.removes)
	return result
}

func encodeString(s string) ([]byte, error) {
	return json.Marshal(s)
}

func decodeString(b []byte) (string, error) {
	var s string
	err := json.Unmarshal(b, &s)
	return s, err
}

func newTestStore(peers ...string) (*Store, *mockMesh) {
	mesh := newMockMesh(peers...)
	store := NewStore("local", mesh)
	return store, mesh
}

func newTestStateType(store *Store, listener *mockListener) *StateType[string] {
	cfg := StateTypeConfig[string]{
		Name:         "test",
		Encode:       encodeString,
		Decode:       decodeString,
		Tombstones:   true,
		TombstoneTTL: time.Minute,
	}
	if listener != nil {
		cfg.Listener = listener
	}
	return Register[string](store, cfg)
}

// --- tests ---

func TestLamportClockMonotonicity(t *testing.T) {
	store, _ := newTestStore()
	defer store.Stop()

	var wg sync.WaitGroup
	var maxSeen atomic.Uint64

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				v := store.nextVersion()
				for {
					cur := maxSeen.Load()
					if v <= cur {
						break
					}
					if maxSeen.CompareAndSwap(cur, v) {
						break
					}
				}
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, uint64(10000), store.clock.Load())
}

func TestObserveVersion(t *testing.T) {
	store, _ := newTestStore()
	defer store.Stop()

	store.nextVersion() // 1
	store.nextVersion() // 2

	store.observeVersion(100)
	assert.Equal(t, uint64(100), store.clock.Load())

	// observing a lower version should not change clock
	store.observeVersion(50)
	assert.Equal(t, uint64(100), store.clock.Load())

	v := store.nextVersion()
	assert.Equal(t, uint64(101), v)
}

func TestVersionMerge_NewerWins(t *testing.T) {
	store, _ := newTestStore("peer1")
	defer store.Stop()

	listener := &mockListener{}
	st := newTestStateType(store, listener)

	require.NoError(t, st.Set("key1", "owner1", "value1"))

	// Apply a delta with a higher version
	sm := store.getStateMap("test")
	sm.applyDelta(&entry{
		Key:     "key1",
		Value:   mustEncode("value2"),
		Version: 999,
		Owner:   "owner1",
	})

	val, _, ok := st.Get("key1")
	require.True(t, ok)
	assert.Equal(t, "value2", val)
}

func TestVersionMerge_EqualNoOp(t *testing.T) {
	store, _ := newTestStore("peer1")
	defer store.Stop()

	listener := &mockListener{}
	st := newTestStateType(store, listener)

	require.NoError(t, st.Set("key1", "owner1", "value1"))

	sm := store.getStateMap("test")
	currentVersion := store.clock.Load()

	changesBefore := len(listener.getChanges())

	// Apply delta with same version — should be no-op
	sm.applyDelta(&entry{
		Key:     "key1",
		Value:   mustEncode("value2"),
		Version: currentVersion,
		Owner:   "owner1",
	})

	// listener should not have fired again
	assert.Equal(t, changesBefore, len(listener.getChanges()))

	val, _, ok := st.Get("key1")
	require.True(t, ok)
	assert.Equal(t, "value1", val)
}

func TestVersionMerge_OlderIgnored(t *testing.T) {
	store, _ := newTestStore("peer1")
	defer store.Stop()

	listener := &mockListener{}
	st := newTestStateType(store, listener)

	// Bump clock high
	store.observeVersion(100)
	require.NoError(t, st.Set("key1", "owner1", "value1"))

	sm := store.getStateMap("test")
	changesBefore := len(listener.getChanges())

	// Apply delta with a lower version
	sm.applyDelta(&entry{
		Key:     "key1",
		Value:   mustEncode("old_value"),
		Version: 1,
		Owner:   "owner1",
	})

	assert.Equal(t, changesBefore, len(listener.getChanges()))

	val, _, ok := st.Get("key1")
	require.True(t, ok)
	assert.Equal(t, "value1", val)
}

func TestTombstoneLifecycle(t *testing.T) {
	store, _ := newTestStore("peer1")
	defer store.Stop()

	listener := &mockListener{}
	st := Register[string](store, StateTypeConfig[string]{
		Name:         "test",
		Encode:       encodeString,
		Decode:       decodeString,
		Tombstones:   true,
		TombstoneTTL: 50 * time.Millisecond,
		Listener:     listener,
	})

	require.NoError(t, st.Set("key1", "owner1", "value1"))
	_, _, ok := st.Get("key1")
	require.True(t, ok)

	st.Delete("key1", "owner1")
	_, _, ok = st.Get("key1")
	assert.False(t, ok, "tombstoned entry should not be visible via Get")

	// The tombstone should exist in the internal map
	sm := store.getStateMap("test")
	e, exists := sm.entries.Get("key1")
	require.True(t, exists)
	assert.True(t, e.Tombstone)

	// After TTL, reaping should remove it
	time.Sleep(60 * time.Millisecond)
	sm.reapTombstones()

	_, exists = sm.entries.Get("key1")
	assert.False(t, exists, "tombstone should be reaped after TTL")
}

func TestAntiEntropyDigest(t *testing.T) {
	store, _ := newTestStore("peer1")
	defer store.Stop()

	st := newTestStateType(store, nil)
	require.NoError(t, st.Set("a", "o1", "v1"))
	require.NoError(t, st.Set("b", "o1", "v2"))

	sm := store.getStateMap("test")
	digest := sm.getDigest()
	assert.Len(t, digest, 2)

	versions := map[string]uint64{}
	for _, d := range digest {
		versions[d.Key] = d.Version
	}
	assert.Contains(t, versions, "a")
	assert.Contains(t, versions, "b")
}

func TestAntiEntropyReconciliation(t *testing.T) {
	// Store A has key1, Store B has key1 and key2
	storeA, meshA := newTestStore("B")
	defer storeA.Stop()

	storeB, _ := newTestStore("A")
	defer storeB.Stop()

	stA := newTestStateType(storeA, nil)
	stB := newTestStateType(storeB, nil)

	require.NoError(t, stA.Set("key1", "o1", "v1"))
	require.NoError(t, stB.Set("key1", "o1", "v1"))
	require.NoError(t, stB.Set("key2", "o1", "v2"))

	// Store A sends its digest to Store B
	smA := storeA.getStateMap("test")
	digestEntries := smA.getDigest()

	// Store B processes the digest and finds key2 is missing from A
	smB := storeB.getStateMap("test")
	remoteVersions := map[string]uint64{}
	for _, de := range digestEntries {
		remoteVersions[de.Key] = de.Version
	}

	var needed []*entry
	smB.entries.IterCb(func(key string, e *entry) {
		rv, exists := remoteVersions[key]
		if !exists || e.Version > rv {
			needed = append(needed, e)
		}
	})

	// Apply the missing entries to store A
	for _, e := range needed {
		smA.applyDelta(e)
	}

	// Store A should now have key2
	val, _, ok := stA.Get("key2")
	require.True(t, ok)
	assert.Equal(t, "v2", val)

	// Verify broadcast happened for the original set
	sent := meshA.getSent()
	assert.NotEmpty(t, sent)
}

func TestConfirmedPropagation(t *testing.T) {
	store, mesh := newTestStore("peer1", "peer2")
	defer store.Stop()

	st := newTestStateType(store, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- st.SetConfirmed(ctx, "key1", "owner1", "confirmed_value")
	}()

	// Wait for the broadcast to happen
	time.Sleep(50 * time.Millisecond)

	// Simulate acks from both peers
	store.resolveAck("peer1", findRequestId(t, mesh))
	store.resolveAck("peer2", findRequestId(t, mesh))

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("SetConfirmed did not complete in time")
	}

	val, _, ok := st.Get("key1")
	require.True(t, ok)
	assert.Equal(t, "confirmed_value", val)
}

func TestConfirmedPropagation_Timeout(t *testing.T) {
	store, _ := newTestStore("peer1")
	defer store.Stop()

	st := newTestStateType(store, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := st.SetConfirmed(ctx, "key1", "owner1", "val")
	assert.Error(t, err, "should timeout waiting for ack")
}

func TestReconcile(t *testing.T) {
	store, _ := newTestStore()
	defer store.Stop()

	listener := &mockListener{}
	st := newTestStateType(store, listener)

	require.NoError(t, st.Set("a", "owner1", "v1"))
	require.NoError(t, st.Set("b", "owner1", "v2"))
	require.NoError(t, st.Set("c", "owner2", "v3"))

	// Reconcile owner1 with only key "a" present
	st.Reconcile("owner1", map[string]struct{}{"a": {}})

	_, _, ok := st.Get("a")
	assert.True(t, ok, "key a should still exist")

	_, _, ok = st.Get("b")
	assert.False(t, ok, "key b should be removed/tombstoned")

	// key c belongs to owner2 and should be unaffected
	_, _, ok = st.Get("c")
	assert.True(t, ok, "key c should still exist")
}

func TestDeleteByOwner(t *testing.T) {
	store, _ := newTestStore()
	defer store.Stop()

	listener := &mockListener{}
	st := newTestStateType(store, listener)

	require.NoError(t, st.Set("a", "owner1", "v1"))
	require.NoError(t, st.Set("b", "owner1", "v2"))
	require.NoError(t, st.Set("c", "owner2", "v3"))

	st.DeleteByOwner("owner1")

	_, _, ok := st.Get("a")
	assert.False(t, ok)
	_, _, ok = st.Get("b")
	assert.False(t, ok)
	_, _, ok = st.Get("c")
	assert.True(t, ok)

	removes := listener.getRemoves()
	ownerRemoves := 0
	for _, r := range removes {
		if r.owner == "owner1" {
			ownerRemoves++
		}
	}
	assert.Equal(t, 2, ownerRemoves)
}

func TestListenerNotifications(t *testing.T) {
	store, _ := newTestStore()
	defer store.Stop()

	listener := &mockListener{}
	st := newTestStateType(store, listener)

	require.NoError(t, st.Set("k1", "o1", "v1"))

	changes := listener.getChanges()
	require.Len(t, changes, 1)
	assert.True(t, changes[0].isCreate)
	assert.Equal(t, OriginLocal, changes[0].origin)
	assert.Equal(t, "v1", changes[0].value)

	// Update existing
	require.NoError(t, st.Set("k1", "o1", "v2"))
	changes = listener.getChanges()
	require.Len(t, changes, 2)
	assert.False(t, changes[1].isCreate)

	// Gossip-origin delta
	sm := store.getStateMap("test")
	sm.applyDelta(&entry{
		Key:     "k2",
		Value:   mustEncode("remote_val"),
		Version: 9999,
		Owner:   "o2",
	})

	changes = listener.getChanges()
	require.Len(t, changes, 3)
	assert.True(t, changes[2].isCreate)
	assert.Equal(t, OriginGossip, changes[2].origin)
}

func TestIterByOwner(t *testing.T) {
	store, _ := newTestStore()
	defer store.Stop()

	st := newTestStateType(store, nil)
	require.NoError(t, st.Set("a", "owner1", "v1"))
	require.NoError(t, st.Set("b", "owner2", "v2"))
	require.NoError(t, st.Set("c", "owner1", "v3"))

	var keys []string
	st.IterByOwner("owner1", func(key string, value string) {
		keys = append(keys, key)
	})

	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "a")
	assert.Contains(t, keys, "c")
}

func TestPeerConnectedSnapshot(t *testing.T) {
	store, mesh := newTestStore("peer1")
	defer store.Stop()

	st := newTestStateType(store, nil)
	require.NoError(t, st.Set("k1", "o1", "v1"))
	require.NoError(t, st.Set("k2", "o1", "v2"))

	mesh.clearSent()

	store.PeerConnected("peer1")

	sent := mesh.getSent()
	require.NotEmpty(t, sent)

	// All sent messages should be deltas to peer1
	for _, s := range sent {
		assert.Equal(t, "peer1", s.peerId)
		assert.Equal(t, gossip_pb.GossipDeltaType, s.msg.ContentType)
	}
}

func TestPeerDisconnectedCleansUpAcks(t *testing.T) {
	store, mesh := newTestStore("peer1")
	defer store.Stop()

	st := newTestStateType(store, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- st.SetConfirmed(ctx, "key1", "owner1", "val")
	}()

	time.Sleep(50 * time.Millisecond)

	_ = mesh // ensure broadcast happened
	store.PeerDisconnected("peer1")

	select {
	case err := <-done:
		assert.NoError(t, err, "PeerDisconnected should resolve pending acks")
	case <-time.After(time.Second):
		t.Fatal("SetConfirmed did not complete after PeerDisconnected")
	}
}

func TestApplyAndBroadcast(t *testing.T) {
	store, mesh := newTestStore("peer1", "peer2")
	defer store.Stop()

	listener := &mockListener{}
	_ = newTestStateType(store, listener)
	sm := store.getStateMap("test")

	// Apply an externally-versioned entry (as if from a router)
	sm.applyAndBroadcast(&entry{
		Key:     "link1",
		Value:   mustEncode("linkdata1"),
		Version: 42,
		Owner:   "routerA",
	})

	// Entry should be stored
	e, ok := sm.entries.Get("link1")
	require.True(t, ok)
	assert.Equal(t, uint64(42), e.Version)
	assert.Equal(t, "routerA", e.Owner)

	// Listener should be notified with OriginLocal
	changes := listener.getChanges()
	require.Len(t, changes, 1)
	assert.Equal(t, "link1", changes[0].key)
	assert.Equal(t, OriginLocal, changes[0].origin)
	assert.True(t, changes[0].isCreate)

	// Should have been broadcast to both peers
	sent := mesh.getSent()
	assert.Len(t, sent, 2)

	// Clock should have advanced past 42
	assert.GreaterOrEqual(t, store.clock.Load(), uint64(42))

	// Stale entry should be rejected
	mesh.clearSent()
	sm.applyAndBroadcast(&entry{
		Key:     "link1",
		Value:   mustEncode("old_data"),
		Version: 10,
		Owner:   "routerA",
	})

	// No new listener events
	assert.Len(t, listener.getChanges(), 1)
	// No broadcast
	assert.Empty(t, mesh.getSent())
	// Value unchanged
	e, _ = sm.entries.Get("link1")
	assert.Equal(t, uint64(42), e.Version)
}

func TestApplyAndBroadcast_Tombstone(t *testing.T) {
	store, mesh := newTestStore("peer1")
	defer store.Stop()

	listener := &mockListener{}
	_ = newTestStateType(store, listener)
	sm := store.getStateMap("test")

	// Create entry
	sm.applyAndBroadcast(&entry{
		Key:     "link1",
		Value:   mustEncode("data"),
		Version: 10,
		Owner:   "routerA",
	})

	mesh.clearSent()

	// Tombstone it
	sm.applyAndBroadcast(&entry{
		Key:       "link1",
		Version:   11,
		Owner:     "routerA",
		Tombstone: true,
	})

	// Listener should have received a remove
	removes := listener.getRemoves()
	require.Len(t, removes, 1)
	assert.Equal(t, "link1", removes[0].key)
	assert.Equal(t, OriginLocal, removes[0].origin)

	// Should have broadcast the tombstone
	assert.NotEmpty(t, mesh.getSent())
}

func TestGetDigestForOwner(t *testing.T) {
	store, _ := newTestStore()
	defer store.Stop()

	st := newTestStateType(store, nil)
	require.NoError(t, st.Set("a", "owner1", "v1"))
	require.NoError(t, st.Set("b", "owner2", "v2"))
	require.NoError(t, st.Set("c", "owner1", "v3"))

	digest := store.GetDigestForOwner("test", "owner1")
	assert.Len(t, digest, 2)

	keys := map[string]bool{}
	for _, d := range digest {
		keys[d.Key] = true
	}
	assert.True(t, keys["a"])
	assert.True(t, keys["c"])
	assert.False(t, keys["b"])

	// Unknown store type returns nil
	assert.Nil(t, store.GetDigestForOwner("nonexistent", "owner1"))
}

// helpers

func mustEncode(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}

func findRequestId(t *testing.T, mesh *mockMesh) string {
	t.Helper()
	for _, s := range mesh.getSent() {
		if s.msg.ContentType == gossip_pb.GossipDeltaType {
			delta := &gossip_pb.GossipDelta{}
			if err := proto.Unmarshal(s.msg.Body, delta); err == nil && delta.RequestId != "" {
				return delta.RequestId
			}
		}
	}
	t.Fatal("no request ID found in sent messages")
	return ""
}
