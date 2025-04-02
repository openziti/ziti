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

package state

import (
	"bufio"
	"crypto"
	"crypto/x509"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/metrics"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/common/runner"
	"github.com/openziti/ziti/controller/oidc_auth"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xgress_common"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"math/rand"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	EventRemovedEdgeSession = "RemovedEdgeSession"

	EventAddedApiSession   = "AddedApiSession"
	EventUpdatedApiSession = "UpdatedApiSession"
	EventRemovedApiSession = "RemovedApiSession"

	RouterDataModelListerBufferSize = 100
	DefaultSubscriptionTimeout      = 5 * time.Minute
)

type RemoveListener func()

type Manager interface {
	//"Network" Sessions
	RemoveEdgeSession(token string)
	AddEdgeSessionRemovedListener(token string, callBack func(token string)) RemoveListener
	WasSessionRecentlyRemoved(token string) bool
	MarkSessionRecentlyRemoved(token string)

	//ApiSessions
	GetApiSession(token string) *ApiSession
	GetApiSessionWithTimeout(token string, timeout time.Duration) *ApiSession
	AddApiSession(apiSession *ApiSession)
	UpdateApiSession(apiSession *ApiSession)
	RemoveApiSession(token string)
	RemoveMissingApiSessions(knownSessions []*edge_ctrl_pb.ApiSession, beforeSessionId string)
	AddConnectedApiSession(token string)
	RemoveConnectedApiSession(token string)
	AddConnectedApiSessionWithChannel(token string, removeCB func(), ch channel.Channel)
	RemoveConnectedApiSessionWithChannel(token string, underlay channel.Channel)
	AddApiSessionRemovedListener(token string, callBack func(token string)) RemoveListener
	ParseJwt(jwtStr string) (*jwt.Token, *common.AccessClaims, error)

	RouterDataModel() *common.RouterDataModel
	SetRouterDataModel(model *common.RouterDataModel, resetSubscription bool)
	GetRouterDataModelPool() goroutines.Pool

	StartHeartbeat(env env.RouterEnv, seconds int, closeNotify <-chan struct{})
	ValidateSessions(ch channel.Channel, chunkSize uint32, minInterval, maxInterval time.Duration)

	DumpApiSessions(c *bufio.ReadWriter) error
	MarkSyncInProgress(trackerId string)
	MarkSyncStopped(trackerId string)
	IsSyncInProgress() bool

	VerifyClientCert(cert *x509.Certificate) error

	StartRouterModelSave(path string, duration time.Duration)
	LoadRouterModel(filePath string)

	AddActiveChannel(ch channel.Channel, session *ApiSession)
	RemoveActiveChannel(ch channel.Channel)
	GetApiSessionFromCh(ch channel.Channel) *ApiSession

	GetEnv() env.RouterEnv
	UpdateChApiSession(channel.Channel, *ApiSession) error

	GetCurrentDataModelSource() string

	env.Xrctrl
}

var _ Manager = (*ManagerImpl)(nil)

func NewManager(stateEnv env.RouterEnv) Manager {
	routerDataModelPoolConfig := goroutines.PoolConfig{
		QueueSize:   uint32(1000),
		MinWorkers:  1,
		MaxWorkers:  uint32(1),
		IdleTime:    30 * time.Second,
		CloseNotify: stateEnv.GetCloseNotify(),
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().
				WithField(logrus.ErrorKey, err).
				WithField("backtrace", string(debug.Stack())).Error("panic during router data model event")
		},
	}

	metrics.ConfigureGoroutinesPoolMetrics(&routerDataModelPoolConfig, stateEnv.GetMetricsRegistry(), "pool.rdm.handler")

	routerDataModelPool, err := goroutines.NewPool(routerDataModelPoolConfig)
	if err != nil {
		panic(errors.Wrap(err, "error creating rdm goroutine pool"))
	}

	result := &ManagerImpl{
		EventEmmiter:            events.New(),
		apiSessionsByToken:      cmap.New[*ApiSession](),
		activeApiSessions:       cmap.New[*MapWithMutex](),
		sessions:                cmap.New[uint32](),
		recentlyRemovedSessions: cmap.New[time.Time](),
		certCache:               cmap.New[*x509.Certificate](),
		activeChannels:          cmap.New[*ApiSession](),
		env:                     stateEnv,
		routerDataModelPool:     routerDataModelPool,
		endpointsChanged:        make(chan env.CtrlEvent, 10),
		modelChanged:            make(chan struct{}, 1),
	}

	cfg := stateEnv.GetConfig()
	result.LoadRouterModel(stateEnv.GetConfig().Edge.Db)

	stateEnv.GetNetworkControllers().AddChangeListener(env.CtrlEventListenerFunc(func(event env.CtrlEvent) {
		if event.Type != env.ControllerLeaderChange {
			select {
			case result.endpointsChanged <- event:
			default:
			}
		}
	}))

	go result.manageRouterDataModelSubscription()
	result.StartRouterModelSave(cfg.Edge.Db, cfg.Edge.DbSaveInterval)

	return result
}

