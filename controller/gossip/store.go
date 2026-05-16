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
	"github.com/openziti/metrics"
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
	peerId           string
	mesh             Mesh
	clock            atomic.Uint64
	types            sync.Map // map[string]*stateMap
	pendingAcks      sync.Map // map[requestId]*pendingAck
	pendingAcksCount atomic.Int64
	closeCh          chan struct{}
	eventsPool       goroutines.Pool
	metricsRegistry  metrics.Registry
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

// SetMetricsRegistry installs the registry used for store-level and per-stateMap
// metrics. Must be called before any state types are registered, so each
// Register call can wire up its own metrics against this registry.
func (s *Store) SetMetricsRegistry(reg metrics.Registry) {
	s.metricsRegistry = reg
	if reg != nil {
		reg.FuncGauge("gossip.pending_acks", func() int64 {
			return s.pendingAcksCount.Load()
		})
	}
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

	sm.markDeltaReceived(int64(len(delta.Entries)))
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

	sm.markDeltaReceived(int64(len(delta.Entries)))
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
	metrics  *stateMapMetrics
}

// stateMapMetrics holds the meters maintained for a stateMap. Gauges are
// registered with the metrics.Registry directly (poll-time closures over
// stateMap fields) and not held here.
type stateMapMetrics struct {
	deltaReceived         metrics.Meter
	deltaApplied          metrics.Meter
	deltaRejectedStale    metrics.Meter
	broadcastSent         metrics.Meter
	antiEntropyOwnersMatched metrics.Meter // owners short-circuited by hash on incoming digest
	antiEntropyOwnersDiffed  metrics.Meter // owners that required per-entry comparison
}

// markDeltaReceived records n entries received from peers (direct broadcast
// or anti-entropy digest response).
func (sm *stateMap) markDeltaReceived(n int64) {
	if sm.metrics != nil && n > 0 {
		sm.metrics.deltaReceived.Mark(n)
	}
}

// markDeltaApplied records an incoming entry that won its version check.
func (sm *stateMap) markDeltaApplied() {
	if sm.metrics != nil {
		sm.metrics.deltaApplied.Mark(1)
	}
}

// markDeltaRejectedStale records an incoming entry rejected because its
// version was not newer than the local copy (or the owner was drained).
func (sm *stateMap) markDeltaRejectedStale() {
	if sm.metrics != nil {
		sm.metrics.deltaRejectedStale.Mark(1)
	}
}

// markBroadcastSent records a broadcast initiated by this controller.
func (sm *stateMap) markBroadcastSent() {
	if sm.metrics != nil {
		sm.metrics.broadcastSent.Mark(1)
	}
}

// markAntiEntropyOwnersMatched records owners short-circuited by hash match
// in an incoming digest.
func (sm *stateMap) markAntiEntropyOwnersMatched(n int64) {
	if sm.metrics != nil && n > 0 {
		sm.metrics.antiEntropyOwnersMatched.Mark(n)
	}
}

// markAntiEntropyOwnersDiffed records owners that required per-entry
// comparison (hash mismatch, or sender did not include a hash for them).
func (sm *stateMap) markAntiEntropyOwnersDiffed(n int64) {
	if sm.metrics != nil && n > 0 {
		sm.metrics.antiEntropyOwnersDiffed.Mark(n)
	}
}

// registerMetrics wires up the meters and poll-time gauges for this stateMap
// against the store's metrics registry. Safe to call with a nil registry.
func (sm *stateMap) registerMetrics(reg metrics.Registry) {
	if reg == nil {
		return
	}
	prefix := "gossip." + sm.name
	sm.metrics = &stateMapMetrics{
		deltaReceived:            reg.Meter(prefix + ".delta.received"),
		deltaApplied:             reg.Meter(prefix + ".delta.applied"),
		deltaRejectedStale:       reg.Meter(prefix + ".delta.rejected_stale"),
		broadcastSent:            reg.Meter(prefix + ".broadcast.sent"),
		antiEntropyOwnersMatched: reg.Meter(prefix + ".anti_entropy.owners_matched"),
		antiEntropyOwnersDiffed:  reg.Meter(prefix + ".anti_entropy.owners_diffed"),
	}
	reg.FuncGauge(prefix+".owners", func() int64 {
		return int64(sm.owners.Count())
	})
	reg.FuncGauge(prefix+".entries.live", func() int64 {
		var live int64
		sm.owners.IterCb(func(_ string, od *ownerData) {
			od.mu.RLock()
			live += od.nonTombstones
			od.mu.RUnlock()
		})
		return live
	})
	reg.FuncGauge(prefix+".entries.tombstones", func() int64 {
		var tombstones int64
		sm.owners.IterCb(func(_ string, od *ownerData) {
			od.mu.RLock()
			tombstones += int64(len(od.entries)) - od.nonTombstones
			od.mu.RUnlock()
		})
		return tombstones
	})
	reg.FuncGauge(prefix+".owners.drained", func() int64 {
		var drained int64
		sm.owners.IterCb(func(_ string, od *ownerData) {
			od.mu.RLock()
			if od.drained {
				drained++
			}
			od.mu.RUnlock()
		})
		return drained
	})
}

