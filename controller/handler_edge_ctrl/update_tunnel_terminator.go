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
	"github.com/openziti/edge/edge_common"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/sirupsen/logrus"
)

type updateTunnelTerminatorHandler struct {
	baseRequestHandler
}

func NewUpdateTunnelTerminatorHandler(appEnv *env.AppEnv, ch channel2.Channel) channel2.ReceiveHandler {
	return &updateTunnelTerminatorHandler{
		baseRequestHandler{
			ch:     ch,
			appEnv: appEnv,
		},
	}
}

func (self *updateTunnelTerminatorHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_UpdateTunnelTerminatorRequestType)
}

func (self *updateTunnelTerminatorHandler) Label() string {
	return "tunnel.update.terminator"
}

func (self *updateTunnelTerminatorHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	req := &edge_ctrl_pb.UpdateTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal UpdateTerminator")
		return
	}

	ctx := &UpdateTerminatorRequestContext{
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg},
		req:                       req,
	}

	go self.UpdateTerminator(ctx)
}

func (self *updateTunnelTerminatorHandler) UpdateTerminator(ctx *UpdateTerminatorRequestContext) {
	logger := logrus.WithField("router", self.ch.Id().Token).WithField("terminatorId", ctx.req.TerminatorId)

	if !ctx.loadRouter() {
		return
	}

	terminator := ctx.verifyTerminator(ctx.req.TerminatorId, edge_common.TunnelBinding)
	ctx.updateTerminator(terminator, ctx.req)
	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	logger.Info("updated terminator")

	responseMsg := channel2.NewMessage(int32(edge_ctrl_pb.ContentType_UpdateTunnelTerminatorResponseType), nil)
	responseMsg.ReplyTo(ctx.msg)
	if err := self.ch.Send(responseMsg); err != nil {
		logger.WithError(err).Error("failed to send update tunnel terminator response")
	}
}

type UpdateTunnelTerminatorRequestContext struct {
	baseSessionRequestContext
	req *edge_ctrl_pb.UpdateTunnelTerminatorRequest
}
