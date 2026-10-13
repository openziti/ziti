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
	"bytes"
	"fmt"
	"maps"
	"math"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/fields"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type createTerminatorV2Handler struct {
	baseRequestHandler
}

func NewCreateTerminatorV2Handler(appEnv *env.AppEnv, ch channel.Channel) channel.TypedReceiveHandler {
	return &createTerminatorV2Handler{
		baseRequestHandler{
			ch:     ch,
			appEnv: appEnv,
		},
	}
}

func (self *createTerminatorV2Handler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_CreateTerminatorV2RequestType)
}

func (self *createTerminatorV2Handler) Label() string {
	return "create.terminator"
}

func (self *createTerminatorV2Handler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	req := &edge_ctrl_pb.CreateTerminatorV2Request{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal CreateTerminatorV2Request")
		return
	}

	ctx := &CreateTerminatorV2RequestContext{
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg, env: self.appEnv},
		req:                       req,
	}

	go self.CreateTerminatorV2(ctx)
}

func (self *createTerminatorV2Handler) CreateTerminatorV2(ctx *CreateTerminatorV2RequestContext) {
	start := time.Now()
	logger := pfxlog.ContextLogger(self.ch.Label()).
		WithField("routerId", self.ch.Id()).
		WithField("terminatorId", ctx.req.Address)

	if !ctx.loadRouter() {
		return
	}
	ctx.verifyTerminatorId(ctx.req.Address)
	ctx.loadSession(ctx.req.SessionToken, ctx.req.ApiSessionToken)
	ctx.checkSessionType(db.SessionTypeBind)
	ctx.verifyIdentityEdgeRouterAccess()
	ctx.loadService()

	if ctx.err != nil {
		self.returnError(ctx, ctx.err, logger)
		return
	}

	logger = logger.WithField("serviceId", ctx.service.Id).WithField("service", ctx.service.Name)

	if ctx.req.Cost > math.MaxUint16 {
		ctx.err = invalidCost(fmt.Sprintf("invalid cost %v. cost must be between 0 and %v inclusive", ctx.req.Cost, math.MaxUint16))
		self.returnError(ctx, ctx.err, logger)
		return
	}

	terminator, _ := self.getNetwork().Terminator.Read(ctx.req.Address)
	if terminator != nil {
		if !self.updateExisting(ctx, terminator, logger) {
			return
		}
	} else {
		terminator = &model.Terminator{
			Id:             ctx.req.Address,
			IsSystem:       true,
			Service:        ctx.session.ServiceId,
			Router:         ctx.sourceRouter.Id,
			Binding:        common.EdgeBinding,
			Address:        ctx.req.Address,
			InstanceId:     ctx.req.InstanceId,
			InstanceSecret: ctx.req.InstanceSecret,
			PeerData:       ctx.req.PeerData,
			Precedence:     ctx.req.GetXtPrecedence(),
			Cost:           uint16(ctx.req.Cost),
			HostId:         ctx.session.IdentityId,
			SourceCtrl:     self.appEnv.GetId(),
		}

		cmd := &model.CreateEdgeTerminatorCmd{
			Env:     self.appEnv,
			Entity:  terminator,
			Context: ctx.newChangeContext(),
		}

		createStart := time.Now()
		if err := self.appEnv.GetHostController().GetNetwork().Managers.Dispatcher.Dispatch(cmd); err != nil {
			// terminator might have been created while we were trying to create.
			if terminator, _ = self.getNetwork().Terminator.Read(ctx.req.Address); terminator != nil {
				if !self.updateExisting(ctx, terminator, logger) {
					return
				}
			} else {
				if command.WasRateLimited(err) || command.WasLeaderless(err) {
					self.returnError(ctx, busyError(err), logger)
					return
				}
				self.returnError(ctx, internalError(err), logger)
				return
			}
		} else {
			logger.WithField("terminator", terminator.Id).
				WithField("createTime", time.Since(createStart)).
				Info("created terminator")
		}
	}

	response := &edge_ctrl_pb.CreateTerminatorV2Response{
		TerminatorId: terminator.Id,
		Result:       edge_ctrl_pb.CreateTerminatorResult_Success,
	}

	body, err := proto.Marshal(response)
	if err != nil {
		logger.WithError(err).Error("failed to marshal CreateTunnelTerminatorResponse")
		return
	}

	responseMsg := channel.NewMessage(response.GetContentType(), body)
	responseMsg.ReplyTo(ctx.msg)
	if err = self.ch.Send(responseMsg); err != nil {
		logger.WithError(err).Error("failed to send CreateTunnelTerminatorResponse")
	}

	logger.WithField("elapsed", time.Since(start)).Info("completed create terminator v2 operation")
}

