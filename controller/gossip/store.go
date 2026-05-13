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
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"google.golang.org/protobuf/proto"
)

// ChangeOrigin indicates whether a state change originated locally or from a gossip peer.
type ChangeOrigin byte

const (
	OriginLocal ChangeOrigin = iota
	OriginGossip
)

// Store is the central gossip state store. It owns the Lamport clock and routes
// messages to per-type stateMaps.
type Store struct {
	peerId      string
	mesh        Mesh
	clock       atomic.Uint64
	types       sync.Map // map[string]*stateMap
	pendingAcks sync.Map // map[requestId]*pendingAck
	closeCh     chan struct{}
	eventsPool  goroutines.Pool
}

// NewStore creates a gossip Store for the given local peer identity.
func NewStore(peerId string, mesh Mesh) *Store {
	return &Store{
		peerId:  peerId,
		mesh:    mesh,
		closeCh: make(chan struct{}),
	}
}

// SetEventsPool sets the bounded pool used for processing incoming peer gossip
// messages. Must be called before any peer connections are established.
func (s *Store) SetEventsPool(pool goroutines.Pool) {
	s.eventsPool = pool
}

// queueOrRun submits work to the events pool if one is configured, otherwise
// runs it inline. Returns true if the work was accepted (queued or run inline).
func (s *Store) queueOrRun(work func()) bool {
	if s.eventsPool == nil {
		work()
		return true
	}
	if err := s.eventsPool.QueueOrError(work); err != nil {
		return false
	}
	return true
}

// Stop shuts down background goroutines (anti-entropy, tombstone reaping).
func (s *Store) Stop() {
	select {
	case <-s.closeCh:
	default:
		close(s.closeCh)
	}
}

// ClockValue returns the current Lamport clock value.
func (s *Store) ClockValue() uint64 {
	return s.clock.Load()
}

// StoreStats contains summary statistics for a gossip Store.
type StoreStats struct {
	TypeName   string `json:"typeName"`
	Entries    int    `json:"entries"`
	Tombstones int    `json:"tombstones"`
}

// GetStats returns per-type entry and tombstone counts.
func (s *Store) GetStats() []StoreStats {
	var stats []StoreStats
	s.types.Range(func(key, value any) bool {
		sm := value.(*stateMap)
		entries := 0
		tombstones := 0
		sm.owners.IterCb(func(_ string, od *ownerData) {
			od.mu.RLock()
			for _, e := range od.entries {
				if e.Tombstone {
					tombstones++
				} else {
					entries++
				}
			}
			od.mu.RUnlock()
		})
		stats = append(stats, StoreStats{
			TypeName:   key.(string),
			Entries:    entries,
			Tombstones: tombstones,
		})
		return true
	})
	return stats
}

func (s *Store) nextVersion() uint64 {
	return s.clock.Add(1)
}

func (s *Store) observeVersion(v uint64) {
	for {
		cur := s.clock.Load()
		if v <= cur {
			return
		}
		if s.clock.CompareAndSwap(cur, v) {
			return
		}
	}
}

// ApplyPeerDelta applies a gossip delta from a peer controller.
func (s *Store) ApplyPeerDelta(delta *gossip_pb.GossipDelta) {
	sm := s.getStateMap(delta.StoreType)
	if sm == nil {
		pfxlog.Logger().WithField("storeType", delta.StoreType).Warn("received gossip delta for unknown store type")
		return
	}

	for _, pbEntry := range delta.Entries {
		sm.applyDelta(entryFromProto(pbEntry))
	}
}

// ApplyAndBroadcast applies a gossip delta received from a router (not a peer
// controller) and broadcasts it to peer controllers. Use this for router-
// originated gossip entries that arrive via the ctrl channel.
func (s *Store) ApplyAndBroadcast(delta *gossip_pb.GossipDelta) {
	sm := s.getStateMap(delta.StoreType)
	if sm == nil {
		pfxlog.Logger().WithField("storeType", delta.StoreType).Warn("received gossip delta for unknown store type")
		return
	}

	for _, pbEntry := range delta.Entries {
		sm.applyAndBroadcast(entryFromProto(pbEntry))
	}
}

