package handler_edge_ctrl

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/controller/oidc_auth"
	"github.com/openziti/ziti/controller/xt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type requestHandler interface {
	getAppEnv() *env.AppEnv
	getNetwork() *network.Network
	getChannel() channel.Channel
	Label() string
}

type UpdateTerminatorRequest interface {
	GetCost() uint32
	GetUpdateCost() bool
	GetPrecedence() edge_ctrl_pb.TerminatorPrecedence
	GetUpdatePrecedence() bool
}

type baseRequestHandler struct {
	ch     channel.Channel
	appEnv *env.AppEnv
}

func (self *baseRequestHandler) getNetwork() *network.Network {
	return self.appEnv.GetHostController().GetNetwork()
}

func (self *baseRequestHandler) getAppEnv() *env.AppEnv {
	return self.appEnv
}

func (self *baseRequestHandler) getChannel() channel.Channel {
	return self.ch
}

func (self *baseRequestHandler) returnError(ctx requestContext, err controllerError) {
	responseMsg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_ErrorType), []byte(err.Error()))
	responseMsg.PutUint32Header(edge.ErrorCodeHeader, err.ErrorCode())
	responseMsg.ReplyTo(ctx.GetMessage())
	logger := pfxlog.
		ContextLogger(self.ch.Label()).
		WithError(err).
		WithField("routerId", ctx.GetHandler().getChannel().Id()).
		WithField("operation", ctx.GetHandler().Label())

	if sessionCtx, ok := ctx.(sessionRequestContext); ok {
		logger = logger.WithField("token", sessionCtx.GetSessionToken())
	}

	if sendErr := self.ch.Send(responseMsg); sendErr != nil {
		logger.WithError(err).WithField("sendError", sendErr).Error("failed to send error response")
	} else {
		logger.WithError(err).Error("responded with error")
	}
}

func (self *baseRequestHandler) logResult(ctx requestContext, err error) {
	logger := logrus.
		WithField("routerId", ctx.GetHandler().getChannel().Id()).
		WithField("operation", ctx.GetHandler().Label())

	if sessionCtx, ok := ctx.(sessionRequestContext); ok {
		logger = logger.WithField("token", sessionCtx.GetSessionToken())
	}

	if err != nil {
		logger.WithError(err).Error("operation failed")
	} else {
		logger.Debug("operation success")
	}
}

type requestContext interface {
	GetHandler() requestHandler
	GetMessage() *channel.Message
}

type sessionRequestContext interface {
	requestContext
	GetSessionToken() string
}

type baseSessionRequestContext struct {
	handler      requestHandler
	msg          *channel.Message
	err          controllerError
	sourceRouter *model.Router
	session      *model.Session
	apiSession   *model.ApiSession
	service      *model.EdgeService
	newSession   bool
	logContext   logcontext.Context
	env          model.Env
	accessClaims *common.AccessClaims
}

func (self *baseSessionRequestContext) getApiSessionId() string {
	if self.apiSession != nil {
		return self.apiSession.Id
	}
	return ""
}

func (self *baseSessionRequestContext) newChangeContext() *change.Context {
	result := change.New().SetSourceType(change.SourceTypeControlChannel).
		SetSourceMethod(self.handler.Label()).
		SetSourceLocal(self.handler.getChannel().Underlay().GetLocalAddr().String()).
		SetSourceRemote(self.handler.getChannel().Underlay().GetRemoteAddr().String())
	if self.session != nil {
		result.
			SetChangeAuthorType(change.AuthorTypeIdentity).
			SetChangeAuthorId(self.session.IdentityId)
		if self.apiSession != nil && self.apiSession.Identity != nil {
			result.SetChangeAuthorName(self.apiSession.Identity.Name)
		} else if authorIdentity, _ := self.handler.getAppEnv().Managers.Identity.Read(self.session.IdentityId); authorIdentity != nil {
			result.SetChangeAuthorName(authorIdentity.Name)
		}
	} else if self.sourceRouter != nil {
		result.
			SetChangeAuthorType(change.AuthorTypeRouter).
			SetChangeAuthorId(self.sourceRouter.Id).
			SetChangeAuthorName(self.sourceRouter.Name)
	}
	return result
}

