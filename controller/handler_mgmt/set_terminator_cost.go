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
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/handler_common"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/storage/boltz"
	"math"
)

type setTerminatorCostHandler struct {
	network *network.Network
}

func newSetTerminatorCostHandler(network *network.Network) *setTerminatorCostHandler {
	return &setTerminatorCostHandler{network: network}
}

func (h *setTerminatorCostHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_SetTerminatorCostRequestType)
}

func (h *setTerminatorCostHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	request := &mgmt_pb.SetTerminatorCostRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		handler_common.SendChannel2Failure(msg, ch, err.Error())
		return
	}

	terminator, err := h.network.Terminators.Read(request.TerminatorId)
	if err != nil {
		handler_common.SendChannel2Failure(msg, ch, err.Error())
		return
	}

	var precedence xt.Precedence
	if request.UpdateMask&int32(mgmt_pb.TerminatorChangeMask_Precedence) != 0 {
		if request.Precedence == mgmt_pb.TerminatorPrecedence_Default {
			precedence = xt.Precedences.Default
		} else if request.Precedence == mgmt_pb.TerminatorPrecedence_Required {
			precedence = xt.Precedences.Required
		} else if request.Precedence == mgmt_pb.TerminatorPrecedence_Failed {
			precedence = xt.Precedences.Failed
		} else {
			handler_common.SendChannel2Failure(msg, ch, fmt.Sprintf("invalid precedence: %v", request.Precedence))
			return
		}
	}

	var staticCost uint16
	if request.UpdateMask&int32(mgmt_pb.TerminatorChangeMask_StaticCost) != 0 {
		if request.StaticCost > math.MaxUint16 {
			handler_common.SendChannel2Failure(msg, ch, fmt.Sprintf("invalid static cost %v. Must be less than %v", request.StaticCost, math.MaxUint16))
			return
		}
		staticCost = uint16(request.StaticCost)
	}

	var dynamicCost uint16
	if request.UpdateMask&int32(mgmt_pb.TerminatorChangeMask_DynamicCost) != 0 {
		if request.DynamicCost > math.MaxUint16 {
			handler_common.SendChannel2Failure(msg, ch, fmt.Sprintf("invalid dynamic cost %v. Must be less than %v", request.DynamicCost, math.MaxUint16))
			return
		}
		dynamicCost = uint16(request.DynamicCost)
	}

	updateStaticCost := request.UpdateMask&int32(mgmt_pb.TerminatorChangeMask_StaticCost) != 0
	updatePrecedence := request.UpdateMask&int32(mgmt_pb.TerminatorChangeMask_Precedence) != 0

	if updateStaticCost || updatePrecedence {
		checker := boltz.MapFieldChecker{}

		if updateStaticCost {
			terminator.Cost = staticCost
			checker[db.FieldTerminatorCost] = struct{}{}
		}

		if updatePrecedence {
			terminator.Precedence = precedence
			checker[db.FieldTerminatorPrecedence] = struct{}{}
		}

		if err := h.network.Terminators.Patch(terminator, checker); err != nil {
			handler_common.SendChannel2Failure(msg, ch, err.Error())
			return
		}
	}

	if request.UpdateMask&int32(mgmt_pb.TerminatorChangeMask_DynamicCost) != 0 {
		xt.GlobalCosts().SetDynamicCost(request.TerminatorId, dynamicCost)
	}

	handler_common.SendChannel2Success(msg, ch, "")
}
