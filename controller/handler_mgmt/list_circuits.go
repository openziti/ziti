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

type listCircuitsHandler struct {
	network *network.Network
}

func newListCircuitsHandler(network *network.Network) *listCircuitsHandler {
	return &listCircuitsHandler{network: network}
}

func (h *listCircuitsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_ListCircuitsRequestType)
}

func (h *listCircuitsHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	list := &mgmt_pb.ListCircuitsRequest{}
	if err := proto.Unmarshal(msg.Body, list); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	circuits := h.network.GetAllCircuits()
	response := &mgmt_pb.ListCircuitsResponse{
		Circuits: make([]*mgmt_pb.Circuit, 0),
	}
	for _, circuit := range circuits {
		responseCircuit := &mgmt_pb.Circuit{
			Id:        circuit.Id,
			ClientId:  circuit.ClientId,
			ServiceId: circuit.Service.Id,
			Path:      NewPath(circuit.Path),
		}
		response.Circuits = append(response.Circuits, responseCircuit)
	}
	body, err := proto.Marshal(response)
	if err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	responseMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListCircuitsResponseType), body)
	responseMsg.ReplyTo(msg)
	if err := ch.Send(responseMsg); err != nil {
		pfxlog.ContextLogger(ch.Label()).Errorf("unexpected error sending response (%s)", err)
	}
}

func NewPath(path *network.Path) *mgmt_pb.Path {
	mgmtPath := &mgmt_pb.Path{}
	for _, r := range path.Nodes {
		mgmtPath.Nodes = append(mgmtPath.Nodes, r.Id)
	}
	for _, l := range path.Links {
		mgmtPath.Links = append(mgmtPath.Links, l.Id)
	}
	return mgmtPath
}
