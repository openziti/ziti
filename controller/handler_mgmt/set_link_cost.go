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
	"github.com/netfoundry/ziti-fabric/controller/handler_common"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type setLinkCostHandler struct {
	network *network.Network
}

func newSetLinkCostHandler(network *network.Network) *setLinkCostHandler {
	return &setLinkCostHandler{network: network}
}

func (h *setLinkCostHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_SetLinkCostRequestType)
}

func (h *setLinkCostHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	set := &mgmt_pb.SetLinkCostRequest{}
	if err := proto.Unmarshal(msg.Body, set); err == nil {
		if l, found := h.network.GetLink(&identity.TokenId{Token: set.LinkId}); found {
			l.Cost = int(set.Cost)
			h.network.LinkChanged(l)
			log.Infof("set cost of link [l/%s] to [%d]", set.LinkId, set.Cost)
			handler_common.SendSuccess(msg, ch, "")
		} else {
			handler_common.SendFailure(msg, ch, fmt.Sprintf("unknown link [l/%s]", set.LinkId))
		}
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}
