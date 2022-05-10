/*
	Copyright NetFoundry, Inc.

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
	"google.golang.org/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/ctrl_pb"
)

type routerLinkHandler struct {
	r       *network.Router
	network *network.Network
}

func newRouterLinkHandler(r *network.Router, network *network.Network) *routerLinkHandler {
	return &routerLinkHandler{r: r, network: network}
}

func (h *routerLinkHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_RouterLinksType)
}

func (h *routerLinkHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	link := &ctrl_pb.RouterLinks{}
	if err := proto.Unmarshal(msg.Body, link); err != nil {
		log.WithError(err).Error("failed to unmarshal link message")
		return
	}

	go h.HandleLinks(ch, link)
}

func (h *routerLinkHandler) HandleLinks(ch channel.Channel, links *ctrl_pb.RouterLinks) {
	log := pfxlog.ContextLogger(ch.Label()).WithField("routerId", ch.Id().Token)

	for _, link := range links.Links {
		linkLog := log.WithField("linkId", link.Id).
			WithField("destRouterId", link.DestRouterId)

		created, err := h.network.NotifyExistingLink(link.Id, link.LinkProtocol, h.r, link.DestRouterId)
		if err != nil {
			linkLog.WithError(err).Error("unexpected error adding router reported link")
		} else if created {
			linkLog.Info("router reported link added")
		} else {
			linkLog.Info("router reported link already known")
		}
	}
}
