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
	"fmt"
	"google.golang.org/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/edge_common"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/controller/network"
	"github.com/sirupsen/logrus"
	"math"
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

	go self.CreateTerminator(ctx)
}

func (self *createTunnelTerminatorHandler) CreateTerminator(ctx *CreateTunnelTerminatorRequestContext) {
	logger := logrus.WithField("routerId", self.ch.Id().Token)

	if !ctx.loadRouter() {
		return
	}
	ctx.loadIdentity()
	newApiSession := ctx.ensureApiSession(nil)
	ctx.loadServiceForName(ctx.req.ServiceName)
	ctx.ensureSessionForService(ctx.req.SessionId, persistence.SessionTypeBind)
	ctx.verifyEdgeRouterAccess()

	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	logger = logger.WithField("serviceId", ctx.service.Id).WithField("service", ctx.service.Name)

	if ctx.req.Cost > math.MaxUint16 {
		self.returnError(ctx, invalidCost(fmt.Sprintf("invalid cost %v. cost must be between 0 and %v inclusive", ctx.req.Cost, math.MaxUint16)))
		return
	}

	terminator := &network.Terminator{
		Service:        ctx.session.ServiceId,
		Router:         ctx.sourceRouter.Id,
		Binding:        edge_common.TunnelBinding,
		Address:        ctx.req.Address,
		Identity:       ctx.req.Identity,
		IdentitySecret: ctx.req.IdentitySecret,
		PeerData:       ctx.req.PeerData,
		Precedence:     ctx.req.GetXtPrecedence(),
		Cost:           uint16(ctx.req.Cost),
	}

	n := self.appEnv.GetHostController().GetNetwork()
	err := n.Terminators.Create(terminator)
	if err != nil {
		self.returnError(ctx, internalError(err))
		return
	}
	id := terminator.Id

	logger = logger.WithField("terminator", id)
	logger.Info("created terminator")

	response := &edge_ctrl_pb.CreateTunnelTerminatorResponse{
		Session:      ctx.getCreateSessionResponse(),
		TerminatorId: id,
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
}

type CreateTunnelTerminatorRequestContext struct {
	baseTunnelRequestContext
	req *edge_ctrl_pb.CreateTunnelTerminatorRequest
}