type ManagerImpl struct {
	env                env.RouterEnv
	apiSessionsByToken cmap.ConcurrentMap[string, *ApiSession]

	activeApiSessions cmap.ConcurrentMap[string, *MapWithMutex]
	activeChannels    cmap.ConcurrentMap[string, *ApiSession]

	sessions                cmap.ConcurrentMap[string, uint32]
	recentlyRemovedSessions cmap.ConcurrentMap[string, time.Time]

	Hostname       string
	ControllerAddr string
	ClusterId      string
	NodeId         string
	events.EventEmmiter
	heartbeatRunner    runner.Runner
	heartbeatOperation *heartbeatOperation
	currentSync        string
	syncLock           sync.Mutex

	certCache           cmap.ConcurrentMap[string, *x509.Certificate]
	routerDataModel     atomic.Pointer[common.RouterDataModel]
	routerDataModelPool goroutines.Pool

	endpointsChanged    chan env.CtrlEvent
	modelChanged        chan struct{}
	dataModelSubCtrlId  concurrenz.AtomicValue[string]
	dataModelSubTimeout time.Time
}

func (self *ManagerImpl) GetCurrentDataModelSource() string {
	return self.dataModelSubCtrlId.Load()
}

func (self *ManagerImpl) manageRouterDataModelSubscription() {
	<-self.env.GetRouterDataModelEnabledConfig().GetInitNotifyChannel()
	if !self.env.IsRouterDataModelEnabled() {
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-self.env.GetCloseNotify():
			return
		case event := <-self.endpointsChanged:
			// if the controller we're subscribed to has changed, resubscribe
			if event.Controller.Channel().Id() == self.GetCurrentDataModelSource() {
				pfxlog.Logger().WithField("ctrlId", event.Controller.Channel().Id()).WithField("change", event.Type).
					Info("currently subscribed controller has changed, resubscribing")
				self.dataModelSubCtrlId.Store("")
			}
		case <-ticker.C:
		case <-self.modelChanged:
		}

		allEndpointChangesProcessed := false
		for !allEndpointChangesProcessed {
			select {
			case event := <-self.endpointsChanged:
				// if the controller we're subscribed to has changed, resubscribe
				if event.Controller.Channel().Id() == self.GetCurrentDataModelSource() {
					pfxlog.Logger().WithField("ctrlId", event.Controller.Channel().Id()).WithField("change", event.Type).
						Info("currently subscribed controller has changed, resubscribing")
					self.dataModelSubCtrlId.Store("")
				}
			default:
				allEndpointChangesProcessed = true
			}
		}

		self.checkRouterDataModelSubscription()
	}
}

func (self *ManagerImpl) checkRouterDataModelSubscription() {
	if !self.env.IsRouterDataModelRequired() {
		return
	}

	ctrl := self.env.GetNetworkControllers().GetNetworkController(self.dataModelSubCtrlId.Load())
	if ctrl == nil || time.Now().After(self.dataModelSubTimeout) {
		if bestCtrl := self.env.GetNetworkControllers().AnyCtrlChannel(); bestCtrl != nil {
			logger := pfxlog.Logger().WithField("ctrlId", bestCtrl.Id()).WithField("prevCtrlId", self.dataModelSubCtrlId.Load())
			if ctrl == nil {
				logger.Info("no current data model subscription active, subscribing")
			} else {
				logger.Info("current data model subscription expired, resubscribing")
			}
			self.subscribeToDataModelUpdates(bestCtrl)
		}
	} else if !ctrl.IsConnected() || ctrl.TimeSinceLastContact() > 30*time.Second {
		bestCtrl := self.env.GetNetworkControllers().AnyCtrlChannel()
		if bestCtrl != nil && bestCtrl.Id() != ctrl.Channel().Id() {
			pfxlog.Logger().WithField("ctrlId", bestCtrl.Id()).
				WithField("prevCtrlId", self.dataModelSubCtrlId.Load()).
				Info("current data model subscription source unreliable, changing subscription")
			self.subscribeToDataModelUpdates(bestCtrl)
		}
	}
}

