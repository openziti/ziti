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
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"google.golang.org/protobuf/proto"
)

type apiSessionRemovedHandler struct {
	sm Manager
}

func NewApiSessionRemovedHandler(sm Manager) *apiSessionRemovedHandler {
	return &apiSessionRemovedHandler{
		sm: sm,
	}
}

func (h *apiSessionRemovedHandler) ContentType() int32 {
	return env.ApiSessionRemovedType
}

func (h *apiSessionRemovedHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
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
					WithField("ctrlId", ch.Id()).
					Debugf("removing api session [token: %s] [id: %s]", token, id)

				apiSessionToken := NewApiSessionTokenFromLegacyToken(token)
				h.sm.RemoveLegacyApiSession(apiSessionToken)
			}
		} else {
			pfxlog.Logger().Panic("could not convert message as session removed")
		}
	}()
}
