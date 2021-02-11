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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"math"
)

type updateTerminatorHandler struct {
	baseRequestHandler
}

func NewUpdateTerminatorHandler(appEnv *env.AppEnv, ch channel2.Channel) channel2.ReceiveHandler {
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

func (self *updateTerminatorHandler) sendResponse(ctx *UpdateTerminatorRequestContext) {
	log := pfxlog.ContextLogger(self.ch.Label())

	responseMsg := channel2.NewMessage(int32(edge_ctrl_pb.ContentType_UpdateTerminatorResponseType), nil)
	responseMsg.ReplyTo(ctx.msg)
	if err := self.ch.Send(responseMsg); err != nil {
		log.WithError(err).WithField("token", ctx.req.SessionToken).Error("failed to send update terminator response")
	}
}

func (self *updateTerminatorHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	req := &edge_ctrl_pb.UpdateTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal UpdateTerminator")
		return
	}

	ctx := &UpdateTerminatorRequestContext{
		baseRequestContext: baseRequestContext{handler: self, msg: msg},
		req:                req,
	}

	go self.UpdateTerminator(ctx)
}

func (self *updateTerminatorHandler) UpdateTerminator(ctx *UpdateTerminatorRequestContext) {
	if !ctx.loadRouter() {
		return
	}

	ctx.loadSession(ctx.req.SessionToken)
	ctx.checkSessionType(persistence.SessionTypeBind)
	ctx.checkSessionFingerprints(ctx.req.Fingerprints)
	terminator := ctx.verifyTerminator(ctx.req.TerminatorId)

	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	request := ctx.req

	if !request.UpdateCost && !request.UpdatePrecedence {
		// nothing to do
		self.sendResponse(ctx)
		return
	}

	checker := boltz.MapFieldChecker{}

	if request.UpdateCost {
		if request.Cost > math.MaxUint16 {
			self.returnError(ctx, errors.Errorf("invalid cost %v. cost must be between 0 and %v inclusive", ctx.req.Cost, math.MaxUint16))
			return
		}
		terminator.Cost = uint16(request.Cost)
		checker[db.FieldTerminatorCost] = struct{}{}
	}

	if request.UpdatePrecedence {
		if request.UpdatePrecedence {
			if request.Precedence == edge_ctrl_pb.TerminatorPrecedence_Default {
				terminator.Precedence = xt.Precedences.Default
			} else if request.Precedence == edge_ctrl_pb.TerminatorPrecedence_Required {
				terminator.Precedence = xt.Precedences.Required
			} else if request.Precedence == edge_ctrl_pb.TerminatorPrecedence_Failed {
				terminator.Precedence = xt.Precedences.Failed
			} else {
				self.returnError(ctx, errors.Errorf("invalid precedence: %v", request.Precedence))
				return
			}
		}
		checker[db.FieldTerminatorPrecedence] = struct{}{}
	}

	if err := self.getNetwork().Terminators.Patch(terminator, checker); err != nil {
		self.returnError(ctx, err)
		return
	}

	logrus.
		WithField("token", ctx.req.SessionToken).
		WithField("terminator", ctx.req.TerminatorId).
		Info("updated terminator")

	self.sendResponse(ctx)
}

type UpdateTerminatorRequestContext struct {
	baseRequestContext
	req *edge_ctrl_pb.UpdateTerminatorRequest
}

func (self *UpdateTerminatorRequestContext) GetSessionToken() string {
	return self.req.SessionToken
}
