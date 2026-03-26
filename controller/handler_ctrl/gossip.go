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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"google.golang.org/protobuf/proto"
)

// ctrlGossipDeltaHandler receives gossip deltas from routers (new-style gossip
// participants) and injects them into the gossip store, broadcasting to peer
// controllers. This is the router-originated gossip path.
type ctrlGossipDeltaHandler struct {
	r       *model.Router
	network *network.Network
}

func newCtrlGossipDeltaHandler(r *model.Router, network *network.Network) *ctrlGossipDeltaHandler {
	return &ctrlGossipDeltaHandler{r: r, network: network}
}

func (h *ctrlGossipDeltaHandler) ContentType() int32 {
	return gossip_pb.GossipDeltaType
}

func (h *ctrlGossipDeltaHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	log := pfxlog.Logger().WithField("routerId", h.r.Id)

	delta := &gossip_pb.GossipDelta{}
	if err := proto.Unmarshal(msg.Body, delta); err != nil {
		log.WithError(err).Error("failed to unmarshal gossip delta from router")
		return
	}

	// Security: validate that all entries are owned by the connected router.
	for _, entry := range delta.Entries {
		if entry.Owner != h.r.Id {
			log.WithField("entryOwner", entry.Owner).
				WithField("entryKey", entry.Key).
				Warn("rejecting gossip entry with mismatched owner")
			return
		}
	}

	if err := h.network.GetRouterEventsPool().QueueOrError(func() {
		h.network.GossipStore.ApplyAndBroadcast(delta)
	}); err != nil {
		log.Warn("router events pool full, dropping gossip delta")
	}
}

// ctrlGossipDigestResponseHandler receives digest responses from routers during
// reconnect anti-entropy. The router sends its current state + tombstones for
// entries it no longer has.
type ctrlGossipDigestResponseHandler struct {
	r       *model.Router
	network *network.Network
}

func newCtrlGossipDigestResponseHandler(r *model.Router, network *network.Network) *ctrlGossipDigestResponseHandler {
	return &ctrlGossipDigestResponseHandler{r: r, network: network}
}

func (h *ctrlGossipDigestResponseHandler) ContentType() int32 {
	return gossip_pb.GossipDigestResponseType
}

func (h *ctrlGossipDigestResponseHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	log := pfxlog.Logger().WithField("routerId", h.r.Id)

	resp := &gossip_pb.GossipDigestResponse{}
	if err := proto.Unmarshal(msg.Body, resp); err != nil {
		log.WithError(err).Error("failed to unmarshal gossip digest response from router")
		return
	}

	// Security: validate ownership
	for _, entry := range resp.Entries {
		if entry.Owner != h.r.Id {
			log.WithField("entryOwner", entry.Owner).
				Warn("rejecting digest response entry with mismatched owner")
			return
		}
	}

	if err := h.network.GetRouterEventsPool().QueueOrError(func() {
		h.network.GossipStore.ApplyAndBroadcast(&gossip_pb.GossipDelta{
			StoreType: resp.StoreType,
			Entries:   resp.Entries,
		})
	}); err != nil {
		log.Warn("router events pool full, dropping gossip digest response")
	}
}

// ctrlGossipDigestRequestHandler receives digest requests from routers that
// have detected this controller may be behind on gossip. It triggers the same
// digest exchange used on reconnect.
type ctrlGossipDigestRequestHandler struct {
	r       *model.Router
	network *network.Network
}

func newCtrlGossipDigestRequestHandler(r *model.Router, network *network.Network) *ctrlGossipDigestRequestHandler {
	return &ctrlGossipDigestRequestHandler{r: r, network: network}
}

func (h *ctrlGossipDigestRequestHandler) ContentType() int32 {
	return gossip_pb.GossipDigestRequestType
}

func (h *ctrlGossipDigestRequestHandler) HandleReceive(_ *channel.Message, ch channel.Channel) {
	if err := h.network.GetRouterEventsPool().QueueOrError(func() {
		sendRouterGossipDigest(ch, h.r, h.network, network.LinkGossipStoreType)
	}); err != nil {
		pfxlog.Logger().WithField("routerId", h.r.Id).Warn("router events pool full, dropping gossip digest request")
	}
}

// sendRouterGossipDigest sends a gossip digest for entries owned by the given
// router in the specified store type. Called when a router connects so it can
// reconcile its state with the controller's gossip store.
func sendRouterGossipDigest(ch channel.Channel, router *model.Router, n *network.Network, storeType string) {
	digest := n.GossipStore.GetDigestForOwner(storeType, router.Id)

	pfxlog.Logger().WithField("routerId", router.Id).
		WithField("storeType", storeType).
		WithField("digestEntries", len(digest)).
		Info("sending gossip digest to router")

	pbDigest := &gossip_pb.GossipDigest{
		StoreType: storeType,
		Entries:   digest,
	}
	body, err := proto.Marshal(pbDigest)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to marshal gossip digest for router")
		return
	}
	msg := channel.NewMessage(gossip_pb.GossipDigestType, body)
	if err := msg.Send(ch); err != nil && !ch.IsClosed() {
		pfxlog.Logger().WithError(err).WithField("routerId", router.Id).
			Error("failed to send gossip digest to router")
	}
}
