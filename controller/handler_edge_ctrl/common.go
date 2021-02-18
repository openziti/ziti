package handler_edge_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/util/stringz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"time"
)

type requestHandler interface {
	getAppEnv() *env.AppEnv
	getNetwork() *network.Network
	getChannel() channel2.Channel
	ContentType() int32
	Label() string
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
		WithField("token", ctx.GetSessionToken()).
		WithField("operation", ctx.GetHandler().Label())

	if err = self.ch.Send(responseMsg); err != nil {
		logger.Error("failed to send error response")
	} else {
		logger.Debug("sent error response")
	}
}

type requestContext interface {
	GetHandler() requestHandler
	GetSessionToken() string
	GetMessage() *channel2.Message
}

type baseRequestContext struct {
	handler      requestHandler
	msg          *channel2.Message
	err          error
	sourceRouter *network.Router
	session      *model.Session
	service      *model.Service
}

func (self *baseRequestContext) GetMessage() *channel2.Message {
	return self.msg
}

func (self *baseRequestContext) GetHandler() requestHandler {
	return self.handler
}

func (self *baseRequestContext) GetSessionFields(ctx requestContext) logrus.Fields {
	result := logrus.Fields{}
	if self.session != nil {
		result["sessionId"] = self.session.Id
	} else {
		result["token"] = ctx.GetSessionToken()
	}
	result["router"] = self.handler.getChannel().Id().Token
	result["operation"] = self.handler.Label()
	return result
}

func (self *baseRequestContext) loadRouter() bool {
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

func (self *baseRequestContext) loadSession(token string) {
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

func (self *baseRequestContext) checkSessionType(sessionType string) {
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

func (self *baseRequestContext) checkSessionFingerprints(fingerprints []string) {
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

func (self *baseRequestContext) verifyEdgeRouterAccess() {
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

func (self *baseRequestContext) loadService() {
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

func (self *baseRequestContext) verifyTerminator(terminatorId string) *network.Terminator {
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
			self.err = errors.Errorf("remove request for terminator %v on router %v came from router %v",
				terminatorId, terminator.Router, self.sourceRouter.Id)

			log := logrus.
				WithField("operation", self.handler.Label()).
				WithField("sourceRouter", self.sourceRouter.Id).
				WithField("terminatorId", terminatorId).
				WithField("terminatorRouter", terminator.Router).
				WithError(self.err)
			if self.session != nil {
				log = log.WithField("sessionId", self.session.Id)
			}
			log.Error("not allowed to remove terminators on other routers")
		}
		return terminator
	}
	return nil
}
