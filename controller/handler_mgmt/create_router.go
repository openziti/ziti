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
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/handler_common"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type createRouterHandler struct {
	network *network.Network
}

func newCreateRouterHandler(network *network.Network) *createRouterHandler {
	return &createRouterHandler{network: network}
}

func (h *createRouterHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_CreateRouterRequestType)
}

func (h *createRouterHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	create := &mgmt_pb.CreateRouterRequest{}
	if err := proto.Unmarshal(msg.Body, create); err == nil {
		r := network.NewRouter(create.Router.Id, create.Router.Fingerprint)
		if err := h.network.CreateRouter(r); err == nil {
			log.Infof("created router [r/%s] with fingerprint [%s]", r.Id, r.Fingerprint)
			handler_common.SendSuccess(msg, ch, "")
		} else {
			handler_common.SendFailure(msg, ch, err.Error())
		}
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}