func (self *baseSessionRequestContext) newTunnelChangeContext() *change.Context {
	return change.New().SetSourceType(change.SourceTypeControlChannel).
		SetSourceMethod(self.handler.Label()).
		SetSourceLocal(self.handler.getChannel().Underlay().GetLocalAddr().String()).
		SetSourceRemote(self.handler.getChannel().Underlay().GetRemoteAddr().String()).
		SetChangeAuthorType(change.AuthorTypeRouter).
		SetChangeAuthorId(self.sourceRouter.Id).
		SetChangeAuthorName(self.sourceRouter.Name)
}

func (self *baseSessionRequestContext) GetMessage() *channel.Message {
	return self.msg
}

func (self *baseSessionRequestContext) GetHandler() requestHandler {
	return self.handler
}

func (self *baseSessionRequestContext) loadRouter() bool {
	routerId := self.handler.getChannel().Id()
	var err error
	self.sourceRouter, err = self.handler.getNetwork().GetRouter(routerId)
	if err != nil {
		self.err = internalError(err)
		logrus.
			WithField("router", routerId).
			WithField("operation", self.handler.Label()).
			WithError(self.err).Errorf("could not find router closing channel")
		_ = self.handler.getChannel().Close()
		return false
	}
	return true
}

func (self *baseSessionRequestContext) loadSession(sessionToken string, apiSessionToken string) {
	if strings.HasPrefix(sessionToken, oidc_auth.JwtTokenPrefix) {
		self.loadFromTokens(sessionToken, apiSessionToken)
	} else {
		self.loadFromBolt(sessionToken)
	}

	if self.err != nil {
		return
	}

	if self.session == nil {
		self.err = internalError(errors.New("session was not found after load"))
		return
	}

	if self.apiSession == nil {
		self.err = internalError(errors.New("api session was not found after load"))
		return
	}

	self.logContext = logcontext.NewContext()
	traceSpec := self.handler.getAppEnv().TraceManager.GetIdentityTrace(self.apiSession.IdentityId)
	traceEnabled := traceSpec != nil && time.Now().Before(traceSpec.Until)
	if traceEnabled {
		self.logContext.SetChannelsMask(traceSpec.ChannelMask)
		self.logContext.WithField("traceId", traceSpec.TraceId)
	}
	self.logContext.WithField("sessionId", self.session.Id)
	self.logContext.WithField("apiSessionId", self.apiSession.Id)

	if traceEnabled {
		pfxlog.ChannelLogger(logcontext.EstablishPath).
			Wire(self.logContext).
			Debug("tracing enabled for this session")
	}
}

func (self *baseSessionRequestContext) loadFromTokens(sessionToken, apiSessionToken string) {
	if self.err != nil {
		return
	}

	var err error
	self.accessClaims, err = self.env.ValidateAccessToken(apiSessionToken)

	if err != nil {
		self.err = internalError(err)
		return
	}

	serviceAccessClaims, err := self.env.ValidateServiceAccessToken(sessionToken, &self.accessClaims.ApiSessionId)

	if err != nil {
		self.err = internalError(err)
		return
	}

	if self.accessClaims.Subject != serviceAccessClaims.IdentityId {
		self.err = internalError(fmt.Errorf("access and service tokens do not match, got access identity id %s and service identity id %s", self.accessClaims.Subject, serviceAccessClaims.IdentityId))
		return
	}

	self.session = &model.Session{
		BaseEntity: models.BaseEntity{
			Id:        serviceAccessClaims.ID,
			CreatedAt: serviceAccessClaims.IssuedAt.Time,
			UpdatedAt: serviceAccessClaims.IssuedAt.Time,
		},
		Token:        sessionToken,
		IdentityId:   serviceAccessClaims.IdentityId,
		ApiSessionId: serviceAccessClaims.ApiSessionId,
		ServiceId:    serviceAccessClaims.Subject,
		Type:         serviceAccessClaims.Type,
	}

	tokenIdentity, err := self.env.GetManagers().Identity.Read(self.accessClaims.Subject)

	if err != nil {
		self.err = internalError(err)
		return
	}

	self.apiSession = &model.ApiSession{
		BaseEntity: models.BaseEntity{
			Id: serviceAccessClaims.ApiSessionId,
		},
		Token:              apiSessionToken,
		IdentityId:         serviceAccessClaims.IdentityId,
		Identity:           tokenIdentity,
		IPAddress:          self.accessClaims.RemoteAddress,
		ConfigTypes:        self.accessClaims.ConfigTypesAsMap(),
		MfaComplete:        false,
		MfaRequired:        false,
		ExpiresAt:          time.Time{},
		ExpirationDuration: 0,
		LastActivityAt:     time.Time{},
		AuthenticatorId:    "",
	}
}

