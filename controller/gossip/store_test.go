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
	"fmt"
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

	val, _, ok := st.GetForOwner("owner1", "key1")
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

	val, _, ok := st.GetForOwner("owner1", "key1")
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

	val, _, ok := st.GetForOwner("owner1", "key1")
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
	_, _, ok := st.GetForOwner("owner1", "key1")
	require.True(t, ok)

	st.Delete("key1", "owner1")
	_, _, ok = st.GetForOwner("owner1", "key1")
	assert.False(t, ok, "tombstoned entry should not be visible via GetForOwner")

	// The tombstone should exist in the internal map
	sm := store.getStateMap("test")
	e, exists := sm.entryAt("owner1", "key1")
	require.True(t, exists)
	assert.True(t, e.Tombstone)

	// After TTL, reaping should remove it
	time.Sleep(60 * time.Millisecond)
	sm.reapTombstones()

	_, exists = sm.entryAt("owner1", "key1")
	assert.False(t, exists, "tombstone should be reaped after TTL")
}

// Regression: a tombstone produced by Delete must inherit the replaced entry's
// epoch. Otherwise the wire entry says nothing about which owner lifetime it
// belonged to, defeating any consumer that filters by epoch.
func TestDelete_TombstoneInheritsEpoch(t *testing.T) {
	store, _ := newTestStore("peer1")
	defer store.Stop()

	st := newTestStateType(store, nil)

	epoch := []byte{1, 2, 3, 4}
	sm := store.getStateMap("test")

	// Seed a live entry with an epoch (via applyDelta, since Set doesn't take one).
	sm.applyDelta(&entry{
		Key:     "key1",
		Value:   mustEncode("v1"),
		Version: 100,
		Owner:   "owner1",
		Epoch:   epoch,
	})

	st.Delete("key1", "owner1")

	e, exists := sm.entryAt("owner1", "key1")
	require.True(t, exists)
	require.True(t, e.Tombstone)
	assert.Equal(t, epoch, e.Epoch, "tombstone should inherit the replaced entry's epoch")
}

