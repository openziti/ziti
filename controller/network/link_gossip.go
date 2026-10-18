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

package network

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/logging"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/gossip"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/openziti/ziti/v2/controller/model"
	"google.golang.org/protobuf/proto"
)

// linkGossipLog is the logger for controller-side link gossip propagation. Its
// channel name is "controller.link.gossip".
var linkGossipLog = logging.For("controller.link.gossip")

const LinkGossipStoreType = "links"

// ForwardLinkFaultTimeout bounds forwarding a link fault to an endpoint router. A healthy channel accepts
// immediately, so only a stalled router reaches it, and giving up on one is better than holding the caller.
// Not as short as the digest send's, since anti-entropy repeats a digest and nothing repeats this: the router
// keeps a link the controller has removed until its own failure detection notices.
const ForwardLinkFaultTimeout = 5 * time.Second

// LinkGossipKey builds the composite gossip key for a link instance.
func LinkGossipKey(linkId string, iteration uint32) string {
	return fmt.Sprintf("%s:%d", linkId, iteration)
}

// ParseLinkGossipKey extracts the linkId and iteration from a composite gossip key.
func ParseLinkGossipKey(key string) (string, uint32) {
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

// initLinkGossip registers the link gossip state type. Called once from initGossip, which owns the store
// this registers against.
func (network *Network) initLinkGossip() {
	listener := &linkGossipListener{network: network}

	linkType := gossip.Register[*ctrl_pb.RouterLinks_RouterLink](network.GossipStore, gossip.StateTypeConfig[*ctrl_pb.RouterLinks_RouterLink]{
		Name: LinkGossipStoreType,
		Encode: func(v *ctrl_pb.RouterLinks_RouterLink) ([]byte, error) {
			return proto.Marshal(v)
		},
		Decode: func(b []byte) (*ctrl_pb.RouterLinks_RouterLink, error) {
			v := &ctrl_pb.RouterLinks_RouterLink{}
			return v, proto.Unmarshal(b, v)
		},
		RequiredCapability:  capabilities.ControllerLinkGossip,
		Tombstones:          true,
		TombstoneTTL:        5 * time.Minute,
		AntiEntropy:         true,
		AntiEntropyInterval: 30 * time.Second,
		Listener:            listener,
	})

	network.LinkGossipType = linkType
	network.registerGossipType(LinkGossipStoreType, linkType)
	network.onRouterDeleted(LinkGossipStoreType, linkType.DropOwner)
}

// gossipLinkRef names an entry without holding what it said, for a walk that applies entries once it has
// ended. Applying one reaches the link manager and can queue a notice on the router messaging loop, and the
// store holds an owner's lock across the whole of a walk.
type gossipLinkRef struct {
	key   string
	owner string
}

// collectGossipLinkRefsForOwner names every live link entry the given router published.
func (network *Network) collectGossipLinkRefsForOwner(owner string) []gossipLinkRef {
	var refs []gossipLinkRef
	network.LinkGossipType.IterByOwner(owner, func(key string, _ *ctrl_pb.RouterLinks_RouterLink) {
		refs = append(refs, gossipLinkRef{key: key, owner: owner})
	})
	return refs
}

// applyGossipLinkEntries re-reads each named entry and applies it, reporting how many links it built and
// how many entries it still could not judge.
//
// Each entry is read and applied under the store's delivery lock for its key, so the pair is ordered
// against the store's own listener callbacks. Reading first and applying after would let a tombstone land
// in between: its callback removes the link, and this then rebuilds it from a value the store has retired.
func (network *Network) applyGossipLinkEntries(refs []gossipLinkRef, trigger linkEntryTrigger) (applied int, awaitingCatchUp int) {
	for _, ref := range refs {
		network.LinkGossipType.WithEntry(ref.owner, ref.key, func(value *ctrl_pb.RouterLinks_RouterLink, found bool) {
			if !found {
				return
			}
			switch network.applyGossipLinkEntry(ref.key, ref.owner, value, trigger) {
			case model.LinkReportCreated:
				applied++
			case model.LinkReportDestinationUnknown:
				awaitingCatchUp++
			}
		})
	}
	return applied, awaitingCatchUp
}

// gossipLinkSource returns the router that published a link gossip entry, or nil if it is not known here at
// all. A router that is not connected to this controller is loaded from the database, so the link is still
// recorded: this controller cannot route over it either way, and it is the dialer's to retire.
func (network *Network) gossipLinkSource(owner string) *model.Router {
	if src := network.Router.GetConnected(owner); src != nil {
		return src
	}
	src, err := network.Router.Read(owner)
	if err != nil {
		return nil
	}
	return src
}

// linkEntryTrigger says why an entry is being applied: it has just arrived, or a walk of the store is
// asking about it again. A re-walk carries no news about a link that is already current, so it does not
// raise the event an arrival does. A link it builds is news either way.
type linkEntryTrigger int

const (
	// linkEntryArrived is the store's delivery of an entry, which is news.
	linkEntryArrived linkEntryTrigger = iota
	// linkEntryReconciled is a walk for a router that has just connected, picking up entries whose delivery
	// was skipped because the owner was unknown then. A first sight like an arrival, but not news.
	linkEntryReconciled
	// linkEntrySwept is the sweep reading back an entry whose destination could not be judged before. Not
	// news either.
	linkEntrySwept
)

// applyGossipLinkEntry applies one link entry to the link manager and reports what became of it, so that an
// entry is acted on the same way whether it has just arrived or is being re-walked out of the store.
//
// LinkReportDestinationUnknown marks the entry for the sweep and is the only outcome that is not final for
// it. An entry whose owning router is not known here at all is reported as refused, since nothing can be
// built from it and nothing further will change that.
//
// Must not be called while iterating the gossip store.
func (network *Network) applyGossipLinkEntry(key, owner string, value *ctrl_pb.RouterLinks_RouterLink, trigger linkEntryTrigger) model.LinkReportOutcome {
	linkId, _ := ParseLinkGossipKey(key)

	log := linkGossipLog.With(
		"linkId", linkId,
		"gossipKey", key,
		"srcRouterId", owner,
		"destRouterId", value.DestRouterId,
		"iteration", value.Iteration)

	src := network.gossipLinkSource(owner)
	if src == nil {
		log.Debug("ignoring gossip link for unknown router")
		return model.LinkReportRefused
	}

	dst := network.Router.GetConnected(value.DestRouterId)

	link, outcome := network.Link.RouterReportedLink(value, src, dst)

	switch outcome {
	case model.LinkReportDestinationAbsent:
		// Proved gone. An entry naming a destination that no longer exists is the evidence that its dialer
		// never got the removal, so answer it with one. The dialer then drops the link and retires the entry.
		// The entry is no longer held, so if the notice cannot go now it is kept and retried on its own.
		network.notifyProvedDestinationRemoved(src, value.DestRouterId)

	case model.LinkReportRefused:
		// Refused for a reason other than the destination. The notice is still offered, but only on its own
		// evidence, since nothing here proved the destination gone.
		network.notifyDestinationRemovedIfGone(src, value.DestRouterId, dst)

	case model.LinkReportDestinationUnknown:
		// No copy of the report is held, only its name. The entry stays in the store, and the sweep reads it
		// from there once this controller can judge the destination.
		network.markGossipLinkSweepPending(gossipLinkRef{key: key, owner: owner})

	case model.LinkReportCreated:
		// Reconcile latency from the link-metrics store for both ends. A metrics
		// entry can arrive before the link-state entry that creates this link
		// (the two stores replicate independently), so on create or iteration
		// replace we pull the current latency from the store rather than wait for
		// the next metrics publish. RouterReportedLink reports created for both a
		// brand-new link and an iteration replacement, so this covers re-dials.
		network.reconcileLinkMetricsForLink(link)

		network.NotifyLinkEvent(link, event.LinkFromRouterNew)
		log.Debug("gossip: link added")

	case model.LinkReportCurrent:
		// Apply updated ConnState only when the gossip entry's iteration matches
		// the link's iteration. An old-iteration entry arriving after the link has
		// been replaced carries a higher StateIteration counter from the old link
		// lifetime, which would permanently block updates from the new iteration.
		if value.ConnState != nil && value.Iteration == link.Iteration {
			link.SetConnsState(value.ConnState)
		}
		if trigger == linkEntryArrived {
			// A walk re-asking about a link that was already current has nothing to report, and raising the
			// event anyway puts the whole store on the event stream once a cycle for as long as a sweep is
			// owed, which is while this controller is already behind.
			network.NotifyLinkEvent(link, event.LinkFromRouterKnown)
		}
		log.Debug("gossip: link updated")
	}

	return outcome
}

// markGossipLinkSweepPending records an entry whose destination this controller could not judge, for the
// next sweep to ask about again.
func (network *Network) markGossipLinkSweepPending(ref gossipLinkRef) {
	network.gossipLinkSweepLock.Lock()
	defer network.gossipLinkSweepLock.Unlock()
	if network.gossipLinkSweepPending == nil {
		network.gossipLinkSweepPending = map[gossipLinkRef]struct{}{}
	}
	network.gossipLinkSweepPending[ref] = struct{}{}
}

// takeGossipLinkSweepPending empties the pending set and returns what it held.
func (network *Network) takeGossipLinkSweepPending() []gossipLinkRef {
	network.gossipLinkSweepLock.Lock()
	defer network.gossipLinkSweepLock.Unlock()
	refs := make([]gossipLinkRef, 0, len(network.gossipLinkSweepPending))
	for ref := range network.gossipLinkSweepPending {
		refs = append(refs, ref)
	}
	network.gossipLinkSweepPending = nil
	return refs
}

// GossipLinkSweepPendingCount is how many link gossip entries are waiting on this controller to catch up
// before their destination can be judged.
func (network *Network) GossipLinkSweepPendingCount() int {
	network.gossipLinkSweepLock.Lock()
	defer network.gossipLinkSweepLock.Unlock()
	return len(network.gossipLinkSweepPending)
}

// ReconcileGossipLinksForRouter ensures that every live gossip entry owned by the given router has a
// corresponding link in the link manager. This covers entries that were applied while the router was
// unknown (e.g., due to raft replication delay) and whose listener notification was skipped.
func (network *Network) ReconcileGossipLinksForRouter(router *model.Router) {
	reconciled, _ := network.applyGossipLinkEntries(network.collectGossipLinkRefsForOwner(router.Id), linkEntryReconciled)
	if reconciled > 0 {
		linkGossipLog.Info("reconciled missing links from gossip store on router connect",
			"routerId", router.Id,
			"count", reconciled)
	}
}

// sweepGossipLinks asks the store again about the entries whose destination could not be judged when they
// arrived, because this controller was behind on raft. Called on the network cycle.
//
// The pending set is taken before anything is applied, so an entry that cannot be judged during the pass
// puts itself back rather than having its mark cleared out from under it. In steady state the set is empty
// and the pass does nothing.
//
// Deliberately without an attempt limit. The store holds the entry whichever way this controller decides,
// so nothing is lost by asking again. A destination that is genuinely gone settles once a read barrier
// covers it: the entry is refused, its dialer is told the destination was removed, and the dialer retires the
// entry out of the store.
func (network *Network) sweepGossipLinks() {
	refs := network.takeGossipLinkSweepPending()
	if len(refs) == 0 {
		return
	}

	applied, awaitingCatchUp := network.applyGossipLinkEntries(refs, linkEntrySwept)

	linkGossipLog.Info("asked the link gossip store again about entries awaiting catch-up",
		"entries", len(refs),
		"applied", applied,
		"awaitingCatchUp", awaitingCatchUp)
}

// IsLinkInGossip reports whether link is currently published in the gossip store by its dialer. Only the
// dialer publishes a link, so its ownership is where a live entry would be. A link reported by a router
// that does not gossip has no entry, and is removed by the pre-gossip path instead.
func (network *Network) IsLinkInGossip(link *model.Link) bool {
	src := link.GetSrc()
	if src == nil {
		return false
	}
	_, _, found := network.LinkGossipType.GetForOwner(src.Id, LinkGossipKey(link.Id, link.Iteration))
	return found
}

// LinkFaultedViaGossip tombstones a link in the gossip store. This is for genuine
// link failures reported by routers, not controller-router disconnect.
func (network *Network) LinkFaultedViaGossip(link *model.Link) {
	key := LinkGossipKey(link.Id, link.Iteration)
	network.LinkGossipType.Delete(key, link.GetSrc().Id)
}

// linkGossipListener implements gossip.StateListener for link state.
type linkGossipListener struct {
	network *Network
}

func (l *linkGossipListener) EntryChanged(key string, value *ctrl_pb.RouterLinks_RouterLink, _ uint64, owner string, _ bool, _ gossip.ChangeOrigin) {
	l.network.applyGossipLinkEntry(key, owner, value, linkEntryArrived)
}

// GossipLinkInspectResult is the JSON structure returned by the "gossip-links" inspect key.
type GossipLinkInspectResult struct {
	Entries []GossipLinkEntry `json:"entries"`
	Count   int               `json:"count"`
	// SweepPending is how many entries name a destination this controller cannot judge until it catches up.
	SweepPending int `json:"sweepPending"`
}

// GossipLinkEntry describes a single link entry in the gossip store.
type GossipLinkEntry struct {
	Key        string `json:"key"`
	LinkId     string `json:"linkId"`
	Iteration  uint32 `json:"iteration"`
	Owner      string `json:"owner"`
	Version    uint64 `json:"version"`
	DestRouter string `json:"destRouter"`
	Epoch      string `json:"epoch,omitempty"`
}

// GossipStoreInspectResult is the JSON structure returned by the "gossip-store" inspect key.
type GossipStoreInspectResult struct {
	TypeStats    []gossip.StoreStats `json:"typeStats"`
	LamportClock uint64              `json:"lamportClock"`
}

func (network *Network) inspectGossipLinks() *GossipLinkInspectResult {
	result := &GossipLinkInspectResult{}
	network.LinkGossipType.IterFull(func(key string, value *ctrl_pb.RouterLinks_RouterLink, owner string, version uint64, epoch []byte) {
		linkId, iteration := ParseLinkGossipKey(key)
		result.Entries = append(result.Entries, GossipLinkEntry{
			Key:        key,
			LinkId:     linkId,
			Iteration:  iteration,
			Owner:      owner,
			Version:    version,
			DestRouter: value.DestRouterId,
			Epoch:      idgen.FormatEpoch(epoch),
		})
	})
	result.Count = len(result.Entries)
	result.SweepPending = network.GossipLinkSweepPendingCount()
	return result
}

func (network *Network) inspectGossipStore() *GossipStoreInspectResult {
	return &GossipStoreInspectResult{
		TypeStats:    network.GossipStore.GetStats(),
		LamportClock: network.GossipStore.ClockValue(),
	}
}

// EntriesDiscarded is a no-op. The links built from these entries are live and are reported again by
// whatever means replaces gossip, so removing them here would tear down working links.
func (l *linkGossipListener) EntriesDiscarded() {}

func (l *linkGossipListener) EntryRemoved(key string, owner string, version uint64, _ gossip.ChangeOrigin) {
	linkId, iteration := ParseLinkGossipKey(key)

	log := linkGossipLog.With(
		"linkId", linkId,
		"gossipKey", key,
		"srcRouterId", owner,
		"version", version)

	link, found := l.network.Link.Get(linkId)
	if !found {
		return
	}

	// Only an endpoint may retire a link. Entry ownership is validated at ingress, so the owner really is the
	// router that published this, but nothing stops a router publishing a tombstone keyed to a link between
	// two others. The tombstone reaches every controller, so without this check one router could evict
	// arbitrary links network-wide and have the eviction propagated as authoritative.
	if !link.HasEndpoint(owner) {
		srcId := ""
		if src := link.GetSrc(); src != nil {
			srcId = src.Id
		}
		log.Warn("ignoring link tombstone from a router that is not an endpoint of the link",
			"linkSrcRouterId", srcId,
			"linkDestRouterId", link.DstId)
		return
	}

	// A tombstone for an older iteration must not remove a link that has
	// already been replaced by a newer iteration. But a tombstone for a newer
	// iteration means the link has moved on and died; any older iteration
	// still in the link manager is certainly stale.
	if link.Iteration > iteration {
		log.Debug("gossip: ignoring tombstone for older iteration",
			"linkIteration", link.Iteration,
			"tombstoneIteration", iteration)
		return
	}

	wasUsable := link.IsUsable()

	// Forward the fault to any endpoint router connected to this controller so it
	// closes and (for the dialer) re-dials. The tombstone reaches every
	// controller via gossip, so wherever the dialer is connected, that controller
	// notifies it. This replaces the old direct forward in the fault handler,
	// which only worked when the peer happened to be connected to the same
	// controller that received the fault.
	l.forwardLinkFaultToConnectedEndpoints(linkId, iteration, link)

	link.SetState(model.Failed)
	l.network.NotifyLinkEvent(link, event.LinkFault)

	log.Info("gossip: removing faulted link",
		"linkIteration", link.Iteration,
		"tombstoneIteration", iteration)
	l.network.Link.Remove(link)

	if wasUsable {
		l.network.RerouteLink(link)
	}
}

// forwardLinkFaultToConnectedEndpoints sends a LinkFault to each of the link's
// endpoint routers that is connected to this controller, telling it to close the
// faulted link. The router that originally reported the fault has already closed
// its link and will simply ignore the message.
func (l *linkGossipListener) forwardLinkFaultToConnectedEndpoints(linkId string, iteration uint32, link *model.Link) {
	endpoints := make([]string, 0, 2)
	if link.GetSrc() != nil {
		endpoints = append(endpoints, link.GetSrc().Id)
	}
	if link.DstId != "" {
		endpoints = append(endpoints, link.DstId)
	}

	for _, routerId := range endpoints {
		router := l.network.Router.GetConnected(routerId)
		if router == nil || !router.Connected.Load() {
			continue
		}
		ctrl := router.Control
		if ctrl == nil {
			continue
		}
		fault := &ctrl_pb.Fault{
			Subject:   ctrl_pb.FaultSubject_LinkFault,
			Id:        linkId,
			Iteration: iteration,
		}

		log := linkGossipLog.With("linkId", linkId, "iteration", iteration, "routerId", routerId)

		// Sent from the I/O pool rather than here. This runs from the gossip listener, on the pool that
		// applies incoming deltas, and a send waits on the router's channel: a burst of tombstones for
		// stalled routers would occupy every apply worker and stop deltas and canaries being processed at
		// all. Bounded for the same reason the pool is: a stalled router must not hold a worker either.
		sender := ctrl.GetDefaultSender()
		queued := l.network.GetIoPool().QueueOrError(func() {
			if err := protobufs.MarshalTyped(fault).WithTimeout(ForwardLinkFaultTimeout).Send(sender); err != nil {
				log.Error("failed to forward link fault to connected endpoint", "error", err)
			}
		})

		if queued != nil {
			l.network.PoolDrops.LinkFaultForward.Drop()
		}
	}
}
