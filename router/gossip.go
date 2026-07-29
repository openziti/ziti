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

package router

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xlink"
	cmap "github.com/orcaman/concurrent-map/v2"
	"google.golang.org/protobuf/proto"
)

const linkGossipStoreType = "links"

const (
	staleCanaryThreshold = uint64(6) // 6 ticks × 5s = 30 seconds
	refreshCheckInterval = 15 * time.Second
)

// Controller-digest reconcile tuning. A router periodically asks every connected
// controller for its digest so HandleDigest can tombstone entries a controller
// still holds that the router no longer has. Two gates keep this cheap at scale:
//   - hash-gate: the request carries the router's entry hash; the controller
//     replies with a full digest only when it differs from its own view, so an
//     in-sync pair exchanges only the tiny request.
//   - churn-gate: a router reconciles only while its advertised set has changed
//     within gossipReconcileQuietPeriod. A stable router can't have caused a
//     newly-missed tombstone, so it has nothing to heal; controller reconnects
//     are covered separately by the bind-time digest. This makes the cost
//     proportional to churn, not to router count.
//
// Vars (not consts) so they're tunable. The gates make a short interval cheap,
// so heal latency is ~1m (well within the 15m validation deadline); the quiet
// period is insurance after churn stops (controller reconnects past it are
// covered by the bind-time digest).
var (
	gossipReconcileInterval    = 1 * time.Minute
	gossipReconcileQuietPeriod = 3 * time.Minute
)

// maxSentVersions tracks the highest gossip version successfully sent per store
// type. Only advances after a confirmed send, never on clock observation or
// failed sends.
type maxSentVersions struct {
	versions sync.Map // storeType (string) -> *atomic.Uint64
}

func (m *maxSentVersions) update(storeType string, v uint64) {
	val, _ := m.versions.LoadOrStore(storeType, &atomic.Uint64{})
	p := val.(*atomic.Uint64)
	for {
		cur := p.Load()
		if v <= cur {
			return
		}
		if p.CompareAndSwap(cur, v) {
			return
		}
	}
}

func (m *maxSentVersions) get(storeType string) uint64 {
	val, ok := m.versions.Load(storeType)
	if !ok {
		return 0
	}
	return val.(*atomic.Uint64).Load()
}

func (m *maxSentVersions) getAll() map[string]uint64 {
	result := map[string]uint64{}
	m.versions.Range(func(key, value any) bool {
		result[key.(string)] = value.(*atomic.Uint64).Load()
		return true
	})
	return result
}

// gossipSource is the per-store-type adapter over a router subsystem's source of
// truth (links today; terminators later). It yields the entries the router
// should currently be advertising for its type, so the generic gossip core can
// reconcile and answer digests without knowing about any specific subsystem.
type gossipSource interface {
	storeType() string
	// iterateAdvertised yields the gossip key and marshaled value for each
	// source-of-truth item that should currently be advertised.
	iterateAdvertised(fn func(key string, value []byte))
}

// linkGossipSource adapts the router's xlink registry as a gossip source. Only
// dialer-side links are advertised (the dialer owns the gossip entry). Its
// linkIterator is wired after the link registry is created.
type linkGossipSource struct {
	linkIterator func() <-chan xlink.Xlink
}

func (s *linkGossipSource) storeType() string { return linkGossipStoreType }

func (s *linkGossipSource) iterateAdvertised(fn func(key string, value []byte)) {
	if s.linkIterator == nil {
		return
	}
	for l := range s.linkIterator() {
		if !l.IsDialed() {
			continue
		}
		value, err := marshalLink(l)
		if err != nil {
			continue
		}
		fn(linkGossipKey(l.Id(), l.Iteration()), value)
	}
}

// gossipStore holds the router's advertised entry set for one store type plus
// its source of truth. The generic gossip core (gossipClient) operates over a
// set of these, one per registered store type.
type gossipStore struct {
	source         gossipSource
	currentEntries cmap.ConcurrentMap[string, *gossip_pb.GossipEntry]
	lastChange     atomic.Int64              // UnixNano of the last currentEntries change (reconcile churn-gate)
	cached         atomic.Pointer[hashCount] // cached hash+count derived from currentEntries; nil = dirty
}