func (self *baseSessionRequestContext) loadFromBolt(token string) {
	if self.err != nil {
		return
	}

	var err error
	self.session, err = self.handler.getAppEnv().Managers.Session.ReadByToken(token)
	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			self.err = InvalidSessionError{}
		} else {
			self.err = internalError(err)
		}
		logrus.
			WithField("token", token).
			WithField("operation", self.handler.Label()).
			WithError(self.err).Errorf("invalid session")
		return
	}
	apiSession, err := self.handler.getAppEnv().Managers.ApiSession.Read(self.session.ApiSessionId)
	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			self.err = InvalidApiSessionError{}
		} else {
			self.err = internalError(err)
		}
		logrus.
			WithField("token", token).
			WithField("operation", self.handler.Label()).
			WithError(self.err).Errorf("invalid api-session")
		return
	}
	self.apiSession = apiSession
}

func (self *baseSessionRequestContext) checkSessionType(sessionType string) {
	if self.err == nil {
		if self.session.Type != sessionType {
			self.err = WrongSessionTypeError{}
			logrus.
				WithField("sessionId", self.session.Id).
				WithField("operation", self.handler.Label()).
				WithError(self.err).Errorf("wrong session type")
		}
	}
}

func (self *baseSessionRequestContext) verifyIdentityEdgeRouterAccess() {
	if self.err == nil {
		self.verifyEdgeRouterAccess(self.session.IdentityId, self.session.ServiceId)
	}
}

func (self *baseSessionRequestContext) verifyEdgeRouterServiceBindAccess() {
	if self.err == nil {
		self.verifyServiceBindAccess(self.sourceRouter.Id, self.service.Id)
	}
}

func (self *baseSessionRequestContext) verifyServiceBindAccess(identityId string, serviceId string) {
	if self.err == nil {
		// validate edge router
		result, err := self.handler.getAppEnv().Managers.EdgeService.IsBindableByIdentity(serviceId, identityId)
		if err != nil {
			self.err = internalError(err)
			logrus.
				WithField("routerId", self.sourceRouter.Id).
				WithField("identityId", identityId).
				WithField("serviceId", serviceId).
				WithField("operation", self.handler.Label()).
				WithError(err).Error("unable to verify edge router access to bind service")
			return
		} else if !result {
			self.err = InvalidServiceError{}
		}
	}
}

func (self *baseSessionRequestContext) verifyRouterEdgeRouterAccess() {
	if self.err == nil {
		self.verifyEdgeRouterAccess(self.sourceRouter.Id, self.service.Id)
	}
}

func (self *baseSessionRequestContext) verifyEdgeRouterAccess(identityId string, serviceId string) {
	if self.err == nil {
		// validate edge router
		erMgr := self.handler.getAppEnv().Managers.EdgeRouter
		edgeRouterAllowed, err := erMgr.IsAccessToEdgeRouterAllowed(identityId, serviceId, self.sourceRouter.Id)
		if err != nil {
			self.err = internalError(err)
			logrus.
				WithField("routerId", self.sourceRouter.Id).
				WithField("identityId", identityId).
				WithField("serviceId", serviceId).
				WithField("operation", self.handler.Label()).
				WithError(err).Error("unable to verify edge router access")
			return
		}

		if !edgeRouterAllowed {
			self.err = InvalidEdgeRouterForSessionError{}
		}
	}
}

func (self *baseSessionRequestContext) loadService() {
	if self.err == nil {
		var err error
		self.service, err = self.handler.getAppEnv().Managers.EdgeService.Read(self.session.ServiceId)

		if err != nil {
			if boltz.IsErrNotFoundErr(err) {
				self.err = InvalidServiceError{}
			} else {
				self.err = internalError(err)
			}
			logrus.
				WithField("sessionId", self.session.Id).
				WithField("operation", self.handler.Label()).
				WithField("serviceId", self.session.ServiceId).
				WithError(self.err).
				Error("service not found")
		}
	}
}

