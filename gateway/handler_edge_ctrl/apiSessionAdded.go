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
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/gateway/internal/fabric"
	"github.com/netfoundry/ziti-edge/pb/edge_ctrl_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type apiSessionAddedHandler struct {
	sm fabric.StateManager
}

func NewApiSessionAddedHandler(sm fabric.StateManager) *apiSessionAddedHandler {
	return &apiSessionAddedHandler{
		sm: sm,
	}
}

func (h *apiSessionAddedHandler) ContentType() int32 {
	return env.ApiSessionAddedType
}

func (h *apiSessionAddedHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	go func() {
		req := &edge_ctrl_pb.ApiSessionAdded{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {
			for _, session := range req.ApiSessions {
				h.sm.AddApiSession(session)
			}

			if req.IsFullState {
				h.sm.RemoveMissingApiSessions(req.ApiSessions)
			}
		} else {
			pfxlog.Logger().Panic("could not convert message as network session added")
		}
	}()
}
