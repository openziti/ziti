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
	"github.com/openziti/fabric/controller/handler_common"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
)

type getServiceHandler struct {
	network *network.Network
}

func newGetServiceHandler(network *network.Network) *getServiceHandler {
	return &getServiceHandler{network: network}
}

func (h *getServiceHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_GetServiceRequestType)
}

func (h *getServiceHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	rs := &mgmt_pb.GetServiceRequest{}
	err := proto.Unmarshal(msg.Body, rs)

	if err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	response := &mgmt_pb.GetServiceResponse{}
	svc, err := h.network.Services.Read(rs.ServiceId)
	if err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	response.Service = toApiService(svc)
	body, err := proto.Marshal(response)
	if err == nil {
		responseMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_GetServiceResponseType), body)
		responseMsg.ReplyTo(msg)
		ch.Send(responseMsg)
	} else {
		pfxlog.ContextLogger(ch.Label()).Errorf("unexpected error (%s)", err)
	}
}

func toApiService(s *network.Service) *mgmt_pb.Service {
	var terminators []*mgmt_pb.Terminator
	for _, terminator := range s.Terminators {
		terminators = append(terminators, toApiTerminator(terminator))
	}

	return &mgmt_pb.Service{
		Id:                 s.Id,
		TerminatorStrategy: s.TerminatorStrategy,
		Terminators:        terminators,
	}
}
