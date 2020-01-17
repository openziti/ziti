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
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type removeSessionHandler struct {
	network *network.Network
}

func newRemoveSessionHandler(network *network.Network) *removeSessionHandler {
	return &removeSessionHandler{network: network}
}

func (handler *removeSessionHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_RemoveSessionRequestType)
}

func (handler *removeSessionHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	request := &mgmt_pb.RemoveSessionRequest{}
	if err := proto.Unmarshal(msg.Body, request); err == nil {
		sessionId := &identity.TokenId{Token: request.SessionId}
		if err := handler.network.RemoveSession(sessionId, request.Now); err == nil {
			sendSuccess(msg, ch, "")
		} else {
			pfxlog.Logger().Errorf("unexpected error removing session s/[%s] (%s)", sessionId.Token, err)
			sendFailure(msg, ch, err.Error())
		}
	} else {
		sendFailure(msg, ch, err.Error())
	}
}
