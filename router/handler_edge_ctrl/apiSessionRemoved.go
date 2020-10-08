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

func (h *apiSessionRemovedHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	go func() {
		req := &edge_ctrl_pb.ApiSessionRemoved{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {
			for _, t := range req.Tokens {
				h.sm.RemoveApiSession(t)
			}
		} else {
			pfxlog.Logger().Panic("could not convert message as session removed")
		}
	}()
}
