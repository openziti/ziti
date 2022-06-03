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

package handler_edge_mgmt

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_mgmt_pb"
	"github.com/openziti/fabric/handler_common"
	"google.golang.org/protobuf/proto"
)

type initEdgeHandler struct {
	appEnv *env.AppEnv
}

func NewInitEdgeHandler(appEnv *env.AppEnv) *initEdgeHandler {
	return &initEdgeHandler{
		appEnv: appEnv,
	}
}

func (h *initEdgeHandler) ContentType() int32 {
	return int32(edge_mgmt_pb.CommandType_InitEdgeType)
}

func (h *initEdgeHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.Logger().WithField("channel", ch.LogicalName())

	request := &edge_mgmt_pb.InitEdgeRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		log.WithError(err).Error("unable to parse InitEdgeRequest, closing channel")
		if err = ch.Close(); err != nil {
			log.WithError(err).Error("error closing mgmt channel")
		}
		return
	}

	if err := h.appEnv.Managers.Identity.InitializeDefaultAdmin(request.Username, request.Password, request.Name); err != nil {
		handler_common.SendOpResult(msg, ch, "init.edge", err.Error(), false)
	} else {
		handler_common.SendOpResult(msg, ch, "init.edge", err.Error(), true)
	}
}
