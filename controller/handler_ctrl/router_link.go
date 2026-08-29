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
	"github.com/openziti/channel/v5"
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
	log := pfxlog.ContextLogger(ch.Label()).WithField("routerId", h.r.Id)

	// Read once and report what was read: a reconnect can flip these between the decision and the log,
	// leaving the warning naming a state that would not have discarded anything.
	routerConnected, channelClosed := h.r.Connected.Load(), ch.IsClosed()

	// A router announces its links once per reconnect and is never asked again, so a discard here leaves
	// the controller with no links for that router until it reconnects. Do not drop it silently.
	if !routerConnected || channelClosed {
		log.WithField("routerConnected", routerConnected).
			WithField("channelClosed", channelClosed).
			Warn("discarding link report, this connection is not the router's current one")
		return
	}

	link := &ctrl_pb.RouterLinks{}
	if err := proto.Unmarshal(msg.Body, link); err != nil {
		log.WithError(err).Error("failed to unmarshal link message")
		return
	}

	log.WithField("linkCount", len(link.Links)).
		WithField("fullRefresh", link.FullRefresh).
		Info("received link report from router")

	h.HandleLinks(link)
}

func (h *routerLinkHandler) HandleLinks(links *ctrl_pb.RouterLinks) {
	if links.FullRefresh {
		for _, link := range linksToPrune(h.r.Id, h.network.Link.LinksForRouter(h.r.Id), links.Links) {
			h.network.LinkFaulted(link, false)
			pfxlog.Logger().WithField("linkId", link.Id).Info("removed link not present in full reported set")
		}
	}

	for _, link := range links.Links {
		h.network.NotifyExistingLink(h.r, link)
	}
}

// linksToPrune returns the links a full refresh implies are gone: those routerId is the source of that the
// report does not list. current may also hold links routerId only accepts; those are the dialing router's to
// report, so an omission here says nothing about them.
func linksToPrune(routerId string, current []*model.Link, reported []*ctrl_pb.RouterLinks_RouterLink) []*model.Link {
	reportedIds := make(map[string]struct{}, len(reported))
	for _, link := range reported {
		reportedIds[link.Id] = struct{}{}
	}

	var toRemove []*model.Link
	for _, link := range current {
		if link.Src.Id != routerId {
			continue
		}
		if _, ok := reportedIds[link.Id]; !ok {
			toRemove = append(toRemove, link)
		}
	}
	return toRemove
}