func (self *baseSessionRequestContext) verifyTerminator(terminatorId string, binding string) *model.Terminator {
	if self.err == nil {
		var terminator *model.Terminator
		var err error
		terminator, err = self.handler.getNetwork().Terminator.Read(terminatorId)

		if err != nil {
			if boltz.IsErrNotFoundErr(err) {
				self.err = invalidTerminator("invalid terminator: not found")
			} else {
				self.err = internalError(err)
			}
			log := logrus.
				WithField("operation", self.handler.Label()).
				WithField("terminatorId", terminatorId).
				WithError(self.err)
			if self.session != nil {
				log = log.WithField("sessionId", self.session.Id)
			}
			log.Error("terminator not found")
			return nil
		}

		if terminator != nil && terminator.Router != self.sourceRouter.Id {
			self.err = invalidTerminator(fmt.Sprintf("%v request for terminator %v on router %v came from router %v",
				self.handler.Label(), terminatorId, terminator.Router, self.sourceRouter.Id))

			log := logrus.
				WithField("operation", self.handler.Label()).
				WithField("sourceRouter", self.sourceRouter.Id).
				WithField("terminatorId", terminatorId).
				WithField("terminatorRouter", terminator.Router).
				WithError(self.err)
			if self.session != nil {
				log = log.WithField("sessionId", self.session.Id)
			}
			log.Error("not allowed to operate on terminators on other routers")
			return nil
		}

		if terminator != nil && terminator.Binding != binding {
			self.err = invalidTerminator(fmt.Sprintf("can't operate on terminator %v with wrong binding, expected binding %v, was %v ",
				terminatorId, binding, terminator.Binding))

			log := logrus.
				WithField("operation", self.handler.Label()).
				WithField("sourceRouter", self.sourceRouter.Id).
				WithField("terminatorId", terminatorId).
				WithField("terminatorRouter", terminator.Router).
				WithField("binding", terminator.Binding).
				WithField("expectedBinding", binding).
				WithError(self.err)
			if self.session != nil {
				log = log.WithField("sessionId", self.session.Id)
			}
			log.Error("incorrect binding")
			return nil
		}

		return terminator
	}
	return nil
}

func (self *baseSessionRequestContext) verifyTerminatorId(id string) {
	if self.err == nil {
		if id == "" {
			self.err = invalidTerminator("provided terminator id is blank")
		}
	}
}

func (self *baseSessionRequestContext) updateTerminator(terminator *model.Terminator, request UpdateTerminatorRequest, ctx *change.Context) {
	if self.err == nil {
		checker := fields.UpdatedFieldsMap{}

		if request.GetUpdateCost() {
			if request.GetCost() > math.MaxUint16 {
				self.err = invalidCost(fmt.Sprintf("invalid cost %v. cost must be between 0 and %v inclusive", request.GetCost(), math.MaxUint16))
				return
			}
			terminator.Cost = uint16(request.GetCost())
			checker[db.FieldTerminatorCost] = struct{}{}
		}

		if request.GetUpdatePrecedence() {
			if request.GetPrecedence() == edge_ctrl_pb.TerminatorPrecedence_Default {
				terminator.Precedence = xt.Precedences.Default
			} else if request.GetPrecedence() == edge_ctrl_pb.TerminatorPrecedence_Required {
				terminator.Precedence = xt.Precedences.Required
			} else if request.GetPrecedence() == edge_ctrl_pb.TerminatorPrecedence_Failed {
				terminator.Precedence = xt.Precedences.Failed
			} else {
				self.err = invalidPrecedence(fmt.Sprintf("invalid precedence: %v", request.GetPrecedence()))
				return
			}

			checker[db.FieldTerminatorPrecedence] = struct{}{}
		}

		self.err = internalError(self.handler.getNetwork().Terminator.Update(terminator, checker, ctx))
	}
}

func (self *baseSessionRequestContext) newCircuitCreateParms(serviceId string, peerData map[uint32][]byte) model.CreateCircuitParams {
	return &sessionCircuitParams{
		serviceId:    serviceId,
		sourceRouter: self.sourceRouter,
		clientId:     &identity.TokenId{Token: self.session.Id, Data: peerData},
		logCtx:       self.logContext,
		deadline:     time.Now().Add(self.handler.getAppEnv().GetHostController().GetNetwork().GetOptions().RouteTimeout),
		reqCtx:       self,
	}
}

func (self *baseSessionRequestContext) newTunnelCircuitCreateParms(serviceId string, peerData map[uint32][]byte) model.CreateCircuitParams {
	return &tunnelCircuitParams{
		serviceId:    serviceId,
		sourceRouter: self.sourceRouter,
		clientId:     &identity.TokenId{Token: self.sourceRouter.Id, Data: peerData},
		logCtx:       self.logContext,
		deadline:     time.Now().Add(self.handler.getAppEnv().GetHostController().GetNetwork().GetOptions().RouteTimeout),
		reqCtx:       self,
	}
}

