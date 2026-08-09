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
	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/concurrency"
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
	peerId string
	// mesh is the transport peer messages travel over, fixed for the store's lifetime. A controller that is
	// not part of a cluster gets one with no peers rather than none, so no send path has to check.
	mesh             Mesh
	clock            atomic.Uint64
	types            sync.Map // map[string]*stateMap
	pendingAcks      sync.Map // map[requestId]*pendingAck
	pendingAcksCount atomic.Int64
	closeCh          chan struct{}
	eventsPool       goroutines.Pool
	ioPool           goroutines.Pool
	metricsRegistry  metrics.Registry
}

// NewStore creates a gossip Store for the given local peer identity. mesh must not be nil; a controller with
// no peers passes NewNoopMesh.
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

// SetIoPool sets the bounded pool used for outbound, blocking network I/O
// (peer broadcasts). Keeping I/O off the apply path means a slow or stalled peer
// send can't pin the worker that produced the delta. Best-effort: a full pool
// drops the broadcast and anti-entropy repairs the peer.
func (s *Store) SetIoPool(pool goroutines.Pool) {
	s.ioPool = pool
}

// queueBroadcast submits an outbound broadcast to the I/O pool, or runs it inline
// when no pool is configured (e.g. tests, single-controller mode). Returns false
// only when a configured pool is full, so the caller can record the drop.
func (s *Store) queueBroadcast(work func()) bool {
	if s.ioPool == nil {
		work()
		return true
	}
	return s.ioPool.QueueOrError(work) == nil
}

// SetMetricsRegistry installs the registry used for store-level and per-stateMap
// metrics. Must be called before any state types are registered, so each
// Register call can wire up its own metrics against this registry.
func (s *Store) SetMetricsRegistry(reg metrics.Registry) {
	s.metricsRegistry = reg
	registerFuncGauge(reg, "gossip.pending_acks", func() int64 {
		return s.pendingAcksCount.Load()
	})
}

// registerFuncGauge registers a functional gauge, disposing any gauge already registered under the name.
//
// The dispose is what makes it safe to call more than once. Every gauge here reads through a closure over a
// Store or a stateMap, and metrics.Registry.FuncGauge is get-or-create: it keeps the FIRST closure it is
// given and silently ignores later ones. So a second registration against the same registry leaves the gauge
// reporting whichever instance registered first, which is the one a caller registering again is replacing.
// Disposing first makes the newest win.
func registerFuncGauge(reg metrics.Registry, name string, f func() int64) {
	if reg == nil {
		return
	}
	if existing := reg.GetGauge(name); existing != nil {
		existing.Dispose()
	}
	reg.FuncGauge(name, f)
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
	// Owners is how many owners the type holds data for, including those whose entries have all drained
	// but whose data has not been compacted out. Entry and tombstone counts cannot show that: an owner
	// that leaks after its last entry is gone contributes nothing to either, so this is the count that
	// reveals per-owner state accumulating across a fleet's churn.
	Owners int `json:"owners"`
}

