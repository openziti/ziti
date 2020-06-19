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
	"github.com/golang/protobuf/ptypes"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/handler_common"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
)

type getTerminatorHandler struct {
	network *network.Network
}

func newGetTerminatorHandler(network *network.Network) *getTerminatorHandler {
	return &getTerminatorHandler{network: network}
}

func (h *getTerminatorHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_GetTerminatorRequestType)
}

func (h *getTerminatorHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	rs := &mgmt_pb.GetTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, rs); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}
	response := &mgmt_pb.GetTerminatorResponse{}
	terminator, err := h.network.Terminators.Read(rs.TerminatorId)
	if err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	response.Terminator = toApiTerminator(terminator)
	body, err := proto.Marshal(response)
	if err == nil {
		responseMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_GetTerminatorResponseType), body)
		responseMsg.ReplyTo(msg)
		if err = ch.Send(responseMsg); err != nil {
			pfxlog.ContextLogger(ch.Label()).Errorf("unexpected error (%s)", err)
		}
	} else {
		pfxlog.ContextLogger(ch.Label()).Errorf("unexpected error (%s)", err)
	}
}

func toApiTerminator(s *network.Terminator) *mgmt_pb.Terminator {
	ts, err := ptypes.TimestampProto(s.CreatedAt)
	if err != nil {
		pfxlog.Logger().Warnf("unexpected bad timestamp conversion: %v", err)
	}
	return &mgmt_pb.Terminator{
		Id:        s.Id,
		ServiceId: s.Service,
		RouterId:  s.Router,
		Binding:   s.Binding,
		Address:   s.Address,
		CreatedAt: ts,
	}
}