func (self *ManagerImpl) subscribeToDataModelUpdates(ch channel.Channel) {
	renew := self.dataModelSubCtrlId.Load() == ch.Id()

	// if we store after success, we may miss an update because the ids don't match yet
	self.dataModelSubCtrlId.Store(ch.Id())

	var currentIndex uint64
	if rdm := self.routerDataModel.Load(); rdm != nil {
		currentIndex, _ = rdm.CurrentIndex()
	}

	timelineId := ""
	if rdm := self.routerDataModel.Load(); rdm != nil {
		timelineId = rdm.GetTimelineId()
	}

	subTimeout := time.Now().Add(DefaultSubscriptionTimeout)
	req := &edge_ctrl_pb.SubscribeToDataModelRequest{
		CurrentIndex:                currentIndex,
		SubscriptionDurationSeconds: uint32(DefaultSubscriptionTimeout.Seconds()),
		Renew:                       renew,
		TimelineId:                  timelineId,
	}

	logger := pfxlog.Logger().
		WithField("ctrlId", ch.Id()).
		WithField("currentIndex", req.CurrentIndex).
		WithField("renew", req.Renew)

	if err := protobufs.MarshalTyped(req).WithTimeout(self.env.GetNetworkControllers().DefaultRequestTimeout()).SendAndWaitForWire(ch); err != nil {
		self.dataModelSubCtrlId.Store("")
		logger.WithError(err).Error("error to subscribing to router data model changes")
	} else {
		logger.Info("subscribed to new controller for router data model changes")
		self.dataModelSubTimeout = subTimeout
	}
}

func (sm *ManagerImpl) GetRouterDataModelPool() goroutines.Pool {
	return sm.routerDataModelPool
}

func (sm *ManagerImpl) UpdateChApiSession(ch channel.Channel, newApiSession *ApiSession) error {
	if newApiSession == nil {
		return errors.New("nil api session")
	}

	if newApiSession.Claims == nil {
		return errors.New("nil api session claims")
	}

	if newApiSession.Claims.Type != common.TokenTypeAccess {
		return fmt.Errorf("bearer token is of invalid type: expected %s, got: %s", common.TokenTypeAccess, newApiSession.Claims.Type)
	}

	currentApiSession := sm.GetApiSessionFromCh(ch)

	if newApiSession.Claims.Subject != currentApiSession.Claims.Subject {
		return fmt.Errorf("bearer token subjects did not match, current: %s, new: %s", currentApiSession.Claims.Subject, newApiSession.Claims.Subject)
	}

	if newApiSession.Claims.ApiSessionId != currentApiSession.Claims.ApiSessionId {
		return fmt.Errorf("bearer token api session ids did not match, current: %s, new: %s", currentApiSession.Claims.ApiSessionId, newApiSession.Claims.ApiSessionId)
	}

	sm.activeChannels.Set(ch.Id(), newApiSession)

	return nil
}

func (sm *ManagerImpl) GetEnv() env.RouterEnv {
	return sm.env
}

func (sm *ManagerImpl) GetApiSessionFromCh(ch channel.Channel) *ApiSession {
	apiSession, _ := sm.activeChannels.Get(ch.Id())

	return apiSession
}

func (sm *ManagerImpl) AddActiveChannel(ch channel.Channel, session *ApiSession) {
	sm.activeChannels.Set(ch.Id(), session)
}

func (sm *ManagerImpl) RemoveActiveChannel(ch channel.Channel) {
	sm.activeChannels.Remove(ch.Id())
}

func (sm *ManagerImpl) StartRouterModelSave(filePath string, duration time.Duration) {
	go func() {
		for {
			select {
			case <-sm.env.GetCloseNotify():
				return
			case <-time.After(duration):
				sm.RouterDataModel().Save(filePath)
			}
		}
	}()
}

func (sm *ManagerImpl) LoadRouterModel(filePath string) {
	model, err := common.NewReceiverRouterDataModelFromFile(filePath, RouterDataModelListerBufferSize, sm.env.GetCloseNotify())

	if err != nil {
		if !os.IsNotExist(err) {
			pfxlog.Logger().WithError(err).Errorf("could not load router model from file [%s]", filePath)
		} else {
			pfxlog.Logger().Infof("router data model file does not exist [%s]", filePath)
		}
		model = common.NewReceiverRouterDataModel(RouterDataModelListerBufferSize, sm.env.GetCloseNotify())
	} else {
		index, _ := model.CurrentIndex()
		pfxlog.Logger().WithField("path", filePath).WithField("index", index).Info("loaded router model from file")
	}

	sm.SetRouterDataModel(model, false)
}