// GetStats returns per-type entry, tombstone and owner counts.
func (s *Store) GetStats() []StoreStats {
	var stats []StoreStats
	s.types.Range(func(key, value any) bool {
		sm := value.(*stateMap)
		entries := 0
		tombstones := 0
		owners := 0
		sm.owners.IterCb(func(_ string, od *ownerData) {
			owners++
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
			Owners:     owners,
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

// ApplyPeerDelta applies a gossip delta broadcast by a peer controller.
func (s *Store) ApplyPeerDelta(delta *gossip_pb.GossipDelta) {
	s.applyPeerEntries(delta, func(sm *stateMap) *applyMeters { return sm.deltaMeters() })
}

// ApplyAntiEntropyResponse applies the entries a peer sent in answer to an anti-entropy digest. Identical to
// ApplyPeerDelta apart from which counters it records to: a peer answering a digest re-sends every entry for
// an owner the two disagreed about, so most of what arrives here is expected to lose its version check, and
// counting that against the broadcast counters would leave the broadcast path's stale rate unreadable.
func (s *Store) ApplyAntiEntropyResponse(delta *gossip_pb.GossipDelta) {
	s.applyPeerEntries(delta, func(sm *stateMap) *applyMeters { return sm.antiEntropyMeters() })
}

func (s *Store) applyPeerEntries(delta *gossip_pb.GossipDelta, meters func(*stateMap) *applyMeters) {
	sm := s.getStateMap(delta.StoreType)
	if sm == nil {
		pfxlog.Logger().WithField("storeType", delta.StoreType).Warn("received gossip delta for unknown store type")
		return
	}

	m := meters(sm)
	m.markReceived(int64(len(delta.Entries)))
	for _, pbEntry := range delta.Entries {
		sm.applyDeltaFrom(entryFromProto(pbEntry), m)
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

	sm.deltaMeters().markReceived(int64(len(delta.Entries)))

	applied := make([]*entry, 0, len(delta.Entries))
	for _, pbEntry := range delta.Entries {
		e := entryFromProto(pbEntry)
		if sm.applyEntry(e) {
			applied = append(applied, e)
		}
	}

	// One broadcast for the whole delta, not one per entry. See broadcastEntries.
	sm.broadcastEntries(applied)
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

	// ExpiresAt is when this tombstone may be reaped, decided once by whoever created it and replicated
	// with it, so every node expires it at the same moment rather than TTL-from-when-it-last-arrived.
	// Zero on a live entry, and on a tombstone that predates this field.
	ExpiresAt time.Time
}

func (e *entry) toProto() *gossip_pb.GossipEntry {
	pb := &gossip_pb.GossipEntry{
		Key:       e.Key,
		Value:     e.Value,
		Version:   e.Version,
		Owner:     e.Owner,
		Tombstone: e.Tombstone,
		Epoch:     e.Epoch,
	}
	if !e.ExpiresAt.IsZero() {
		pb.ExpiresAt = e.ExpiresAt.UnixNano()
	}
	return pb
}

// untypedListener is the type-erased listener stored in a stateMap.
type untypedListener interface {
	entryChanged(key string, value []byte, version uint64, owner string, isCreate bool, origin ChangeOrigin)
	entryRemoved(key string, owner string, version uint64, origin ChangeOrigin)
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
	name        string
	config      stateMapConfig
	owners      cmap.ConcurrentMap[string, *ownerData]
	store       *Store
	listener    untypedListener
	metrics     *stateMapMetrics
	notifyLocks *concurrency.StripedIdLocker // serializes listener delivery per (owner,key); see notifyEntry
}

// stateMapMetrics holds the meters maintained for a stateMap. Gauges are
// registered with the metrics.Registry directly (poll-time closures over
// stateMap fields) and not held here.
type stateMapMetrics struct {
	delta                    applyMeters // entries from routers and from peer broadcasts
	antiEntropy              applyMeters // entries a peer sent in answer to a digest
	broadcastSent            metrics.Meter
	broadcastDropped         metrics.Meter // broadcasts dropped because the I/O pool was full
	antiEntropyOwnersMatched metrics.Meter // owners short-circuited by hash on incoming digest
	antiEntropyOwnersDiffed  metrics.Meter // owners that required per-entry comparison
}

// applyMeters counts the outcome of applying incoming entries. There is one set per way entries arrive, so
// that repair traffic and live traffic can be read apart: anti-entropy deliberately re-sends entries the
// receiver may already hold, and a single shared stale counter would make the far more interesting
// broadcast-path stale rate impossible to interpret.
//
// All three methods tolerate a nil receiver, so a stateMap built without a metrics registry needs no
// branching at the call sites.
type applyMeters struct {
	received      metrics.Meter
	applied       metrics.Meter
	rejectedStale metrics.Meter
}

// deltaMeters returns the counters for entries arriving from routers or from a peer's broadcast.
func (sm *stateMap) deltaMeters() *applyMeters {
	if sm.metrics == nil {
		return nil
	}
	return &sm.metrics.delta
}

// antiEntropyMeters returns the counters for entries arriving in answer to an anti-entropy digest.
func (sm *stateMap) antiEntropyMeters() *applyMeters {
	if sm.metrics == nil {
		return nil
	}
	return &sm.metrics.antiEntropy
}

// markReceived records n entries arriving to be applied.
func (m *applyMeters) markReceived(n int64) {
	if m != nil && n > 0 {
		m.received.Mark(n)
	}
}

// markApplied records an incoming entry that won its version check.
func (m *applyMeters) markApplied() {
	if m != nil {
		m.applied.Mark(1)
	}
}

// markRejectedStale records an incoming entry rejected because its version was
// not newer than the local copy (or the owner was drained).
func (m *applyMeters) markRejectedStale() {
	if m != nil {
		m.rejectedStale.Mark(1)
	}
}

// markBroadcastSent records a broadcast initiated by this controller.
func (sm *stateMap) markBroadcastSent(n int64) {
	if sm.metrics != nil && n > 0 {
		sm.metrics.broadcastSent.Mark(n)
	}
}

// markBroadcastDropped records a broadcast dropped because the I/O pool was full.
// The peer is reconciled by anti-entropy, so the drop is recoverable; a sustained
// nonzero rate means the I/O pool is undersized or peers are stalled.
func (sm *stateMap) markBroadcastDropped(n int64) {
	if sm.metrics != nil && n > 0 {
		sm.metrics.broadcastDropped.Mark(n)
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
		delta: applyMeters{
			received:      reg.Meter(prefix + ".delta.received"),
			applied:       reg.Meter(prefix + ".delta.applied"),
			rejectedStale: reg.Meter(prefix + ".delta.rejected_stale"),
		},
		antiEntropy: applyMeters{
			received:      reg.Meter(prefix + ".anti_entropy.entries.received"),
			applied:       reg.Meter(prefix + ".anti_entropy.entries.applied"),
			rejectedStale: reg.Meter(prefix + ".anti_entropy.entries.rejected_stale"),
		},
		broadcastSent:            reg.Meter(prefix + ".broadcast.sent"),
		broadcastDropped:         reg.Meter(prefix + ".broadcast.dropped"),
		antiEntropyOwnersMatched: reg.Meter(prefix + ".anti_entropy.owners_matched"),
		antiEntropyOwnersDiffed:  reg.Meter(prefix + ".anti_entropy.owners_diffed"),
	}
	// Every gauge below has to go through registerFuncGauge: a stateMap is re-registered whenever its store
	// is rebuilt, and a plain registration would leave the gauge closed over the discarded one.
	funcGauge := func(name string, f func() int64) {
		registerFuncGauge(reg, name, f)
	}
	funcGauge(prefix+".owners", func() int64 {
		return int64(sm.owners.Count())
	})
	funcGauge(prefix+".entries.live", func() int64 {
		var live int64
		sm.owners.IterCb(func(_ string, od *ownerData) {
			od.mu.RLock()
			live += od.nonTombstones
			od.mu.RUnlock()
		})
		return live
	})
	funcGauge(prefix+".entries.tombstones", func() int64 {
		var tombstones int64
		sm.owners.IterCb(func(_ string, od *ownerData) {
			od.mu.RLock()
			tombstones += int64(len(od.entries)) - od.nonTombstones
			od.mu.RUnlock()
		})
		return tombstones
	})
	funcGauge(prefix+".owners.drained", func() int64 {
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
// For tombstones=true state types, the ownerData waits for its tombstones to
// age out via reapTombstones; the reaper then removes the ownerData from
// sm.owners under the cmap shard write lock with a check that re-validates
// drained && empty atomically. For tombstones=false state types there is no
// reaper, so dropOwner compacts the ownerData synchronously after clearing
// entries. In both cases, if a new write for the same owner arrives after
// removal, a fresh non-drained ownerData is created.
type ownerData struct {
	mu            sync.RWMutex
	entries       map[string]*entry // key -> entry, including tombstones
	hash          uint64            // FNV-64a of sorted (key||version) over live entries
	hashDirty     bool              // hash needs recomputation
	fullHash      uint64            // FNV-64a over all entries including tombstones
	fullHashDirty bool              // fullHash needs recomputation
	maxVersion    uint64            // highest version ever observed for this owner
	nonTombstones int64             // count of live (non-tombstone) entries
	drained       bool              // dropOwner called; rejects writes and eligible for compaction
}

func newOwnerData() *ownerData {
	return &ownerData{
		entries:       map[string]*entry{},
		hashDirty:     true,
		fullHashDirty: true,
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
		name:        name,
		config:      cfg,
		owners:      cmap.New[*ownerData](),
		store:       store,
		listener:    listener,
		notifyLocks: concurrency.NewStripedIdLocker(256),
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
// hashDirty, fullHashDirty) are updated in-place. The caller is responsible
// for listener notification and broadcast after releasing the lock.
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
	// fullHash sees every entry change (live or tombstone) since it covers
	// both. hashDirty only flips when the live set changes.
	od.fullHashDirty = true
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

	sm.notifyEntry(od, e, isCreate, origin)

	if origin == OriginLocal {
		sm.broadcastDelta(e)
	}
}

// applyDelta applies an entry broadcast by a peer controller.
func (sm *stateMap) applyDelta(incoming *entry) {
	sm.applyDeltaFrom(incoming, sm.deltaMeters())
}

// applyDeltaFrom applies an entry received from a peer controller, recording the outcome against m. The
// counters are the caller's choice because the same apply serves both the broadcast and the anti-entropy
// repair path, and those two want to be read apart.
func (sm *stateMap) applyDeltaFrom(incoming *entry, m *applyMeters) {
	incoming.UpdatedAt = time.Now()
	sm.clampDeadline(incoming)
	sm.store.observeVersion(incoming.Version)

	od := sm.getOrCreateOwner(incoming.Owner)
	od.mu.Lock()
	wasSet, isCreate := sm.applyEntryLocked(od, incoming)
	od.mu.Unlock()

	if !wasSet {
		m.markRejectedStale()
		return
	}
	m.markApplied()

	sm.notifyEntry(od, incoming, isCreate, OriginGossip)
}

// applyAndBroadcast applies an externally-versioned entry (e.g., from a router's
// own Lamport clock) and broadcasts it to peer controllers. This is the path for
// router-originated gossip: the entry arrives via the ctrl channel, gets applied
// locally, and is fanned out to peers.
func (sm *stateMap) applyAndBroadcast(incoming *entry) {
	if sm.applyEntry(incoming) {
		sm.broadcastEntries([]*entry{incoming})
	}
}

// applyEntry applies an externally-versioned entry and delivers its listener notification, reporting
// whether it won its version check and so needs sending on to peers. Callers applying several entries
// collect the winners and broadcast them together.
func (sm *stateMap) applyEntry(incoming *entry) bool {
	incoming.UpdatedAt = time.Now()
	sm.clampDeadline(incoming)
	sm.store.observeVersion(incoming.Version)

	od := sm.getOrCreateOwner(incoming.Owner)
	od.mu.Lock()
	wasSet, isCreate := sm.applyEntryLocked(od, incoming)
	od.mu.Unlock()

	if !wasSet {
		sm.deltaMeters().markRejectedStale()
		return false
	}
	sm.deltaMeters().markApplied()

	sm.notifyEntry(od, incoming, isCreate, OriginLocal)
	return true
}

// notifyKey returns the striped-lock id that serializes listener delivery for a
// single (owner, key). The separator keeps distinct (owner, key) pairs from
// colliding into one id.
func notifyKey(owner, key string) string {
	return owner + "\x00" + key
}

// notifyEntry delivers the listener callback for an applied entry, serialized
// per (owner, key) so that concurrent applies for the same key cannot deliver
// their callbacks out of version order. The apply itself is version-ordered
// under od.mu, but the listener call happens outside that lock (holding it
// across the link-manager lock would invert lock order); without serialization
// a stale live entry's entryChanged could land after a newer tombstone's
// entryRemoved, resurrecting a link in the manager even though the store
// correctly holds the tombstone. Under the per-key lock we re-check that e is
// still the current entry for its key; if a newer version has superseded it,
// the callback is dropped, since that newer entry delivers the authoritative
// state. This keeps listener-driven consumers (the link manager, the canary
// map) consistent with the store.
func (sm *stateMap) notifyEntry(od *ownerData, e *entry, isCreate bool, origin ChangeOrigin) {
	if sm.listener == nil {
		return
	}
	unlock := sm.notifyLocks.LockFor(notifyKey(e.Owner, e.Key))
	defer unlock()

	od.mu.RLock()
	current := od.entries[e.Key] == e
	od.mu.RUnlock()
	if !current {
		return
	}

	if e.Tombstone {
		sm.listener.entryRemoved(e.Key, e.Owner, e.Version, origin)
	} else {
		sm.listener.entryChanged(e.Key, e.Value, e.Version, e.Owner, isCreate, origin)
	}
}

// notifyRemoval delivers an entryRemoved callback for a key that was deleted
// outright (tombstones-disabled state types). Serialized per (owner, key) like
// notifyEntry; the callback is dropped if a newer apply has re-added the key,
// since that apply's notifyEntry delivers the live state.
func (sm *stateMap) notifyRemoval(od *ownerData, key, owner string, version uint64, origin ChangeOrigin) {
	if sm.listener == nil {
		return
	}
	unlock := sm.notifyLocks.LockFor(notifyKey(owner, key))
	defer unlock()

	od.mu.RLock()
	_, present := od.entries[key]
	od.mu.RUnlock()
	if present {
		return
	}

	sm.listener.entryRemoved(key, owner, version, origin)
}

// delete removes the entry for (owner, key), by tombstone where the state type keeps them and outright where
// it does not, reporting whether the entry went away.
//
// A tombstone is versioned from this controller's clock and has to beat the entry it replaces, so it can lose:
// a controller whose clock is still below an entry's version cannot tombstone it. The clock catches up as soon
// as anything is applied, so this is a window rather than a wedge, but a caller that reports on what it removed
// has to ask rather than assume.
func (sm *stateMap) delete(key, owner string, origin ChangeOrigin) bool {
	return sm.deleteIf(key, owner, origin, nil) == deleteApplied
}

// deleteOutcome says what a conditional delete did, so a caller reporting on a removal can say which of the
// ways it did not happen applies.
type deleteOutcome int

const (
	// deleteApplied means the entry was tombstoned, or removed outright where the state type keeps no
	// tombstones.
	deleteApplied deleteOutcome = iota

	// deleteSkipped means the precondition no longer held: what was at the key had changed, or gone.
	deleteSkipped

	// deleteRejected means the removal lost its version check, which for a tombstone means this controller's
	// clock is still below the version of the entry it was trying to replace.
	deleteRejected
)

// deleteIf removes the entry at key, but only while stillWanted holds for whatever is there. A nil stillWanted
// removes unconditionally.
//
// The precondition is evaluated under the same lock as the removal, which is the point of it. A caller that
// selected entries in an earlier pass is acting on a snapshot: the entry at a key can be replaced or reaped in
// between, so an unconditional removal would tombstone whatever now occupies the key, and report having removed
// the one that was chosen. For an epoch sweep that means a replacement carrying a newer epoch could be
// tombstoned in place of the old entry it superseded.
func (sm *stateMap) deleteIf(key, owner string, origin ChangeOrigin, stillWanted func(existing *entry, present bool) bool) deleteOutcome {
	if sm.config.tombstones {
		version := sm.store.nextVersion()
		e := &entry{
			Key:       key,
			Version:   version,
			Owner:     owner,
			Tombstone: true,
			UpdatedAt: time.Now(),
			ExpiresAt: sm.tombstoneDeadline(),
		}

		od := sm.getOrCreateOwner(owner)
		od.mu.Lock()
		existing, present := od.entries[key]
		if stillWanted != nil && !stillWanted(existing, present) {
			od.mu.Unlock()
			return deleteSkipped
		}
		if present {
			// Carry the replaced entry's epoch onto the tombstone so the wire
			// entry stays self-consistent with what it supersedes.
			e.Epoch = existing.Epoch
		}
		wasSet, _ := sm.applyEntryLocked(od, e)
		od.mu.Unlock()

		if !wasSet {
			return deleteRejected
		}

		sm.notifyEntry(od, e, false, origin)
		if origin == OriginLocal {
			sm.broadcastDelta(e)
		}
		return deleteApplied
	} else {
		od := sm.getOrCreateOwner(owner)
		od.mu.Lock()
		existing, ok := od.entries[key]
		if stillWanted != nil && !stillWanted(existing, ok) {
			od.mu.Unlock()
			return deleteSkipped
		}
		delete(od.entries, key)
		notify := ok && !existing.Tombstone
		var version uint64
		if ok {
			od.fullHashDirty = true
			version = existing.Version
		}
		if notify {
			od.nonTombstones--
			od.hashDirty = true
		}
		od.mu.Unlock()

		if notify {
			sm.notifyRemoval(od, key, owner, version, origin)
		}
		if !ok {
			return deleteSkipped
		}
		return deleteApplied
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
		key     string
		e       *entry // nil for tombstones=false (no broadcast)
		version uint64 // deleted entry's version, for tombstones=false removals
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
				ExpiresAt: sm.tombstoneDeadline(),
			}
			od.entries[key] = ts
			od.nonTombstones--
			if version > od.maxVersion {
				od.maxVersion = version
			}
			produced = append(produced, tombstoned{key: key, e: ts})
		}
	} else {
		for key, existing := range od.entries {
			produced = append(produced, tombstoned{key: key, version: existing.Version})
		}
		od.entries = map[string]*entry{}
		od.nonTombstones = 0
	}
	if len(produced) > 0 {
		od.hashDirty = true
		od.fullHashDirty = true
	}
	od.mu.Unlock()

	// Fire listener and broadcast outside the lock to avoid lock-order
	// inversions with downstream consumers (e.g., the link manager lock).
	// Route through the notify helpers so delivery stays serialized and
	// version-ordered against any concurrent apply for this owner.
	for _, t := range produced {
		if t.e != nil {
			sm.notifyEntry(od, t.e, false, origin)
			if origin == OriginLocal {
				sm.broadcastDelta(t.e)
			}
		} else {
			sm.notifyRemoval(od, t.key, owner, t.version, origin)
		}
	}

	// For tombstones=false state types there's nothing to age out and the
	// reaper does not run, so the ownerData would be drained forever and
	// never compacted out of sm.owners. Compact it synchronously here. The
	// RemoveCb predicate re-checks drained && empty under the owner lock so
	// a racing write (which would have been rejected anyway thanks to
	// drained=true) doesn't cause us to drop a fresh ownerData.
	if !sm.config.tombstones {
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
	oldEpochs := map[string][]byte{}
	toDelete := od.collectKeysToDelete(func(key string, e *entry) bool {
		if bytes.Compare(e.Epoch, epoch) < 0 {
			oldEpochs[key] = e.Epoch
			return true
		}
		return false
	})

	// Removed under a recheck of the epoch, and logged per outcome. The keys were selected in the pass above, so
	// by now the entry at one can have been replaced or reaped: removing unconditionally would tombstone
	// whatever is there, which for a replacement carrying a newer epoch means deleting the state that
	// superseded what this sweep is for. Reporting the intent rather than the outcome also left the log
	// asserting a cleanup that had not happened, which is worse than silence for anyone reading back to work
	// out why old-epoch entries were still there.
	for _, key := range toDelete {
		log := pfxlog.Logger().
			WithField("key", key).
			WithField("owner", owner).
			WithField("entryEpoch", fmt.Sprintf("%x", oldEpochs[key])).
			WithField("cleanupEpoch", fmt.Sprintf("%x", epoch))

		outcome := sm.deleteIf(key, owner, origin, func(existing *entry, present bool) bool {
			return present && bytes.Compare(existing.Epoch, epoch) < 0
		})

		switch outcome {
		case deleteApplied:
			log.Info("epoch cleanup: deleted old-epoch entry")
		case deleteSkipped:
			log.Info("epoch cleanup: old-epoch entry already replaced or gone, left alone")
		default:
			log.Warn("epoch cleanup: old-epoch entry not deleted, tombstone lost the version check")
		}
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

// getOwnerDigests returns per-owner FNV-64a hashes (over live + tombstone
// entries) for every owner that has any entries. Used to populate
// GossipDigest.OwnerDigests so the receiver can short-circuit owners that
// already match without doing per-entry version comparison.
//
// Uses hashForOwnerFull rather than the live-only hashForOwner so tombstone
// divergence (e.g., one controller has a tombstone the other never saw) is
// still detectable by anti-entropy.
func (sm *stateMap) getOwnerDigests() []*gossip_pb.OwnerDigest {
	// Collect owner identifiers first, then hash outside the cmap iteration
	// so hashForOwnerFull (which acquires its own owner.mu) doesn't nest
	// with the cmap shard lock more than necessary.
	var owners []string
	sm.owners.IterCb(func(owner string, _ *ownerData) {
		owners = append(owners, owner)
	})
	digests := make([]*gossip_pb.OwnerDigest, 0, len(owners))
	for _, owner := range owners {
		digests = append(digests, &gossip_pb.OwnerDigest{
			Owner: owner,
			Hash:  sm.hashForOwnerFull(owner),
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

// tombstoneDeadline returns when a tombstone created now may be reaped. Zero when the store keeps no
// tombstones or expires none, so entries from such a store carry no deadline.
func (sm *stateMap) tombstoneDeadline() time.Time {
	if !sm.config.tombstones || sm.config.tombstoneTTL == 0 {
		return time.Time{}
	}
	return time.Now().Add(sm.config.tombstoneTTL)
}

// clampDeadline bounds an incoming tombstone's deadline to one TTL from now, and supplies one when the
// entry carries none.
//
// A creator whose clock runs fast would otherwise stamp a deadline far enough ahead that the tombstone is
// never reaped anywhere, which is worse than the leak the deadline exists to prevent, since a tombstone
// that cannot be reaped also stops its owner's data being compacted. A deadline already in the past is
// left alone: it belongs to a tombstone that has done its job and should go on the next pass.
func (sm *stateMap) clampDeadline(e *entry) {
	if !e.Tombstone || !sm.config.tombstones || sm.config.tombstoneTTL == 0 {
		return
	}
	limit := time.Now().Add(sm.config.tombstoneTTL)
	if e.ExpiresAt.IsZero() || e.ExpiresAt.After(limit) {
		e.ExpiresAt = limit
	}
}

// reapTombstones drops tombstones whose deadline has passed.
//
// Expiry is the reaper's alone. An expired tombstone arriving from a peer is still applied, because a
// creator with a slow clock would otherwise have its removals refused on arrival and the entry they
// delete would survive: losing a delete is worse than reaping one late. Such a tombstone applies, removes
// the live entry, and is dropped on the next pass, so a tombstone returned by anti-entropy after another
// node reaped it costs one interval rather than a full TTL.
func (sm *stateMap) reapTombstones() {
	if !sm.config.tombstones || sm.config.tombstoneTTL == 0 {
		return
	}
	now := time.Now()

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
		reaped := 0
		for key, e := range od.entries {
			if e.Tombstone && !e.ExpiresAt.IsZero() && e.ExpiresAt.Before(now) {
				delete(od.entries, key)
				reaped++
			}
		}
		// Reaping removes tombstones from od.entries, which changes the
		// full-hash set (not the live-only hash — those are already
		// excluded). Mark dirty so anti-entropy short-circuit recomputes.
		if reaped > 0 {
			od.fullHashDirty = true
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

// broadcastDelta fans a single entry out to peer controllers, for the paths that produce one entry at a
// time. Paths applying a whole delta use broadcastEntries so the fan-out is one message rather than one per
// entry.
func (sm *stateMap) broadcastDelta(e *entry) {
	sm.broadcastEntries([]*entry{e})
}

// broadcastEntries fans applied entries out to peer controllers as a single delta.
//
// Batching per delta rather than per entry matters at scale. A delta from a reconnecting router in a large
// mesh carries an entry per link, so broadcasting each separately multiplied the messages every peer
// received, the protos marshalled, and the I/O pool work by the entry count. Measured on a 400 router mesh
// that was enough to drop about a fifth of broadcasts outright.
func (sm *stateMap) broadcastEntries(entries []*entry) {
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
		pfxlog.Logger().WithError(err).Error("failed to marshal gossip delta")
		return
	}
	msg := channel.NewMessage(gossip_pb.GossipDeltaType, body)
	// Broadcast on the I/O pool so a slow/stalled peer send can't pin the apply
	// worker that produced this delta. Best-effort: a full pool drops the
	// broadcast and anti-entropy repairs the peer.
	//
	// The counters stay per entry so they remain comparable with the delta counters and across this change,
	// even though a drop now discards a whole delta's worth at once.
	if sm.store.queueBroadcast(func() { sm.store.mesh.Broadcast(msg) }) {
		sm.markBroadcastSent(int64(len(entries)))
	} else {
		sm.markBroadcastDropped(int64(len(entries)))
	}
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

// hashForOwnerFull returns a FNV-64a hash of the owner's entries including
// tombstones. Used by anti-entropy's per-owner short-circuit. The live-only
// hashForOwner is unsuitable there because two controllers can have matching
// live entries while differing on tombstones, and the short-circuit would
// suppress the divergence repair.
//
// Tombstone keys are mixed in with a sentinel byte so a key that exists as
// live with version V hashes differently from the same key as tombstone with
// version V.
func (sm *stateMap) hashForOwnerFull(owner string) uint64 {
	od := sm.getOwner(owner)
	if od == nil {
		return 0
	}

	od.mu.RLock()
	if !od.fullHashDirty {
		h := od.fullHash
		od.mu.RUnlock()
		return h
	}
	od.mu.RUnlock()

	od.mu.Lock()
	defer od.mu.Unlock()
	if !od.fullHashDirty {
		return od.fullHash
	}

	type entryKey struct {
		key       string
		version   uint64
		tombstone bool
	}
	keys := make([]entryKey, 0, len(od.entries))
	for k, e := range od.entries {
		keys = append(keys, entryKey{key: k, version: e.Version, tombstone: e.Tombstone})
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].key < keys[j].key })

	h := fnv.New64a()
	var buf [8]byte
	for _, k := range keys {
		_, _ = h.Write([]byte(k.key))
		binary.LittleEndian.PutUint64(buf[:], k.version)
		_, _ = h.Write(buf[:])
		if k.tombstone {
			_, _ = h.Write([]byte{1})
		} else {
			_, _ = h.Write([]byte{0})
		}
	}
	od.fullHash = h.Sum64()
	od.fullHashDirty = false
	return od.fullHash
}

func entryFromProto(pb *gossip_pb.GossipEntry) *entry {
	e := &entry{
		Key:       pb.Key,
		Value:     pb.Value,
		Version:   pb.Version,
		Owner:     pb.Owner,
		Tombstone: pb.Tombstone,
		Epoch:     pb.Epoch,
		UpdatedAt: time.Now(),
	}
	if pb.ExpiresAt != 0 {
		e.ExpiresAt = time.Unix(0, pb.ExpiresAt)
	}
	return e
}
