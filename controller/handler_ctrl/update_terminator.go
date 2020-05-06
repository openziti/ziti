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

package handler_ctrl

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/netfoundry/ziti-fabric/controller/handler_common"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/controller/xt"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"math"
)

type updateTerminatorHandler struct {
	network *network.Network
}

func newUpdateTerminatorHandler(network *network.Network) *updateTerminatorHandler {
	return &updateTerminatorHandler{network: network}
}

func (h *updateTerminatorHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_UpdateTerminatorRequestType)
}

func (h *updateTerminatorHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	request := &ctrl_pb.UpdateTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	terminator, err := h.network.Terminators.Read(request.TerminatorId)
	if err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	var precedence xt.Precedence
	if request.UpdatePrecedence {
		if request.Precedence == ctrl_pb.TerminatorPrecedence_Default {
			precedence = xt.Precedences.Default
		} else if request.Precedence == ctrl_pb.TerminatorPrecedence_Required {
			precedence = xt.Precedences.Required
		} else if request.Precedence == ctrl_pb.TerminatorPrecedence_Failed {
			precedence = xt.Precedences.Failed
		} else {
			handler_common.SendFailure(msg, ch, fmt.Sprintf("invalid precedence: %v", request.Precedence))
			return
		}
	}

	var staticCost uint16
	if request.UpdateCost {
		if request.Cost > math.MaxUint16 {
			handler_common.SendFailure(msg, ch, fmt.Sprintf("invalid static cost %v. Must be less than %v", request.Cost, math.MaxUint16))
			return
		}
		staticCost = uint16(request.Cost)
	}

	if request.UpdateCost {
		terminator.Cost = staticCost
		checker := boltz.MapFieldChecker{
			"cost": struct{}{},
		}
		if err := h.network.Terminators.Patch(terminator, checker); err != nil {
			handler_common.SendFailure(msg, ch, err.Error())
			return
		}
	}

	if request.UpdatePrecedence {
		xt.GlobalCosts().SetPrecedence(request.TerminatorId, precedence)
	}

	handler_common.SendSuccess(msg, ch, "")
}
