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
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type updateTerminatorHandler struct {
	baseRequestHandler
}

func NewUpdateTerminatorHandler(appEnv *env.AppEnv, ch channel.Channel) channel.TypedReceiveHandler {
	return &updateTerminatorHandler{
		baseRequestHandler{
			ch:     ch,
			appEnv: appEnv,
		},
	}
}

func (self *updateTerminatorHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_UpdateTerminatorRequestType)
}

func (self *updateTerminatorHandler) Label() string {
	return "update.terminator"
}

func (self *updateTerminatorHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	req := &edge_ctrl_pb.UpdateTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal UpdateTerminator")
		return
	}

	ctx := &UpdateTerminatorRequestContext{
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg, env: self.appEnv},
		req:                       req,
	}

	go self.UpdateTerminator(ctx)
}

func (self *updateTerminatorHandler) UpdateTerminator(ctx *UpdateTerminatorRequestContext) {
	if !ctx.loadRouter() {
		return
	}

	logger := logrus.
		WithField("routerId", self.ch.Id()).
		WithField("token", ctx.req.SessionToken).
		WithField("terminatorId", ctx.req.TerminatorId).
		WithField("cost", ctx.req.Cost).
		WithField("updateCost", ctx.req.UpdateCost).
		WithField("precedence", ctx.req.Precedence).
		WithField("updatePrecedence", ctx.req.UpdatePrecedence)

	logger.Debug("update request received")

	ctx.loadSession(ctx.req.SessionToken, ctx.req.ApiSessionToken)
	ctx.checkSessionType(db.SessionTypeBind)
	terminator := ctx.verifyTerminator(ctx.req.TerminatorId, common.EdgeBinding)

	ctx.updateTerminator(terminator, ctx.req, ctx.newChangeContext())

	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	logger = logger.WithField("serviceId", terminator.Service)
	logger.Info("updated terminator")

	responseMsg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_UpdateTerminatorResponseType), nil)
	responseMsg.ReplyTo(ctx.msg)
	if err := self.ch.Send(responseMsg); err != nil {
		logger.WithError(err).Error("failed to send update terminator response")
	}
}

type UpdateTerminatorRequestContext struct {
	baseSessionRequestContext
	req *edge_ctrl_pb.UpdateTerminatorRequest
}

func (self *UpdateTerminatorRequestContext) GetSessionToken() string {
	return self.req.SessionToken
}