// GetDigestForOwner returns a digest of entries owned by the given owner in the
// specified store type. Used for router-specific anti-entropy on reconnect.
func (s *Store) GetDigestForOwner(storeType, owner string) []*gossip_pb.DigestEntry {
	sm := s.getStateMap(storeType)
	if sm == nil {
		return nil
	}
	return sm.getDigestForOwner(owner)
}

func (s *Store) getStateMap(name string) *stateMap {
	v, ok := s.types.Load(name)
	if !ok {
		return nil
	}
	return v.(*stateMap)
}

// entry is a single versioned gossip entry in a stateMap.
type entry struct {
	Key       string
	Value     []byte
	Version   uint64
	Owner     string
	Tombstone bool
	Epoch     []byte
	UpdatedAt time.Time
}

func (e *entry) toProto() *gossip_pb.GossipEntry {
	return &gossip_pb.GossipEntry{
		Key:       e.Key,
		Value:     e.Value,
		Version:   e.Version,
		Owner:     e.Owner,
		Tombstone: e.Tombstone,
		Epoch:     e.Epoch,
	}
}

// untypedListener is the type-erased listener stored in a stateMap.
type untypedListener interface {
	entryChanged(key string, value []byte, version uint64, owner string, isCreate bool, origin ChangeOrigin)
	entryRemoved(key string, owner string, origin ChangeOrigin)
}

type stateMapConfig struct {
	tombstones          bool
	tombstoneTTL        time.Duration
	antiEntropy         bool
	antiEntropyInterval time.Duration
}

// stateMap holds the gossip entries for a single registered type, organized
// by owner. Each owner has its own ownerData containing that owner's entries
// plus aggregate state (hash cache, max version, non-tombstone count). The
// owner-first layout lets per-owner operations (HashForOwner, GetDigestForOwner,
// DeleteByOwnerBefore, Reconcile) run in O(entries-for-owner) instead of
// O(total entries).
type stateMap struct {
	name     string
	config   stateMapConfig
	owners   cmap.ConcurrentMap[string, *ownerData]
	store    *Store
	listener untypedListener
}

// ownerData holds all state for a single owner: the owner's entries plus
// derived aggregates. The mutex serializes mutations to the entries map and
// keeps the aggregates consistent with it.
type ownerData struct {
	mu            sync.RWMutex
	entries       map[string]*entry // key -> entry, including tombstones
	hash          uint64            // FNV-64a of sorted (key||version) over live entries
	hashDirty     bool              // hash needs recomputation
	maxVersion    uint64            // highest version ever observed for this owner
	nonTombstones int64             // count of live (non-tombstone) entries
}

func newOwnerData() *ownerData {
	return &ownerData{
		entries:   map[string]*entry{},
		hashDirty: true,
	}
}

// getOwner returns the ownerData for the given owner, or nil if none exists.
// Callers that only read state and want a "zero" answer for unknown owners
// should use this and check for nil.
func (sm *stateMap) getOwner(owner string) *ownerData {
	if od, ok := sm.owners.Get(owner); ok {
		return od
	}
	return nil
}

// getOrCreateOwner returns the ownerData for the given owner, creating it if
// needed. The cmap's Upsert callback runs under the shard lock, so concurrent
// callers receive the same instance.
func (sm *stateMap) getOrCreateOwner(owner string) *ownerData {
	var result *ownerData
	sm.owners.Upsert(owner, nil, func(exist bool, existing *ownerData, _ *ownerData) *ownerData {
		if exist {
			result = existing
			return existing
		}
		result = newOwnerData()
		return result
	})
	return result
}

func newStateMap(name string, cfg stateMapConfig, store *Store, listener untypedListener) *stateMap {
	return &stateMap{
		name:     name,
		config:   cfg,
		owners:   cmap.New[*ownerData](),
		store:    store,
		listener: listener,
	}
}

// entryAt returns the raw entry for (owner, key), including tombstones, along
// with whether it exists. Package-private helper used by tests and the typed
// StateType.GetForOwner; production callers should use the typed accessors.
func (sm *stateMap) entryAt(owner, key string) (*entry, bool) {
	od := sm.getOwner(owner)
	if od == nil {
		return nil, false
	}
	od.mu.RLock()
	e, ok := od.entries[key]
	od.mu.RUnlock()
	return e, ok
}

