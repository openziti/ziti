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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"math"
	"time"
)

type createTunnelTerminatorHandler struct {
	baseRequestHandler
	*TunnelState
}

func NewCreateTunnelTerminatorHandler(appEnv *env.AppEnv, ch channel.Channel, tunnelState *TunnelState) channel.TypedReceiveHandler {
	return &createTunnelTerminatorHandler{
		baseRequestHandler: baseRequestHandler{ch: ch, appEnv: appEnv},
		TunnelState:        tunnelState,
	}
}

func (self *createTunnelTerminatorHandler) getTunnelState() *TunnelState {
	return self.TunnelState
}

func (self *createTunnelTerminatorHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_CreateTunnelTerminatorRequestType)
}

func (self *createTunnelTerminatorHandler) Label() string {
	return "tunnel.create.terminator"
}

func (self *createTunnelTerminatorHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	startTime := time.Now()

	req := &edge_ctrl_pb.CreateTunnelTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal CreateTerminatorRequest")
		return
	}

	ctx := &CreateTunnelTerminatorRequestContext{
		baseTunnelRequestContext: baseTunnelRequestContext{
			baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg},
			apiSession:                nil,
			identity:                  nil,
		},
		req: req,
	}

	go self.CreateTerminator(ctx, startTime)
}

func (self *createTunnelTerminatorHandler) CreateTerminator(ctx *CreateTunnelTerminatorRequestContext, startTime time.Time) {
	logger := logrus.
		WithField("routerId", self.ch.Id()).
		WithField("terminatorId", ctx.req.Address)

	if !ctx.loadRouter() {
		return
	}
	ctx.loadIdentity()
	newApiSession := ctx.ensureApiSession(nil)
	ctx.loadServiceForName(ctx.req.ServiceName)
	ctx.ensureSessionForService(ctx.req.SessionId, db.SessionTypeBind)
	ctx.verifyEdgeRouterAccess()

	if ctx.err != nil {
		self.logResult(ctx, ctx.err)
		return
	}

	logger = logger.WithField("serviceId", ctx.service.Id).WithField("service", ctx.service.Name)

	if ctx.req.Cost > math.MaxUint16 {
		self.returnError(ctx, invalidCost(fmt.Sprintf("invalid cost %v. cost must be between 0 and %v inclusive", ctx.req.Cost, math.MaxUint16)))
		return
	}

	terminator, _ := self.getNetwork().Terminators.Read(ctx.req.Address)
	if terminator != nil {
		if err := ctx.validateExistingTerminator(terminator, logger); err != nil {
			self.returnError(ctx, err)
			return
		}
	} else {
		terminator = &network.Terminator{
			BaseEntity: models.BaseEntity{
				Id:       ctx.req.Address,
				IsSystem: true,
			},
			Service:        ctx.session.ServiceId,
			Router:         ctx.sourceRouter.Id,
			Binding:        common.TunnelBinding,
			Address:        ctx.req.Address,
			InstanceId:     ctx.req.InstanceId,
			InstanceSecret: ctx.req.InstanceSecret,
			PeerData:       ctx.req.PeerData,
			Precedence:     ctx.req.GetXtPrecedence(),
			Cost:           uint16(ctx.req.Cost),
			HostId:         ctx.session.IdentityId,
		}

		n := self.appEnv.GetHostController().GetNetwork()
		if err := n.Terminators.Create(terminator, ctx.newTunnelChangeContext()); err != nil {
			// terminator might have been created while we were trying to create.
			if terminator, _ = self.getNetwork().Terminators.Read(ctx.req.Address); terminator != nil {
				if validateError := ctx.validateExistingTerminator(terminator, logger); validateError != nil {
					self.returnError(ctx, validateError)
					return
				}
			} else {
				self.returnError(ctx, internalError(err))
				return
			}
		} else {
			logger.Info("created terminator")
		}
	}

	response := &edge_ctrl_pb.CreateTunnelTerminatorResponse{
		Session:      ctx.getCreateSessionResponse(),
		TerminatorId: ctx.req.Address,
		StartTime:    ctx.req.StartTime,
	}

	if newApiSession {
		var err error
		response.ApiSession, err = ctx.getCreateApiSessionResponse()
		if err != nil {
			self.returnError(ctx, internalError(err))
			return
		}
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

	logger.WithField("elapsedTime", time.Since(startTime)).Info("completed create tunnel terminator operation")
}

type CreateTunnelTerminatorRequestContext struct {
	baseTunnelRequestContext
	req *edge_ctrl_pb.CreateTunnelTerminatorRequest
}

func (self *CreateTunnelTerminatorRequestContext) validateExistingTerminator(terminator *network.Terminator, log *logrus.Entry) controllerError {
	if terminator.Binding != common.TunnelBinding {
		log.WithField("binding", common.TunnelBinding).
			WithField("conflictingBinding", terminator.Binding).
			Error("selected terminator address conflicts with a terminator for a different binding")
		return internalError(errors.New("selected id conflicts with terminator for different binding"))
	}

	if terminator.Service != self.session.ServiceId {
		log.WithField("conflictingService", terminator.Service).
			Error("selected terminator address conflicts with a terminator for a different service")
		return internalError(errors.New("selected id conflicts with terminator for different service"))
	}

	if terminator.Router != self.sourceRouter.Id {
		log.WithField("conflictingRouter", terminator.Router).
			Error("selected terminator address conflicts with a terminator for a different router")
		return internalError(errors.New("selected id conflicts with terminator for different router"))
	}

	if terminator.HostId != self.session.IdentityId {
		log.WithField("identityId", self.session.IdentityId).
			WithField("conflictingIdentity", terminator.HostId).
			Error("selected terminator address conflicts with a terminator for a different identity")
		return internalError(errors.New("selected id conflicts with terminator for different identity"))
	}

	log.Info("terminator already exists")
	return nil
}
