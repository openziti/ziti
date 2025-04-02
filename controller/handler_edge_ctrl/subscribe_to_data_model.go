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

package handler_edge_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"google.golang.org/protobuf/proto"
)

type subscribeToDataModelHandler struct {
	appEnv   *env.AppEnv
	callback func(routerId string, respHello *edge_ctrl_pb.SubscribeToDataModelRequest)
}

func NewSubscribeToDataModelHandler(appEnv *env.AppEnv, callback func(routerId string, respHello *edge_ctrl_pb.SubscribeToDataModelRequest)) *subscribeToDataModelHandler {
	return &subscribeToDataModelHandler{
		appEnv:   appEnv,
		callback: callback,
	}
}

func (self *subscribeToDataModelHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_SubscribeToDataModelRequestType)
}

func (h *subscribeToDataModelHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	logger := pfxlog.Logger().WithField("routerId", ch.Id())

	request := &edge_ctrl_pb.SubscribeToDataModelRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		pfxlog.Logger().WithError(err).Error("could not unmarshal SubscribeToDataModelRequest")
		return
	}

	logger = logger.WithField("index", request.CurrentIndex).
		WithField("timelineId", request.TimelineId)
	logger.Info("data model subscription request received")

	h.callback(ch.Id(), request)
}
