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

type removeServiceHandler struct {
	network *network.Network
}

func newRemoveServiceHandler(network *network.Network) *removeServiceHandler {
	return &removeServiceHandler{network: network}
}

func (h *removeServiceHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_RemoveServiceRequestType)
}

func (h *removeServiceHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	request := &mgmt_pb.RemoveServiceRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	_, err := h.network.Services.Read(request.ServiceId)
	if err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	if err := h.network.Services.Delete(request.ServiceId); err == nil {
		log.Infof("removed service [s/%v]", request.ServiceId)
		handler_common.SendSuccess(msg, ch, "")
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}
