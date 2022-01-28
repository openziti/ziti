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

type apiSessionRemovedHandler struct {
	sm fabric.StateManager
}

func NewApiSessionRemovedHandler(sm fabric.StateManager) *apiSessionRemovedHandler {
	return &apiSessionRemovedHandler{
		sm: sm,
	}
}

func (h *apiSessionRemovedHandler) ContentType() int32 {
	return env.ApiSessionRemovedType
}

func (h *apiSessionRemovedHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	go func() {
		req := &edge_ctrl_pb.ApiSessionRemoved{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {

			//older controllers will not provide req.Ids
			hasIds := len(req.Ids) == len(req.Tokens)

			for i, token := range req.Tokens {
				id := "unknown"
				if hasIds {
					id = req.Ids[i]
				}

				pfxlog.Logger().
					WithField("apiSessionToken", token).
					WithField("apiSessionId", id).
					Debugf("removing api session [token: %s] [id: %s]", token, id)

				h.sm.RemoveApiSession(token)
			}
		} else {
			pfxlog.Logger().Panic("could not convert message as session removed")
		}
	}()
}
