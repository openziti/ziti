/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type hostRequestHandler struct {
	r       *network.Router
	network *network.Network
}

var successResponse = &ctrl_pb.BindResponse{Success: true}

func newHostRequestHandler(r *network.Router, network *network.Network) *hostRequestHandler {
	return &hostRequestHandler{r: r, network: network}
}

func (h *hostRequestHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_BindRequestType)
}

func (h *hostRequestHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	request := &ctrl_pb.BindRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		log.Errorf("unexpected error (%s)", err)
		return
	}

	response := successResponse

	var err error
	if request.BindType == ctrl_pb.BindType_Bind {
		err = h.network.BindService(h.r, request.Token, request.ServiceId, request.PeerData)
	} else if request.BindType == ctrl_pb.BindType_Unbind {
		err = h.network.UnbindService(h.r, request.Token, request.ServiceId)
	}

	if err != nil {
		log.Errorf("unexpected error (%s)", err)
		response = &ctrl_pb.BindResponse{Success: false, Message: err.Error()}
	}

	if body, err := proto.Marshal(response); err == nil {
		responseMsg := channel2.NewMessage(int32(ctrl_pb.ContentType_BindResponseType), body)
		responseMsg.ReplyTo(msg)
		if err := h.r.Control.Send(responseMsg); err != nil {
			log.Errorf("unable to respond (%s)", err)
		}
	} else {
		log.Errorf("unexpected error (%s)", err)
	}
}
