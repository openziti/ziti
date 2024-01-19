package handler_edge_ctrl

import (
	"fmt"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/fields"
	"math"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/controller/xt"
	"github.com/sirupsen/logrus"
)

type requestHandler interface {
	getAppEnv() *env.AppEnv
	getNetwork() *network.Network
	getChannel() channel.Channel
	ContentType() int32
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
	sourceRouter *network.Router
	session      *model.Session
	apiSession   *model.ApiSession
	service      *model.Service
	newSession   bool
	logContext   logcontext.Context
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

func (self *baseSessionRequestContext) loadSession(token string) {
	if self.err == nil {
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

		self.logContext = logcontext.NewContext()
		traceSpec := self.handler.getAppEnv().TraceManager.GetIdentityTrace(apiSession.IdentityId)
		traceEnabled := traceSpec != nil && time.Now().Before(traceSpec.Until)
		if traceEnabled {
			self.logContext.SetChannelsMask(traceSpec.ChannelMask)
			self.logContext.WithField("traceId", traceSpec.TraceId)
		}
		self.logContext.WithField("sessionId", self.session.Id)
		self.logContext.WithField("apiSessionId", apiSession.Id)

		if traceEnabled {
			pfxlog.ChannelLogger(logcontext.EstablishPath).
				Wire(self.logContext).
				Debug("tracing enabled for this session")
		}
	}
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

func (self *baseSessionRequestContext) checkSessionFingerprints(fingerprints []string) {
	if self.err == nil {
		var apiSessionCertFingerprints []string

		found := false
		err := self.GetHandler().getAppEnv().Managers.ApiSession.VisitFingerprintsForApiSessionId(self.session.ApiSessionId, func(fingerprint string) bool {
			apiSessionCertFingerprints = append(apiSessionCertFingerprints, fingerprint)
			if stringz.Contains(fingerprints, fingerprint) {
				found = true
				return true
			}
			return false
		})

		self.err = internalError(err)

		if self.err != nil || !found {
			if self.err == nil {
				self.err = InvalidApiSessionError{}
			}
			logrus.
				WithField("sessionId", self.session.Id).
				WithField("operation", self.handler.Label()).
				WithField("apiSessionFingerprints", apiSessionCertFingerprints).
				WithField("clientFingerprints", fingerprints).
				Error("matching fingerprint not found for connect")
		}
	}
}

func (self *baseSessionRequestContext) verifyEdgeRouterAccess() {
	if self.err == nil {
		// validate edge router
		erMgr := self.handler.getAppEnv().Managers.EdgeRouter
		edgeRouterAllowed, err := erMgr.IsAccessToEdgeRouterAllowed(self.session.IdentityId, self.session.ServiceId, self.sourceRouter.Id)
		if err != nil {
			self.err = internalError(err)
			logrus.
				WithField("sessionId", self.session.Id).
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

func (self *baseSessionRequestContext) verifyTerminator(terminatorId string, binding string) *network.Terminator {
	if self.err == nil {
		var terminator *network.Terminator
		var err error
		terminator, err = self.handler.getNetwork().Terminators.Read(terminatorId)

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

func (self *baseSessionRequestContext) updateTerminator(terminator *network.Terminator, request UpdateTerminatorRequest, ctx *change.Context) {
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

		self.err = internalError(self.handler.getNetwork().Terminators.Update(terminator, checker, ctx))
	}
}

func (self *baseSessionRequestContext) newCircuitCreateParms(serviceId string, peerData map[uint32][]byte) network.CreateCircuitParams {
	return &circuitParams{
		serviceId:    serviceId,
		sourceRouter: self.sourceRouter,
		clientId:     &identity.TokenId{Token: self.session.Id, Data: peerData},
		logCtx:       self.logContext,
		deadline:     time.Now().Add(self.handler.getAppEnv().GetHostController().GetNetwork().GetOptions().RouteTimeout),
		reqCtx:       self,
	}
}

func (self *baseSessionRequestContext) createCircuit(terminatorInstanceId string, peerData map[uint32][]byte) (*network.Circuit, map[uint32][]byte) {
	var circuit *network.Circuit
	returnPeerData := map[uint32][]byte{}

	if self.err == nil {
		if self.service.EncryptionRequired && peerData[edge.PublicKeyHeader] == nil {
			self.err = encryptionDataMissing("encryption required on service, initiator did not send public header")
			return nil, nil
		}

		serviceId := self.session.ServiceId
		if terminatorInstanceId != "" {
			serviceId = terminatorInstanceId + "@" + serviceId
		}

		n := self.handler.getAppEnv().GetHostController().GetNetwork()
		params := self.newCircuitCreateParms(serviceId, peerData)
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

			if self.service.EncryptionRequired && returnPeerData[edge.PublicKeyHeader] == nil {
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

type circuitParams struct {
	serviceId    string
	sourceRouter *network.Router
	clientId     *identity.TokenId
	logCtx       logcontext.Context
	deadline     time.Time
	reqCtx       *baseSessionRequestContext
}

func (self *circuitParams) GetServiceId() string {
	return self.serviceId
}

func (self *circuitParams) GetSourceRouter() *network.Router {
	return self.sourceRouter
}

func (self *circuitParams) GetClientId() *identity.TokenId {
	return self.clientId
}

func (self *circuitParams) GetCircuitTags(t xt.CostedTerminator) map[string]string {
	if t == nil {
		return map[string]string{
			"serviceId": self.reqCtx.session.ServiceId,
			"clientId":  self.reqCtx.session.IdentityId,
		}
	}

	hostId := t.GetHostId()
	return map[string]string{
		"serviceId": self.reqCtx.session.ServiceId,
		"clientId":  self.reqCtx.session.IdentityId,
		"hostId":    hostId,
	}
}

func (self *circuitParams) GetLogContext() logcontext.Context {
	return self.logCtx
}

func (self *circuitParams) GetDeadline() time.Time {
	return self.deadline
}