type circuitParamsFactory = func(serviceId string, peerData map[uint32][]byte) model.CreateCircuitParams

func (self *baseSessionRequestContext) createCircuit(terminatorInstanceId string, peerData map[uint32][]byte, paramsFactory circuitParamsFactory) (*model.Circuit, map[uint32][]byte) {
	var circuit *model.Circuit
	returnPeerData := map[uint32][]byte{}

	if self.err == nil {
		if self.service.EncryptionRequired && peerData[uint32(edge.PublicKeyHeader)] == nil {
			self.err = encryptionDataMissing("encryption required on service, initiator did not send public header")
			return nil, nil
		}

		serviceId := self.service.Id
		if terminatorInstanceId != "" {
			serviceId = terminatorInstanceId + "@" + serviceId
		}

		n := self.handler.getAppEnv().GetHostController().GetNetwork()
		params := paramsFactory(serviceId, peerData)
		var err error
		circuit, err = n.CreateCircuit(params)
		if err != nil {
			self.err = internalError(err)
		}

		if circuit != nil {
			//static terminator peer data
			for k, v := range circuit.Terminator.GetPeerData() {
				returnPeerData[k] = v
			}

			//runtime peer data
			for k, v := range circuit.PeerData {
				returnPeerData[k] = v
			}

			if self.service.EncryptionRequired && returnPeerData[uint32(edge.PublicKeyHeader)] == nil {
				self.err = encryptionDataMissing("encryption required on service, terminator did not send public header")
				if err := n.RemoveCircuit(circuit.Id, true); err != nil {
					logrus.
						WithField("operation", self.handler.Label()).
						WithField("sourceRouter", self.sourceRouter.Id).
						WithError(err).
						Error("failed to remove session")
				}
				return nil, nil
			}
		}
	}
	return circuit, returnPeerData
}

type sessionCircuitParams struct {
	serviceId    string
	sourceRouter *model.Router
	clientId     *identity.TokenId
	logCtx       logcontext.Context
	deadline     time.Time
	reqCtx       *baseSessionRequestContext
}

func (self *sessionCircuitParams) GetServiceId() string {
	return self.serviceId
}

func (self *sessionCircuitParams) GetSourceRouter() *model.Router {
	return self.sourceRouter
}

func (self *sessionCircuitParams) GetClientId() *identity.TokenId {
	return self.clientId
}

func (self *sessionCircuitParams) GetCircuitTags(t xt.CostedTerminator) map[string]string {
	if t == nil {
		return map[string]string{
			"serviceId": self.serviceId,
			"clientId":  self.reqCtx.session.IdentityId,
		}
	}

	hostId := t.GetHostId()
	return map[string]string{
		"serviceId": self.serviceId,
		"clientId":  self.reqCtx.session.IdentityId,
		"hostId":    hostId,
	}
}

func (self *sessionCircuitParams) GetLogContext() logcontext.Context {
	return self.logCtx
}

func (self *sessionCircuitParams) GetDeadline() time.Time {
	return self.deadline
}

type tunnelCircuitParams struct {
	serviceId    string
	sourceRouter *model.Router
	clientId     *identity.TokenId
	logCtx       logcontext.Context
	deadline     time.Time
	reqCtx       *baseSessionRequestContext
}

func (self *tunnelCircuitParams) GetServiceId() string {
	return self.serviceId
}

func (self *tunnelCircuitParams) GetSourceRouter() *model.Router {
	return self.sourceRouter
}

func (self *tunnelCircuitParams) GetClientId() *identity.TokenId {
	return self.clientId
}

func (self *tunnelCircuitParams) GetCircuitTags(t xt.CostedTerminator) map[string]string {
	if t == nil {
		return map[string]string{
			"serviceId": self.serviceId,
			"clientId":  self.sourceRouter.Id,
		}
	}

	hostId := t.GetHostId()
	return map[string]string{
		"serviceId": self.serviceId,
		"clientId":  self.sourceRouter.Id,
		"hostId":    hostId,
	}
}

func (self *tunnelCircuitParams) GetLogContext() logcontext.Context {
	return self.logCtx
}

func (self *tunnelCircuitParams) GetDeadline() time.Time {
	return self.deadline
}