// hashCount is the hash and non-tombstone count derived from a gossipStore's
// currentEntries in a single pass and cached together, so the hash and count a
// router reports are always from the same snapshot and cannot disagree. Deriving
// the count (rather than maintaining a separate incremental counter) avoids the
// drift that left routers permanently off-by-one against the controller.
type hashCount struct {
	hash  uint64
	count int64
}

func newGossipStore(source gossipSource) *gossipStore {
	return &gossipStore{
		source:         source,
		currentEntries: cmap.New[*gossip_pb.GossipEntry](),
	}
}

// invalidateHash marks the cached hash+count dirty and stamps the change time.
// Called on every currentEntries mutation.
func (s *gossipStore) invalidateHash() {
	s.cached.Store(nil)
	s.lastChange.Store(time.Now().UnixNano())
}

// changedWithin reports whether this store's advertised set changed within the
// given window (drives the reconcile churn-gate).
func (s *gossipStore) changedWithin(window time.Duration) bool {
	last := s.lastChange.Load()
	return last != 0 && time.Since(time.Unix(0, last)) <= window
}

// getCached returns the store's cached hash+count, deriving both from
// currentEntries in a single pass on the first call after a mutation. The hash is
// FNV-64a over sorted non-tombstone (key,version) pairs and must match the
// controller's HashForOwner algorithm; the count is the number of non-tombstone
// entries in that same pass, so hash and count never disagree.
func (s *gossipStore) getCached() *hashCount {
	if hc := s.cached.Load(); hc != nil {
		return hc
	}
	type keyVersion struct {
		key     string
		version uint64
	}
	var kvs []keyVersion
	s.currentEntries.IterCb(func(key string, e *gossip_pb.GossipEntry) {
		if !e.Tombstone {
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
	hc := &hashCount{hash: h.Sum64(), count: int64(len(kvs))}
	s.cached.Store(hc)
	return hc
}

// hash returns the cached FNV-64a hash of this store's non-tombstone entries.
func (s *gossipStore) hash() uint64 {
	return s.getCached().hash
}

// count returns the cached non-tombstone entry count, derived in the same pass
// as the hash so the two always agree.
func (s *gossipStore) count() int64 {
	return s.getCached().count
}

// gossipClient is a lightweight gossip participant for the router. It maintains a
// Lamport clock and a set of per-store-type gossipStores, generating GossipDelta
// messages for state changes and sending them to the subscription controller and
// any stale controllers.
type gossipClient struct {
	routerId         string
	epoch            []byte // UUIDv7 generated on startup, identifies this router lifetime
	ctrls            env.NetworkControllers
	clock            atomic.Uint64
	maxSent          maxSentVersions
	stores           map[string]*gossipStore // store type -> store; set at construction, immutable after
	staleCtrlIds     atomic.Value            // holds map[string]bool
	staleSendEntries metrics.Meter           // entries duplicate-sent via sendToStaleControllers; nil-safe
}

func newGossipClient(routerId string, ctrls env.NetworkControllers, reg metrics.Registry) *gossipClient {
	epoch := idgen.NewEpochBytes()
	pfxlog.Logger().WithField("routerId", routerId).
		WithField("epoch", idgen.FormatEpoch(epoch)).
		Info("gossip client starting with new epoch")

	g := &gossipClient{
		routerId: routerId,
		epoch:    epoch,
		ctrls:    ctrls,
		stores: map[string]*gossipStore{
			linkGossipStoreType: newGossipStore(&linkGossipSource{}),
			linkMetricsGossipStoreType: newGossipStore(&linkMetricsGossipSource{
				metricsRegistry: reg,
				lastPublished:   map[string]publishedMetric{},
			}),
		},
	}
	g.staleCtrlIds.Store(map[string]bool{})
	if reg != nil {
		g.staleSendEntries = reg.Meter("router.gossip.stale_send.entries")
	}
	return g
}

// linkStore returns the links gossip store.
func (g *gossipClient) linkStore() *gossipStore {
	return g.stores[linkGossipStoreType]
}

// setLinkIterator wires the link registry's iterator into the link gossip
// sources (link state and link metrics). Called after the link registry is
// created.
func (g *gossipClient) setLinkIterator(fn func() <-chan xlink.Xlink) {
	g.linkStore().source.(*linkGossipSource).linkIterator = fn
	g.linkMetricsStore().source.(*linkMetricsGossipSource).linkIterator = fn
}

func (g *gossipClient) nextVersion() uint64 {
	return g.clock.Add(1)
}

// NextVersion returns the next Lamport clock version. Exposed so the link
// registry can stamp versions in the event loop at collection time.
func (g *gossipClient) NextVersion() uint64 {
	return g.nextVersion()
}

// GetMaxSentVersions returns the max successfully sent gossip version for each
// store type. Used by the canary emitter to report staleness information.
func (g *gossipClient) GetMaxSentVersions() map[string]uint64 {
	return g.maxSent.getAll()
}

// GetEntryHashes returns a per-store-type FNV-64a hash of sorted non-tombstone
// gossip keys. Used by the canary emitter (and digest requests) so controllers
// can detect state divergence per store type.
func (g *gossipClient) GetEntryHashes() map[string]uint64 {
	result := make(map[string]uint64, len(g.stores))
	for st, s := range g.stores {
		result[st] = s.hash()
	}
	return result
}

// GetEntryCounts returns the number of non-tombstone entries per store type.
func (g *gossipClient) GetEntryCounts() map[string]int64 {
	result := make(map[string]int64, len(g.stores))
	for st, s := range g.stores {
		result[st] = s.count()
	}
	return result
}

// changedWithin reports whether any store's advertised set changed within the
// given window. Used to gate the periodic controller-digest reconcile to routers
// that have actually churned (see gossipReconcileQuietPeriod).
func (g *gossipClient) changedWithin(window time.Duration) bool {
	for _, s := range g.stores {
		if s.changedWithin(window) {
			return true
		}
	}
	return false
}

// observeVersion advances the Lamport clock to at least the given version.
func (g *gossipClient) observeVersion(v uint64) {
	for {
		cur := g.clock.Load()
		if v <= cur {
			return
		}
		if g.clock.CompareAndSwap(cur, v) {
			return
		}
	}
}

// linkGossipKey builds the composite gossip key for a link instance.
func linkGossipKey(linkId string, iteration uint32) string {
	return fmt.Sprintf("%s:%d", linkId, iteration)
}

// parseLinkGossipKey extracts the linkId and iteration from a composite gossip key.
func parseLinkGossipKey(key string) (string, uint32) {
	idx := strings.LastIndex(key, ":")
	if idx < 0 {
		return key, 0
	}
	iter, err := strconv.ParseUint(key[idx+1:], 10, 32)
	if err != nil {
		return key, 0
	}
	return key[:idx], uint32(iter)
}

// InspectGossipLinks returns the router's own published link gossip entries, for
// diffing against the link registry.
func (g *gossipClient) InspectGossipLinks() *inspect.RouterGossipLinksInspect {
	result := &inspect.RouterGossipLinksInspect{}
	g.linkStore().currentEntries.IterCb(func(key string, e *gossip_pb.GossipEntry) {
		linkId, iteration := parseLinkGossipKey(key)
		entry := inspect.RouterGossipLinkEntry{
			Key:       key,
			LinkId:    linkId,
			Iteration: iteration,
			Owner:     e.Owner,
			Version:   e.Version,
			Tombstone: e.Tombstone,
			Epoch:     idgen.FormatEpoch(e.Epoch),
		}
		if !e.Tombstone && len(e.Value) > 0 {
			link := &ctrl_pb.RouterLinks_RouterLink{}
			if err := proto.Unmarshal(e.Value, link); err == nil {
				entry.DestRouter = link.DestRouterId
			}
		}
		result.Entries = append(result.Entries, entry)
	})
	result.Count = len(result.Entries)
	return result
}

// InspectGossipStore returns summary stats for the router's local gossip store.
func (g *gossipClient) InspectGossipStore() *inspect.RouterGossipStoreInspect {
	live, tombstones := 0, 0
	var entryCount int64
	for _, s := range g.stores {
		s.currentEntries.IterCb(func(_ string, e *gossip_pb.GossipEntry) {
			if e.Tombstone {
				tombstones++
			} else {
				live++
			}
		})
		entryCount += s.count()
	}
	return &inspect.RouterGossipStoreInspect{
		RouterId:     g.routerId,
		Epoch:        idgen.FormatEpoch(g.epoch),
		LamportClock: g.clock.Load(),
		EntryCount:   entryCount,
		LiveEntries:  live,
		Tombstones:   tombstones,
		EntryHash:    g.linkStore().hash(),
		MaxSent:      g.maxSent.getAll(),
	}
}

func (g *gossipClient) getStaleControllers() map[string]bool {
	return g.staleCtrlIds.Load().(map[string]bool)
}

func (g *gossipClient) setStaleControllers(stale map[string]bool) {
	g.staleCtrlIds.Store(stale)
}

// NotifyLinks sends gossip deltas for new or updated links to the subscription
// controller and any stale controllers. Each link carries a pre-assigned
// version from NextVersion, stamped at collection time in the event loop.
func (g *gossipClient) NotifyLinks(links []env.VersionedLink) error {
	sub := g.ctrls.GetSubscriptionController()
	if sub == nil || !sub.IsConnected() {
		return errors.New("subscription controller unavailable")
	}

	s := g.linkStore()
	var entries []*gossip_pb.GossipEntry
	for _, vl := range links {
		value, err := marshalLink(vl.Link)
		if err != nil {
			pfxlog.Logger().WithError(err).WithField("linkId", vl.Link.Id()).
				Error("failed to marshal link for gossip")
			continue
		}

		entry := &gossip_pb.GossipEntry{
			Key:     linkGossipKey(vl.Link.Id(), vl.Link.Iteration()),
			Value:   value,
			Version: vl.Version,
			Owner:   g.routerId,
			Epoch:   g.epoch,
		}
		entries = append(entries, entry)
	}

	if len(entries) == 0 {
		return nil
	}

	if err := g.sendDelta(sub.Channel(), s.source.storeType(), entries); err != nil {
		return err
	}

	for _, entry := range entries {
		s.currentEntries.Set(entry.Key, entry)
	}
	s.invalidateHash()
	for _, e := range entries {
		pfxlog.Logger().WithField("gossipKey", e.Key).
			WithField("version", e.Version).
			Info("sent link gossip entry")
	}
	g.updateMaxSentFromEntries(s.source.storeType(), entries)
	g.sendToStaleControllers(sub.Channel().Id(), s.source.storeType(), entries)

	// Supersede older iterations of any link just published. A re-dial bumps the
	// iteration; the previous iteration's entry must not linger in the advertised
	// set, since its own fault may have failed to send and then been coalesced
	// into the newer iteration's fault. Best-effort: the live publish already
	// succeeded, and a later publish or fault for the link will retry.
	maxByLink := map[string]uint32{}
	for _, e := range entries {
		linkId, iter := parseLinkGossipKey(e.Key)
		if cur, ok := maxByLink[linkId]; !ok || iter > cur {
			maxByLink[linkId] = iter
		}
	}
	if tombs := g.buildSupersededTombstones(s, maxByLink); len(tombs) > 0 {
		if err := g.applyTombstones(sub, s, tombs); err != nil {
			pfxlog.Logger().WithError(err).
				Warn("failed to send superseded-iteration tombstones, will retry")
		} else {
			pfxlog.Logger().WithField("tombstones", len(tombs)).
				Info("superseded older link iterations via gossip")
		}
	}
	return nil
}

// NotifyLinkFault sends a gossip tombstone for a faulted link. It also tombstones
// any older-iteration entries still held for the same link (tombstone-the-range):
// a fault for iteration N supersedes every iteration up to N. This prevents an
// older iteration from lingering when its own fault was never delivered (e.g. the
// send failed and was then coalesced into a newer iteration's fault). Returns an
// error if the subscription controller was unavailable or the send failed.
func (g *gossipClient) NotifyLinkFault(linkId string, iteration uint32) error {
	sub := g.ctrls.GetSubscriptionController()
	if sub == nil || !sub.IsConnected() {
		return errors.New("subscription controller unavailable")
	}

	s := g.linkStore()
	key := linkGossipKey(linkId, iteration)
	entries := []*gossip_pb.GossipEntry{g.newTombstone(key)}
	// Add tombstones for any still-advertised older iterations of this link.
	entries = append(entries, g.buildSupersededTombstones(s, map[string]uint32{linkId: iteration})...)

	if err := g.applyTombstones(sub, s, entries); err != nil {
		return err
	}

	pfxlog.Logger().WithField("linkId", linkId).
		WithField("iteration", iteration).
		WithField("tombstones", len(entries)).
		Info("sent link fault tombstone via gossip")
	return nil
}

// newTombstone builds a tombstone gossip entry for the given key with a freshly
// allocated version.
func (g *gossipClient) newTombstone(key string) *gossip_pb.GossipEntry {
	return &gossip_pb.GossipEntry{
		Key:       key,
		Version:   g.nextVersion(),
		Owner:     g.routerId,
		Tombstone: true,
		Epoch:     g.epoch,
	}
}

// buildSupersededTombstones returns tombstone entries for every live entry in
// currentEntries belonging to one of the given links whose iteration is older
// than the link's threshold iteration. These are iterations that have been
// superseded by a re-dial or a fault for a newer iteration; tombstoning them
// keeps the router's advertised set from diverging from the controller when an
// older iteration's own fault was never delivered.
func (g *gossipClient) buildSupersededTombstones(s *gossipStore, throughIteration map[string]uint32) []*gossip_pb.GossipEntry {
	var result []*gossip_pb.GossipEntry
	s.currentEntries.IterCb(func(key string, e *gossip_pb.GossipEntry) {
		if e.Tombstone {
			return
		}
		linkId, iter := parseLinkGossipKey(key)
		if minIter, ok := throughIteration[linkId]; ok && iter < minIter {
			result = append(result, g.newTombstone(key))
		}
	})
	return result
}

// applyTombstones sends the given tombstones and, only on a successful send,
// removes the tombstoned keys from currentEntries. Mutating local tracking only
// after the send keeps the canary count from drifting on retry: a caller that
// retries after a failed send sees the entries still present and resends them.
func (g *gossipClient) applyTombstones(sub env.NetworkController, s *gossipStore, entries []*gossip_pb.GossipEntry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := g.sendDelta(sub.Channel(), s.source.storeType(), entries); err != nil {
		return err
	}
	for _, e := range entries {
		s.currentEntries.Remove(e.Key)
	}
	s.invalidateHash()
	g.updateMaxSentFromEntries(s.source.storeType(), entries)
	g.sendToStaleControllers(sub.Channel().Id(), s.source.storeType(), entries)
	return nil
}

// reconcileEntries converges currentEntries to the source of truth: the set of
// established dialed links. Any non-tombstone entry with no backing established
// dialed link is stale (e.g. left behind by a live-vs-tombstone race or a
// dropped fault) and is tombstoned, which removes it locally and propagates the
// removal to controllers. This is the router-side heal for stale gossip entries:
// rather than maintaining currentEntries perfectly through every concurrent
// mutation path, we periodically re-derive it from the link registry. Driven by
// the gossipRefresher tick; safe to run repeatedly (a no-op when in sync).
func (g *gossipClient) reconcileEntries() {
	for _, s := range g.stores {
		g.reconcileStore(s)
	}
}

// reconcileStore converges one store's advertised set to its source of truth.
func (g *gossipClient) reconcileStore(s *gossipStore) {
	// Snapshot the currently-advertised (non-tombstone) keys first, THEN remove
	// the keys the source of truth still backs. Ordering matters: an entry
	// published mid-sweep is added after this snapshot, so it is never a
	// tombstone candidate. (Deriving the source set first instead would wrongly
	// flag such an entry as stale and tombstone a live entry.) Whatever remains
	// was advertised but is no longer backed by the source, so it's genuinely
	// stale. Re-dials bump the iteration, so a re-established link has a different
	// key than any stale older-iteration entry; the sweep can't tombstone an
	// item that just came back.
	candidates := map[string]struct{}{}
	s.currentEntries.IterCb(func(key string, e *gossip_pb.GossipEntry) {
		if !e.Tombstone {
			candidates[key] = struct{}{}
		}
	})
	if len(candidates) == 0 {
		return
	}

	s.source.iterateAdvertised(func(key string, _ []byte) {
		delete(candidates, key)
	})
	if len(candidates) == 0 {
		return
	}

	sub := g.ctrls.GetSubscriptionController()
	if sub == nil || !sub.IsConnected() {
		return // can't send now; the next tick retries (entries stay until then)
	}

	stale := make([]*gossip_pb.GossipEntry, 0, len(candidates))
	for key := range candidates {
		stale = append(stale, g.newTombstone(key))
	}

	if err := g.applyTombstones(sub, s, stale); err != nil {
		pfxlog.Logger().WithError(err).Warn("failed to send reconcile tombstones, will retry")
	} else {
		pfxlog.Logger().WithField("storeType", s.source.storeType()).
			WithField("count", len(stale)).
			Info("reconciled stale gossip entries against source of truth")
	}
}

// requestDigestFromAllControllers asks every connected controller to send its
// digest of this router's entries. The router's HandleDigest then tombstones any
// entry a controller holds that the router no longer has — clearing entries a
// controller retained because it missed a tombstone (e.g. while partitioned).
// This is the controller-side analog of reconcileEntries: an unconditional,
// periodic reconcile that heals controller-held stale entries without depending
// on canary-mismatch detection (which is slow and unreliable under churn). The
// existing canary-driven digest still fires on its own; this is the backstop.
func (g *gossipClient) requestDigestFromAllControllers() {
	for _, ctrl := range g.ctrls.GetAll() {
		if ctrl.IsConnected() && !ctrl.IsUnresponsive() {
			g.sendDigestRequest(ctrl.Channel())
		}
	}
}

// sendToStaleControllers sends entries to any controllers marked as stale.
func (g *gossipClient) sendToStaleControllers(subId, storeType string, entries []*gossip_pb.GossipEntry) {
	stale := g.getStaleControllers()
	if len(stale) == 0 {
		return
	}

	all := g.ctrls.GetAll()
	sent := 0
	for ctrlId := range stale {
		if ctrlId == subId {
			continue
		}
		ctrl, ok := all[ctrlId]
		if !ok || !ctrl.IsConnected() {
			continue
		}
		if err := g.sendDelta(ctrl.Channel(), storeType, entries); err == nil {
			sent += len(entries)
		}
	}
	if sent > 0 && g.staleSendEntries != nil {
		g.staleSendEntries.Mark(int64(sent))
	}
}

func (g *gossipClient) sendDelta(ch channel.Channel, storeType string, entries []*gossip_pb.GossipEntry) error {
	delta := &gossip_pb.GossipDelta{
		StoreType: storeType,
		Entries:   entries,
	}

	body, err := proto.Marshal(delta)
	if err != nil {
		return fmt.Errorf("failed to marshal gossip delta: %w", err)
	}

	msg := channel.NewMessage(gossip_pb.GossipDeltaType, body)
	if err := msg.WithTimeout(10 * time.Second).SendAndWaitForWire(ch); err != nil {
		if !ch.IsClosed() {
			return fmt.Errorf("failed to send gossip delta: %w", err)
		}
		// Channel closed — the reconnect/anti-entropy cycle will handle recovery.
		return nil
	}
	return nil
}

// sendDigestRequest sends a GossipDigestRequest to trigger the controller to
// send its digest for this router's entries, initiating a catch-up exchange.
func (g *gossipClient) sendDigestRequest(ch channel.Channel) {
	// Carry our per-store-type hashes so the controller can hash-gate its
	// response: it replies with a full digest only for store types whose view of
	// our entries differs.
	req := &gossip_pb.GossipDigestRequest{EntryHashes: g.GetEntryHashes()}
	body, err := proto.Marshal(req)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to marshal gossip digest request")
		return
	}
	msg := channel.NewMessage(gossip_pb.GossipDigestRequestType, body)
	if err := msg.WithTimeout(10 * time.Second).Send(ch); err != nil {
		if !ch.IsClosed() {
			pfxlog.Logger().WithError(err).Error("failed to send gossip digest request")
		}
	}
}

// HandleDigest processes a gossip digest from the controller for one store type.
// The controller sends what it knows about our entries; we respond with entries
// the controller is missing or has stale, plus tombstones for entries the
// controller has that our source of truth no longer backs.
func (g *gossipClient) HandleDigest(msg *channel.Message, ch channel.Channel) {
	digest := &gossip_pb.GossipDigest{}
	if err := proto.Unmarshal(msg.Body, digest); err != nil {
		pfxlog.Logger().WithError(err).Error("failed to unmarshal gossip digest from controller")
		return
	}

	s := g.stores[digest.StoreType]
	if s == nil {
		return
	}

	// Build map of what the controller has, and advance our clock past
	// the highest version so that any tombstones we create will have a
	// version higher than the existing entries. This is critical after a
	// router restart when the Lamport clock resets to 0.
	controllerVersions := make(map[string]uint64, len(digest.Entries))
	for _, de := range digest.Entries {
		controllerVersions[de.Key] = de.Version
		g.observeVersion(de.Version)
	}

	// Collect our current advertised state from the source of truth.
	localKeys := map[string]bool{}
	var responseEntries []*gossip_pb.GossipEntry

	s.source.iterateAdvertised(func(key string, value []byte) {
		localKeys[key] = true
		ctrlVersion, ctrlHas := controllerVersions[key]

		// Use the stored entry (preserving its version) if present, else create one.
		var entry *gossip_pb.GossipEntry
		if stored, ok := s.currentEntries.Get(key); ok && !stored.Tombstone {
			entry = stored
		} else {
			entry = &gossip_pb.GossipEntry{
				Key:     key,
				Value:   value,
				Version: g.nextVersion(),
				Owner:   g.routerId,
				Epoch:   g.epoch,
			}
			s.currentEntries.Set(key, entry)
		}

		if !ctrlHas || ctrlVersion < entry.Version {
			responseEntries = append(responseEntries, entry)
		}
	})

	// Tombstone entries the controller has that our source of truth doesn't.
	for key := range controllerVersions {
		if !localKeys[key] {
			entry := g.newTombstone(key)
			s.currentEntries.Remove(key)
			responseEntries = append(responseEntries, entry)
		}
	}

	s.invalidateHash()

	if len(responseEntries) == 0 {
		pfxlog.Logger().WithField("digestEntries", len(digest.Entries)).
			Info("gossip digest exchange: nothing to send back")
		return
	}

	pfxlog.Logger().WithField("digestEntries", len(digest.Entries)).
		WithField("responseEntries", len(responseEntries)).
		Info("gossip digest exchange: sending response")

	resp := &gossip_pb.GossipDigestResponse{
		StoreType: digest.StoreType,
		Entries:   responseEntries,
	}
	body, err := proto.Marshal(resp)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to marshal gossip digest response")
		return
	}
	respMsg := channel.NewMessage(gossip_pb.GossipDigestResponseType, body)
	if err := respMsg.Send(ch); err != nil {
		if !ch.IsClosed() {
			pfxlog.Logger().WithError(err).Debug("failed to send gossip digest response to controller")
		}
	} else {
		g.updateMaxSentFromEntries(digest.StoreType, responseEntries)
	}
}

// updateMaxSentFromEntries updates the maxSent tracker with the highest version
// in the given entry batch. Called after a successful send.
func (g *gossipClient) updateMaxSentFromEntries(storeType string, entries []*gossip_pb.GossipEntry) {
	var max uint64
	for _, e := range entries {
		if e.Version > max {
			max = e.Version
		}
	}
	if max > 0 {
		g.maxSent.update(storeType, max)
	}
}

func marshalLink(l xlink.Xlink) ([]byte, error) {
	rl := &ctrl_pb.RouterLinks_RouterLink{
		Id:           l.Id(),
		DestRouterId: l.DestinationId(),
		LinkProtocol: l.LinkProtocol(),
		DialAddress:  l.DialAddress(),
		Iteration:    l.Iteration(),
		ConnState:    l.GetLinkConnState(),
	}
	return proto.Marshal(rl)
}

// gossipRefresher periodically checks for stale controllers and manages the
// stale set on the gossipClient. When a controller is first detected as stale,
// it sends a GossipDigestRequest to trigger catch-up. While stale, the
// gossipClient dual-sends new entries to those controllers.
type gossipRefresher struct {
	client     *gossipClient
	closeC     <-chan struct{}
	previous   map[string]bool      // previous stale set, for detecting transitions
	staleSince map[string]time.Time // when each currently-stale ctrl entered the stale set
	lastSubId  string               // previous subscription controller ID, for detecting changes
}

func newGossipRefresher(client *gossipClient, closeC <-chan struct{}) *gossipRefresher {
	return &gossipRefresher{
		client:     client,
		closeC:     closeC,
		previous:   map[string]bool{},
		staleSince: map[string]time.Time{},
	}
}

func (r *gossipRefresher) run() {
	ticker := time.NewTicker(refreshCheckInterval)
	defer ticker.Stop()

	var lastReconcile time.Time
	for {
		select {
		case <-r.closeC:
			return
		case <-ticker.C:
			r.check()
			// Heal any stale advertised entries by re-deriving from the link
			// registry. Independent of check()'s early returns above.
			r.client.reconcileEntries()

			// Publish per-link latency that changed meaningfully since the last
			// tick (and force-publish any re-dialed link). Runs on the same tick so
			// the publish cadence is naturally bounded to once per refresh interval
			// per link.
			r.client.publishLinkMetrics()

			// Periodically reconcile the other direction too: ask every connected
			// controller for its digest so HandleDigest can tombstone entries a
			// controller still holds that this router no longer has (e.g. a
			// tombstone the controller missed while partitioned). Hash-gated on the
			// controller side and churn-gated here, so a stable mesh does ~no work.
			now := time.Now()
			if now.Sub(lastReconcile) >= gossipReconcileInterval {
				lastReconcile = now
				if r.client.changedWithin(gossipReconcileQuietPeriod) {
					r.client.requestDigestFromAllControllers()
				}
			}
		}
	}
}

func (r *gossipRefresher) check() {
	sub := r.client.ctrls.GetSubscriptionController()
	if sub == nil || !sub.IsConnected() {
		return
	}

	subId := sub.Channel().Id()

	// When the subscription controller changes (e.g., leader election or
	// reconnect to a different controller), trigger a digest exchange so
	// the new primary can clean up any stale entries it received from a
	// peer snapshot.
	if r.lastSubId != "" && r.lastSubId != subId {
		pfxlog.Logger().
			WithField("oldSubId", r.lastSubId).
			WithField("newSubId", subId).
			Info("subscription controller changed, requesting gossip digest exchange")
		r.client.sendDigestRequest(sub.Channel())
	}
	r.lastSubId = subId

	all := r.client.ctrls.GetAll()
	if len(all) < 2 {
		return
	}

	subSeq := sub.GetCanarySeq()
	if subSeq == 0 {
		return
	}
	newStale := map[string]bool{}

	for ctrlId, ctrl := range all {
		if ctrlId == subId {
			continue
		}
		if !ctrl.IsConnected() || ctrl.IsUnresponsive() {
			continue
		}

		ctrlSeq := ctrl.GetCanarySeq()
		if ctrlSeq == 0 {
			continue // newly connected, bind-time digest handles reconciliation
		}

		if subSeq > ctrlSeq && subSeq-ctrlSeq >= staleCanaryThreshold {
			newStale[ctrlId] = true

			if !r.previous[ctrlId] {
				// Newly stale — trigger a digest exchange for catch-up
				pfxlog.Logger().
					WithField("ctrlId", ctrlId).
					WithField("subSeq", subSeq).
					WithField("ctrlSeq", ctrlSeq).
					WithField("delta", subSeq-ctrlSeq).
					Info("controller canary is stale, requesting gossip digest exchange")

				r.staleSince[ctrlId] = time.Now()
				r.client.sendDigestRequest(ctrl.Channel())
			}
		}
	}

	// Log exit transitions so we can measure stale-episode duration from logs.
	// Anything that was in previous but not in newStale recovered this cycle.
	for ctrlId := range r.previous {
		if newStale[ctrlId] {
			continue
		}
		log := pfxlog.Logger().WithField("ctrlId", ctrlId)
		if since, ok := r.staleSince[ctrlId]; ok {
			log = log.WithField("staleDurationMs", time.Since(since).Milliseconds())
			delete(r.staleSince, ctrlId)
		}
		log.Info("controller canary recovered")
	}

	r.client.setStaleControllers(newStale)
	r.previous = newStale
}