// applyEntryLocked installs incoming into the owner's entries map under
// owner.mu (already held by the caller). Returns (wasSet, isCreate). When
// wasSet is false the version check rejected the incoming entry and nothing
// was changed. Aggregates (maxVersion, nonTombstones, hashDirty) are updated
// in-place. The caller is responsible for listener notification and broadcast
// after releasing the lock.
func (sm *stateMap) applyEntryLocked(od *ownerData, incoming *entry) (wasSet, isCreate bool) {
	existing, ok := od.entries[incoming.Key]
	if ok && existing.Version >= incoming.Version {
		return false, false
	}

	wasLive := ok && !existing.Tombstone
	isCreate = !ok || existing.Tombstone

	od.entries[incoming.Key] = incoming

	if incoming.Version > od.maxVersion {
		od.maxVersion = incoming.Version
	}
	if incoming.Tombstone {
		if wasLive {
			od.nonTombstones--
			od.hashDirty = true
		}
	} else {
		od.hashDirty = true
		if isCreate {
			od.nonTombstones++
		}
	}
	return true, isCreate
}

func (sm *stateMap) set(key, owner string, value []byte, origin ChangeOrigin) {
	version := sm.store.nextVersion()

	e := &entry{
		Key:       key,
		Value:     value,
		Version:   version,
		Owner:     owner,
		Tombstone: false,
		UpdatedAt: time.Now(),
	}

	od := sm.getOrCreateOwner(owner)
	od.mu.Lock()
	wasSet, isCreate := sm.applyEntryLocked(od, e)
	od.mu.Unlock()

	if !wasSet {
		return
	}

	if sm.listener != nil {
		sm.listener.entryChanged(key, value, version, owner, isCreate, origin)
	}

	if origin == OriginLocal {
		sm.broadcastDelta(e)
	}
}

func (sm *stateMap) applyDelta(incoming *entry) {
	incoming.UpdatedAt = time.Now()
	sm.store.observeVersion(incoming.Version)

	od := sm.getOrCreateOwner(incoming.Owner)
	od.mu.Lock()
	wasSet, isCreate := sm.applyEntryLocked(od, incoming)
	od.mu.Unlock()

	if !wasSet {
		return
	}

	if incoming.Tombstone {
		if sm.listener != nil {
			sm.listener.entryRemoved(incoming.Key, incoming.Owner, OriginGossip)
		}
	} else if sm.listener != nil {
		sm.listener.entryChanged(incoming.Key, incoming.Value, incoming.Version, incoming.Owner, isCreate, OriginGossip)
	}
}

// applyAndBroadcast applies an externally-versioned entry (e.g., from a router's
// own Lamport clock) and broadcasts it to peer controllers. This is the path for
// router-originated gossip: the entry arrives via the ctrl channel, gets applied
// locally, and is fanned out to peers.
func (sm *stateMap) applyAndBroadcast(incoming *entry) {
	incoming.UpdatedAt = time.Now()
	sm.store.observeVersion(incoming.Version)

	od := sm.getOrCreateOwner(incoming.Owner)
	od.mu.Lock()
	wasSet, isCreate := sm.applyEntryLocked(od, incoming)
	od.mu.Unlock()

	if !wasSet {
		return
	}

	sm.notifyListener(incoming, isCreate)
	sm.broadcastDelta(incoming)
}

func (sm *stateMap) notifyListener(e *entry, isCreate bool) {
	if sm.listener == nil {
		return
	}
	if e.Tombstone {
		sm.listener.entryRemoved(e.Key, e.Owner, OriginLocal)
	} else {
		sm.listener.entryChanged(e.Key, e.Value, e.Version, e.Owner, isCreate, OriginLocal)
	}
}

