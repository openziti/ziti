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
	"github.com/openziti/channel/v3"
	"github.com/openziti/ziti/common/ctrl_msg"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"google.golang.org/protobuf/proto"
)

type createCircuitHandler struct {
	baseRequestHandler
}

func NewCreateCircuitHandler(appEnv *env.AppEnv, ch channel.Channel) channel.TypedReceiveHandler {
	handler := &createCircuitHandler{
		baseRequestHandler: baseRequestHandler{
			ch:     ch,
			appEnv: appEnv,
		},
	}
	return &channel.AsyncFunctionReceiveAdapter{
		Type:    int32(edge_ctrl_pb.ContentType_CreateCircuitRequestType),
		Handler: handler.HandleReceiveCreateCircuitV1,
	}
}

func NewCreateCircuitV2Handler(appEnv *env.AppEnv, ch channel.Channel) channel.TypedReceiveHandler {
	handler := &createCircuitHandler{
		baseRequestHandler: baseRequestHandler{
			ch:     ch,
			appEnv: appEnv,
		},
	}
	return &channel.AsyncFunctionReceiveAdapter{
		Type:    int32(edge_ctrl_pb.ContentType_CreateCircuitV2RequestType),
		Handler: handler.HandleReceiveCreateCircuitV2,
	}
}

func (self *createCircuitHandler) Label() string {
	return "create.circuit"
}

func (self *createCircuitHandler) HandleReceiveCreateCircuitV1(msg *channel.Message, ch channel.Channel) {
	req := &edge_ctrl_pb.CreateCircuitRequest{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not unmarshal CreateCircuitRequest")
		return
	}

	ctx := &CreateCircuitRequestContext{
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg},
		req:                       req,
	}

	self.CreateCircuit(ctx, self.CreateCircuitV1Response)
}

func (self *createCircuitHandler) CreateCircuitV1Response(circuitInfo *model.Circuit, peerData map[uint32][]byte) (*channel.Message, error) {
	response := &edge_ctrl_pb.CreateCircuitResponse{
		CircuitId: circuitInfo.Id,
		Address:   circuitInfo.Path.IngressId,
		PeerData:  peerData,
		Tags:      circuitInfo.Tags,
	}

	body, err := proto.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal protobuf create circuit response (%w)", err)
	}
	responseMsg := channel.NewMessage(response.GetContentType(), body)
	return responseMsg, nil
}

func (self *createCircuitHandler) HandleReceiveCreateCircuitV2(msg *channel.Message, ch channel.Channel) {
	req, err := ctrl_msg.DecodeCreateCircuitRequest(msg)
	if err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not decode CreateCircuitRequest")
		return
	}

	ctx := &CreateCircuitRequestContext{
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg, env: self.appEnv},
		req:                       req,
	}

	self.CreateCircuit(ctx, self.CreateCircuitV2Response)
}

func (self *createCircuitHandler) CreateCircuitV2Response(circuitInfo *model.Circuit, peerData map[uint32][]byte) (*channel.Message, error) {
	response := &ctrl_msg.CreateCircuitResponse{
		CircuitId: circuitInfo.Id,
		Address:   circuitInfo.Path.IngressId,
		PeerData:  peerData,
		Tags:      circuitInfo.Tags,
	}

	return response.ToMessage(), nil
}

func (self *createCircuitHandler) CreateCircuit(ctx *CreateCircuitRequestContext, f createCircuitResponseFactory) {
	if !ctx.loadRouter() {
		return
	}
	ctx.loadSession(ctx.req.GetSessionToken(), ctx.req.GetApiSessionToken())
	ctx.checkSessionType(db.SessionTypeDial)
	ctx.verifyIdentityEdgeRouterAccess()
	ctx.loadService()
	circuitInfo, peerData := ctx.createCircuit(ctx.req.GetTerminatorInstanceId(), ctx.req.GetPeerData(), ctx.newCircuitCreateParms)

	if ctx.err != nil {
		self.returnError(ctx, ctx.err)
		return
	}

	log := pfxlog.ContextLogger(self.ch.Label()).WithField("token", ctx.req.GetSessionToken())
	responseMsg, err := f(circuitInfo, peerData)
	if err != nil {
		log.WithError(err).Error("error generating create circuit response")
	}
	log.Debugf("responding with successful circuit setup")

	responseMsg.ReplyTo(ctx.msg)
	if err = self.ch.Send(responseMsg); err != nil {
		log.WithError(err).WithField("token", ctx.req.GetSessionToken()).Error("failed to send create circuit response")
	}
}

type createCircuitResponseFactory func(*model.Circuit, map[uint32][]byte) (*channel.Message, error)

var _ CreateCircuitRequest = (*edge_ctrl_pb.CreateCircuitRequest)(nil)

type CreateCircuitRequest interface {
	GetSessionToken() string
	GetApiSessionToken() string
	GetTerminatorInstanceId() string
	GetPeerData() map[uint32][]byte
}

type CreateCircuitRequestContext struct {
	baseSessionRequestContext
	req CreateCircuitRequest
}

func (self *CreateCircuitRequestContext) GetSessionToken() string {
	return self.req.GetSessionToken()
}
