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
	"github.com/openziti/fabric/controller/handler_common"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
	"reflect"
)

type listTerminatorsHandler struct {
	network *network.Network
}

func newListTerminatorsHandler(network *network.Network) *listTerminatorsHandler {
	return &listTerminatorsHandler{network: network}
}

func (h *listTerminatorsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_ListTerminatorsRequestType)
}

func (h *listTerminatorsHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	ls := &mgmt_pb.ListTerminatorsRequest{}
	if err := proto.Unmarshal(msg.Body, ls); err != nil {
		handler_common.SendChannel2Failure(msg, ch, err.Error())
		return
	}
	response := &mgmt_pb.ListTerminatorsResponse{Terminators: make([]*mgmt_pb.Terminator, 0)}

	result, err := h.network.Terminators.BaseList(ls.Query)
	if err == nil {
		for _, entity := range result.Entities {
			terminator, ok := entity.(*network.Terminator)
			if !ok {
				errorMsg := fmt.Sprintf("unexpected result in terminator list of type: %v", reflect.TypeOf(entity))
				handler_common.SendChannel2Failure(msg, ch, errorMsg)
				return
			}
			response.Terminators = append(response.Terminators, toApiTerminator(terminator))
		}

		body, err := proto.Marshal(response)
		if err == nil {
			responseMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListTerminatorsResponseType), body)
			responseMsg.ReplyTo(msg)
			if err := ch.Send(responseMsg); err != nil {
				pfxlog.ContextLogger(ch.Label()).Errorf("unexpected error sending response (%s)", err)
			}
		} else {
			handler_common.SendChannel2Failure(msg, ch, err.Error())
		}
	} else {
		handler_common.SendChannel2Failure(msg, ch, err.Error())
	}
}
