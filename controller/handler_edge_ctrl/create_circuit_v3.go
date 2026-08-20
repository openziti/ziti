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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/openziti/ziti/v2/common/logcontext"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/xt"
	"github.com/sirupsen/logrus"
)

// NewCreateCircuitV3Handler creates a handler for CreateCircuitV3 requests. These requests
// come from routers that have already authorized the dial locally via RDM, so no service
// session token is required. An API session token is still required: it proves the dial is
// being made on behalf of an authenticated identity, and the controller uses its claims,
// not the request's identity ID, as the authoritative dialing identity. The request also
// carries the service ID and a pre-assigned circuit ID.
func NewCreateCircuitV3Handler(appEnv *env.AppEnv, ch channel.Channel) channel.TypedReceiveHandler {
	handler := &createCircuitHandler{
		baseRequestHandler: baseRequestHandler{
			ch:     ch,
			appEnv: appEnv,
		},
	}
	return &channel.AsyncFunctionReceiveAdapter{
		Type:    int32(edge_ctrl_pb.ContentType_CreateCircuitV3RequestType),
		Handler: handler.HandleReceiveCreateCircuitV3,
	}
}

func (self *createCircuitHandler) HandleReceiveCreateCircuitV3(msg *channel.Message, ch channel.Channel) {
	req, err := ctrl_msg.DecodeCreateCircuitV3Request(msg)
	if err != nil {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("could not decode CreateCircuitV3Request")
		return
	}

	ctx := &createCircuitV3RequestContext{
		baseSessionRequestContext: baseSessionRequestContext{handler: self, msg: msg, env: self.appEnv},
		req:                       req,
	}

	self.createCircuitV3(ctx, self.CreateCircuitV3Response)
}

func (self *createCircuitHandler) CreateCircuitV3Response(circuitInfo *model.Circuit, peerData map[uint32][]byte) (*channel.Message, error) {
	response := &ctrl_msg.CreateCircuitV3Response{
		CircuitId: circuitInfo.Id,
		Address:   circuitInfo.Path.IngressId,
		PeerData:  peerData,
		Tags:      circuitInfo.Tags,
	}

	return response.ToMessage(), nil
}

func (self *createCircuitHandler) createCircuitV3(ctx *createCircuitV3RequestContext, f createCircuitResponseFactory) {
	if !ctx.loadRouter() {
		return
	}
	ctx.validateApiSession()
	ctx.setupLogContext()
	ctx.loadServiceByIdForDial()
	ctx.verifyEdgeRouterAccessForIdentity()
	circuitInfo, peerData := ctx.createCircuit(ctx.req.TerminatorInstanceId, ctx.req.PeerData, ctx.newCircuitCreateParms)

	if ctx.err != nil {
		if circuitInfo != nil {
			ctx.errRespF = func(resp *channel.Message) {
				resp.PutStringHeader(edge.CircuitIdHeader, circuitInfo.Id)
			}
		}
		self.returnError(ctx, ctx.err)
		return
	}

	log := pfxlog.ContextLogger(self.ch.Label()).
		WithField("identityId", ctx.identityId()).
		WithField("serviceId", ctx.req.ServiceId).
		WithField("circuitId", circuitInfo.Id)

	responseMsg, err := f(circuitInfo, peerData)
	if err != nil {
		log.WithError(err).Error("error generating create circuit v3 response")
	}
	log.Debug("responding with successful circuit v3 setup")

	responseMsg.ReplyTo(ctx.msg)
	if err = self.ch.Send(responseMsg); err != nil {
		log.WithError(err).Error("failed to send create circuit v3 response")
	}
}

type createCircuitV3RequestContext struct {
	baseSessionRequestContext
	req      *ctrl_msg.CreateCircuitV3Request
	errRespF func(m *channel.Message)
}

func (self *createCircuitV3RequestContext) UpdateResponse(m *channel.Message) {
	if self.errRespF != nil {
		self.errRespF(m)
	}
}

// validateApiSession validates the API session token accompanying the request and
// establishes the dialing identity from its claims. Connect-v2 routers authorize the dial
// locally against their data model, but the identity ID in the request is router-supplied,
// so the controller validates the token itself. This both proves the dial is being made on
// behalf of an authenticated identity and re-checks expiration and revocation, which the
// router may not have seen yet.
func (self *createCircuitV3RequestContext) validateApiSession() {
	if self.err != nil {
		return
	}

	log := logrus.WithField("routerId", self.sourceRouter.Id).
		WithField("operation", self.handler.Label()).
		WithField("requestedIdentityId", self.req.IdentityId).
		WithField("serviceId", self.req.ServiceId)

	if self.req.ApiSessionToken == "" {
		self.err = InvalidApiSessionError{}
		log.Error("no api session token provided in create circuit v3 request")
		return
	}

	claims, err := self.env.ValidateAccessToken(self.req.ApiSessionToken)
	if err != nil {
		self.err = InvalidApiSessionError{}
		log.WithError(err).Error("invalid api session token in create circuit v3 request")
		return
	}

	// The token is authoritative for identity. A mismatch means the router asked for a
	// circuit on behalf of an identity other than the one that authenticated.
	if claims.Subject != self.req.IdentityId {
		self.err = InvalidApiSessionError{}
		log.WithField("apiSessionIdentityId", claims.Subject).
			WithField("apiSessionId", claims.ApiSessionId).
			Error("create circuit v3 request identity does not match api session identity")
		return
	}

	self.accessClaims = claims
}

