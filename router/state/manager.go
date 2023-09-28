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
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/router/env"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/common/runner"
	"github.com/openziti/ziti/controller/oidc_auth"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const (
	EventRemovedEdgeSession = "RemovedEdgeSession"

	EventAddedApiSession   = "AddedApiSession"
	EventUpdatedApiSession = "UpdatedApiSession"
	EventRemovedApiSession = "RemovedApiSession"
)

type RemoveListener func()

type DisconnectCB func(token string)

type Manager interface {
	//"Network" Sessions
	RemoveEdgeSession(token string)
	AddEdgeSessionRemovedListener(token string, callBack func(token string)) RemoveListener

	//ApiSessions
	GetApiSession(token string) *ApiSession
	GetApiSessionWithTimeout(token string, timeout time.Duration) *ApiSession
	AddApiSession(apiSession *edge_ctrl_pb.ApiSession)
	UpdateApiSession(apiSession *edge_ctrl_pb.ApiSession)
	RemoveApiSession(token string)
	RemoveMissingApiSessions(knownSessions []*edge_ctrl_pb.ApiSession, beforeSessionId string)
	AddConnectedApiSession(token string)
	RemoveConnectedApiSession(token string)
	AddConnectedApiSessionWithChannel(token string, removeCB func(), ch channel.Channel)
	RemoveConnectedApiSessionWithChannel(token string, underlay channel.Channel)
	AddApiSessionRemovedListener(token string, callBack func(token string)) RemoveListener

	RouterDataModel() *common.RouterDataModel
	SetRouterDataModel(model *common.RouterDataModel)

	StartHeartbeat(env env.RouterEnv, seconds int, closeNotify <-chan struct{})
	ValidateSessions(ch channel.Channel, chunkSize uint32, minInterval, maxInterval time.Duration)

	DumpApiSessions(c *bufio.ReadWriter) error
	MarkSyncInProgress(trackerId string)
	MarkSyncStopped(trackerId string)
	IsSyncInProgress() bool

	VerifyClientCert(cert *x509.Certificate) error

	StartRouterModelSave(routerEnv env.RouterEnv, path string, duration time.Duration)
	LoadRouterModel(filePath string)
}

var _ Manager = (*ManagerImpl)(nil)

type ManagerImpl struct {
	apiSessionsByToken      cmap.ConcurrentMap[string, *edge_ctrl_pb.ApiSession]
	activeApiSessions       cmap.ConcurrentMap[string, *MapWithMutex]
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

	certCache       cmap.ConcurrentMap[string, *x509.Certificate]
	routerDataModel *common.RouterDataModel
}

func (sm *ManagerImpl) StartRouterModelSave(routerEnv env.RouterEnv, filePath string, duration time.Duration) {
	go func() {
		for {
			select {
			case <-routerEnv.GetCloseNotify():
				return
			case <-time.After(duration):
				sm.RouterDataModel().Save(filePath)
			}
		}
	}()
}

func (sm *ManagerImpl) LoadRouterModel(filePath string) {
	model, err := common.NewReceiverRouterDataModelFromFile(filePath)

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not load router model from file [%s]", filePath)
		model = common.NewReceiverRouterDataModel()
	}

	sm.SetRouterDataModel(model)
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

	for keysTuple := range sm.routerDataModel.PublicKeys.IterBuffered() {
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

	for keysTuple := range sm.routerDataModel.PublicKeys.IterBuffered() {
		if contains(keysTuple.Val.Usages, edge_ctrl_pb.DataState_PublicKey_JWTValidation) {
			if kid == keysTuple.Val.Kid {
				return sm.parsePublicKey(keysTuple.Val)
			}
		}
	}

	return nil, errors.New("public key not found")
}

func (sm *ManagerImpl) RouterDataModel() *common.RouterDataModel {
	if sm.routerDataModel == nil {
		sm.routerDataModel = common.NewReceiverRouterDataModel()
	}
	return sm.routerDataModel
}

func (sm *ManagerImpl) SetRouterDataModel(model *common.RouterDataModel) {
	sm.routerDataModel = model
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
	return sm.currentSync == ""
}

func NewManager() Manager {
	return &ManagerImpl{
		EventEmmiter:            events.New(),
		apiSessionsByToken:      cmap.New[*edge_ctrl_pb.ApiSession](),
		activeApiSessions:       cmap.New[*MapWithMutex](),
		sessions:                cmap.New[uint32](),
		recentlyRemovedSessions: cmap.New[time.Time](),
		certCache:               cmap.New[*x509.Certificate](),
	}
}

func (sm *ManagerImpl) AddApiSession(apiSession *edge_ctrl_pb.ApiSession) {
	pfxlog.Logger().
		WithField("apiSessionId", apiSession.Id).
		WithField("apiSessionToken", apiSession.Token).
		WithField("apiSessionCertFingerprints", apiSession.CertFingerprints).
		Debug("adding apiSession")
	sm.apiSessionsByToken.Set(apiSession.Token, apiSession)
	sm.Emit(EventAddedApiSession, apiSession)
}

func (sm *ManagerImpl) UpdateApiSession(apiSession *edge_ctrl_pb.ApiSession) {
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
	sm.apiSessionsByToken.IterCb(func(token string, apiSession *edge_ctrl_pb.ApiSession) {
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
	JwtToken *jwt.Token
	Claims   *common.AccessClaims
}

func (sm *ManagerImpl) GetApiSession(token string) *ApiSession {
	if strings.HasPrefix(token, oidc_auth.JwtTokenPrefix) {
		jwtToken, accessClaims, err := sm.ParseJwt(token)

		if err == nil {
			if accessClaims.Type != common.TokenTypeAccess {
				pfxlog.Logger().Errorf("provided a token with invalid type '%s'", accessClaims.Type)
				return nil
			}

			return &ApiSession{
				ApiSession: &edge_ctrl_pb.ApiSession{
					Token:            token,
					CertFingerprints: accessClaims.CertFingerprints,
					Id:               accessClaims.JWTID,
				},
				JwtToken: jwtToken,
				Claims:   accessClaims,
			}
		}

		//fall through to check if the token is a zt-session
	}

	if apiSession, ok := sm.apiSessionsByToken.Get(token); ok {
		return &ApiSession{
			ApiSession: apiSession,
		}
	}
	return nil
}

func (sm *ManagerImpl) AddEdgeSessionRemovedListener(token string, callBack func(token string)) RemoveListener {
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
	sm.activeApiSessions.Upsert(token, nil, func(exist bool, valueInMap *MapWithMutex, newValue *MapWithMutex) *MapWithMutex {
		if exist {
			return valueInMap
		}
		return newMapWithMutex()
	})
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
	return sm.activeApiSessions.Keys()
}

func (sm *ManagerImpl) flushRecentlyRemoved() {
	now := time.Now()
	var toRemove []string
	sm.recentlyRemovedSessions.IterCb(func(key string, t time.Time) {
		remove := false

		if now.Sub(t) >= time.Minute {
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

	return nil, fmt.Errorf("unsuported public key format: %s", publicKey.Format.String())
}
