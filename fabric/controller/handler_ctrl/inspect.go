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

package handler_ctrl

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-fabric/fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/fabric/pb/ctrl_pb"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
)

type inspectHandler struct {
	network *network.Network
}

func newInspectHandler(network *network.Network) *inspectHandler {
	return &inspectHandler{network: network}
}

func (h *inspectHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_InspectRequestType)
}

func (h *inspectHandler) HandleReceive(request *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	inspectRequest := &ctrl_pb.InspectRequest{}
	if err := proto.Unmarshal(request.Body, inspectRequest); err != nil {
		log.Errorf("unexpected error (%s)", err)
		h.respondWithError(ch, request, fmt.Errorf("failed to decode request: (%v)", err).Error())
	}

	response := &ctrl_pb.InspectResponse{Success: true}
	for _, value := range inspectRequest.RequestedValues {
		if value == "capability" {
			for _, capability := range h.network.GetCapabilities() {
				response.AddValue("capability", capability)
			}
		}
	}
	h.respond(ch, request, response)
}

func (h *inspectHandler) respondWithError(ch channel2.Channel, request *channel2.Message, errs...string) {
	response := &ctrl_pb.InspectResponse{Success: false, Errors: errs }
	h.respond(ch, request, response)
}

func (h *inspectHandler) respond(ch channel2.Channel, request *channel2.Message, response *ctrl_pb.InspectResponse) {
	log := pfxlog.ContextLogger(ch.Label())

	if body, err := proto.Marshal(response); err == nil {
		responseMsg := channel2.NewMessage(int32(ctrl_pb.ContentType_InspectResponseType), body)
		responseMsg.ReplyTo(request)
		if err := ch.Send(responseMsg); err != nil {
			log.Errorf("unable to respond to inspect request(%s)", err)
		}
	} else {
		log.Errorf("unexpected error marshalling response (%s)", err)
	}
}
