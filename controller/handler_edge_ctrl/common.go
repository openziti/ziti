package handler_edge_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/stringz"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"math"
	"time"
)

type requestHandler interface {
	getAppEnv() *env.AppEnv
	getNetwork() *network.Network
	getChannel() channel2.Channel
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
	ch     channel2.Channel
	appEnv *env.AppEnv
}

func (self *baseRequestHandler) getNetwork() *network.Network {
	return self.appEnv.GetHostController().GetNetwork()
}

func (self *baseRequestHandler) getAppEnv() *env.AppEnv {
	return self.appEnv
}

func (self *baseRequestHandler) getChannel() channel2.Channel {
	return self.ch
}

func (self *baseRequestHandler) returnError(ctx requestContext, err error) {
	responseMsg := channel2.NewMessage(int32(edge_ctrl_pb.ContentType_ErrorType), []byte(err.Error()))
	responseMsg.ReplyTo(ctx.GetMessage())
	logger := pfxlog.
		ContextLogger(self.ch.Label()).
		WithError(err).
		WithField("router", ctx.GetHandler().getChannel().Id().Token).
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

type requestContext interface {
	GetHandler() requestHandler
	GetMessage() *channel2.Message
}

type sessionRequestContext interface {
	requestContext
	GetSessionToken() string
}

type baseSessionRequestContext struct {
	handler      requestHandler
	msg          *channel2.Message
	err          error
	sourceRouter *network.Router
	session      *model.Session
	service      *model.Service
}

func (self *baseSessionRequestContext) GetMessage() *channel2.Message {
	return self.msg
}

func (self *baseSessionRequestContext) GetHandler() requestHandler {
	return self.handler
}

func (self *baseSessionRequestContext) loadRouter() bool {
	routerId := self.handler.getChannel().Id().Token
	self.sourceRouter, self.err = self.handler.getNetwork().GetRouter(routerId)
	if self.err != nil {
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
		self.session, self.err = self.handler.getAppEnv().Handlers.Session.ReadByToken(token)
		if self.err != nil {
			logrus.
				WithField("token", token).
				WithField("operation", self.handler.Label()).
				WithError(self.err).Errorf("invalid session")
		}
	}
}

func (self *baseSessionRequestContext) checkSessionType(sessionType string) {
	if self.err == nil {
		if self.session.Type != sessionType {
			logrus.
				WithField("sessionId", self.session.Id).
				WithField("operation", self.handler.Label()).
				WithError(self.err).Errorf("wrong session type")
			self.err = errors.New("invalid session")
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
			self.err = self.GetHandler().getAppEnv().Handlers.ApiSession.VisitFingerprintsForApiSessionId(self.session.ApiSessionId, func(fingerprint string) bool {
				sessionFingerprints = append(sessionFingerprints, fingerprint)
				if stringz.Contains(fingerprints, fingerprint) {
					found = true
					return true
				}
				return false
			})
		}

		if self.err != nil && !found {
			logrus.
				WithField("sessionId", self.session.Id).
				WithField("operation", self.handler.Label()).
				WithField("sessionFingerprints", sessionFingerprints).
				WithField("clientFingerprints", fingerprints).
				Error("matching fingerprint not found for connect")
			self.err = errors.New("invalid session")
		}
	}
}

func (self *baseSessionRequestContext) verifyEdgeRouterAccess() {
	if self.err == nil {
		// validate edge router
		result, err := self.handler.getAppEnv().Handlers.EdgeRouter.ListForSession(self.session.Id)
		if err != nil {
			self.err = errors.Wrap(err, "unable to verify edge router access")
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
			self.err = errors.New("invalid edge router for session")
		}
	}
}

func (self *baseSessionRequestContext) loadService() {
	if self.err == nil {
		self.service, self.err = self.handler.getAppEnv().Handlers.EdgeService.Read(self.session.ServiceId)

		if self.err != nil {
			logrus.
				WithField("sessionId", self.session.Id).
				WithField("operation", self.handler.Label()).
				WithField("serviceId", self.session.ServiceId).
				WithError(self.err).
				Error("service not found")
		}
	}
}

func (self *baseSessionRequestContext) loadServiceForName(name string) {
	if self.err == nil {
		self.service, self.err = self.handler.getAppEnv().Handlers.EdgeService.ReadByName(name)

		if self.err != nil {
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
		terminator, self.err = self.handler.getNetwork().Terminators.Read(terminatorId)

		if self.err != nil {
			log := logrus.
				WithField("operation", self.handler.Label()).
				WithField("terminatorId", terminatorId).
				WithError(self.err)
			if self.session != nil {
				log = log.WithField("sessionId", self.session.Id)
			}
			log.Error("terminator not found")
		}

		if terminator != nil && terminator.Router != self.sourceRouter.Id {
			self.err = errors.Errorf("%v request for terminator %v on router %v came from router %v",
				self.handler.Label(), terminatorId, terminator.Router, self.sourceRouter.Id)

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
		}

		if terminator != nil && terminator.Binding != binding {
			self.err = errors.Errorf("can't operate on terminator %v with wrong binding, expected binding %v, was %v ",
				terminatorId, binding, terminator.Binding)

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
				self.err = errors.Errorf("invalid cost %v. cost must be between 0 and %v inclusive", request.GetCost(), math.MaxUint16)
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
				self.err = errors.Errorf("invalid precedence: %v", request.GetPrecedence())
				return
			}

			checker[db.FieldTerminatorPrecedence] = struct{}{}
		}

		self.err = self.handler.getNetwork().Terminators.Patch(terminator, checker)
	}
}

func (self *baseSessionRequestContext) createCircuit(terminatorIdentity string, peerData map[uint32][]byte) (*network.Session, map[uint32][]byte) {
	var circuit *network.Session
	returnPeerData := map[uint32][]byte{}

	if self.err == nil {
		if self.service.EncryptionRequired && peerData[edge.PublicKeyHeader] == nil {
			self.err = errors.New("encryption required on service, initiator did not send public header")
			return nil, nil
		}

		serviceId := self.session.ServiceId
		if terminatorIdentity != "" {
			serviceId = terminatorIdentity + "@" + serviceId
		}

		clientId := &identity.TokenId{Token: self.session.Id, Data: peerData}

		n := self.handler.getAppEnv().GetHostController().GetNetwork()
		circuit, self.err = n.CreateSession(self.sourceRouter, clientId, serviceId)

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
				self.err = errors.New("encryption required on service, terminator did not send public header")
				if err := n.RemoveSession(circuit.Id, true); err != nil {
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