func contains[T comparable](values []T, element T) bool {
	for _, val := range values {
		if val == element {
			return true
		}
	}

	return false
}

func (sm *ManagerImpl) getX509FromData(kid string, data []byte) (*x509.Certificate, error) {
	if cert, found := sm.certCache.Get(kid); found {
		return cert, nil
	}

	cert, err := x509.ParseCertificate(data)

	if err != nil {
		return nil, err
	}

	sm.certCache.Set(kid, cert)

	return cert, nil
}

func (sm *ManagerImpl) VerifyClientCert(cert *x509.Certificate) error {

	rootPool := x509.NewCertPool()

	rdm := sm.routerDataModel.Load()

	for keysTuple := range rdm.PublicKeys.IterBuffered() {
		if contains(keysTuple.Val.Usages, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation) {
			cert, err := sm.getX509FromData(keysTuple.Val.Kid, keysTuple.Val.GetData())

			if err != nil {
				pfxlog.Logger().WithField("kid", keysTuple.Val.Kid).WithError(err).Error("could not parse x509 certificate data")
				continue
			}

			rootPool.AddCert(cert)
		}
	}

	opts := x509.VerifyOptions{
		Roots:         rootPool,
		Intermediates: x509.NewCertPool(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		CurrentTime:   cert.NotBefore,
	}

	if _, err := cert.Verify(opts); err != nil {
		return fmt.Errorf("could not verify client certificate %w", err)
	}

	return nil
}

func (sm *ManagerImpl) ParseJwt(jwtStr string) (*jwt.Token, *common.AccessClaims, error) {
	//pubKeyLookup also handles extJwtSigner.enabled checking
	accessClaims := &common.AccessClaims{}
	jwtToken, err := jwt.ParseWithClaims(jwtStr, accessClaims, sm.pubKeyLookup)

	if err != nil {
		return nil, nil, err
	}

	if accessClaims.Type == common.TokenTypeAccess {
		return jwtToken, accessClaims, nil
	}

	return nil, nil, fmt.Errorf("invalid access token type: %s", accessClaims.Type)
}

func (sm *ManagerImpl) pubKeyLookup(token *jwt.Token) (any, error) {
	kidVal, ok := token.Header["kid"]

	if !ok {
		return nil, errors.New("could not lookup JWT signer, kid header missing")
	}

	kid, ok := kidVal.(string)

	if !ok {
		return nil, fmt.Errorf("kid header value is not a string, got type %T", kidVal)
	}

	kid = strings.TrimSpace(kid)

	rdm := sm.routerDataModel.Load()
	publicKeys := rdm.PublicKeys.IterBuffered()
	for keysTuple := range publicKeys {
		if contains(keysTuple.Val.Usages, edge_ctrl_pb.DataState_PublicKey_JWTValidation) {

			if kid == keysTuple.Val.Kid {
				return sm.parsePublicKey(keysTuple.Val)
			}
		}
	}

	return nil, errors.New("public key not found")
}

func (sm *ManagerImpl) RouterDataModel() *common.RouterDataModel {
	return sm.routerDataModel.Load()
}

func (sm *ManagerImpl) SetRouterDataModel(model *common.RouterDataModel, resetSubscription bool) {
	index, _ := model.CurrentIndex()
	logger := pfxlog.Logger().WithField("index", index)

	publicKeys := model.PublicKeys.Items()
	logger.Debugf("number of public keys in rdm: %d", len(publicKeys))

	if resetSubscription {
		sm.dataModelSubCtrlId.Store("")
	}
	logger.Info("replacing router data model")
	existing := sm.routerDataModel.Swap(model)
	if existing != nil {
		existing.Stop()
		model.InheritLocalData(existing)
		existingIndex, _ := existing.CurrentIndex()
		logger = logger.WithField("existingIndex", existingIndex)
		if index < existingIndex {
			sm.env.GetIndexWatchers().NotifyOfIndexReset()
		}
	}
	model.SyncAllSubscribers()

	if resetSubscription {
		// notify subscription manager code to resubscribe with updated model and index
		select {
		case sm.modelChanged <- struct{}{}:
		default:
		}
	}

	logger.Infof("router data model replacement complete, old: %p, new: %p", existing, model)
}

func (sm *ManagerImpl) MarkSyncInProgress(trackerId string) {
	sm.syncLock.Lock()
	defer sm.syncLock.Unlock()
	sm.currentSync = trackerId
}

func (sm *ManagerImpl) MarkSyncStopped(trackerId string) {
	sm.syncLock.Lock()
	defer sm.syncLock.Unlock()
	if sm.currentSync == trackerId {
		sm.currentSync = ""
	}
}

func (sm *ManagerImpl) IsSyncInProgress() bool {
	sm.syncLock.Lock()
	defer sm.syncLock.Unlock()
	return sm.currentSync != ""
}

func (sm *ManagerImpl) AddApiSession(apiSession *ApiSession) {
	pfxlog.Logger().
		WithField("apiSessionId", apiSession.Id).
		WithField("apiSessionToken", apiSession.Token).
		WithField("apiSessionCertFingerprints", apiSession.CertFingerprints).
		Debug("adding apiSession")
	sm.apiSessionsByToken.Set(apiSession.Token, apiSession)
	sm.Emit(EventAddedApiSession, apiSession)
}

func (sm *ManagerImpl) UpdateApiSession(apiSession *ApiSession) {
	pfxlog.Logger().
		WithField("apiSessionId", apiSession.Id).
		WithField("apiSessionToken", apiSession.Token).
		WithField("apiSessionCertFingerprints", apiSession.CertFingerprints).
		Debug("updating apiSession")
	sm.apiSessionsByToken.Set(apiSession.Token, apiSession)
	sm.Emit(EventUpdatedApiSession, apiSession)
}

func (sm *ManagerImpl) RemoveApiSession(token string) {
	if ns, ok := sm.apiSessionsByToken.Get(token); ok {
		pfxlog.Logger().WithField("apiSessionToken", token).Debug("removing api session")
		sm.apiSessionsByToken.Remove(token)
		eventName := sm.getApiSessionRemovedEventName(token)
		sm.Emit(eventName)
		sm.RemoveAllListeners(eventName)
		sm.Emit(EventRemovedApiSession, ns)
	} else {
		pfxlog.Logger().WithField("apiSessionToken", token).Debug("could not remove api session")
	}
}

// RemoveMissingApiSessions removes API Sessions not present in the knownApiSessions argument. If the beforeSessionId
// value is not empty string, it will be used as a monotonic comparison between it and  API session ids. API session ids
// later than the sync will be ignored.
func (sm *ManagerImpl) RemoveMissingApiSessions(knownApiSessions []*edge_ctrl_pb.ApiSession, beforeSessionId string) {
	validTokens := map[string]bool{}
	for _, apiSession := range knownApiSessions {
		validTokens[apiSession.Token] = true
	}

	var tokensToRemove []string
	sm.apiSessionsByToken.IterCb(func(token string, apiSession *ApiSession) {
		if _, ok := validTokens[token]; !ok && (beforeSessionId == "" || apiSession.Id <= beforeSessionId) {
			tokensToRemove = append(tokensToRemove, token)
		}
	})

	for _, token := range tokensToRemove {
		sm.RemoveApiSession(token)
	}
}

func (sm *ManagerImpl) RemoveEdgeSession(token string) {
	pfxlog.Logger().WithField("sessionToken", token).Debug("removing network session")
	eventName := sm.getEdgeSessionRemovedEventName(token)
	sm.Emit(eventName)

	sm.RemoveAllListeners(eventName)
	sm.sessions.RemoveCb(token, func(key string, _ uint32, exists bool) bool {
		if exists {
			sm.recentlyRemovedSessions.Set(token, time.Now())
		}
		return true
	})

}

func (sm *ManagerImpl) GetApiSessionWithTimeout(token string, timeout time.Duration) *ApiSession {
	deadline := time.Now().Add(timeout)
	session := sm.GetApiSession(token)

	if session == nil {
		//convert this to return a channel instead of sleeping
		waitTime := time.Millisecond
		for time.Now().Before(deadline) {
			session = sm.GetApiSession(token)
			if session != nil {
				return session
			}
			time.Sleep(waitTime)
			if waitTime < time.Second {
				waitTime = waitTime * 2
			}
		}
	}
	return session
}

type ApiSession struct {
	*edge_ctrl_pb.ApiSession
	JwtToken     *jwt.Token
	Claims       *common.AccessClaims
	ControllerId string //used for non HA API Sessions
}

func (a *ApiSession) SelectCtrlCh(ctrls env.NetworkControllers) channel.Channel {
	if a == nil {
		return nil
	}

	if a.ControllerId != "" {
		return ctrls.GetCtrlChannel(a.ControllerId)
	}

	return ctrls.AnyCtrlChannel()
}

func (a *ApiSession) SelectModelUpdateCtrlCh(ctrls env.NetworkControllers) channel.Channel {
	if a == nil {
		return nil
	}

	if a.ControllerId != "" {
		return ctrls.GetCtrlChannel(a.ControllerId)
	}

	return ctrls.GetModelUpdateCtrlChannel()
}

func NewApiSessionFromToken(jwtToken *jwt.Token, accessClaims *common.AccessClaims) (*ApiSession, error) {
	subj, err := jwtToken.Claims.GetSubject()
	if err != nil {
		return nil, fmt.Errorf("unable to get the api session identity from the JWT subject (%w)", err)
	}
	jwtToken.Claims.(*common.AccessClaims).Subject = subj
	return &ApiSession{
		ApiSession: &edge_ctrl_pb.ApiSession{
			Token:            jwtToken.Raw,
			CertFingerprints: accessClaims.CertFingerprints,
			Id:               accessClaims.ApiSessionId,
			IdentityId:       subj,
		},
		JwtToken: jwtToken,
		Claims:   accessClaims,
	}, nil
}

func (sm *ManagerImpl) GetApiSession(token string) *ApiSession {
	if strings.HasPrefix(token, oidc_auth.JwtTokenPrefix) {
		jwtToken, accessClaims, err := sm.ParseJwt(token)

		if err == nil {
			if !accessClaims.HasAudience(common.ClaimAudienceOpenZiti) && !accessClaims.HasAudience(common.ClaimLegacyNative) {
				pfxlog.Logger().Errorf("provided a token with invalid audience '%s' of type [%T], expected: %s or %s", accessClaims.Audience, accessClaims.Audience, common.ClaimAudienceOpenZiti, common.ClaimLegacyNative)
				return nil
			}

			if accessClaims.Type != common.TokenTypeAccess {
				pfxlog.Logger().Errorf("provided a token with invalid type '%s'", accessClaims.Type)
				return nil
			}

			if apiSession, err := NewApiSessionFromToken(jwtToken, accessClaims); err != nil {
				pfxlog.Logger().WithError(err).Error("failed to create api session from JWT")
				return nil
			} else {
				return apiSession
			}
		} else {
			pfxlog.Logger().WithError(err).Error("JWT validation failed")
			return nil
		}
	}

	if apiSession, ok := sm.apiSessionsByToken.Get(token); ok {
		return apiSession
	}
	return nil
}

func (sm *ManagerImpl) WasSessionRecentlyRemoved(token string) bool {
	return sm.recentlyRemovedSessions.Has(token)
}

func (sm *ManagerImpl) MarkSessionRecentlyRemoved(token string) {
	sm.recentlyRemovedSessions.Set(token, time.Now())
}

func (sm *ManagerImpl) AddEdgeSessionRemovedListener(token string, callBack func(token string)) RemoveListener {
	if xgress_common.IsBearerToken(token) {
		return func() {}
	}

	if sm.recentlyRemovedSessions.Has(token) {
		go callBack(token) // callback can be long process with network traffic. Don't block event processing
		return func() {}
	}

	sm.sessions.Upsert(token, 0, func(exist bool, valueInMap uint32, newValue uint32) uint32 {
		if !exist {
			return uint32(1)
		}
		return valueInMap + 1
	})

	eventName := sm.getEdgeSessionRemovedEventName(token)

	listener := func(args ...interface{}) {
		go callBack(token) // callback can be long process with network traffic. Don't block event processing
	}
	sm.AddListener(eventName, listener)

	once := &sync.Once{}
	return func() {
		once.Do(func() {
			sm.SessionConnectionClosed(token)
			go sm.RemoveListener(eventName, listener) // likely to be called from Emit, which will cause a deadlock
		})
	}
}

func (sm *ManagerImpl) SessionConnectionClosed(token string) {
	sm.sessions.Upsert(token, 0, func(exist bool, valueInMap uint32, newValue uint32) uint32 {
		if !exist {
			return uint32(0)
		}
		return valueInMap + 1
	})

	sm.sessions.RemoveCb(token, func(key string, v uint32, exists bool) bool {
		if !exists {
			return false
		}

		if v == 0 {
			sm.recentlyRemovedSessions.Set(token, time.Now())
			return true
		}

		return false
	})
}

func (sm *ManagerImpl) AddApiSessionRemovedListener(token string, callBack func(token string)) RemoveListener {
	eventName := sm.getApiSessionRemovedEventName(token)
	listener := func(args ...interface{}) {
		callBack(token)
	}
	sm.AddListener(eventName, listener)

	return func() {
		go sm.RemoveListener(eventName, listener) // likely to be called from Emit, which will cause a deadlock
	}
}

func (sm *ManagerImpl) getEdgeSessionRemovedEventName(token string) events.EventName {
	eventName := EventRemovedEdgeSession + "-" + token
	return events.EventName(eventName)
}

func (sm *ManagerImpl) getApiSessionRemovedEventName(token string) events.EventName {
	eventName := EventRemovedApiSession + "-" + token
	return events.EventName(eventName)
}

func (sm *ManagerImpl) StartHeartbeat(env env.RouterEnv, intervalSeconds int, closeNotify <-chan struct{}) {
	sm.heartbeatOperation = newHeartbeatOperation(env, time.Duration(intervalSeconds)*time.Second, sm)

	var err error
	sm.heartbeatRunner, err = runner.NewRunner(1*time.Second, 24*time.Hour, func(e error, operation runner.Operation) {
		pfxlog.Logger().WithError(err).Error("error during heartbeat runner")
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Panic("could not create heartbeat runner")
	}

	if err := sm.heartbeatRunner.AddOperation(sm.heartbeatOperation); err != nil {
		pfxlog.Logger().WithError(err).Panic("could not add heartbeat operation to runner")
	}

	if err := sm.heartbeatRunner.Start(closeNotify); err != nil {
		pfxlog.Logger().WithError(err).Panic("could not start heartbeat runner")
	}

	pfxlog.Logger().Info("heartbeat starting")
}

func (sm *ManagerImpl) AddConnectedApiSession(token string) {
	sm.activeApiSessions.Set(token, nil)
}

func (sm *ManagerImpl) RemoveConnectedApiSession(token string) {
	sm.activeApiSessions.Remove(token)
}

func (sm *ManagerImpl) AddConnectedApiSessionWithChannel(token string, removeCB func(), ch channel.Channel) {
	var sessions *MapWithMutex

	for sessions == nil {
		if sessions, _ = sm.activeApiSessions.Get(token); sessions != nil {
			sessions.Put(ch, removeCB)
		} else {
			sessions = newMapWithMutex()
			sessions.Put(ch, removeCB)
			if !sm.activeApiSessions.SetIfAbsent(token, sessions) {
				sessions = nil
			}
		}
	}
}

func (sm *ManagerImpl) RemoveConnectedApiSessionWithChannel(token string, ch channel.Channel) {
	if sessions, ok := sm.activeApiSessions.Get(token); ok {
		if !ok {
			pfxlog.Logger().Panic("could not convert active sessions to map")
		}

		sessions.Lock()
		defer sessions.Unlock()

		if removeCB, found := sessions.m[ch]; found {
			removeCB()
			delete(sessions.m, ch)
		}

		if len(sessions.m) == 0 {
			sm.activeApiSessions.Remove(token)
		}
	}
}

func (sm *ManagerImpl) ActiveApiSessionTokens() []string {
	var toClose []func()
	var activeKeys []string
	for i := range sm.activeApiSessions.IterBuffered() {
		token := i.Key
		chMutex := i.Val
		if chMutex == nil {
			// An xgress_edge_tunnel api-session won't have associated channels
			activeKeys = append(activeKeys, token)
		} else {
			chMutex.Visit(func(ch channel.Channel, closeCb func()) {
				if ch.IsClosed() {
					toClose = append(toClose, closeCb)
				} else {
					activeKeys = append(activeKeys, token)
				}
			})
		}
	}

	for _, f := range toClose {
		f()
	}

	return activeKeys
}

func (sm *ManagerImpl) flushRecentlyRemoved() {
	now := time.Now()
	var toRemove []string
	sm.recentlyRemovedSessions.IterCb(func(key string, t time.Time) {
		remove := false

		if now.Sub(t) >= 5*time.Minute {
			remove = true
		}

		if remove {
			toRemove = append(toRemove, key)
		}
	})

	for _, key := range toRemove {
		sm.recentlyRemovedSessions.Remove(key)
	}
}

func (sm *ManagerImpl) DumpApiSessions(c *bufio.ReadWriter) error {
	ch := make(chan string, 15)

	go func() {
		defer close(ch)
		i := 0
		deadline := time.After(time.Second)
		timedOut := false

		for _, session := range sm.apiSessionsByToken.Items() {
			i++
			val := fmt.Sprintf("%v: id: %v, token: %v\n", i, session.Id, session.Token)
			select {
			case ch <- val:
			case <-deadline:
				timedOut = true
				break
			}
			if i%10000 == 0 {
				// allow a second to dump each 10k entries
				deadline = time.After(time.Second)
			}

		}

		if timedOut {
			select {
			case ch <- "timed out":
			case <-time.After(time.Second):
			}
		}
	}()

	for val := range ch {
		if _, err := c.WriteString(val); err != nil {
			return err
		}
	}
	return c.Flush()
}

func newMapWithMutex() *MapWithMutex {
	return &MapWithMutex{
		m: map[channel.Channel]func(){},
	}
}

type MapWithMutex struct {
	sync.Mutex
	m map[channel.Channel]func()
}

func (self *MapWithMutex) Put(ch channel.Channel, f func()) {
	self.Lock()
	defer self.Unlock()
	self.m[ch] = f
}

func (self *MapWithMutex) Visit(cb func(ch channel.Channel, closeCb func())) {
	self.Lock()
	defer self.Unlock()

	for ch, closeCb := range self.m {
		cb(ch, closeCb)
	}
}

func (sm *ManagerImpl) ValidateSessions(ch channel.Channel, chunkSize uint32, minInterval, maxInterval time.Duration) {
	sessionTokens := sm.sessions.Keys()

	for len(sessionTokens) > 0 {
		var chunk []string

		if len(sessionTokens) > int(chunkSize) {
			chunk = sessionTokens[:chunkSize]
			sessionTokens = sessionTokens[chunkSize:]
		} else {
			chunk = sessionTokens
			sessionTokens = nil
		}

		request := &edge_ctrl_pb.ValidateSessionsRequest{
			SessionTokens: chunk,
		}

		logrus.Debugf("validating edge sessions: %v", chunk)

		body, err := proto.Marshal(request)
		if err != nil {
			logrus.WithError(err).Error("failed to marshal validate sessions request")
			return
		}

		msg := channel.NewMessage(request.GetContentType(), body)
		if err := ch.Send(msg); err != nil {
			logrus.WithError(err).Error("failed to send validate sessions request")
			return
		}

		if len(sessionTokens) > 0 {
			interval := minInterval
			if minInterval < maxInterval {
				/* #nosec */
				delta := rand.Int63n(int64(maxInterval - minInterval))
				interval += minInterval + time.Duration(delta)
			}
			time.Sleep(interval)
		}
	}

}

func (sm *ManagerImpl) parsePublicKey(publicKey *edge_ctrl_pb.DataState_PublicKey) (crypto.PublicKey, error) {
	switch publicKey.Format {
	case edge_ctrl_pb.DataState_PublicKey_X509CertDer:
		certs, err := x509.ParseCertificates(publicKey.Data)
		if err != nil {
			return nil, err
		}

		if len(certs) == 0 {
			return nil, errors.New("could not parse certificates, der was empty")
		}

		return certs[0].PublicKey, nil
	case edge_ctrl_pb.DataState_PublicKey_PKIXPublicKey:
		return x509.ParsePKIXPublicKey(publicKey.Data)
	}

	return nil, fmt.Errorf("unsupported public key format: %s", publicKey.Format.String())
}

func (sm *ManagerImpl) LoadConfig(cfgmap map[interface{}]interface{}) error {
	return nil
}

func (sm *ManagerImpl) BindChannel(binding channel.Binding) error {
	binding.AddTypedReceiveHandler(NewSessionRemovedHandler(sm))
	binding.AddTypedReceiveHandler(NewApiSessionAddedHandler(sm, binding))
	binding.AddTypedReceiveHandler(NewApiSessionRemovedHandler(sm))
	binding.AddTypedReceiveHandler(NewApiSessionUpdatedHandler(sm))
	binding.AddTypedReceiveHandler(NewDataStateHandler(sm))
	binding.AddTypedReceiveHandler(NewDataStateEventHandler(sm))
	binding.AddTypedReceiveHandler(NewValidateDataStateRequestHandler(sm, sm.env))
	return nil
}

func (sm *ManagerImpl) Enabled() bool {
	return true
}

func (sm *ManagerImpl) Run(env.RouterEnv) error {
	return nil
}

func (sm *ManagerImpl) NotifyOfReconnect(ch channel.Channel) {
}

func (sm *ManagerImpl) GetTraceDecoders() []channel.TraceMessageDecoder {
	return nil
}
