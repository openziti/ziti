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

// InitLinkGossip registers the link gossip state type. Must be called after
// InitGossipStore so that GossipStore is non-nil.
func (network *Network) InitLinkGossip() *gossip.StateType[*ctrl_pb.RouterLinks_RouterLink] {
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
		Tombstones:          true,
		TombstoneTTL:        5 * time.Minute,
		AntiEntropy:         true,
		AntiEntropyInterval: 30 * time.Second,
		Listener:            listener,
	})

	network.LinkGossipType = linkType
	network.RegisterGossipType(LinkGossipStoreType, linkType)
	return linkType
}

// NotifyLinkViaGossip is called when an old router reports a link. Only the
// designated writer (raft leader or single controller) translates old-router
// reports into gossip entries.
func (network *Network) NotifyLinkViaGossip(srcRouter *model.Router, reportedLink *ctrl_pb.RouterLinks_RouterLink) {
	key := LinkGossipKey(reportedLink.Id, reportedLink.Iteration)
	if err := network.LinkGossipType.Set(key, srcRouter.Id, reportedLink); err != nil {
		linkGossipLog.Error("failed to set link in gossip store", "error", err, "linkId", reportedLink.Id)
	}
}

// ReconcileLinksViaGossip handles a full refresh: removes links for the given
// router that are not in the reported set.
func (network *Network) ReconcileLinksViaGossip(srcRouterId string, reportedKeys map[string]struct{}) {
	network.LinkGossipType.Reconcile(srcRouterId, reportedKeys)
}

// ReconcileGossipLinksForRouter ensures that every non-tombstoned gossip entry
// owned by the given router has a corresponding link in the link manager. This
// covers entries that were applied while the router was unknown (e.g., due to raft
// replication delay) and whose listener notification was skipped.
func (network *Network) ReconcileGossipLinksForRouter(router *model.Router) {
	reconciled := 0
	network.LinkGossipType.IterByOwner(router.Id, func(key string, value *ctrl_pb.RouterLinks_RouterLink) {
		dst := network.Router.GetConnected(value.DestRouterId)
		link, created := network.Link.RouterReportedLink(value, router, dst)
		if link != nil && created {
			if !router.Connected.Load() || dst == nil || !dst.Connected.Load() {
				link.SetDown(true)
			}
			reconciled++
		}
	})
	if reconciled > 0 {
		linkGossipLog.Info("reconciled missing links from gossip store on router connect",
			"routerId", router.Id,
			"count", reconciled)
	}
}

// LinkFaultedViaGossip tombstones a link in the gossip store. This is for genuine
// link failures reported by routers, not controller-router disconnect.
func (network *Network) LinkFaultedViaGossip(link *model.Link) {
	key := LinkGossipKey(link.Id, link.Iteration)
	network.LinkGossipType.Delete(key, link.Src.Id)
}

// linkGossipListener implements gossip.StateListener for link state.
type linkGossipListener struct {
	network *Network
}

func (l *linkGossipListener) EntryChanged(key string, value *ctrl_pb.RouterLinks_RouterLink, _ uint64, owner string, created bool, _ gossip.ChangeOrigin) {
	linkId, _ := ParseLinkGossipKey(key)

	log := linkGossipLog.With(
		"linkId", linkId,
		"gossipKey", key,
		"srcRouterId", owner,
		"destRouterId", value.DestRouterId,
		"iteration", value.Iteration)

	src := l.network.Router.GetConnected(owner)
	if src == nil {
		// Source router not connected to this controller. Try to load from
		// the database so we can still store the link (marked as down).
		var err error
		src, err = l.network.Router.Read(owner)
		if err != nil || src == nil {
			log.Debug("ignoring gossip link for unknown router")
			return
		}
	}

	dst := l.network.Router.GetConnected(value.DestRouterId)

	link, linkCreated := l.network.Link.RouterReportedLink(value, src, dst)
	if link == nil {
		return
	}

	// If the source router is not connected to this controller, mark the link
	// as locally unusable.
	if !src.Connected.Load() {
		link.SetDown(true)
	}

	if linkCreated {
		// Reconcile latency from the link-metrics store for both ends. A metrics
		// entry can arrive before the link-state entry that creates this link
		// (the two stores replicate independently), so on create or iteration
		// replace we pull the current latency from the store rather than wait for
		// the next metrics publish. RouterReportedLink reports created for both a
		// brand-new link and an iteration replacement, so this covers re-dials.
		l.network.reconcileLinkMetricsForLink(link)

		l.network.NotifyLinkEvent(link, event.LinkFromRouterNew)
		log.Info("gossip: link added")
	} else {
		// Apply updated ConnState only when the gossip entry's iteration matches
		// the link's iteration. An old-iteration entry arriving after the link has
		// been replaced carries a higher StateIteration counter from the old link
		// lifetime, which would permanently block updates from the new iteration.
		if value.ConnState != nil && value.Iteration == link.Iteration {
			link.SetConnsState(value.ConnState)
		}
		l.network.NotifyLinkEvent(link, event.LinkFromRouterKnown)
		log.Debug("gossip: link updated")
	}
}

// GossipLinkInspectResult is the JSON structure returned by the "gossip-links" inspect key.
type GossipLinkInspectResult struct {
	Entries []GossipLinkEntry `json:"entries"`
	Count   int               `json:"count"`
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
	return result
}

func (network *Network) inspectGossipStore() *GossipStoreInspectResult {
	return &GossipStoreInspectResult{
		TypeStats:    network.GossipStore.GetStats(),
		LamportClock: network.GossipStore.ClockValue(),
	}
}

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
	if link.Src != nil {
		endpoints = append(endpoints, link.Src.Id)
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
		if err := protobufs.MarshalTyped(fault).Send(ctrl.GetDefaultSender()); err != nil {
			linkGossipLog.Error("failed to forward link fault to connected endpoint",
				"error", err,
				"linkId", linkId,
				"iteration", iteration,
				"routerId", routerId)
		}
	}
}
