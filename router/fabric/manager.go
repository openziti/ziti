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

package fabric

import (
	"bufio"
	"crypto/sha1"
	"crypto/x509"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/common/runner"
	"github.com/openziti/ziti/controller/oidc_auth"
	"github.com/openziti/ziti/router/env"
	cmap "github.com/orcaman/concurrent-map/v2"
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

type StateManager interface {
	//"Network" Sessions
	RemoveEdgeSession(token string)
	AddEdgeSessionRemovedListener(token string, callBack func(token string)) RemoveListener
	WasSessionRecentlyRemoved(token string) bool

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

	AddSignerPublicCert(keys [][]byte)

	StartHeartbeat(env env.RouterEnv, seconds int, closeNotify <-chan struct{})
	ValidateSessions(ch channel.Channel, chunkSize uint32, minInterval, maxInterval time.Duration)

	DumpApiSessions(c *bufio.ReadWriter) error
	MarkSyncInProgress(trackerId string)
	MarkSyncStopped(trackerId string)
	IsSyncInProgress() bool
}

type StateManagerImpl struct {
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
	signerPublicCerts  cmap.ConcurrentMap[string, *x509.Certificate]
}

func (sm *StateManagerImpl) AddSignerPublicCert(keys [][]byte) {
	added := 0
	ignored := 0

	for _, key := range keys {
		cert, err := x509.ParseCertificate(key)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not parse signer public key")
			continue
		}

		kid := fmt.Sprintf("%x", sha1.Sum(key))
		sm.signerPublicCerts.Upsert(kid, cert, func(exist bool, valueInMap *x509.Certificate, newValue *x509.Certificate) *x509.Certificate {
			if exist {
				ignored = ignored + 1
			} else {
				added = added + 1
			}
			return valueInMap
		})
	}

	pfxlog.Logger().WithField("received", len(keys)).WithField("added", added).WithField("ignored", ignored).Info("received signer public certificates")

}

func (sm *StateManagerImpl) MarkSyncInProgress(trackerId string) {
	sm.syncLock.Lock()
	defer sm.syncLock.Unlock()
	sm.currentSync = trackerId
}

func (sm *StateManagerImpl) MarkSyncStopped(trackerId string) {
	sm.syncLock.Lock()
	defer sm.syncLock.Unlock()
	if sm.currentSync == trackerId {
		sm.currentSync = ""
	}
}

func (sm *StateManagerImpl) IsSyncInProgress() bool {
	sm.syncLock.Lock()
	defer sm.syncLock.Unlock()
	return sm.currentSync == ""
}

func NewStateManager() StateManager {
	return &StateManagerImpl{
		EventEmmiter:            events.New(),
		apiSessionsByToken:      cmap.New[*edge_ctrl_pb.ApiSession](),
		activeApiSessions:       cmap.New[*MapWithMutex](),
		sessions:                cmap.New[uint32](),
		recentlyRemovedSessions: cmap.New[time.Time](),
		signerPublicCerts:       cmap.New[*x509.Certificate](),
	}
}

func (sm *StateManagerImpl) AddApiSession(apiSession *edge_ctrl_pb.ApiSession) {
	pfxlog.Logger().
		WithField("apiSessionId", apiSession.Id).
		WithField("apiSessionToken", apiSession.Token).
		WithField("apiSessionCertFingerprints", apiSession.CertFingerprints).
		Debug("adding apiSession")
	sm.apiSessionsByToken.Set(apiSession.Token, apiSession)
	sm.Emit(EventAddedApiSession, apiSession)
}

func (sm *StateManagerImpl) UpdateApiSession(apiSession *edge_ctrl_pb.ApiSession) {
	pfxlog.Logger().
		WithField("apiSessionId", apiSession.Id).
		WithField("apiSessionToken", apiSession.Token).
		WithField("apiSessionCertFingerprints", apiSession.CertFingerprints).
		Debug("updating apiSession")
	sm.apiSessionsByToken.Set(apiSession.Token, apiSession)
	sm.Emit(EventUpdatedApiSession, apiSession)
}

