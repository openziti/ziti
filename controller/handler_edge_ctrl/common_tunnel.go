package handler_edge_ctrl

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/sirupsen/logrus"
	"sync"
	"sync/atomic"
	"time"
)

type TunnelState struct {
	configTypes          []string
	currentApiSessionId  atomic.Value
	createApiSessionLock sync.Mutex
}

func (self *TunnelState) getCurrentApiSessionId() string {
	val := self.currentApiSessionId.Load()
	if val != nil {
		return val.(string)
	}
	return ""
}

func (self *TunnelState) clearCurrentApiSessionId() {
	self.currentApiSessionId.Store("")
}

func (self *TunnelState) setCurrentApiSessionId(val string) {
	self.currentApiSessionId.Store(val)
}

type tunnelRequestHandler interface {
	requestHandler
	getTunnelState() *TunnelState
}

type baseTunnelRequestContext struct {
	baseSessionRequestContext
	apiSession *model.ApiSession
	identity   *model.Identity
}

func (self *baseTunnelRequestContext) getTunnelState() *TunnelState {
	return self.handler.(tunnelRequestHandler).getTunnelState()
}

func (self *baseTunnelRequestContext) loadIdentity() {
	if self.err == nil {
		var err error
		self.identity, err = self.handler.getAppEnv().GetHandlers().Identity.Read(self.sourceRouter.Id)
		if err != nil {
			if boltz.IsErrNotFoundErr(err) {
				self.err = TunnelingNotEnabledError{}
			} else {
				self.err = internalError(err)
			}
			return
		}

		if self.identity.IdentityTypeId != persistence.RouterIdentityType {
			self.err = TunnelingNotEnabledError{}
			return
		}

		self.logContext = logcontext.NewContext()
		traceSpec := self.handler.getAppEnv().TraceManager.GetIdentityTrace(self.identity.Id)
		if traceSpec != nil && time.Now().After(traceSpec.Until) {
			self.logContext.SetChannelsMask(traceSpec.ChannelMask)
			self.logContext.WithField("traceId", traceSpec.TraceId)
		}
	}
}

func (self *baseTunnelRequestContext) ensureApiSession(configTypes []string) bool {
	if self.err == nil {
		logger := logrus.
			WithField("operation", self.handler.Label()).
			WithField("router", self.sourceRouter.Name)

		state := self.getTunnelState()
		apiSessionId := state.getCurrentApiSessionId()
		if apiSessionId != "" {
			apiSession, err := self.handler.getAppEnv().Handlers.ApiSession.Read(apiSessionId)
			if apiSession != nil && apiSession.IdentityId == self.identity.Id {
				self.apiSession = apiSession

				if _, err := self.handler.getAppEnv().GetHandlers().ApiSession.MarkActivityByTokens(self.apiSession.Token); err != nil {
					logger.WithError(err).Error("unexpected error while marking api session activity")
				}
				return false
			}

			if !boltz.IsErrNotFoundErr(err) {
				self.err = internalError(err)
				return false
			}
			logger.WithField("apiSessionId", apiSessionId).Info("api session not found, creating new api session")
			state.clearCurrentApiSessionId()
		}

		state.createApiSessionLock.Lock()
		defer state.createApiSessionLock.Unlock()

		// If none are passed in use the cached set. If the cached set is empty, use 'all'
		if len(configTypes) == 0 {
			configTypes = state.configTypes

			if len(configTypes) == 0 {
				configTypes = []string{"all"}
			}
		}

		apiSession := &model.ApiSession{
			Token:          uuid.NewString(),
			IdentityId:     self.identity.Id,
			ConfigTypes:    self.handler.getAppEnv().Handlers.ConfigType.MapConfigTypeNamesToIds(configTypes, self.identity.Id),
			LastActivityAt: time.Now(),
		}

		var err error
		apiSession.Id, err = self.handler.getAppEnv().GetHandlers().ApiSession.Create(apiSession)
		if err != nil {
			self.err = internalError(err)
			return false
		}

		apiSession, err = self.handler.getAppEnv().GetHandlers().ApiSession.Read(apiSession.Id)
		if err != nil {
			self.err = internalError(err)
			return false
		}

		self.apiSession = apiSession
		state.setCurrentApiSessionId(apiSession.Id)
		state.configTypes = configTypes
		if self.logContext != nil {
			self.logContext.WithField("apiSessionId", apiSession.Id)
		}
		return true
	}
	return false
}

func (self *baseTunnelRequestContext) loadServiceForName(name string) {
	if self.err == nil {
		var err error
		self.service, err = self.handler.getAppEnv().Handlers.EdgeService.ReadByName(name)

		if err != nil {
			if boltz.IsErrNotFoundErr(err) {
				err = InvalidServiceError{}
			} else {
				err = internalError(err)
			}

			logrus.
				WithField("apiSessionId", self.apiSession.Id).
				WithField("operation", self.handler.Label()).
				WithField("router", self.sourceRouter.Name).
				WithField("serviceName", name).
				WithError(self.err).
				Error("service not found")
		}
	}
}

