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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/edge_common"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/sirupsen/logrus"
)

type removeTerminatorHandler struct {
	baseRequestHandler
}

func NewRemoveTerminatorHandler(appEnv *env.AppEnv, ch channel.Channel) channel.TypedReceiveHandler {
	return &removeTerminatorHandler{
		baseRequestHandler{
			ch:     ch,
			appEnv: appEnv,
		},
	}
}

func (self *removeTerminatorHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_RemoveTerminatorRequestType)
}

func (self *removeTerminatorHandler) Label() string {
	return "remove.terminator"
}

func (self *removeTerminatorHandler) sendResponse(ctx *RemoveTerminatorRequestContext) {
	log := pfxlog.ContextLogger(self.ch.Label())

	responseMsg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_RemoveTerminatorResponseType), nil)
	responseMsg.ReplyTo(ctx.msg)
	if err := self.ch.Send(responseMsg); err != nil {
		log.WithError(err).WithField("token", ctx.req.SessionToken).Error("failed to send remove terminator response")
	}
}

func (self *removeTerminatorHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	req := &edge_ctrl_pb.RemoveTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal RemoveTerminator")
		return
	}

	ctx := &RemoveTerminatorRequestContext{
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg},
		req:                       req,
	}

	go self.RemoveTerminator(ctx)
}

func (self *removeTerminatorHandler) RemoveTerminator(ctx *RemoveTerminatorRequestContext) {
	if !ctx.loadRouter() {
		return
	}

	terminator := ctx.verifyTerminator(ctx.req.TerminatorId, edge_common.EdgeBinding)
	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	ctx.loadSession(ctx.req.SessionToken)
	if ctx.err != nil {
		// if the session is invalid, we still want to delete the terminator if the session is gone, but
		// the terminator matches the sessions
		if terminator.Address == "hosted:"+ctx.req.SessionToken {
			ctx.err = nil
		} else {
			self.returnError(ctx, ctx.err)
			return
		}
	} else {
		ctx.checkSessionType(persistence.SessionTypeBind)
		ctx.checkSessionFingerprints(ctx.req.Fingerprints)
	}

	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	err := self.getNetwork().Terminators.Delete(ctx.req.TerminatorId)
	if err != nil {
		self.returnError(ctx, internalError(err))
		return
	}

	logrus.
		WithField("routerId", self.ch.Id().Token).
		WithField("serviceId", terminator.Service).
		WithField("token", ctx.req.SessionToken).
		WithField("terminator", ctx.req.TerminatorId).
		Info("removed terminator")

	self.sendResponse(ctx)
}

type RemoveTerminatorRequestContext struct {
	baseSessionRequestContext
	req *edge_ctrl_pb.RemoveTerminatorRequest
}

func (self *RemoveTerminatorRequestContext) GetSessionToken() string {
	return self.req.SessionToken
}
