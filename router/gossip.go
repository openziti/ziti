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
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
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
	staleCanaryThreshold    = uint64(6)    // 6 ticks × 5s = 30 seconds
	refreshCheckInterval    = 15 * time.Second
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

// gossipClient is a lightweight gossip participant for the router. It maintains
// a Lamport clock and generates GossipDelta messages for link state changes,
// sending them to the subscription controller and any stale controllers.
type gossipClient struct {
	routerId       string
	epoch          []byte // UUIDv7 generated on startup, identifies this router lifetime
	ctrls          env.NetworkControllers
	clock          atomic.Uint64
	maxSent        maxSentVersions
	linkIterator   func() <-chan xlink.Xlink // set after link registry is created
	currentEntries cmap.ConcurrentMap[string, *gossip_pb.GossipEntry]
	entryCount     atomic.Int64 // non-tombstone entry count, tracked incrementally
	staleCtrlIds   atomic.Value // holds map[string]bool
	entryHash      atomic.Pointer[uint64] // cached hash; nil = dirty
}

func newGossipClient(routerId string, ctrls env.NetworkControllers) *gossipClient {
	epoch := idgen.NewEpochBytes()
	pfxlog.Logger().WithField("routerId", routerId).
		WithField("epoch", idgen.FormatEpoch(epoch)).
		Info("gossip client starting with new epoch")

	g := &gossipClient{
		routerId:       routerId,
		epoch:          epoch,
		ctrls:          ctrls,
		currentEntries: cmap.New[*gossip_pb.GossipEntry](),
	}
	g.staleCtrlIds.Store(map[string]bool{})
	return g
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

// GetEntryHashes returns a FNV-64a hash of sorted non-tombstone gossip keys
// per store type. Used by the canary emitter so controllers can detect state
// divergence.
func (g *gossipClient) GetEntryHashes() map[string]uint64 {
	if p := g.entryHash.Load(); p != nil {
		return map[string]uint64{linkGossipStoreType: *p}
	}

	type keyVersion struct {
		key     string
		version uint64
	}
	var kvs []keyVersion
	g.currentEntries.IterCb(func(key string, e *gossip_pb.GossipEntry) {
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
	result := h.Sum64()
	g.entryHash.Store(&result)
	return map[string]uint64{linkGossipStoreType: result}
}

// GetEntryCounts returns the number of non-tombstone entries per store type.
func (g *gossipClient) GetEntryCounts() map[string]int64 {
	return map[string]int64{linkGossipStoreType: g.entryCount.Load()}
}

func (g *gossipClient) invalidateEntryHash() {
	g.entryHash.Store(nil)
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

	var entries []*gossip_pb.GossipEntry
	for _, vl := range links {
		value, err := g.marshalLink(vl.Link)
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

	if err := g.sendDelta(sub.Channel(), entries); err != nil {
		return err
	}

	for _, entry := range entries {
		if !g.currentEntries.Has(entry.Key) {
			g.entryCount.Add(1)
		}
		g.currentEntries.Set(entry.Key, entry)
	}
	g.invalidateEntryHash()
	for _, e := range entries {
		pfxlog.Logger().WithField("gossipKey", e.Key).
			WithField("version", e.Version).
			Info("sent link gossip entry")
	}
	g.updateMaxSentFromEntries(linkGossipStoreType, entries)
	g.sendToStaleControllers(sub.Channel().Id(), entries)
	return nil
}

// NotifyLinkFault sends a gossip tombstone for a faulted link. Returns an
// error if the subscription controller was unavailable or the send failed.
func (g *gossipClient) NotifyLinkFault(linkId string, iteration uint32) error {
	sub := g.ctrls.GetSubscriptionController()
	if sub == nil || !sub.IsConnected() {
		return errors.New("subscription controller unavailable")
	}

	key := linkGossipKey(linkId, iteration)
	entry := &gossip_pb.GossipEntry{
		Key:       key,
		Version:   g.nextVersion(),
		Owner:     g.routerId,
		Tombstone: true,
		Epoch:     g.epoch,
	}
	if g.currentEntries.Has(key) {
		g.currentEntries.Remove(key)
		g.entryCount.Add(-1)
	}
	g.invalidateEntryHash()

	entries := []*gossip_pb.GossipEntry{entry}
	if err := g.sendDelta(sub.Channel(), entries); err != nil {
		return err
	}
	pfxlog.Logger().WithField("linkId", linkId).
		WithField("iteration", iteration).
		WithField("version", entry.Version).
		Info("sent link fault tombstone via gossip")
	g.maxSent.update(linkGossipStoreType, entry.Version)
	g.sendToStaleControllers(sub.Channel().Id(), entries)
	return nil
}

// sendToStaleControllers sends entries to any controllers marked as stale.
func (g *gossipClient) sendToStaleControllers(subId string, entries []*gossip_pb.GossipEntry) {
	stale := g.getStaleControllers()
	if len(stale) == 0 {
		return
	}

	all := g.ctrls.GetAll()
	for ctrlId := range stale {
		if ctrlId == subId {
			continue
		}
		ctrl, ok := all[ctrlId]
		if !ok || !ctrl.IsConnected() {
			continue
		}
		_ = g.sendDelta(ctrl.Channel(), entries)
	}
}

func (g *gossipClient) sendDelta(ch channel.Channel, entries []*gossip_pb.GossipEntry) error {
	delta := &gossip_pb.GossipDelta{
		StoreType: linkGossipStoreType,
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
	msg := channel.NewMessage(gossip_pb.GossipDigestRequestType, nil)
	if err := msg.WithTimeout(10 * time.Second).Send(ch); err != nil {
		if !ch.IsClosed() {
			pfxlog.Logger().WithError(err).Error("failed to send gossip digest request")
		}
	}
}

// HandleDigest processes a gossip digest from the controller. The controller
// sends us what it knows about our links. We respond with entries the controller
// is missing or has stale, plus tombstones for entries the controller has that
// we don't.
func (g *gossipClient) HandleDigest(msg *channel.Message, ch channel.Channel) {
	digest := &gossip_pb.GossipDigest{}
	if err := proto.Unmarshal(msg.Body, digest); err != nil {
		pfxlog.Logger().WithError(err).Error("failed to unmarshal gossip digest from controller")
		return
	}

	if g.linkIterator == nil {
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

	// Collect our current link state
	localKeys := map[string]bool{}
	var responseEntries []*gossip_pb.GossipEntry

	for l := range g.linkIterator() {
		if !l.IsDialed() {
			continue
		}
		key := linkGossipKey(l.Id(), l.Iteration())
		localKeys[key] = true

		ctrlVersion, ctrlHas := controllerVersions[key]

		// Use the stored entry version if available, otherwise create a new one
		var entry *gossip_pb.GossipEntry
		if stored, ok := g.currentEntries.Get(key); ok && !stored.Tombstone {
			entry = stored
		} else {
			value, err := g.marshalLink(l)
			if err != nil {
				continue
			}
			entry = &gossip_pb.GossipEntry{
				Key:     key,
				Value:   value,
				Version: g.nextVersion(),
				Owner:   g.routerId,
				Epoch:   g.epoch,
			}
			if !g.currentEntries.Has(key) {
				g.entryCount.Add(1)
			}
			g.currentEntries.Set(key, entry)
		}

		if !ctrlHas || ctrlVersion < entry.Version {
			responseEntries = append(responseEntries, entry)
		}
	}

	// Tombstone entries the controller has that we don't
	for key := range controllerVersions {
		if !localKeys[key] {
			entry := &gossip_pb.GossipEntry{
				Key:       key,
				Version:   g.nextVersion(),
				Owner:     g.routerId,
				Tombstone: true,
				Epoch:     g.epoch,
			}
			if g.currentEntries.Has(key) {
				g.currentEntries.Remove(key)
				g.entryCount.Add(-1)
			}
			responseEntries = append(responseEntries, entry)
		}
	}

	g.invalidateEntryHash()

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

func (g *gossipClient) marshalLink(l xlink.Xlink) ([]byte, error) {
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
	client    *gossipClient
	closeC    <-chan struct{}
	previous  map[string]bool // previous stale set, for detecting transitions
	lastSubId string         // previous subscription controller ID, for detecting changes
}

func newGossipRefresher(client *gossipClient, closeC <-chan struct{}) *gossipRefresher {
	return &gossipRefresher{
		client:   client,
		closeC:   closeC,
		previous: map[string]bool{},
	}
}

func (r *gossipRefresher) run() {
	ticker := time.NewTicker(refreshCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.closeC:
			return
		case <-ticker.C:
			r.check()
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

				r.client.sendDigestRequest(ctrl.Channel())
			}
		}
	}

	r.client.setStaleControllers(newStale)
	r.previous = newStale
}
