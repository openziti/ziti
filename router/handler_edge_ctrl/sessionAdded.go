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

package handler_edge_ctrl

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/router/internal/fabric"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/foundation/channel2"
)

type sessionAddedHandler struct {
	sm fabric.StateManager
}

func NewSessionAddedHandler(sm fabric.StateManager) *sessionAddedHandler {
	return &sessionAddedHandler{
		sm: sm,
	}
}

func (h *sessionAddedHandler) ContentType() int32 {
	return env.SessionAddedType
}

func (h *sessionAddedHandler) HandleReceive(msg *channel2.Message, _ channel2.Channel) {
	go func() {
		req := &edge_ctrl_pb.SessionAdded{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {
			for _, session := range req.Sessions {
				pfxlog.Logger().Debugf("received new session %+v", session)
				h.sm.AddSession(session)
			}

			if req.IsFullState {
				h.sm.RemoveMissingSessions(req.Sessions)
			}
		} else {
			pfxlog.Logger().Panic("could not convert message as network session added")
		}
	}()
}
