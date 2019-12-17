/*
	Copyright 2019 Netfoundry, Inc.

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

package handler_edge_ctrl

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/edge/controller/env"
	"github.com/netfoundry/ziti-edge/edge/pb/edge_ctrl_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type sessionHeartbeatHandler struct {
	appEnv *env.AppEnv
}

func NewSessionHeartbeatHandler(appEnv *env.AppEnv) *sessionHeartbeatHandler {
	return &sessionHeartbeatHandler{
		appEnv: appEnv,
	}
}

func (h *sessionHeartbeatHandler) ContentType() int32 {
	return env.ApiSessionHeartbeatType
}

func (h *sessionHeartbeatHandler) HandleReceive(msg *channel2.Message, _ channel2.Channel) {
	go func() {
		req := &edge_ctrl_pb.ApiSessionHeartbeat{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {

			err := h.appEnv.GetHandlers().ApiSession.HandleMarkActivity(req.Tokens)

			if err != nil {
				pfxlog.Logger().
					WithError(err).
					Error("unable to set activity for heartbeat")
			}

		} else {
			pfxlog.Logger().Panic("could not convert message as session heartbeat")
		}
	}()
}
