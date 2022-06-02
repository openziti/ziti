package handler_edge_ctrl

import (
	"fmt"
	"math"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/stringz"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/storage/boltz"
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
	ctx.CleanupOnError()
	responseMsg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_ErrorType), []byte(err.Error()))
	responseMsg.PutUint32Header(edge.ErrorCodeHeader, err.ErrorCode())
	responseMsg.ReplyTo(ctx.GetMessage())
	logger := pfxlog.
		ContextLogger(self.ch.Label()).
		WithError(err).
		WithField("routerId", ctx.GetHandler().getChannel().Id().Token).
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
		WithField("routerId", ctx.GetHandler().getChannel().Id().Token).
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
	CleanupOnError()
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
	service      *model.Service
	newSession   bool
	logContext   logcontext.Context
}

func (self *baseSessionRequestContext) CleanupOnError() {
	if self.newSession && self.session != nil {
		logger := logrus.
			WithField("operation", self.handler.Label()).
			WithField("routerId", self.sourceRouter.Name)

		if err := self.handler.getAppEnv().Managers.Session.Delete(self.session.Id); err != nil {
			logger.WithError(err).Error("unable to delete session created before error encountered")
		}
	}
}

func (self *baseSessionRequestContext) GetMessage() *channel.Message {
	return self.msg
}

func (self *baseSessionRequestContext) GetHandler() requestHandler {
	return self.handler
}

func (self *baseSessionRequestContext) loadRouter() bool {
	routerId := self.handler.getChannel().Id().Token
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
				self.err = InvalidApiSessionError{}
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
		now := time.Now()

		var sessionFingerprints []string
		for _, cert := range self.session.SessionCerts {
			if !cert.ValidFrom.After(now) && !cert.ValidTo.Before(now) {
				sessionFingerprints = append(sessionFingerprints, cert.Fingerprint)
			}
		}

		found := stringz.ContainsAny(sessionFingerprints, fingerprints...)
		if !found {
			err := self.GetHandler().getAppEnv().Managers.ApiSession.VisitFingerprintsForApiSessionId(self.session.ApiSessionId, func(fingerprint string) bool {
				sessionFingerprints = append(sessionFingerprints, fingerprint)
				if stringz.Contains(fingerprints, fingerprint) {
					found = true
					return true
				}
				return false
			})
			self.err = internalError(err)
		}

		if self.err != nil || !found {
			if self.err == nil {
				self.err = InvalidApiSessionError{}
			}
			logrus.
				WithField("sessionId", self.session.Id).
				WithField("operation", self.handler.Label()).
				WithField("sessionFingerprints", sessionFingerprints).
				WithField("clientFingerprints", fingerprints).
				Error("matching fingerprint not found for connect")
		}
	}
}

func (self *baseSessionRequestContext) verifyEdgeRouterAccess() {
	if self.err == nil {
		// validate edge router
		result, err := self.handler.getAppEnv().Managers.EdgeRouter.ListForSession(self.session.Id)
		if err != nil {
			self.err = internalError(err)
			logrus.
				WithField("sessionId", self.session.Id).
				WithField("operation", self.handler.Label()).
				WithError(err).Error("unable to verify edge router access")
			return
		}

		edgeRouterAllowed := false
		for _, er := range result.EdgeRouters {
			if er.Id == self.sourceRouter.Id {
				edgeRouterAllowed = true
				break
			}
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
				err = InvalidServiceError{}
			} else {
				err = internalError(err)
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

func (self *baseSessionRequestContext) updateTerminator(terminator *network.Terminator, request UpdateTerminatorRequest) {
	if self.err == nil {
		checker := boltz.MapFieldChecker{}

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

		self.err = internalError(self.handler.getNetwork().Terminators.Update(terminator, checker))
	}
}

func (self *baseSessionRequestContext) createCircuit(terminatorIdentity string, peerData map[uint32][]byte) (*network.Circuit, map[uint32][]byte) {
	var circuit *network.Circuit
	returnPeerData := map[uint32][]byte{}

	if self.err == nil {
		if self.service.EncryptionRequired && peerData[edge.PublicKeyHeader] == nil {
			self.err = encryptionDataMissing("encryption required on service, initiator did not send public header")
			return nil, nil
		}

		serviceId := self.session.ServiceId
		if terminatorIdentity != "" {
			serviceId = terminatorIdentity + "@" + serviceId
		}

		clientId := &identity.TokenId{Token: self.session.Id, Data: peerData}

		n := self.handler.getAppEnv().GetHostController().GetNetwork()
		var err error
		circuit, err = n.CreateCircuit(self.sourceRouter, clientId, serviceId, self.logContext, time.Now().Add(network.DefaultNetworkOptionsRouteTimeout))
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