func (self *baseTunnelRequestContext) ensureSessionForService(sessionId, sessionType string) {
	if self.err == nil {
		logger := logrus.
			WithField("operation", self.handler.Label()).
			WithField("router", self.sourceRouter.Name)

		if sessionId != "" {
			session, err := self.handler.getAppEnv().Handlers.Session.Read(sessionId)
			if err != nil {
				if !boltz.IsErrNotFoundErr(err) {
					self.err = internalError(err)
					return
				}
			}
			if session != nil {
				if session.ServiceId == self.service.Id && session.ApiSessionId == self.apiSession.Id && session.Type == sessionType {
					self.session = session
					return
				}
				logger.Errorf("required session did not match service or api session. "+
					"session.id=%v session.type=%v session.serviceId=%v session.apiSessionId=%v "+
					"requested type=%v serviceId=%v apiSessionId=%v",
					session.Id, session.Type, session.ServiceId, session.ApiSessionId, sessionType, self.service.Id, self.apiSession.Id)
			}
		}

		session := &model.Session{
			Token:        uuid.NewString(),
			ApiSessionId: self.apiSession.Id,
			ServiceId:    self.service.Id,
			IdentityId: self.identity.Id,
			Type:         sessionType,
		}

		id, err := self.handler.getAppEnv().Handlers.Session.Create(session)
		if err != nil {
			self.err = internalError(err)
			return
		}

		self.session, err = self.handler.getAppEnv().Handlers.Session.Read(id)
		if err != nil {
			err = internalError(err)
			return
		}
		self.newSession = true
		if self.logContext != nil {
			self.logContext.WithField("sessionId", self.session.Id)
		}
	}
}

func (self *baseTunnelRequestContext) getCreateApiSessionResponse() (*edge_ctrl_pb.CreateApiSessionResponse, error) {
	precedence := edge_ctrl_pb.TerminatorPrecedence_Default
	if self.identity.DefaultHostingPrecedence == ziti.PrecedenceRequired {
		precedence = edge_ctrl_pb.TerminatorPrecedence_Required
	} else if self.identity.DefaultHostingPrecedence == ziti.PrecedenceFailed {
		precedence = edge_ctrl_pb.TerminatorPrecedence_Failed
	}

	appDataJson, err := mapToJson(self.identity.AppData)
	if err != nil {
		return nil, err
	}

	return &edge_ctrl_pb.CreateApiSessionResponse{
		SessionId:                self.apiSession.Id,
		Token:                    self.apiSession.Token,
		RefreshIntervalSeconds:   uint32((self.apiSession.ExpirationDuration - (10 * time.Second)).Seconds()),
		IdentityId:               self.identity.Id,
		IdentityName:             self.identity.Name,
		DefaultHostingPrecedence: precedence,
		DefaultHostingCost:       uint32(self.identity.DefaultHostingCost),
		AppDataJson:              appDataJson,
	}, nil
}

func mapToJson(m map[string]interface{}) (string, error) {
	if len(m) == 0 {
		return "", nil
	}

	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	err := encoder.Encode(m)
	return buf.String(), err
}

func (self *baseTunnelRequestContext) getCreateSessionResponse() *edge_ctrl_pb.CreateSessionResponse {
	return &edge_ctrl_pb.CreateSessionResponse{
		SessionId: self.session.Id,
		Token:     self.session.Token,
	}
}

func (self *baseTunnelRequestContext) updateIdentityInfo(envInfo *edge_ctrl_pb.EnvInfo, sdkInfo *edge_ctrl_pb.SdkInfo) {
	if self.err == nil {
		if envInfo != nil {
			self.identity.EnvInfo = &model.EnvInfo{}
			self.identity.EnvInfo.Arch = envInfo.Arch
			self.identity.EnvInfo.Os = envInfo.Os
			self.identity.EnvInfo.OsRelease = envInfo.OsRelease
			self.identity.EnvInfo.OsVersion = envInfo.OsVersion
		}

		if sdkInfo != nil {
			self.identity.SdkInfo = &model.SdkInfo{}
			self.identity.SdkInfo.AppId = sdkInfo.AppId
			self.identity.SdkInfo.AppVersion = sdkInfo.AppVersion
			self.identity.SdkInfo.Branch = sdkInfo.Branch
			self.identity.SdkInfo.Revision = sdkInfo.Revision
			self.identity.SdkInfo.Type = sdkInfo.Type
			self.identity.SdkInfo.Version = sdkInfo.Version
		}

		if envInfo != nil || sdkInfo != nil {
			self.err = internalError(self.handler.getAppEnv().GetHandlers().Identity.PatchInfo(self.identity))
		}
	}
}
