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

package handler_edge_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
)

func NewCreateCircuitV2Handler(appEnv *env.AppEnv, ch channel.Channel) channel.TypedReceiveHandler {
	handler := &createCircuitHandler{
		baseRequestHandler: baseRequestHandler{
			ch:     ch,
			appEnv: appEnv,
		},
	}
	return &channel.AsyncFunctionReceiveAdapter{
		Type:    int32(edge_ctrl_pb.ContentType_CreateCircuitV2RequestType),
		Handler: handler.HandleReceiveCreateCircuitV2,
	}
}

func (self *createCircuitHandler) HandleReceiveCreateCircuitV2(msg *channel.Message, ch channel.Channel) {
	req, err := ctrl_msg.DecodeCreateCircuitV2Request(msg)
	if err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not decode CreateCircuitRequest")
		return
	}

	ctx := &CreateCircuitRequestContext{
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg, env: self.appEnv},
		req:                       req,
	}

	self.CreateCircuit(ctx, self.CreateCircuitV2Response)
}

func (self *createCircuitHandler) CreateCircuitV2Response(circuitInfo *model.Circuit, peerData map[uint32][]byte) (*channel.Message, error) {
	response := &ctrl_msg.CreateCircuitV2Response{
		CircuitId: circuitInfo.Id,
		Address:   circuitInfo.Path.IngressId,
		PeerData:  peerData,
		Tags:      circuitInfo.Tags,
	}

	return response.ToMessage(), nil
}
