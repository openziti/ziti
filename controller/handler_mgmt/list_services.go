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
	"reflect"
)

type listServicesHandler struct {
	network *network.Network
}

func newListServicesHandler(network *network.Network) *listServicesHandler {
	return &listServicesHandler{network: network}
}

func (h *listServicesHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_ListServicesRequestType)
}

func (h *listServicesHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	ls := &mgmt_pb.ListServicesRequest{}
	err := proto.Unmarshal(msg.Body, ls)
	if err == nil {
		response := &mgmt_pb.ListServicesResponse{Services: make([]*mgmt_pb.Service, 0)}
		result, err := h.network.Services.BaseList(ls.Query)
		if err == nil {
			for _, entity := range result.Entities {
				service, ok := entity.(*network.Service)
				if !ok {
					errorMsg := fmt.Sprintf("unexpected result in service list of type: %v", reflect.TypeOf(entity))
					handler_common.SendFailure(msg, ch, errorMsg)
					return
				}
				response.Services = append(response.Services, toApiService(service))
			}

			body, err := proto.Marshal(response)
			if err == nil {
				responseMsg := channel.NewMessage(int32(mgmt_pb.ContentType_ListServicesResponseType), body)
				responseMsg.ReplyTo(msg)
				if err := ch.Send(responseMsg); err != nil {
					pfxlog.ContextLogger(ch.Label()).Errorf("unexpected error sending response (%s)", err)
				}

			} else {
				handler_common.SendFailure(msg, ch, err.Error())
			}
		} else {
			handler_common.SendFailure(msg, ch, err.Error())
		}
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}

func toApiService(s *network.Service) *mgmt_pb.Service {
	var terminators []*mgmt_pb.Terminator
	for _, terminator := range s.Terminators {
		terminators = append(terminators, toApiTerminator(terminator))
	}

	return &mgmt_pb.Service{
		Id:                 s.Id,
		Name:               s.Name,
		TerminatorStrategy: s.TerminatorStrategy,
		Terminators:        terminators,
	}
}

func toApiTerminator(s *network.Terminator) *mgmt_pb.Terminator {
	precedence := mgmt_pb.TerminatorPrecedence_Default
	if s.Precedence.IsRequired() {
		precedence = mgmt_pb.TerminatorPrecedence_Required
	} else if s.Precedence.IsFailed() {
		precedence = mgmt_pb.TerminatorPrecedence_Failed
	}

	return &mgmt_pb.Terminator{
		Id:         s.Id,
		ServiceId:  s.Service,
		RouterId:   s.Router,
		Binding:    s.Binding,
		Address:    s.Address,
		Identity:   s.Identity,
		Cost:       uint32(s.Cost),
		Precedence: precedence,
	}
}