func (sm *stateMap) delete(key, owner string, origin ChangeOrigin) {
	if sm.config.tombstones {
		version := sm.store.nextVersion()
		e := &entry{
			Key:       key,
			Version:   version,
			Owner:     owner,
			Tombstone: true,
			UpdatedAt: time.Now(),
		}

		od := sm.getOrCreateOwner(owner)
		od.mu.Lock()
		if existing, ok := od.entries[key]; ok {
			// Carry the replaced entry's epoch onto the tombstone so the wire
			// entry stays self-consistent with what it supersedes.
			e.Epoch = existing.Epoch
		}
		wasSet, _ := sm.applyEntryLocked(od, e)
		od.mu.Unlock()

		if !wasSet {
			return
		}

		if sm.listener != nil {
			sm.listener.entryRemoved(key, owner, origin)
		}
		if origin == OriginLocal {
			sm.broadcastDelta(e)
		}
	} else {
		od := sm.getOrCreateOwner(owner)
		od.mu.Lock()
		existing, ok := od.entries[key]
		delete(od.entries, key)
		notify := ok && !existing.Tombstone
		if notify {
			od.nonTombstones--
			od.hashDirty = true
		}
		od.mu.Unlock()

		if sm.listener != nil && notify {
			sm.listener.entryRemoved(key, owner, origin)
		}
	}
}

// collectKeysToDelete iterates the owner's live entries under RLock and
// returns those for which filter returns true. Used by deleteByOwner and
// friends to capture the deletion set before mutating.
func (od *ownerData) collectKeysToDelete(filter func(key string, e *entry) bool) []string {
	od.mu.RLock()
	defer od.mu.RUnlock()
	var toDelete []string
	for key, e := range od.entries {
		if !e.Tombstone && filter(key, e) {
			toDelete = append(toDelete, key)
		}
	}
	return toDelete
}

func (sm *stateMap) deleteByOwner(owner string, origin ChangeOrigin) {
	od := sm.getOwner(owner)
	if od == nil {
		return
	}
	toDelete := od.collectKeysToDelete(func(string, *entry) bool { return true })
	for _, key := range toDelete {
		sm.delete(key, owner, origin)
	}
}

func (sm *stateMap) deleteByOwnerBefore(owner string, epoch []byte, origin ChangeOrigin) {
	od := sm.getOwner(owner)
	if od == nil {
		return
	}
	toDelete := od.collectKeysToDelete(func(key string, e *entry) bool {
		if bytes.Compare(e.Epoch, epoch) < 0 {
			pfxlog.Logger().
				WithField("key", key).
				WithField("owner", owner).
				WithField("entryEpoch", fmt.Sprintf("%x", e.Epoch)).
				WithField("cleanupEpoch", fmt.Sprintf("%x", epoch)).
				Info("epoch cleanup: deleting old-epoch entry")
			return true
		}
		return false
	})
	for _, key := range toDelete {
		sm.delete(key, owner, origin)
	}
}

func (sm *stateMap) reconcile(owner string, currentKeys map[string]struct{}, origin ChangeOrigin) {
	od := sm.getOwner(owner)
	if od == nil {
		return
	}
	toDelete := od.collectKeysToDelete(func(key string, _ *entry) bool {
		_, ok := currentKeys[key]
		return !ok
	})
	for _, key := range toDelete {
		sm.delete(key, owner, origin)
	}
}

func (sm *stateMap) getDigest() []*gossip_pb.DigestEntry {
	var digest []*gossip_pb.DigestEntry
	sm.owners.IterCb(func(_ string, od *ownerData) {
		od.mu.RLock()
		for key, e := range od.entries {
			digest = append(digest, &gossip_pb.DigestEntry{
				Key:     key,
				Version: e.Version,
			})
		}
		od.mu.RUnlock()
	})
	return digest
}

func (sm *stateMap) getDigestForOwner(owner string) []*gossip_pb.DigestEntry {
	od := sm.getOwner(owner)
	if od == nil {
		return nil
	}
	od.mu.RLock()
	defer od.mu.RUnlock()
	digest := make([]*gossip_pb.DigestEntry, 0, len(od.entries))
	for key, e := range od.entries {
		digest = append(digest, &gossip_pb.DigestEntry{
			Key:     key,
			Version: e.Version,
		})
	}
	return digest
}

// maxVersionForOwner returns the highest version among all entries (including
// tombstones) belonging to the given owner. Returns 0 if no entries exist.
func (sm *stateMap) maxVersionForOwner(owner string) uint64 {
	od := sm.getOwner(owner)
	if od == nil {
		return 0
	}
	od.mu.RLock()
	defer od.mu.RUnlock()
	return od.maxVersion
}

