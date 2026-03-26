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
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"google.golang.org/protobuf/proto"
)

type routerLinkHandler struct {
	r       *model.Router
	network *network.Network
}

func newRouterLinkHandler(r *model.Router, network *network.Network) *routerLinkHandler {
	return &routerLinkHandler{r: r, network: network}
}

func (h *routerLinkHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_RouterLinksType)
}

func (h *routerLinkHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	if !h.r.Connected.Load() || ch.IsClosed() {
		return
	}

	log := pfxlog.ContextLogger(ch.Label())

	link := &ctrl_pb.RouterLinks{}
	if err := proto.Unmarshal(msg.Body, link); err != nil {
		log.WithError(err).Error("failed to unmarshal link message")
		return
	}

	h.HandleLinks(link)
}

// HandleLinks processes link reports from old routers (those not using the gossip
// protocol directly). In HA mode, only the raft leader writes to gossip — non-
// leaders handle links locally. New routers bypass this handler entirely and send
// GossipDelta messages directly.
func (h *routerLinkHandler) HandleLinks(links *ctrl_pb.RouterLinks) {
	if h.network.IsLeader() {
		h.handleLinksAsLeader(links)
	} else {
		h.handleLinksAsNonLeader(links)
	}
}

// handleLinksAsLeader is the designated gossip writer path. Translates old-router
// link reports into gossip entries using the controller's Lamport clock.
func (h *routerLinkHandler) handleLinksAsLeader(links *ctrl_pb.RouterLinks) {
	if links.FullRefresh {
		keyMap := map[string]struct{}{}
		for _, link := range links.Links {
			keyMap[network.LinkGossipKey(link.Id, link.Iteration)] = struct{}{}
		}
		h.network.ReconcileLinksViaGossip(h.r.Id, keyMap)
	}

	for _, link := range links.Links {
		h.network.NotifyLinkViaGossip(h.r, link)
	}
}

// handleLinksAsNonLeader handles links locally without writing to gossip. The
// leader will receive the same reports from the old router and gossip them.
func (h *routerLinkHandler) handleLinksAsNonLeader(links *ctrl_pb.RouterLinks) {
	for _, link := range links.Links {
		h.network.NotifyExistingLink(h.r, link)
	}
}
