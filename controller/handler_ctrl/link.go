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
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/channel"
)

type linkHandler struct {
	r       *network.Router
	network *network.Network
}

func newLinkHandler(r *network.Router, network *network.Network) *linkHandler {
	return &linkHandler{r: r, network: network}
}

func (h *linkHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_LinkType)
}

func (h *linkHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	link := &ctrl_pb.Link{}
	if err := proto.Unmarshal(msg.Body, link); err != nil {
		log.WithError(err).Error("failed to unmarshal link message")
		return
	}

	go h.HandleLink(msg, ch, link)
}

func (h *linkHandler) HandleLink(msg *channel.Message, ch channel.Channel, link *ctrl_pb.Link) {
	log := pfxlog.ContextLogger(ch.Label())

	if err := h.network.LinkConnected(link.Id, true); err == nil {
		log.Infof("link connected [l/%s]", link.Id)
	} else {
		log.WithError(err).Error("unexpected error marking link connected")
	}
}