// ownerData holds all state for a single owner: the owner's entries plus
// derived aggregates. The mutex serializes mutations to the entries map and
// keeps the aggregates consistent with it.
//
// Lifecycle: a fresh ownerData has drained=false and accepts writes. When
// DropOwner is called (e.g., a router is deleted), drained is set under the
// write lock after all live entries have been tombstoned. While drained,
// applyEntryLocked rejects all writes and read methods return zero values.
// Once tombstones have aged out via reapTombstones (so entries is empty), the
// reaper removes the ownerData from sm.owners under the cmap shard write lock
// with a check that re-validates drained && empty atomically. If a new write
// for the same owner arrives after removal, a fresh non-drained ownerData is
// created.
type ownerData struct {
	mu            sync.RWMutex
	entries       map[string]*entry // key -> entry, including tombstones
	hash          uint64            // FNV-64a of sorted (key||version) over live entries
	hashDirty     bool              // hash needs recomputation
	maxVersion    uint64            // highest version ever observed for this owner
	nonTombstones int64             // count of live (non-tombstone) entries
	drained       bool              // dropOwner called; rejects writes and eligible for compaction
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
// wasSet is false the version check rejected the incoming entry, the owner is
// drained, or there was no change. Aggregates (maxVersion, nonTombstones,
// hashDirty) are updated in-place. The caller is responsible for listener
// notification and broadcast after releasing the lock.
func (sm *stateMap) applyEntryLocked(od *ownerData, incoming *entry) (wasSet, isCreate bool) {
	if od.drained {
		return false, false
	}
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
		sm.markDeltaRejectedStale()
		return
	}
	sm.markDeltaApplied()

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
		sm.markDeltaRejectedStale()
		return
	}
	sm.markDeltaApplied()

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

// dropOwner tombstones every live entry for the owner and marks the owner
// drained. Subsequent writes for the owner are rejected by applyEntryLocked
// until the reaper compacts the ownerData out of sm.owners; after that, a
// fresh write would create a new non-drained ownerData.
//
// Used when the owner is gone for good (e.g., router deletion). For state
// types with tombstones=false, entries are simply cleared with no broadcast,
// since there is no tombstone mechanism to propagate the removal.
func (sm *stateMap) dropOwner(owner string, origin ChangeOrigin) {
	od := sm.getOwner(owner)
	if od == nil {
		// Mark the owner as drained even if there are no entries yet, so that
		// any race with an in-flight write is rejected. Create the ownerData
		// in the cmap so the reaper can find and compact it out.
		od = sm.getOrCreateOwner(owner)
	}

	type tombstoned struct {
		key string
		e   *entry // nil for tombstones=false (no broadcast)
	}
	var produced []tombstoned

	od.mu.Lock()
	if od.drained {
		od.mu.Unlock()
		return
	}
	od.drained = true

	if sm.config.tombstones {
		for key, existing := range od.entries {
			if existing.Tombstone {
				continue
			}
			version := sm.store.nextVersion()
			ts := &entry{
				Key:       key,
				Version:   version,
				Owner:     owner,
				Tombstone: true,
				Epoch:     existing.Epoch,
				UpdatedAt: time.Now(),
			}
			od.entries[key] = ts
			od.nonTombstones--
			if version > od.maxVersion {
				od.maxVersion = version
			}
			produced = append(produced, tombstoned{key: key, e: ts})
		}
	} else {
		for key := range od.entries {
			produced = append(produced, tombstoned{key: key})
		}
		od.entries = map[string]*entry{}
		od.nonTombstones = 0
	}
	if len(produced) > 0 {
		od.hashDirty = true
	}
	od.mu.Unlock()

	// Fire listener and broadcast outside the lock to avoid lock-order
	// inversions with downstream consumers (e.g., the link manager lock).
	for _, t := range produced {
		if sm.listener != nil {
			sm.listener.entryRemoved(t.key, owner, origin)
		}
		if origin == OriginLocal && t.e != nil {
			sm.broadcastDelta(t.e)
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

// getOwnerDigests returns per-owner FNV-64a hashes for every owner that has
// any entries. Used to populate GossipDigest.OwnerDigests so the receiver
// can short-circuit owners that already match without doing per-entry
// version comparison.
func (sm *stateMap) getOwnerDigests() []*gossip_pb.OwnerDigest {
	// Collect owner identifiers first, then hash outside the cmap iteration
	// so hashForOwner (which acquires its own owner.mu) doesn't nest with
	// the cmap shard lock more than necessary.
	var owners []string
	sm.owners.IterCb(func(owner string, _ *ownerData) {
		owners = append(owners, owner)
	})
	digests := make([]*gossip_pb.OwnerDigest, 0, len(owners))
	for _, owner := range owners {
		digests = append(digests, &gossip_pb.OwnerDigest{
			Owner: owner,
			Hash:  sm.hashForOwner(owner),
		})
	}
	return digests
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
	//
	// Also collect drained owners whose entries have fully drained, so they
	// can be compacted out of sm.owners below.
	var emptyDrained []string
	sm.owners.IterCb(func(owner string, od *ownerData) {
		od.mu.Lock()
		for key, e := range od.entries {
			if e.Tombstone && e.UpdatedAt.Before(cutoff) {
				delete(od.entries, key)
			}
		}
		if od.drained && len(od.entries) == 0 {
			emptyDrained = append(emptyDrained, owner)
		}
		od.mu.Unlock()
	})

	// Compact empty drained owners out of sm.owners. RemoveCb holds the cmap
	// shard write lock for the duration of the callback, so the re-check of
	// drained && empty under the owner lock is atomic with the delete: a
	// concurrent write that won the race (created a fresh ownerData or
	// recreated entries on this one) is preserved by the predicate returning
	// false.
	for _, owner := range emptyDrained {
		sm.owners.RemoveCb(owner, func(_ string, od *ownerData, exists bool) bool {
			if !exists {
				return false
			}
			od.mu.Lock()
			defer od.mu.Unlock()
			return od.drained && len(od.entries) == 0
		})
	}
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
	sm.markBroadcastSent()
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
