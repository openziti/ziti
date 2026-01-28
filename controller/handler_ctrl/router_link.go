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

func (h *routerLinkHandler) HandleLinks(links *ctrl_pb.RouterLinks) {
	if links.FullRefresh {
		linkIdMap := map[string]struct{}{}

		for _, link := range links.Links {
			linkIdMap[link.Id] = struct{}{}
		}

		var toRemove []*model.Link

		for entry := range h.network.Link.IterateLinks() {
			if entry.Val.Src.Id == h.r.Id {
				if _, ok := linkIdMap[entry.Key]; !ok {
					toRemove = append(toRemove, entry.Val)
				}
			}
		}

		for _, link := range toRemove {
			h.network.LinkFaulted(link, false)
			pfxlog.Logger().WithField("linkId", link.Id).Info("removed link not present in full reported set")
		}
	}

	for _, link := range links.Links {
		h.network.NotifyExistingLink(h.r, link)
	}
}