// identityId returns the identity the circuit is being created for, taken from the
// validated API session token claims. Only valid once validateApiSession has succeeded.
func (self *createCircuitV3RequestContext) identityId() string {
	if self.accessClaims == nil {
		return ""
	}
	return self.accessClaims.Subject
}

func (self *createCircuitV3RequestContext) setupLogContext() {
	if self.err != nil {
		return
	}

	self.logContext = logcontext.NewContext()
	traceSpec := self.handler.getAppEnv().TraceManager.GetIdentityTrace(self.identityId())
	if traceSpec != nil && time.Now().Before(traceSpec.Until) {
		self.logContext.SetChannelsMask(traceSpec.ChannelMask)
		self.logContext.WithField("traceId", traceSpec.TraceId)
	}
	self.logContext.WithField("apiSessionId", self.accessClaims.ApiSessionId)
}

func (self *createCircuitV3RequestContext) loadServiceByIdForDial() {
	if self.err != nil {
		return
	}

	var err error
	self.service, err = self.handler.getAppEnv().Managers.EdgeService.Read(self.req.ServiceId)
	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			self.err = InvalidServiceError{}
		} else {
			self.err = internalError(err)
		}
		logrus.WithField("serviceId", self.req.ServiceId).
			WithField("operation", self.handler.Label()).
			WithError(self.err).
			Error("service not found")
		return
	}

	dialable, err := self.handler.getAppEnv().Managers.EdgeService.IsDialableByIdentity(self.req.ServiceId, self.identityId())
	if err != nil {
		self.err = internalError(err)
		logrus.WithField("serviceId", self.req.ServiceId).
			WithField("identityId", self.identityId()).
			WithField("operation", self.handler.Label()).
			WithError(err).
			Error("unable to verify dial access to service")
		return
	}

	if !dialable {
		self.err = InvalidServiceError{}
		logrus.WithField("serviceId", self.req.ServiceId).
			WithField("identityId", self.identityId()).
			WithField("operation", self.handler.Label()).
			Error("identity does not have dial access to service")
	}
}

func (self *createCircuitV3RequestContext) verifyEdgeRouterAccessForIdentity() {
	if self.err != nil {
		return
	}
	self.verifyEdgeRouterAccess(self.identityId(), self.service.Id)
}

func (self *createCircuitV3RequestContext) newCircuitCreateParms(serviceId string, peerData map[uint32][]byte) model.CreateCircuitParams {
	return &connectV3CircuitParams{
		circuitId:    self.req.CircuitId,
		serviceId:    serviceId,
		identityId:   self.identityId(),
		sourceRouter: self.sourceRouter,
		clientId:     &identity.TokenId{Token: self.identityId(), Data: peerData},
		logCtx:       self.logContext,
		deadline:     time.Now().Add(self.handler.getAppEnv().GetHostController().GetNetwork().GetOptions().RouteTimeout),
	}
}

type connectV3CircuitParams struct {
	circuitId    string
	serviceId    string
	identityId   string
	sourceRouter *model.Router
	clientId     *identity.TokenId
	logCtx       logcontext.Context
	deadline     time.Time
}

func (self *connectV3CircuitParams) GetServiceId() string {
	return self.serviceId
}

func (self *connectV3CircuitParams) GetSourceRouter() *model.Router {
	return self.sourceRouter
}

func (self *connectV3CircuitParams) GetClientId() *identity.TokenId {
	return self.clientId
}

func (self *connectV3CircuitParams) GetCircuitTags(t xt.CostedTerminator) map[string]string {
	if t == nil {
		return map[string]string{
			"serviceId": self.serviceId,
			"clientId":  self.identityId,
		}
	}
	return map[string]string{
		"serviceId": self.serviceId,
		"clientId":  self.identityId,
		"hostId":    t.GetHostId(),
	}
}

func (self *connectV3CircuitParams) GetLogContext() logcontext.Context {
	return self.logCtx
}

func (self *connectV3CircuitParams) GetDeadline() time.Time {
	return self.deadline
}

func (self *connectV3CircuitParams) GetCircuitId() string {
	return self.circuitId
}