func (sm *StateManagerImpl) RemoveApiSession(token string) {
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
func (sm *StateManagerImpl) RemoveMissingApiSessions(knownApiSessions []*edge_ctrl_pb.ApiSession, beforeSessionId string) {
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

func (sm *StateManagerImpl) RemoveEdgeSession(token string) {
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

func (sm *StateManagerImpl) GetApiSessionWithTimeout(token string, timeout time.Duration) *ApiSession {
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
	Claims   *oidc_auth.AccessClaims
}

func (sm *StateManagerImpl) GetApiSession(token string) *ApiSession {
	if strings.HasPrefix(token, oidc_auth.JwtTokenPrefix) {
		accessClaims := &oidc_auth.AccessClaims{}
		jwtToken, err := jwt.ParseWithClaims(token, accessClaims, sm.keyFunc)

		if err == nil {
			if accessClaims.Type != oidc_auth.TokenTypeAccess {
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

func (sm *StateManagerImpl) WasSessionRecentlyRemoved(token string) bool {
	return sm.recentlyRemovedSessions.Has(token)
}

func (sm *StateManagerImpl) AddEdgeSessionRemovedListener(token string, callBack func(token string)) RemoveListener {
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

func (sm *StateManagerImpl) SessionConnectionClosed(token string) {
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

func (sm *StateManagerImpl) AddApiSessionRemovedListener(token string, callBack func(token string)) RemoveListener {
	eventName := sm.getApiSessionRemovedEventName(token)
	listener := func(args ...interface{}) {
		callBack(token)
	}
	sm.AddListener(eventName, listener)

	return func() {
		go sm.RemoveListener(eventName, listener) // likely to be called from Emit, which will cause a deadlock
	}
}

func (sm *StateManagerImpl) getEdgeSessionRemovedEventName(token string) events.EventName {
	eventName := EventRemovedEdgeSession + "-" + token
	return events.EventName(eventName)
}

func (sm *StateManagerImpl) getApiSessionRemovedEventName(token string) events.EventName {
	eventName := EventRemovedApiSession + "-" + token
	return events.EventName(eventName)
}

func (sm *StateManagerImpl) StartHeartbeat(env env.RouterEnv, intervalSeconds int, closeNotify <-chan struct{}) {
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

func (sm *StateManagerImpl) AddConnectedApiSession(token string) {
	sm.activeApiSessions.Upsert(token, nil, func(exist bool, valueInMap *MapWithMutex, newValue *MapWithMutex) *MapWithMutex {
		if exist {
			return valueInMap
		}
		return newMapWithMutex()
	})
}

func (sm *StateManagerImpl) RemoveConnectedApiSession(token string) {
	sm.activeApiSessions.Remove(token)
}

func (sm *StateManagerImpl) AddConnectedApiSessionWithChannel(token string, removeCB func(), ch channel.Channel) {
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

func (sm *StateManagerImpl) RemoveConnectedApiSessionWithChannel(token string, ch channel.Channel) {
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

func (sm *StateManagerImpl) ActiveApiSessionTokens() []string {
	return sm.activeApiSessions.Keys()
}

func (sm *StateManagerImpl) flushRecentlyRemoved() {
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

func (sm *StateManagerImpl) DumpApiSessions(c *bufio.ReadWriter) error {
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

func (sm *StateManagerImpl) ValidateSessions(ch channel.Channel, chunkSize uint32, minInterval, maxInterval time.Duration) {
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

func (sm *StateManagerImpl) keyFunc(token *jwt.Token) (interface{}, error) {
	kidVal, kidHeaderFound := token.Header["kid"]

	if !kidHeaderFound {
		return nil, fmt.Errorf("token does not have kid header, unable to lookup")
	}

	kid := kidVal.(string)

	key, keyFound := sm.signerPublicCerts.Get(kid)

	if !keyFound {
		sm.RefreshSigners()

		key, keyFound = sm.signerPublicCerts.Get(kid)

		if !keyFound {
			return nil, fmt.Errorf("key for kid %s not found after refresh", kid)
		}
	}

	return key, nil
}

func (sm *StateManagerImpl) RefreshSigners() {
	//nothing atm
}
