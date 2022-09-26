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
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"google.golang.org/protobuf/proto"
)

type linkConnectedHandler struct {
	r       *network.Router
	network *network.Network
}

func newLinkConnectedHandler(r *network.Router, network *network.Network) *linkConnectedHandler {
	return &linkConnectedHandler{r: r, network: network}
}

func (h *linkConnectedHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_LinkConnectedType)
}

func (h *linkConnectedHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	link := &ctrl_pb.LinkConnected{}
	if err := proto.Unmarshal(msg.Body, link); err != nil {
		log.WithError(err).Error("failed to unmarshal link message")
		return
	}

	go h.HandleLink(ch, link)
}

func (h *linkConnectedHandler) HandleLink(ch channel.Channel, link *ctrl_pb.LinkConnected) {
	log := pfxlog.ContextLogger(ch.Label()).WithField("linkId", link.Id)

	if err := h.network.LinkConnected(link); err == nil {
		log.Info("link connected")
	} else {
		log.WithError(err).Error("unexpected error marking link connected")
	}
}
