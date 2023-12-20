package handler_edge_ctrl

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/model"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

func NewTunnelState() *TunnelState {
	sessionCache, _ := lru.New[string, string](256)
	return &TunnelState{
		sessionCache: sessionCache,
	}
}

type TunnelState struct {
	configTypes          []string
	currentApiSessionId  concurrenz.AtomicValue[string]
	createApiSessionLock sync.Mutex
	sessionCache         *lru.Cache[string, string]
}

func (self *TunnelState) getCurrentApiSessionId() string {
	return self.currentApiSessionId.Load()
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
		self.identity, err = self.handler.getAppEnv().GetManagers().Identity.Read(self.sourceRouter.Id)
		if err != nil {
			if boltz.IsErrNotFoundErr(err) {
				self.err = TunnelingNotEnabledError{}
			} else {
				self.err = internalError(err)
			}
			return
		}

		if self.identity.IdentityTypeId != db.RouterIdentityType {
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
	return self.ensureApiSessionLocking(configTypes, false)
}

func (self *baseTunnelRequestContext) ensureApiSessionLocking(configTypes []string, locked bool) bool {
	if self.err == nil {
		logger := logrus.
			WithField("operation", self.handler.Label()).
			WithField("router", self.sourceRouter.Name)

		state := self.getTunnelState()
		apiSessionId := state.getCurrentApiSessionId()
		if apiSessionId != "" {
			apiSession, err := self.handler.getAppEnv().Managers.ApiSession.Read(apiSessionId)
			if apiSession != nil && apiSession.IdentityId == self.identity.Id {
				self.apiSession = apiSession

				if _, _, err := self.handler.getAppEnv().GetManagers().ApiSession.MarkLastActivityByTokens(self.apiSession.Token); err != nil {
					logger.WithError(err).Error("unexpected error while marking api session activity")
				}
				return false
			}

			if err != nil && !boltz.IsErrNotFoundErr(err) {
				self.err = internalError(err)
				return false
			}

			logger.WithField("apiSessionId", apiSessionId).Info("api session not found, creating new api session")
			state.clearCurrentApiSessionId()
		}

		if !locked {
			state.createApiSessionLock.Lock()
			defer state.createApiSessionLock.Unlock()
			return self.ensureApiSessionLocking(configTypes, true)
		}

		// If none are passed in use the cached set. If the cached set is empty, use 'all'
		if len(configTypes) == 0 {
			configTypes = state.configTypes

			if len(configTypes) == 0 {
				configTypes = []string{"all"}
			}
		}

		identityMgr := self.handler.getAppEnv().Managers.Identity
		if cachedApiSessionId, _ := identityMgr.GetAnnotation(self.identity.Id, "apiSessionId"); cachedApiSessionId != nil {
			apiSession, _ := self.handler.getAppEnv().Managers.ApiSession.Read(*cachedApiSessionId)
			if apiSession != nil && apiSession.IdentityId == self.identity.Id {
				self.apiSession = apiSession
				if _, _, err := self.handler.getAppEnv().GetManagers().ApiSession.MarkLastActivityByTokens(self.apiSession.Token); err != nil {
					logger.WithError(err).Error("unexpected error while marking api session activity")
				}
				state.setCurrentApiSessionId(apiSession.Id)
				return true
			}
		}

		apiSession := &model.ApiSession{
			Token:          uuid.NewString(),
			IdentityId:     self.identity.Id,
			ConfigTypes:    self.handler.getAppEnv().Managers.ConfigType.MapConfigTypeNamesToIds(configTypes, self.identity.Id),
			LastActivityAt: time.Now(),
			IPAddress:      self.handler.getChannel().Underlay().GetRemoteAddr().String(),
		}

		err := self.handler.getAppEnv().GetDbProvider().GetDb().Update(self.newTunnelChangeContext().NewMutateContext(), func(ctx boltz.MutateContext) error {
			var err error
			apiSession.Id, err = self.handler.getAppEnv().GetManagers().ApiSession.Create(ctx, apiSession, nil)
			if err != nil {
				return err
			}

			if err = identityMgr.Annotate(ctx, self.identity.Id, "apiSessionId", apiSession.Id); err != nil {
				logger.WithError(err).Error("failed to cache new api session on router identity")
			}

			apiSession, err = self.handler.getAppEnv().GetManagers().ApiSession.ReadInTx(ctx.Tx(), apiSession.Id)
			return err
		})

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
		self.service, err = self.handler.getAppEnv().Managers.EdgeService.ReadByName(name)

		if err != nil {
			if boltz.IsErrNotFoundErr(err) {
				self.err = InvalidServiceError{}
			} else {
				self.err = internalError(err)
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

func (self *baseTunnelRequestContext) isSessionValid(sessionId, sessionType string) bool {
	logger := logrus.
		WithField("operation", self.handler.Label()).
		WithField("router", self.sourceRouter.Name).
		WithField("routerId", self.sourceRouter.Id)

	if sessionId != "" {
		session, err := self.handler.getAppEnv().Managers.Session.Read(sessionId)
		if err != nil {
			if !boltz.IsErrNotFoundErr(err) {
				self.err = internalError(err)
				return false
			}
		}
		if session != nil {
			if session.ServiceId == self.service.Id && session.ApiSessionId == self.apiSession.Id && session.Type == sessionType {
				self.session = session
				return true
			}
			logger.Infof("required session did not match service or api session. "+
				"session.id=%v session.type=%v session.serviceId=%v session.apiSessionId=%v "+
				"requested type=%v serviceId=%v apiSessionId=%v",
				session.Id, session.Type, session.ServiceId, session.ApiSessionId, sessionType, self.service.Id, self.apiSession.Id)
		}
	}
	return false
}

func (self *baseTunnelRequestContext) ensureSessionForService(sessionId, sessionType string) {
	if self.err == nil {
		logger := logrus.
			WithField("operation", self.handler.Label()).
			WithField("router", self.sourceRouter.Name).
			WithField("routerId", self.sourceRouter.Id).
			WithField("sessionType", sessionType)

		if self.isSessionValid(sessionId, sessionType) {
			logger.WithField("sessionId", sessionId).Debug("session valid")
			return
		}

		cacheKey := self.service.Id + "." + sessionType
		logger = logger.WithField("cacheKey", cacheKey)

		if sessionId, found := self.getTunnelState().sessionCache.Get(cacheKey); found {
			if self.isSessionValid(sessionId, sessionType) {
				logger.WithField("sessionId", sessionId).Debug("found valid cached session")
				self.newSession = true
				if self.logContext != nil {
					self.logContext.WithField("sessionId", self.session.Id)
				}
				return
			}
			logger.WithField("sessionId", sessionId).Debug("found invalid cached session")
		}

		session := &model.Session{
			Token:        uuid.NewString(),
			ApiSessionId: self.apiSession.Id,
			ServiceId:    self.service.Id,
			IdentityId:   self.identity.Id,
			Type:         sessionType,
		}

		id, err := self.handler.getAppEnv().Managers.Session.Create(session, self.newTunnelChangeContext())
		if err != nil {
			self.err = internalError(err)
			return
		}

		self.session, err = self.handler.getAppEnv().Managers.Session.Read(id)
		if err != nil {
			self.err = internalError(err)
			return
		}
		self.newSession = true
		if self.logContext != nil {
			self.logContext.WithField("sessionId", self.session.Id)
		}

		self.getTunnelState().sessionCache.Add(cacheKey, self.session.Id)
		logger.WithField("sessionId", sessionId).Debug("created new session")
	}
}

func (self *baseTunnelRequestContext) getCreateApiSessionResponse() (*edge_ctrl_pb.CreateApiSessionResponse, error) {
	appDataJson, err := mapToJson(self.identity.AppData)
	if err != nil {
		return nil, err
	}

	servicePrecedences := map[string]edge_ctrl_pb.TerminatorPrecedence{}
	for k, v := range self.identity.ServiceHostingPrecedences {
		servicePrecedences[k] = edge_ctrl_pb.GetPrecedence(v)
	}

	serviceCosts := map[string]uint32{}
	for k, v := range self.identity.ServiceHostingCosts {
		serviceCosts[k] = uint32(v)
	}

	return &edge_ctrl_pb.CreateApiSessionResponse{
		SessionId:                self.apiSession.Id,
		Token:                    self.apiSession.Token,
		RefreshIntervalSeconds:   uint32((self.apiSession.ExpirationDuration - (10 * time.Second)).Seconds()),
		IdentityId:               self.identity.Id,
		IdentityName:             self.identity.Name,
		DefaultHostingPrecedence: edge_ctrl_pb.GetPrecedence(self.identity.DefaultHostingPrecedence),
		DefaultHostingCost:       uint32(self.identity.DefaultHostingCost),
		AppDataJson:              appDataJson,
		ServicePrecedences:       servicePrecedences,
		ServiceCosts:             serviceCosts,
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
		updateIdentity := false
		if envInfo != nil {
			newEnvInfo := &model.EnvInfo{
				Arch:      envInfo.Arch,
				Os:        envInfo.Os,
				OsRelease: envInfo.OsRelease,
				OsVersion: envInfo.OsVersion,
			}
			if !self.identity.EnvInfo.Equals(newEnvInfo) {
				self.identity.EnvInfo = newEnvInfo
				updateIdentity = true
			}
		}

		if sdkInfo != nil {
			newSdkInfo := &model.SdkInfo{
				AppId:      sdkInfo.AppId,
				AppVersion: sdkInfo.AppVersion,
				Branch:     sdkInfo.Branch,
				Revision:   sdkInfo.Revision,
				Type:       sdkInfo.Type,
				Version:    sdkInfo.Version,
			}
			if !self.identity.SdkInfo.Equals(newSdkInfo) {
				self.identity.SdkInfo = newSdkInfo
				updateIdentity = true
			}
		}

		if updateIdentity {
			self.err = internalError(self.handler.getAppEnv().GetManagers().Identity.PatchInfo(self.identity, self.newTunnelChangeContext()))
		}
	}
}
