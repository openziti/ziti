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
	"github.com/golang/protobuf/proto"
)

type createServiceHandler struct {
	network *network.Network
}

func newCreateServiceHandler(network *network.Network) *createServiceHandler {
	return &createServiceHandler{network: network}
}

func (h *createServiceHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_CreateServiceRequestType)
}

func (h *createServiceHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	cs := &mgmt_pb.CreateServiceRequest{}
	err := proto.Unmarshal(msg.Body, cs)
	if err == nil {
		binding := "transport"
		if cs.Service.Binding != "" {
			binding = cs.Service.Binding
		}
		if err = h.network.CreateService(&network.Service{
			Id:              cs.Service.Id,
			Binding:         binding,
			EndpointAddress: cs.Service.EndpointAddress,
			Egress:          cs.Service.Egress,
		}); err == nil {
			sendSuccess(msg, ch, "")
		} else {
			sendFailure(msg, ch, err.Error())
		}
	} else {
		sendFailure(msg, ch, err.Error())
	}
}