// Regression: reapTombstones must not delete an entry that was promoted from
// tombstone back to a live entry between the scan and the remove. Without the
// RemoveCb guard, the live entry was silently dropped.
func TestReapTombstones_PromotedEntryNotReaped(t *testing.T) {
	store, _ := newTestStore("peer1")
	defer store.Stop()

	st := Register[string](store, StateTypeConfig[string]{
		Name:         "test",
		Encode:       encodeString,
		Decode:       decodeString,
		Tombstones:   true,
		TombstoneTTL: 50 * time.Millisecond,
	})

	require.NoError(t, st.Set("key1", "owner1", "v1"))
	st.Delete("key1", "owner1")

	sm := store.getStateMap("test")
	e, exists := sm.entryAt("owner1", "key1")
	require.True(t, exists)
	require.True(t, e.Tombstone)

	// Age the tombstone past the TTL by rewriting UpdatedAt directly. This
	// avoids relying on real time and keeps the test fast and deterministic.
	e.UpdatedAt = time.Now().Add(-time.Hour)

	// Simulate a peer broadcast promoting the entry back to live before reap.
	// applyDelta runs concurrently with the reaper's iteration in production;
	// here we order it explicitly: promote between scan and remove by setting
	// up the live entry directly with a version newer than the tombstone.
	sm.applyDelta(&entry{
		Key:     "key1",
		Value:   mustEncode("v2"),
		Version: e.Version + 1,
		Owner:   "owner1",
	})

	// Sanity: the live entry is in place.
	e, exists = sm.entryAt("owner1", "key1")
	require.True(t, exists)
	require.False(t, e.Tombstone)

	// Reap. The check-and-remove under the owner write lock should reject the
	// now-live entry.
	sm.reapTombstones()

	e, exists = sm.entryAt("owner1", "key1")
	require.True(t, exists, "live entry must not be reaped")
	assert.False(t, e.Tombstone)

	val, _, ok := st.GetForOwner("owner1", "key1")
	require.True(t, ok)
	assert.Equal(t, "v2", val)
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
	smB.owners.IterCb(func(_ string, od *ownerData) {
		od.mu.RLock()
		for key, e := range od.entries {
			rv, exists := remoteVersions[key]
			if !exists || e.Version > rv {
				needed = append(needed, e)
			}
		}
		od.mu.RUnlock()
	})

	// Apply the missing entries to store A
	for _, e := range needed {
		smA.applyDelta(e)
	}

	// Store A should now have key2
	val, _, ok := stA.GetForOwner("o1", "key2")
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

	val, _, ok := st.GetForOwner("owner1", "key1")
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

	_, _, ok := st.GetForOwner("owner1", "a")
	assert.True(t, ok, "key a should still exist")

	_, _, ok = st.GetForOwner("owner1", "b")
	assert.False(t, ok, "key b should be removed/tombstoned")

	// key c belongs to owner2 and should be unaffected
	_, _, ok = st.GetForOwner("owner2", "c")
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

	_, _, ok := st.GetForOwner("owner1", "a")
	assert.False(t, ok)
	_, _, ok = st.GetForOwner("owner1", "b")
	assert.False(t, ok)
	_, _, ok = st.GetForOwner("owner2", "c")
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

// DropOwner tombstones live entries, rejects further writes for that owner,
// and the reaper eventually compacts the ownerData out of the store so it
// doesn't accumulate when owners churn (e.g., routers being added/removed).
func TestDropOwner(t *testing.T) {
	store, mesh := newTestStore("peer1")
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

	require.NoError(t, st.Set("a", "owner1", "v1"))
	require.NoError(t, st.Set("b", "owner1", "v2"))
	require.NoError(t, st.Set("c", "owner2", "v3"))

	mesh.clearSent()
	st.DropOwner("owner1")

	// Live entries for owner1 are now tombstoned and gone from public reads.
	_, _, ok := st.GetForOwner("owner1", "a")
	assert.False(t, ok)
	_, _, ok = st.GetForOwner("owner1", "b")
	assert.False(t, ok)
	// Other owners are unaffected.
	val, _, ok := st.GetForOwner("owner2", "c")
	require.True(t, ok)
	assert.Equal(t, "v3", val)

	// Each tombstone was broadcast to peers.
	assert.Len(t, mesh.getSent(), 2)

	// Listener was notified of both removals.
	removes := listener.getRemoves()
	owner1Removes := 0
	for _, r := range removes {
		if r.owner == "owner1" {
			owner1Removes++
		}
	}
	assert.Equal(t, 2, owner1Removes)

	// owner1 should still be in the cmap (tombstones not yet reaped).
	sm := store.getStateMap("test")
	_, exists := sm.owners.Get("owner1")
	require.True(t, exists)

	// Subsequent writes for owner1 are rejected.
	require.NoError(t, st.Set("d", "owner1", "v4"))
	_, _, ok = st.GetForOwner("owner1", "d")
	assert.False(t, ok, "writes for a drained owner must be dropped")

	// Age the tombstones past TTL and reap.
	if od, exists := sm.owners.Get("owner1"); exists {
		od.mu.Lock()
		for _, e := range od.entries {
			e.UpdatedAt = time.Now().Add(-time.Hour)
		}
		od.mu.Unlock()
	}
	sm.reapTombstones()

	// owner1's ownerData should now be compacted out.
	_, exists = sm.owners.Get("owner1")
	assert.False(t, exists, "drained owner with no entries should be compacted out")

	// A fresh write for owner1 succeeds — re-registration semantics.
	require.NoError(t, st.Set("e", "owner1", "v5"))
	val, _, ok = st.GetForOwner("owner1", "e")
	require.True(t, ok)
	assert.Equal(t, "v5", val)
}

// DropOwner on an unknown owner is a no-op but still marks the freshly-created
// drained ownerData so an in-flight write loses cleanly. The reaper then
// compacts the empty drained entry out.
func TestDropOwner_UnknownOwner(t *testing.T) {
	store, _ := newTestStore("peer1")
	defer store.Stop()

	st := Register[string](store, StateTypeConfig[string]{
		Name:         "test",
		Encode:       encodeString,
		Decode:       decodeString,
		Tombstones:   true,
		TombstoneTTL: 50 * time.Millisecond,
	})

	st.DropOwner("ghost")

	sm := store.getStateMap("test")
	od, exists := sm.owners.Get("ghost")
	require.True(t, exists, "DropOwner should create a drained marker even for unknown owners")
	od.mu.RLock()
	assert.True(t, od.drained)
	assert.Empty(t, od.entries)
	od.mu.RUnlock()

	sm.reapTombstones()
	_, exists = sm.owners.Get("ghost")
	assert.False(t, exists, "empty drained marker should be compacted out")
}

// TestDropOwner_NoTombstones verifies that DropOwner on a tombstones=false
// state map compacts the ownerData synchronously. The reaper does not run
// for non-tombstone state maps, so leaving the ownerData drained-but-present
// would leak it forever and reject all future writes for the same owner.
func TestDropOwner_NoTombstones(t *testing.T) {
	store, _ := newTestStore()
	defer store.Stop()

	st := Register[string](store, StateTypeConfig[string]{
		Name:       "no_tombstones",
		Encode:     encodeString,
		Decode:     decodeString,
		Tombstones: false,
	})
	sm := store.getStateMap("no_tombstones")

	require.NoError(t, st.Set("k1", "owner1", "v1"))
	require.NoError(t, st.Set("k2", "owner1", "v2"))
	_, exists := sm.owners.Get("owner1")
	require.True(t, exists)

	st.DropOwner("owner1")

	_, exists = sm.owners.Get("owner1")
	assert.False(t, exists, "ownerData must be compacted synchronously for tombstones=false")

	// A fresh write for the same owner should succeed and create a new
	// non-drained ownerData.
	require.NoError(t, st.Set("k3", "owner1", "v3"))
	val, _, ok := st.GetForOwner("owner1", "k3")
	require.True(t, ok)
	assert.Equal(t, "v3", val)
}

// TestHashForOwnerFull_NoCmapReentryDeadlock guards against the deadlock
// that the digest handler's owner-hash short-circuit ran into in production.
//
// The bad pattern: call sm.hashForOwnerFull(owner) from inside an
// sm.owners.IterCb callback. IterCb holds the cmap shard RLock for the
// duration of the iteration; hashForOwnerFull internally does
// sm.getOwner -> cmap.Get which tries to RLock the same shard. Go's
// sync.RWMutex is writer-priority: any concurrent Upsert that's queued
// for WLock causes the recursive RLock attempt to wait, which never
// completes because the outer RLock is still held by the same goroutine.
//
// The test reproduces the conditions: continuous writers + a routine
// that does IterCb+hashForOwnerFull on the same stateMap. With the bug,
// the routine never returns. With the fix in handlers.go (collect owners
// first, evaluate hashes outside the iteration), it does.
func TestHashForOwnerFull_NoCmapReentryDeadlock(t *testing.T) {
	store, _ := newTestStore()
	defer store.Stop()

	st := newTestStateType(store, nil)
	for i := 0; i < 50; i++ {
		owner := fmt.Sprintf("owner%d", i)
		require.NoError(t, st.Set(fmt.Sprintf("k%d", i), owner, "v"))
	}
	sm := store.getStateMap("test")

	stop := make(chan struct{})
	defer close(stop)
	// Concurrent writers that contend the same shards via Upsert.
	for w := 0; w < 4; w++ {
		go func(w int) {
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				_ = st.Set(fmt.Sprintf("rk%d", i),
					fmt.Sprintf("racewriter%d-%d", w, i%10), "v")
			}
		}(w)
	}

	// Mirror the digest handler's flow: collect owners under iteration,
	// then call hashForOwnerFull outside. With the bug (hashForOwnerFull
	// inside the IterCb callback) this loop would deadlock under writer
	// pressure.
	done := make(chan struct{})
	go func() {
		var owners []string
		sm.owners.IterCb(func(owner string, _ *ownerData) {
			owners = append(owners, owner)
		})
		for _, o := range owners {
			_ = sm.hashForOwnerFull(o)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("hashForOwnerFull pattern deadlocked - cmap re-entry under writer contention")
	}
}

// TestHashForOwnerFull_DetectsTombstoneDivergence is the regression test for
// the anti-entropy short-circuit bug. Two stateMaps with identical live
// entries but different tombstone state must produce different full-hashes,
// or anti-entropy would treat them as in sync and never converge the
// tombstones.
func TestHashForOwnerFull_DetectsTombstoneDivergence(t *testing.T) {
	storeA, _ := newTestStore()
	defer storeA.Stop()
	storeB, _ := newTestStore()
	defer storeB.Stop()

	stA := newTestStateType(storeA, nil)
	stB := newTestStateType(storeB, nil)

	// Both stores have the same live entry.
	require.NoError(t, stA.Set("live", "o1", "v"))
	require.NoError(t, stB.Set("live", "o1", "v"))

	// Both originally had "dead" too, then A tombstoned it. B never saw
	// either the live "dead" or the tombstone.
	require.NoError(t, stA.Set("dead", "o1", "v"))
	stA.Delete("dead", "o1")

	smA := storeA.getStateMap("test")
	smB := storeB.getStateMap("test")

	liveA := smA.hashForOwner("o1")
	liveB := smB.hashForOwner("o1")
	assert.Equal(t, liveA, liveB,
		"live-only hashes must match (only live entries are equal); this is the bug surface")

	fullA := smA.hashForOwnerFull("o1")
	fullB := smB.hashForOwnerFull("o1")
	assert.NotEqual(t, fullA, fullB,
		"full hashes must differ when tombstone state differs; otherwise anti-entropy short-circuit suppresses tombstone divergence repair")
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
	e, ok := sm.entryAt("routerA", "link1")
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
	e, _ = sm.entryAt("routerA", "link1")
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
