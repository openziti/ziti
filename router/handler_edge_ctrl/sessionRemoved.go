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
	"github.com/openziti/channel"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/edge/router/fabric"
)

type sessionRemovedHandler struct {
	sm fabric.StateManager
}

func NewSessionRemovedHandler(sm fabric.StateManager) *sessionRemovedHandler {
	return &sessionRemovedHandler{
		sm: sm,
	}
}

func (h *sessionRemovedHandler) ContentType() int32 {
	return env.SessionRemovedType
}

func (h *sessionRemovedHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	go func() {
		req := &edge_ctrl_pb.SessionRemoved{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {
			hasIds := len(req.Ids) == len(req.Tokens)

			for i, token := range req.Tokens {
				id := "unknown"
				if hasIds {
					id = req.Ids[i]
				}
				pfxlog.Logger().
					WithField("sessionToken", token).
					WithField("sessionId", id).
					Debugf("removing session [token: %s] [id: %s]", token, id)
				h.sm.RemoveEdgeSession(token)
			}
		} else {
			pfxlog.Logger().Panic("could not convert message as network session removed")
		}
	}()
}
