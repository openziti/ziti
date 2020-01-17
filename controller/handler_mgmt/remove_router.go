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

package handler_mgmt

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type removeRouterHandler struct {
	network *network.Network
}

func newRemoveRouterHandler(network *network.Network) *removeRouterHandler {
	return &removeRouterHandler{network: network}
}

func (h *removeRouterHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_RemoveRouterRequestType)
}

func (h *removeRouterHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	remove := &mgmt_pb.RemoveRouterRequest{}
	if err := proto.Unmarshal(msg.Body, remove); err == nil {
		router, err := h.network.GetRouter(remove.RouterId)
		if err == nil {
			if err := h.network.RemoveRouter(router); err == nil {
				log.Infof("removed router [r/%s]", remove.RouterId)
				sendSuccess(msg, ch, "")
			} else {
				sendFailure(msg, ch, err.Error())
			}
		} else {
			sendFailure(msg, ch, err.Error())
		}
	} else {
		sendFailure(msg, ch, err.Error())
	}
}