// nonTombstoneCount returns the number of non-tombstone entries for the given owner.
func (sm *stateMap) nonTombstoneCount(owner string) int64 {
	od := sm.getOwner(owner)
	if od == nil {
		return 0
	}
	od.mu.RLock()
	defer od.mu.RUnlock()
	return od.nonTombstones
}

func (sm *stateMap) getAllEntries() []*entry {
	var result []*entry
	sm.owners.IterCb(func(_ string, od *ownerData) {
		od.mu.RLock()
		for _, e := range od.entries {
			result = append(result, e)
		}
		od.mu.RUnlock()
	})
	return result
}

func (sm *stateMap) reapTombstones() {
	if !sm.config.tombstones || sm.config.tombstoneTTL == 0 {
		return
	}
	cutoff := time.Now().Add(-sm.config.tombstoneTTL)

	// Reap under each owner's write lock, so a tombstone that has been promoted
	// back to a live entry between scan and remove (via a concurrent applyDelta
	// or applyAndBroadcast) can't be silently dropped — the check and the
	// delete are atomic relative to mutations.
	sm.owners.IterCb(func(_ string, od *ownerData) {
		od.mu.Lock()
		for key, e := range od.entries {
			if e.Tombstone && e.UpdatedAt.Before(cutoff) {
				delete(od.entries, key)
			}
		}
		od.mu.Unlock()
	})
}

func (sm *stateMap) broadcastDelta(e *entry) {
	delta := &gossip_pb.GossipDelta{
		StoreType: sm.name,
		Entries:   []*gossip_pb.GossipEntry{e.toProto()},
	}
	body, err := proto.Marshal(delta)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to marshal gossip delta")
		return
	}
	msg := channel.NewMessage(gossip_pb.GossipDeltaType, body)
	sm.store.mesh.Broadcast(msg)
}

func (sm *stateMap) sendSnapshotTo(peerId string) {
	entries := sm.getAllEntries()
	if len(entries) == 0 {
		return
	}

	pbEntries := make([]*gossip_pb.GossipEntry, 0, len(entries))
	for _, e := range entries {
		pbEntries = append(pbEntries, e.toProto())
	}

	delta := &gossip_pb.GossipDelta{
		StoreType: sm.name,
		Entries:   pbEntries,
	}
	body, err := proto.Marshal(delta)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to marshal gossip snapshot")
		return
	}
	msg := channel.NewMessage(gossip_pb.GossipDeltaType, body)
	_ = sm.store.mesh.Send(peerId, msg)
}

// hashForOwner returns a FNV-64a hash of the sorted (key||version) over the
// owner's non-tombstone entries. The cached value is returned when clean.
// On the recompute path the cost is O(entries-for-owner), not O(total entries).
func (sm *stateMap) hashForOwner(owner string) uint64 {
	od := sm.getOwner(owner)
	if od == nil {
		return 0
	}

	// Fast path: cached hash is clean.
	od.mu.RLock()
	if !od.hashDirty {
		h := od.hash
		od.mu.RUnlock()
		return h
	}
	od.mu.RUnlock()

	// Slow path: recompute. Take the write lock, re-check (another goroutine
	// may have computed it), then iterate this owner's entries only.
	od.mu.Lock()
	defer od.mu.Unlock()
	if !od.hashDirty {
		return od.hash
	}

	type keyVersion struct {
		key     string
		version uint64
	}
	kvs := make([]keyVersion, 0, len(od.entries))
	for k, e := range od.entries {
		if !e.Tombstone {
			kvs = append(kvs, keyVersion{key: k, version: e.Version})
		}
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].key < kvs[j].key })

	h := fnv.New64a()
	var buf [8]byte
	for _, kv := range kvs {
		_, _ = h.Write([]byte(kv.key))
		binary.LittleEndian.PutUint64(buf[:], kv.version)
		_, _ = h.Write(buf[:])
	}
	od.hash = h.Sum64()
	od.hashDirty = false
	return od.hash
}

func entryFromProto(pb *gossip_pb.GossipEntry) *entry {
	return &entry{
		Key:       pb.Key,
		Value:     pb.Value,
		Version:   pb.Version,
		Owner:     pb.Owner,
		Tombstone: pb.Tombstone,
		Epoch:     pb.Epoch,
		UpdatedAt: time.Now(),
	}
}
