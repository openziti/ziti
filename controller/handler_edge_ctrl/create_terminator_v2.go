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
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"math"
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
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg},
		req:                       req,
	}

	go self.CreateTerminatorV2(ctx)
}

func (self *createTerminatorV2Handler) CreateTerminatorV2(ctx *CreateTerminatorV2RequestContext) {
	logger := pfxlog.ContextLogger(self.ch.Label()).
		WithField("routerId", self.ch.Id()).
		WithField("token", ctx.req.SessionToken).
		WithField("terminatorId", ctx.req.Address)

	if !ctx.loadRouter() {
		return
	}
	ctx.verifyTerminatorId(ctx.req.Address)
	ctx.loadSession(ctx.req.SessionToken)
	ctx.checkSessionType(db.SessionTypeBind)
	ctx.checkSessionFingerprints(ctx.req.Fingerprints)
	ctx.verifyEdgeRouterAccess()
	ctx.loadService()

	if ctx.err != nil {
		self.returnError(ctx, edge_ctrl_pb.CreateTerminatorResult_FailedOther, ctx.err, logger)
		return
	}

	logger = logger.WithField("serviceId", ctx.service.Id).WithField("service", ctx.service.Name)

	if ctx.req.Cost > math.MaxUint16 {
		ctx.err = invalidCost(fmt.Sprintf("invalid cost %v. cost must be between 0 and %v inclusive", ctx.req.Cost, math.MaxUint16))
		self.returnError(ctx, edge_ctrl_pb.CreateTerminatorResult_FailedOther, ctx.err, logger)
		return
	}

	terminator, _ := self.getNetwork().Terminators.Read(ctx.req.Address)
	if terminator != nil {
		if ctx.err = ctx.validateExistingTerminator(terminator, logger); ctx.err != nil {
			self.returnError(ctx, edge_ctrl_pb.CreateTerminatorResult_FailedIdConflict, ctx.err, logger)
			return
		}

		// if the precedence or cost has changed, update the terminator
		if terminator.Precedence != ctx.req.GetXtPrecedence() || terminator.Cost != uint16(ctx.req.Cost) {
			terminator.Precedence = ctx.req.GetXtPrecedence()
			terminator.Cost = uint16(ctx.req.Cost)
			err := self.appEnv.GetHostController().GetNetwork().Terminators.Update(terminator, fields.UpdatedFieldsMap{
				db.FieldTerminatorPrecedence: struct{}{},
				db.FieldTerminatorCost:       struct{}{},
			}, ctx.newChangeContext())

			if err != nil {
				self.returnError(ctx, edge_ctrl_pb.CreateTerminatorResult_FailedOther, err, logger)
				return
			}
		}
	} else {
		terminator = &network.Terminator{
			BaseEntity: models.BaseEntity{
				Id:       ctx.req.Address,
				IsSystem: true,
			},
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
		}

		cmd := &model.CreateEdgeTerminatorCmd{
			Env:     self.appEnv,
			Entity:  terminator,
			Context: ctx.newChangeContext(),
		}

		if err := self.appEnv.GetHostController().GetNetwork().Managers.Command.Dispatch(cmd); err != nil {
			// terminator might have been created while we were trying to create.
			if terminator, _ = self.getNetwork().Terminators.Read(ctx.req.Address); terminator != nil {
				if validateError := ctx.validateExistingTerminator(terminator, logger); validateError != nil {
					self.returnError(ctx, edge_ctrl_pb.CreateTerminatorResult_FailedIdConflict, validateError, logger)
					return
				}
			} else {
				self.returnError(ctx, edge_ctrl_pb.CreateTerminatorResult_FailedOther, err, logger)
				return
			}
		} else {
			logger.WithField("terminator", terminator.Id).Info("created terminator")
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

	logger.Info("completed create terminator v2 operation")
}

func (self *createTerminatorV2Handler) returnError(ctx *CreateTerminatorV2RequestContext, resultType edge_ctrl_pb.CreateTerminatorResult, err error, logger *logrus.Entry) {
	response := &edge_ctrl_pb.CreateTerminatorV2Response{
		TerminatorId: ctx.req.Address,
		Result:       resultType,
		Msg:          err.Error(),
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

func (self *CreateTerminatorV2RequestContext) GetSessionToken() string {
	return self.req.SessionToken
}

func (self *CreateTerminatorV2RequestContext) validateExistingTerminator(terminator *network.Terminator, log *logrus.Entry) controllerError {
	if terminator.Binding != common.EdgeBinding {
		log.WithField("binding", common.EdgeBinding).
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
