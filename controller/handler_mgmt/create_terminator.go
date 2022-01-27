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
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/handler_common"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/pkg/errors"
	"math"
)

type createTerminatorHandler struct {
	network *network.Network
}

func newCreateTerminatorHandler(network *network.Network) *createTerminatorHandler {
	return &createTerminatorHandler{network: network}
}

func (h *createTerminatorHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_CreateTerminatorRequestType)
}

func (h *createTerminatorHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	cs := &mgmt_pb.CreateTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, cs); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}
	terminator, err := toModelTerminator(h.network, cs.Terminator)
	if err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}
	if id, err := h.network.Terminators.Create(terminator); err == nil {
		handler_common.SendSuccess(msg, ch, id)
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}

func toModelTerminator(n *network.Network, terminator *mgmt_pb.Terminator) (*network.Terminator, error) {
	router, _ := n.GetRouter(terminator.RouterId)
	if router == nil {
		return nil, errors.Errorf("invalid router id %v", terminator.RouterId)
	}

	binding := "transport"
	if terminator.Binding != "" {
		binding = terminator.Binding
	}

	if terminator.Cost > math.MaxUint16 {
		return nil, errors.Errorf("invalid cost %v. cost must be between 0 and %v inclusive", terminator.Cost, math.MaxUint16)
	}

	precedence := xt.Precedences.Default
	if terminator.Precedence == mgmt_pb.TerminatorPrecedence_Required {
		precedence = xt.Precedences.Required
	} else if terminator.Precedence == mgmt_pb.TerminatorPrecedence_Failed {
		precedence = xt.Precedences.Failed
	}

	return &network.Terminator{
		BaseEntity: models.BaseEntity{
			Id:   terminator.Id,
			Tags: nil,
		},
		Service:        terminator.ServiceId,
		Router:         router.Id,
		Binding:        binding,
		Address:        terminator.Address,
		Identity:       terminator.Identity,
		IdentitySecret: terminator.IdentitySecret,
		Cost:           uint16(terminator.Cost),
		Precedence:     precedence,
	}, nil
}
