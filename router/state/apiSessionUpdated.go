/*
	Copyright NetFoundry Inc.

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

package state

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"google.golang.org/protobuf/proto"
)

type apiSessionUpdatedHandler struct {
	sm Manager
}

func NewApiSessionUpdatedHandler(sm Manager) *apiSessionUpdatedHandler {
	return &apiSessionUpdatedHandler{
		sm: sm,
	}
}

func (h *apiSessionUpdatedHandler) ContentType() int32 {
	return env.ApiSessionUpdatedType
}

func (h *apiSessionUpdatedHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	go func() {
		req := &edge_ctrl_pb.ApiSessionUpdated{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {
			for _, session := range req.ApiSessions {
				h.sm.UpdateApiSession(&ApiSession{
					ApiSession:   session,
					ControllerId: ch.Id(),
				})
			}
		} else {
			pfxlog.Logger().Panic("could not convert message as network session updated")
		}
	}()
}
