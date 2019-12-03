/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-fabric/fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
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

func (h *setLinkDownHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	set := &mgmt_pb.SetLinkDownRequest{}
	if err := proto.Unmarshal(msg.Body, set); err == nil {
		if l, found := h.network.GetLink(&identity.TokenId{Token: set.LinkId}); found {
			l.Down = set.Down
			h.network.LinkChanged(l)
			log.Infof("set down state of link [l/%s] to [%t]", set.LinkId, set.Down)
			sendSuccess(msg, ch, "")
		} else {
			sendFailure(msg, ch, fmt.Sprintf("unknown link [l/%s]", set.LinkId))
		}
	} else {
		sendFailure(msg, ch, err.Error())
	}
}
