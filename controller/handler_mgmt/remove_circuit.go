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

type removeCircuitHandler struct {
	network *network.Network
}

func newRemoveCircuitHandler(network *network.Network) *removeCircuitHandler {
	return &removeCircuitHandler{network: network}
}

func (handler *removeCircuitHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_RemoveCircuitRequestType)
}

func (handler *removeCircuitHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	request := &mgmt_pb.RemoveCircuitRequest{}
	if err := proto.Unmarshal(msg.Body, request); err == nil {
		if err := handler.network.RemoveCircuit(request.CircuitId, request.Now); err == nil {
			handler_common.SendSuccess(msg, ch, "")
		} else {
			pfxlog.Logger().WithError(err).WithField("circuitId", request.CircuitId).Error("unexpected error removing circuit")
			handler_common.SendFailure(msg, ch, err.Error())
		}
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}