func (self *createTerminatorV2Handler) returnError(ctx *CreateTerminatorV2RequestContext, err controllerError, logger *logrus.Entry) {
	response := &edge_ctrl_pb.CreateTerminatorV2Response{
		TerminatorId: ctx.req.Address,
		Result:       retryHintToResult(err.GetRetryHint()),
		Msg:          err.Error(),
		ErrorCode:    err.ErrorCode(),
		RetryHint:    uint32(err.GetRetryHint()),
	}

	if sendErr := protobufs.MarshalTyped(response).ReplyTo(ctx.msg).Send(self.ch); sendErr != nil {
		logger.WithError(err).WithField("sendError", sendErr).Error("failed to send error response")
	} else {
		logger.WithError(err).Error("responded with error")
	}
}

type CreateTerminatorV2RequestContext struct {
	baseSessionRequestContext
	req *edge_ctrl_pb.CreateTerminatorV2Request
}

// updateExisting validates that terminator can be adopted by the request and applies the request to it
// with applyToExisting. It reports whether the caller can reply with success; on false the error reply
// has been sent.
func (self *createTerminatorV2Handler) updateExisting(ctx *CreateTerminatorV2RequestContext, terminator *model.Terminator, logger *logrus.Entry) bool {
	if ctx.err = ctx.validateExistingTerminator(terminator, ctx.session.IdentityId, common.EdgeBinding, logger); ctx.err != nil {
		self.returnError(ctx, ctx.err, logger)
		return false
	}

	if err := ctx.applyToExisting(terminator, ctx.newChangeContext()); err != nil {
		// A rate-limited or leaderless dispatch is transient; reply busy so the router requeues
		// promptly instead of treating it as a hard failure.
		if command.WasRateLimited(err) || command.WasLeaderless(err) {
			self.returnError(ctx, busyError(err), logger)
			return false
		}
		self.returnError(ctx, internalError(err), logger)
		return false
	}
	return true
}

// applyToExisting persists the request's cost, precedence and peer data to terminator. The write is
// skipped when terminator already has them and this controller is a caught-up leader or not clustered.
// The cost must already be validated.
func (self *CreateTerminatorV2RequestContext) applyToExisting(terminator *model.Terminator, ctx *change.Context) error {
	precedence := self.req.GetXtPrecedence()
	cost := uint16(self.req.Cost)

	// Not IsLeader: a leader mid-catch-up can hold a value the cluster has since replaced. A leadership
	// change around the read can still skip a needed write; that window is accepted. Peer data can't hit
	// it: sdk keys are random, so a terminator never goes back to an earlier key.
	if self.handler.getNetwork().Dispatcher.IsLeaderAndCaughtUp() &&
		terminator.Precedence == precedence && terminator.Cost == cost &&
		maps.EqualFunc(terminator.PeerData, self.req.PeerData, bytes.Equal) {
		return nil
	}

	terminator.Precedence = precedence
	terminator.Cost = cost
	terminator.PeerData = self.req.PeerData
	return self.handler.getNetwork().Terminator.Update(terminator, fields.UpdatedFieldsMap{
		db.FieldTerminatorPrecedence: struct{}{},
		db.FieldTerminatorCost:       struct{}{},
		db.FieldServerPeerData:       struct{}{},
	}, ctx)
}

func (self *CreateTerminatorV2RequestContext) GetSessionToken() string {
	return self.req.SessionToken
}
