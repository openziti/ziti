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
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"google.golang.org/protobuf/proto"
)

type createTunnelCircuitV2Handler struct {
	baseRequestHandler
}

func NewCreateTunnelCircuitV2Handler(appEnv *env.AppEnv, ch channel.Channel) channel.TypedReceiveHandler {
	return &createTunnelCircuitV2Handler{
		baseRequestHandler: baseRequestHandler{ch: ch, appEnv: appEnv},
	}
}

func (self *createTunnelCircuitV2Handler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_CreateTunnelCircuitV2RequestType)
}

func (self *createTunnelCircuitV2Handler) Label() string {
	return "tunnel.create.circuit.v2"
}

func (self *createTunnelCircuitV2Handler) sendResponse(ctx *CreateTunnelCircuitV2RequestContext, response *edge_ctrl_pb.CreateTunnelCircuitV2Response) {
	log := pfxlog.ContextLogger(self.ch.Label())

	body, err := proto.Marshal(response)
	if err != nil {
		log.WithError(err).WithField("service", ctx.req.ServiceName).Error("failed to marshal CreateTunnelCircuitV2Response")
		return
	}

	responseMsg := channel.NewMessage(response.GetContentType(), body)
	responseMsg.ReplyTo(ctx.msg)
	if err = self.ch.Send(responseMsg); err != nil {
		log.WithError(err).WithField("service", ctx.req.ServiceName).Error("failed to send CreateTunnelCircuitV2Response")
	}
}

func (self *createTunnelCircuitV2Handler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	req := &edge_ctrl_pb.CreateTunnelCircuitV2Request{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal CreateTunnelCircuitV2Request")
		return
	}

	ctx := &CreateTunnelCircuitV2RequestContext{
		baseTunnelRequestContext: baseTunnelRequestContext{
			baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg, env: self.appEnv},
		},
		req: req,
	}

	go self.CreateCircuit(ctx)
}

func (self *createTunnelCircuitV2Handler) CreateCircuit(ctx *CreateTunnelCircuitV2RequestContext) {
	if !ctx.loadRouter() {
		return
	}
	ctx.loadIdentity()
	ctx.loadServiceForName(ctx.req.ServiceName)
	ctx.verifyRouterEdgeRouterAccess()
	circuitInfo, peerData := ctx.createCircuit(ctx.req.TerminatorInstanceId, ctx.req.PeerData, ctx.newTunnelCircuitCreateParms)

	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	response := &edge_ctrl_pb.CreateTunnelCircuitV2Response{
		CircuitId: circuitInfo.Id,
		Address:   circuitInfo.Path.IngressId,
		PeerData:  peerData,
		Tags:      circuitInfo.Tags,
	}

	self.sendResponse(ctx, response)
}

type CreateTunnelCircuitV2RequestContext struct {
	baseTunnelRequestContext
	req *edge_ctrl_pb.CreateTunnelCircuitV2Request
}
