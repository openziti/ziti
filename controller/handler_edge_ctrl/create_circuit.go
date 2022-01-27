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

type createCircuitHandler struct {
	baseRequestHandler
}

func NewCreateCircuitHandler(appEnv *env.AppEnv, ch channel.Channel) channel.TypedReceiveHandler {
	return &createCircuitHandler{
		baseRequestHandler: baseRequestHandler{
			ch:     ch,
			appEnv: appEnv,
		},
	}
}

func (self *createCircuitHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_CreateCircuitRequestType)
}

func (self *createCircuitHandler) Label() string {
	return "create.circuit"
}

func (self *createCircuitHandler) sendResponse(ctx *CreateCircuitRequestContext, response *edge_ctrl_pb.CreateCircuitResponse) {
	log := pfxlog.ContextLogger(self.ch.Label())

	body, err := proto.Marshal(response)
	if err != nil {
		log.WithError(err).WithField("token", ctx.req.SessionToken).Error("failed to marshal create circuit response")
		return
	}

	responseMsg := channel.NewMessage(response.GetContentType(), body)
	responseMsg.ReplyTo(ctx.msg)
	if err = self.ch.Send(responseMsg); err != nil {
		log.WithError(err).WithField("token", ctx.req.SessionToken).Error("failed to send create circuit response")
	}
}

func (self *createCircuitHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	req := &edge_ctrl_pb.CreateCircuitRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal CreateCircuitRequest")
		return
	}

	ctx := &CreateCircuitRequestContext{
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg},
		req:                       req,
	}

	go self.CreateCircuit(ctx)
}

func (self *createCircuitHandler) CreateCircuit(ctx *CreateCircuitRequestContext) {
	if !ctx.loadRouter() {
		return
	}
	ctx.loadSession(ctx.req.SessionToken)
	ctx.checkSessionType(persistence.SessionTypeDial)
	ctx.checkSessionFingerprints(ctx.req.Fingerprints)
	ctx.verifyEdgeRouterAccess()
	ctx.loadService()
	circuitInfo, peerData := ctx.createCircuit(ctx.req.TerminatorIdentity, ctx.req.PeerData)

	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	log := pfxlog.ContextLogger(self.ch.Label()).WithField("token", ctx.req.SessionToken)

	response := &edge_ctrl_pb.CreateCircuitResponse{
		CircuitId: circuitInfo.Id,
		Address:   circuitInfo.Path.IngressId,
		PeerData:  peerData,
	}

	log.Debugf("responding with successful circuit setup")
	self.sendResponse(ctx, response)
}

type CreateCircuitRequestContext struct {
	baseSessionRequestContext
	req *edge_ctrl_pb.CreateCircuitRequest
}

func (self *CreateCircuitRequestContext) GetSessionToken() string {
	return self.req.SessionToken
}
