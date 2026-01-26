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
	"fmt"
	"math"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type createTunnelTerminatorV2Handler struct {
	baseRequestHandler
	*TunnelState
}

func NewCreateTunnelTerminatorV2Handler(appEnv *env.AppEnv, ch channel.Channel) channel.TypedReceiveHandler {
	return &createTunnelTerminatorV2Handler{
		baseRequestHandler: baseRequestHandler{ch: ch, appEnv: appEnv},
	}
}

func (self *createTunnelTerminatorV2Handler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_CreateTunnelTerminatorRequestV2Type)
}

func (self *createTunnelTerminatorV2Handler) Label() string {
	return "tunnel.create.terminator.v2"
}

func (self *createTunnelTerminatorV2Handler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	startTime := time.Now()
	req := &edge_ctrl_pb.CreateTunnelTerminatorRequestV2{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal CreateTunnelTerminatorRequestV2")
		return
	}

	ctx := &createTunnelTerminatorV2RequestContext{
		baseTunnelRequestContext: baseTunnelRequestContext{
			baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg, env: self.appEnv},
		},
		req: req,
	}

	go self.CreateTerminator(ctx, startTime)
}

func (self *createTunnelTerminatorV2Handler) returnError(ctx *createTunnelTerminatorV2RequestContext, err controllerError, logger *logrus.Entry) {
	response := &edge_ctrl_pb.CreateTunnelTerminatorResponseV2{
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

func (self *createTunnelTerminatorV2Handler) CreateTerminator(ctx *createTunnelTerminatorV2RequestContext, startTime time.Time) {
	logger := logrus.
		WithField("routerId", self.ch.Id()).
		WithField("terminatorId", ctx.req.Address)

	if !ctx.loadRouter() {
		return
	}
	ctx.verifyTerminatorId(ctx.req.Address)
	ctx.loadIdentity()
	ctx.loadServiceForId(ctx.req.ServiceId)
	ctx.verifyEdgeRouterServiceBindAccess()
	ctx.verifyRouterEdgeRouterAccess()

	if ctx.err != nil {
		self.returnError(ctx, ctx.err, logger)
		return
	}

	logger = logger.WithField("serviceId", ctx.req.ServiceId).WithField("service", ctx.service.Name)

	if ctx.req.Cost > math.MaxUint16 {
		self.returnError(ctx,
			invalidCost(fmt.Sprintf("invalid cost %v. cost must be between 0 and %v inclusive", ctx.req.Cost, math.MaxUint16)),
			logger)
		return
	}

	terminator, _ := self.getNetwork().Terminator.Read(ctx.req.Address)
	if terminator != nil && self.getNetwork().Dispatcher.IsLeader() {
		if err := ctx.validateExistingTerminator(terminator, ctx.identity.Id, common.TunnelBinding, logger); err != nil {
			self.returnError(ctx, err, logger)
			return
		}
	} else {
		terminator = &model.Terminator{
			BaseEntity: models.BaseEntity{
				Id:       ctx.req.Address,
				IsSystem: true,
			},
			Service:        ctx.req.ServiceId,
			Router:         ctx.sourceRouter.Id,
			Binding:        common.TunnelBinding,
			Address:        ctx.req.Address,
			InstanceId:     ctx.req.InstanceId,
			InstanceSecret: ctx.req.InstanceSecret,
			PeerData:       ctx.req.PeerData,
			Precedence:     ctx.req.GetXtPrecedence(),
			Cost:           uint16(ctx.req.Cost),
			HostId:         ctx.identity.Id,
			SourceCtrl:     self.appEnv.GetId(),
		}

		if err := self.appEnv.Managers.Terminator.Create(terminator, ctx.newTunnelChangeContext()); err != nil {
			// terminator might have been created while we were trying to create.
			if terminator, _ = self.getNetwork().Terminator.Read(ctx.req.Address); terminator != nil {
				if validateError := ctx.validateExistingTerminator(terminator, ctx.identity.Id, common.TunnelBinding, logger); validateError != nil {
					self.returnError(ctx, validateError, logger)
					return
				}
			} else {
				if command.WasRateLimited(err) {
					self.returnError(ctx, busyError(err), logger)
					return
				}
				self.returnError(ctx, internalError(err), logger)
				return
			}
		} else {
			logger.Info("created terminator")
		}
	}

	response := &edge_ctrl_pb.CreateTunnelTerminatorResponseV2{
		TerminatorId: ctx.req.Address,
		StartTime:    ctx.req.StartTime,
		Result:       edge_ctrl_pb.CreateTerminatorResult_Success,
	}

	body, err := proto.Marshal(response)
	if err != nil {
		logger.WithError(err).Error("failed to marshal CreateTunnelTerminatorResponseV2")
		return
	}

	responseMsg := channel.NewMessage(response.GetContentType(), body)
	responseMsg.ReplyTo(ctx.msg)
	if err = self.ch.Send(responseMsg); err != nil {
		logger.WithError(err).Error("failed to send CreateTunnelTerminatorResponseV2")
	}

	logger.WithField("elapsedTime", time.Since(startTime)).Info("completed create tunnel terminator operation")
}

type createTunnelTerminatorV2RequestContext struct {
	baseTunnelRequestContext
	req *edge_ctrl_pb.CreateTunnelTerminatorRequestV2
}
