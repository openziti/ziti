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

package handler_mgmt

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/handler_common"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/mgmt_pb"
)

type setLinkDownHandler struct {
	network *network.Network
}

func newSetLinkDownHandler(network *network.Network) *setLinkDownHandler {
	return &setLinkDownHandler{network: network}
}

func (h *setLinkDownHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_SetLinkDownRequestType)
}

func (h *setLinkDownHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	set := &mgmt_pb.SetLinkDownRequest{}
	if err := proto.Unmarshal(msg.Body, set); err == nil {
		if l, found := h.network.GetLink(set.LinkId); found {
			l.SetDown(set.Down)
			h.network.LinkChanged(l)
			log.Infof("set down state of link [l/%s] to [%t]", set.LinkId, set.Down)
			handler_common.SendSuccess(msg, ch, "")
		} else {
			handler_common.SendFailure(msg, ch, fmt.Sprintf("unknown link [l/%s]", set.LinkId))
		}
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}
