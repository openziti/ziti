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
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/edge_common"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/sirupsen/logrus"
)

type removeTunnelTerminatorHandler struct {
	baseRequestHandler
}

func NewRemoveTunnelTerminatorHandler(appEnv *env.AppEnv, ch channel2.Channel) channel2.ReceiveHandler {
	return &removeTunnelTerminatorHandler{
		baseRequestHandler{
			ch:     ch,
			appEnv: appEnv,
		},
	}
}

func (self *removeTunnelTerminatorHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_RemoveTunnelTerminatorRequestType)
}

func (self *removeTunnelTerminatorHandler) Label() string {
	return "tunnel.remove.terminator"
}

func (self *removeTunnelTerminatorHandler) HandleReceive(msg *channel2.Message, _ channel2.Channel) {
	ctx := &RemoveTunnelTerminatorRequestContext{
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg},
		terminatorId:              string(msg.Body),
	}

	go self.RemoveTerminator(ctx)
}

func (self *removeTunnelTerminatorHandler) RemoveTerminator(ctx *RemoveTunnelTerminatorRequestContext) {
	logger := logrus.WithField("router", self.ch.Id().Token).WithField("terminatorId", ctx.terminatorId)

	if !ctx.loadRouter() {
		return
	}

	t := ctx.verifyTerminator(ctx.terminatorId, edge_common.TunnelBinding)
	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	logger = logger.WithField("serviceId", t.Service)

	err := self.getNetwork().Terminators.Delete(ctx.terminatorId)
	if err != nil {
		self.returnError(ctx, internalError(err))
		return
	}

	logger.Info("removed terminator")

	responseMsg := channel2.NewMessage(int32(edge_ctrl_pb.ContentType_RemoveTunnelTerminatorResponseType), nil)
	responseMsg.ReplyTo(ctx.msg)
	if err := self.ch.Send(responseMsg); err != nil {
		logger.WithError(err).Error("failed to send remove tunnel terminator response")
	}
}

type RemoveTunnelTerminatorRequestContext struct {
	baseSessionRequestContext
	terminatorId string
}
