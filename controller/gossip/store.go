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
		sm.entries.IterCb(func(_ string, e *entry) {
			if e.Tombstone {
				tombstones++
			} else {
				entries++
			}
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

// stateMap holds the gossip entries for a single registered type.
type stateMap struct {
	name     string
	config   stateMapConfig
	entries  cmap.ConcurrentMap[string, *entry]
	store    *Store
	listener untypedListener

	// Per-owner hash cache for staleness detection. The hash covers sorted
	// non-tombstone keys owned by a given router. A nil value means the cache
	// is dirty and needs recomputation. Invalidated on any mutation affecting
	// that owner's entries.
	ownerHashes sync.Map // owner (string) -> *uint64 (nil = dirty)

	// Per-owner stats tracked incrementally on every mutation. Avoids full-map
	// scans for max version and entry count lookups.
	ownerStats sync.Map // owner (string) -> *ownerStats
}

// ownerStats tracks per-owner counters that are updated atomically on every
// mutation, avoiding full-map scans.
type ownerStats struct {
	maxVersion    atomic.Uint64
	nonTombstones atomic.Int64
}

// defaultOwnerStats is returned for owners with no tracked state, avoiding
// allocations in the sync.Map for unknown or reaped owners.
var defaultOwnerStats = &ownerStats{}

// getOwnerStats returns the stats for the given owner, or a zero-value default
// if none exist. Use getOrCreateOwnerStats for mutation paths.
func (sm *stateMap) getOwnerStats(owner string) *ownerStats {
	if v, ok := sm.ownerStats.Load(owner); ok {
		return v.(*ownerStats)
	}
	return defaultOwnerStats
}

// getOrCreateOwnerStats returns the stats for the given owner, creating them if needed.
func (sm *stateMap) getOrCreateOwnerStats(owner string) *ownerStats {
	if v, ok := sm.ownerStats.Load(owner); ok {
		return v.(*ownerStats)
	}
	stats := &ownerStats{}
	actual, _ := sm.ownerStats.LoadOrStore(owner, stats)
	return actual.(*ownerStats)
}

// updateOwnerStats updates the per-owner counters after a successful mutation.
// isTombstone refers to the new entry state; isCreate means the entry is new or
// was previously tombstoned.
func (sm *stateMap) updateOwnerStats(owner string, version uint64, isTombstone bool, isCreate bool) {
	stats := sm.getOrCreateOwnerStats(owner)
	for {
		cur := stats.maxVersion.Load()
		if version <= cur || stats.maxVersion.CompareAndSwap(cur, version) {
			break
		}
	}
	if isCreate && !isTombstone {
		stats.nonTombstones.Add(1)
	} else if !isCreate && isTombstone {
		// live entry being replaced by a tombstone
		stats.nonTombstones.Add(-1)
	}
}

func newStateMap(name string, cfg stateMapConfig, store *Store, listener untypedListener) *stateMap {
	return &stateMap{
		name:     name,
		config:   cfg,
		entries:  cmap.New[*entry](),
		store:    store,
		listener: listener,
	}
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

	var wasSet bool
	var isCreate bool
	sm.entries.Upsert(key, e, func(exist bool, existing *entry, newValue *entry) *entry {
		if exist && existing.Version >= version {
			return existing
		}
		wasSet = true
		isCreate = !exist || existing.Tombstone
		return e
	})

	if !wasSet {
		return
	}

	sm.invalidateOwnerHash(owner)
	sm.updateOwnerStats(owner, version, false, isCreate)

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

	var wasSet bool
	var isCreate bool
	sm.entries.Upsert(incoming.Key, incoming, func(exist bool, existing *entry, newValue *entry) *entry {
		if exist && existing.Version >= incoming.Version {
			return existing
		}
		wasSet = true
		isCreate = !exist || existing.Tombstone
		return incoming
	})

	if !wasSet {
		return
	}

	sm.invalidateOwnerHash(incoming.Owner)
	sm.updateOwnerStats(incoming.Owner, incoming.Version, incoming.Tombstone, isCreate)

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

	var wasSet bool
	var isCreate bool
	sm.entries.Upsert(incoming.Key, incoming, func(exist bool, existing *entry, newValue *entry) *entry {
		if exist && existing.Version >= incoming.Version {
			return existing
		}
		wasSet = true
		isCreate = !exist || existing.Tombstone
		return incoming
	})

	if !wasSet {
		return
	}

	sm.invalidateOwnerHash(incoming.Owner)
	sm.updateOwnerStats(incoming.Owner, incoming.Version, incoming.Tombstone, isCreate)
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

		var wasSet bool
		var wasLive bool
		sm.entries.Upsert(key, e, func(exist bool, existing *entry, newValue *entry) *entry {
			if exist && existing.Version >= version {
				return existing
			}
			wasSet = true
			wasLive = exist && !existing.Tombstone
			return e
		})

		if !wasSet {
			return
		}

		sm.invalidateOwnerHash(owner)
		sm.updateOwnerStats(owner, version, true, !wasLive)
		if sm.listener != nil {
			sm.listener.entryRemoved(key, owner, origin)
		}
		if origin == OriginLocal {
			sm.broadcastDelta(e)
		}
	} else {
		sm.entries.Remove(key)
		sm.invalidateOwnerHash(owner)
		sm.getOrCreateOwnerStats(owner).nonTombstones.Add(-1)
		if sm.listener != nil {
			sm.listener.entryRemoved(key, owner, origin)
		}
	}
}

func (sm *stateMap) deleteByOwner(owner string, origin ChangeOrigin) {
	var toDelete []string
	sm.entries.IterCb(func(key string, e *entry) {
		if e.Owner == owner && !e.Tombstone {
			toDelete = append(toDelete, key)
		}
	})
	for _, key := range toDelete {
		sm.delete(key, owner, origin)
	}
}

func (sm *stateMap) deleteByOwnerBefore(owner string, epoch []byte, origin ChangeOrigin) {
	var toDelete []string
	sm.entries.IterCb(func(key string, e *entry) {
		if e.Owner == owner && !e.Tombstone && bytes.Compare(e.Epoch, epoch) < 0 {
			pfxlog.Logger().
				WithField("key", key).
				WithField("owner", owner).
				WithField("entryEpoch", fmt.Sprintf("%x", e.Epoch)).
				WithField("cleanupEpoch", fmt.Sprintf("%x", epoch)).
				Info("epoch cleanup: deleting old-epoch entry")
			toDelete = append(toDelete, key)
		}
	})
	for _, key := range toDelete {
		sm.delete(key, owner, origin)
	}
}

func (sm *stateMap) reconcile(owner string, currentKeys map[string]struct{}, origin ChangeOrigin) {
	var toDelete []string
	sm.entries.IterCb(func(key string, e *entry) {
		if e.Owner == owner && !e.Tombstone {
			if _, ok := currentKeys[key]; !ok {
				toDelete = append(toDelete, key)
			}
		}
	})
	for _, key := range toDelete {
		sm.delete(key, owner, origin)
	}
}

func (sm *stateMap) getDigest() []*gossip_pb.DigestEntry {
	var digest []*gossip_pb.DigestEntry
	sm.entries.IterCb(func(key string, e *entry) {
		digest = append(digest, &gossip_pb.DigestEntry{
			Key:     key,
			Version: e.Version,
		})
	})
	return digest
}

func (sm *stateMap) getDigestForOwner(owner string) []*gossip_pb.DigestEntry {
	var digest []*gossip_pb.DigestEntry
	sm.entries.IterCb(func(key string, e *entry) {
		if e.Owner == owner {
			digest = append(digest, &gossip_pb.DigestEntry{
				Key:     key,
				Version: e.Version,
			})
		}
	})
	return digest
}

// maxVersionForOwner returns the highest version among all entries (including
// tombstones) belonging to the given owner. Returns 0 if no entries exist.
func (sm *stateMap) maxVersionForOwner(owner string) uint64 {
	return sm.getOwnerStats(owner).maxVersion.Load()
}

// nonTombstoneCount returns the number of non-tombstone entries for the given owner.
func (sm *stateMap) nonTombstoneCount(owner string) int64 {
	return sm.getOwnerStats(owner).nonTombstones.Load()
}

func (sm *stateMap) getAllEntries() []*entry {
	var result []*entry
	sm.entries.IterCb(func(_ string, e *entry) {
		result = append(result, e)
	})
	return result
}

func (sm *stateMap) reapTombstones() {
	if !sm.config.tombstones || sm.config.tombstoneTTL == 0 {
		return
	}
	cutoff := time.Now().Add(-sm.config.tombstoneTTL)

	type tombstone struct {
		key   string
		owner string
	}
	var toRemove []tombstone
	sm.entries.IterCb(func(key string, e *entry) {
		if e.Tombstone && e.UpdatedAt.Before(cutoff) {
			toRemove = append(toRemove, tombstone{key: key, owner: e.Owner})
		}
	})

	affectedOwners := map[string]struct{}{}
	for _, t := range toRemove {
		sm.entries.Remove(t.key)
		affectedOwners[t.owner] = struct{}{}
	}

	// Clean up ownerHashes and ownerStats for owners that no longer have any entries.
	for owner := range affectedOwners {
		hasEntries := false
		sm.entries.IterCb(func(_ string, e *entry) {
			if e.Owner == owner {
				hasEntries = true
			}
		})
		if !hasEntries {
			sm.ownerHashes.Delete(owner)
			sm.ownerStats.Delete(owner)
		}
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

// invalidateOwnerHash marks the cached hash for the given owner as dirty.
func (sm *stateMap) invalidateOwnerHash(owner string) {
	sm.ownerHashes.Store(owner, (*uint64)(nil))
}

// hashForOwner returns a FNV-64a hash of the sorted non-tombstone entry keys
// for the given owner. The result is cached and only recomputed when the
// owner's entries have changed.
func (sm *stateMap) hashForOwner(owner string) uint64 {
	if v, ok := sm.ownerHashes.Load(owner); ok {
		if p, _ := v.(*uint64); p != nil {
			return *p
		}
	}

	type keyVersion struct {
		key     string
		version uint64
	}
	var kvs []keyVersion
	sm.entries.IterCb(func(key string, e *entry) {
		if e.Owner == owner && !e.Tombstone {
			kvs = append(kvs, keyVersion{key: key, version: e.Version})
		}
	})
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].key < kvs[j].key })

	h := fnv.New64a()
	var buf [8]byte
	for _, kv := range kvs {
		_, _ = h.Write([]byte(kv.key))
		binary.LittleEndian.PutUint64(buf[:], kv.version)
		_, _ = h.Write(buf[:])
	}
	result := h.Sum64()
	sm.ownerHashes.Store(owner, &result)
	return result
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
