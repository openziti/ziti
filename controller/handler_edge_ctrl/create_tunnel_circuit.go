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
	"github.com/openziti/edge/pb/edge_ctrl_pb"
)

type createCircuitForServiceHandler struct {
	baseRequestHandler
	*TunnelState
}

func NewCreateCircuitForTunnelHandler(appEnv *env.AppEnv, ch channel.Channel, tunnelState *TunnelState) channel.TypedReceiveHandler {
	return &createCircuitForServiceHandler{
		baseRequestHandler: baseRequestHandler{ch: ch, appEnv: appEnv},
		TunnelState:        tunnelState,
	}
}

func (self *createCircuitForServiceHandler) getTunnelState() *TunnelState {
	return self.TunnelState
}

func (self *createCircuitForServiceHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_CreateCircuitForServiceRequestType)
}

func (self *createCircuitForServiceHandler) Label() string {
	return "tunnel.create.circuit"
}

func (self *createCircuitForServiceHandler) sendResponse(ctx *CreateCircuitForServiceRequestContext, response *edge_ctrl_pb.CreateCircuitForServiceResponse) {
	log := pfxlog.ContextLogger(self.ch.Label())

	body, err := proto.Marshal(response)
	if err != nil {
		log.WithError(err).WithField("service", ctx.req.ServiceName).Error("failed to marshal CreateCircuitForServiceResponse")
		return
	}

	responseMsg := channel.NewMessage(response.GetContentType(), body)
	responseMsg.ReplyTo(ctx.msg)
	if err = self.ch.Send(responseMsg); err != nil {
		log.WithError(err).WithField("service", ctx.req.ServiceName).Error("failed to send CreateCircuitForServiceResponse")
	}
}

func (self *createCircuitForServiceHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	req := &edge_ctrl_pb.CreateCircuitForServiceRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal CreateCircuitForServiceRequest")
		return
	}

	ctx := &CreateCircuitForServiceRequestContext{
		baseTunnelRequestContext: baseTunnelRequestContext{
			baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg},
		},
		req: req,
	}

	go self.CreateCircuit(ctx)
}

func (self *createCircuitForServiceHandler) CreateCircuit(ctx *CreateCircuitForServiceRequestContext) {
	if !ctx.loadRouter() {
		return
	}
	ctx.loadIdentity()
	newApiSession := ctx.ensureApiSession(nil)
	ctx.loadServiceForName(ctx.req.ServiceName)
	ctx.ensureSessionForService(ctx.req.SessionId, persistence.SessionTypeDial)
	ctx.verifyEdgeRouterAccess()
	circuitInfo, peerData := ctx.createCircuit(ctx.req.TerminatorIdentity, ctx.req.PeerData)

	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	response := &edge_ctrl_pb.CreateCircuitForServiceResponse{
		Session:   ctx.getCreateSessionResponse(),
		CircuitId: circuitInfo.Id,
		Address:   circuitInfo.Path.IngressId,
		PeerData:  peerData,
	}

	if newApiSession {
		var err error
		response.ApiSession, err = ctx.getCreateApiSessionResponse()
		if err != nil {
			self.returnError(ctx, internalError(err))
			return
		}
	}

	self.sendResponse(ctx, response)
}

type CreateCircuitForServiceRequestContext struct {
	baseTunnelRequestContext
	req *edge_ctrl_pb.CreateCircuitForServiceRequest
}
