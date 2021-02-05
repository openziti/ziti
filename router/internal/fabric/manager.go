/*
	Copyright NetFoundry, Inc.

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
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/edge/runner"
	"github.com/openziti/foundation/channel2"
	cmap "github.com/orcaman/concurrent-map"
	"strings"
	"sync"
	"time"
)

const (
	EventAddedNetworkSession   = "AddedNetworkSession"
	EventUpdatedNetworkSession = "UpdatedNetworkSession"
	EventRemovedNetworkSession = "RemovedNetworkSession"

	EventAddedSession   = "AddedSession"
	EventUpdatedSession = "UpdatedSession"
	EventRemovedSession = "RemovedSession"
)

type RemoveListener func()

type DisconnectCB func(token string)

type StateManager interface {
	//"Network" Sessions
	GetSession(token string) *edge_ctrl_pb.Session
	GetSessionWithTimeout(token string, timeout time.Duration) *edge_ctrl_pb.Session
	AddSession(ns *edge_ctrl_pb.Session)
	UpdateSession(ns *edge_ctrl_pb.Session)
	RemoveSession(token string)
	RemoveMissingSessions(knownSessions []*edge_ctrl_pb.Session)
	AddSessionRemovedListener(token string, callBack func(token string)) RemoveListener

	//ApiSessions
	GetApiSession(token string) *edge_ctrl_pb.ApiSession
	GetApiSessionWithTimeout(token string, timeout time.Duration) *edge_ctrl_pb.ApiSession
	AddApiSession(ns *edge_ctrl_pb.ApiSession)
	UpdateApiSession(ns *edge_ctrl_pb.ApiSession)
	RemoveApiSession(token string)
	RemoveMissingApiSessions(knownSessions []*edge_ctrl_pb.ApiSession)
	AddConnectedApiSession(token string, removeCB func(), ch channel2.Channel)
	RemoveConnectedApiSession(token string, underlay channel2.Channel)
	AddApiSessionRemovedListener(token string, callBack func(token string)) RemoveListener

	StartHeartbeat(channel channel2.Channel, seconds int)
}

type StateManagerImpl struct {
	sessionsByToken    *sync.Map //"network" sessions
	apiSessionsByToken *sync.Map
	activeApiSessions  cmap.ConcurrentMap

	Hostname       string
	ControllerAddr string
	ClusterId      string
	NodeId         string
	events.EventEmmiter
	heartbeatRunner    runner.Runner
	heartbeatOperation *heartbeatOperation
}

var singleStateManager StateManager

func GetStateManager() StateManager {
	if singleStateManager != nil {
		return singleStateManager
	}

	singleStateManager = &StateManagerImpl{
		EventEmmiter:       events.New(),
		sessionsByToken:    &sync.Map{},
		apiSessionsByToken: &sync.Map{},
		activeApiSessions:  cmap.New(),
	}

	return singleStateManager
}

func (sm *StateManagerImpl) AddApiSession(session *edge_ctrl_pb.ApiSession) {
	pfxlog.Logger().Debugf("adding session [%s] fingerprints [%s]", session.Token, session.CertFingerprints)
	sm.apiSessionsByToken.Store(session.Token, session)
	sm.Emit(EventAddedSession, session)
}

func (sm *StateManagerImpl) UpdateApiSession(session *edge_ctrl_pb.ApiSession) {
	pfxlog.Logger().Debugf("updating session [%s] fingerprints [%s]", session.Token, session.CertFingerprints)
	sm.apiSessionsByToken.Store(session.Token, session)

	sm.sessionsByToken.Range(func(key, value interface{}) bool {
		if ns, ok := value.(*edge_ctrl_pb.Session); ok {
			if ns.ApiSessionId == session.Id { //only update the specific api session's sessions
				ns.CertFingerprints = session.CertFingerprints //session.CertFingerprints is all currently valid
			}
		} else {
			pfxlog.Logger().Warn("could not convert value from concurrent map sessionsByToken to Session, this should not happen")
		}
		return true
	})

	sm.Emit(EventUpdatedSession, session)
}

func (sm *StateManagerImpl) RemoveApiSession(token string) {
	if ns, ok := sm.apiSessionsByToken.Load(token); ok {
		pfxlog.Logger().Debugf("removing session [%s]", token)
		sm.apiSessionsByToken.Delete(token)
		eventName := sm.getSessionRemovedEventName(token)
		sm.Emit(eventName)
		sm.RemoveAllListeners(eventName)
		sm.Emit(EventRemovedSession, ns)
	} else {
		pfxlog.Logger().Debugf("could not remove session [%s]; not found", token)
	}
}
func (sm *StateManagerImpl) RemoveMissingApiSessions(knownSessions []*edge_ctrl_pb.ApiSession) {
	validTokens := map[string]bool{}
	for _, session := range knownSessions {
		validTokens[session.Token] = true
	}

	var tokensToRemove []string
	sm.apiSessionsByToken.Range(func(key, _ interface{}) bool {
		token, _ := key.(string)
		if _, ok := validTokens[token]; !ok {
			tokensToRemove = append(tokensToRemove, token)
		}
		return true
	})

	for _, token := range tokensToRemove {
		sm.RemoveApiSession(token)
	}
}

func (sm *StateManagerImpl) AddSession(ns *edge_ctrl_pb.Session) {
	// BACKWARDS_COMPATIBILITY introduced 0.15.2
	// support 0.14 (and older) style controller generated fingerprints, as fingerprint format changed from 0.14 to 0.15
	for i := 0; i < len(ns.CertFingerprints); i++ {
		ns.CertFingerprints[i] = strings.Replace(strings.ToLower(ns.CertFingerprints[i]), ":", "", -1)
	}

	pfxlog.Logger().Debugf("adding network session [%s] fingerprints [%s] TypeId [%v]", ns.Token, ns.CertFingerprints, ns.Type.String())
	sm.sessionsByToken.Store(ns.Token, ns)
	sm.Emit(EventAddedNetworkSession, ns)
}

func (sm *StateManagerImpl) UpdateSession(ns *edge_ctrl_pb.Session) {
	// BACKWARDS_COMPATIBILITY introduced 0.15.2
	// support 0.14 (and older) style controller generated fingerprints, as fingerprint format changed from 0.14 to 0.15
	for i := 0; i < len(ns.CertFingerprints); i++ {
		ns.CertFingerprints[i] = strings.Replace(strings.ToLower(ns.CertFingerprints[i]), ":", "", -1)
	}

	pfxlog.Logger().Debugf("updating network session [%s] fingerprints [%s]", ns.Token, ns.CertFingerprints)
	sm.sessionsByToken.Store(ns.Token, ns)
	sm.Emit(EventUpdatedNetworkSession, ns)
}

func (sm *StateManagerImpl) RemoveMissingSessions(knownSessions []*edge_ctrl_pb.Session) {
	validTokens := map[string]bool{}
	for _, ns := range knownSessions {
		validTokens[ns.Token] = true
	}

	var tokensToRemove []string
	sm.sessionsByToken.Range(func(key, _ interface{}) bool {
		token, _ := key.(string)
		if _, ok := validTokens[token]; !ok {
			tokensToRemove = append(tokensToRemove, token)
		}
		return true
	})

	for _, token := range tokensToRemove {
		sm.RemoveSession(token)
	}
}

func (sm *StateManagerImpl) RemoveSession(token string) {
	if ns, ok := sm.sessionsByToken.Load(token); ok {
		pfxlog.Logger().Debugf("removing network session [%s]", token)
		sm.sessionsByToken.Delete(token)
		sm.Emit(EventRemovedNetworkSession, ns)
		eventName := sm.getNetworkSessionRemovedEventName(token)
		sm.Emit(eventName)
		sm.RemoveAllListeners(eventName)
	} else {
		pfxlog.Logger().Debugf("could not remove network session [%s]; not found", token)
	}
}

func (sm *StateManagerImpl) GetApiSessionWithTimeout(token string, timeout time.Duration) *edge_ctrl_pb.ApiSession {
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

func (sm *StateManagerImpl) GetApiSession(token string) *edge_ctrl_pb.ApiSession {
	if val, ok := sm.apiSessionsByToken.Load(token); ok {
		if session, ok := val.(*edge_ctrl_pb.ApiSession); ok {
			return session
		}
	}
	return nil
}

func (sm *StateManagerImpl) GetSession(token string) *edge_ctrl_pb.Session {
	if obj, ok := sm.sessionsByToken.Load(token); ok {
		if ns, ok := obj.(*edge_ctrl_pb.Session); ok {
			return ns
		}
		pfxlog.Logger().Panic("encountered non-network session in network session map")
	}

	return nil
}

func (sm *StateManagerImpl) GetSessionWithTimeout(token string, timeout time.Duration) *edge_ctrl_pb.Session {
	deadline := time.Now().Add(timeout)
	session := sm.GetSession(token)

	if session == nil {
		//convert this to return a channel instead of sleeping
		waitTime := time.Millisecond
		for time.Now().Before(deadline) {
			session = sm.GetSession(token)
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

func (sm *StateManagerImpl) AddSessionRemovedListener(token string, callBack func(token string)) RemoveListener {
	eventName := sm.getNetworkSessionRemovedEventName(token)
	listener := func(args ...interface{}) {
		callBack(token)
	}
	sm.AddListener(eventName, listener)

	return func() {
		go sm.RemoveListener(eventName, listener) // likely to be called from Emit, which will cause a deadlock
	}
}

func (sm *StateManagerImpl) AddApiSessionRemovedListener(token string, callBack func(token string)) RemoveListener {
	eventName := sm.getSessionRemovedEventName(token)
	listener := func(args ...interface{}) {
		callBack(token)
	}
	sm.AddListener(eventName, listener)

	return func() {
		go sm.RemoveListener(eventName, listener) // likely to be called from Emit, which will cause a deadlock
	}
}

func (sm *StateManagerImpl) getNetworkSessionRemovedEventName(token string) events.EventName {
	eventName := EventRemovedNetworkSession + "-" + token
	return events.EventName(eventName)
}

func (sm *StateManagerImpl) getSessionRemovedEventName(token string) events.EventName {
	eventName := EventRemovedSession + "-" + token
	return events.EventName(eventName)
}

func (sm *StateManagerImpl) StartHeartbeat(ctrl channel2.Channel, intervalSeconds int) {
	sm.heartbeatOperation = newHeartbeatOperation(ctrl, time.Duration(intervalSeconds)*time.Second, sm)

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

	if err := sm.heartbeatRunner.Start(); err != nil {
		pfxlog.Logger().WithError(err).Panic("could not start heartbeat runner")
	}

	pfxlog.Logger().Info("heartbeat starting")
}

func (sm *StateManagerImpl) AddConnectedApiSession(token string, removeCB func(), ch channel2.Channel) {
	var sessions *MapWithMutex

	for sessions == nil {
		if val, ok := sm.activeApiSessions.Get(token); ok {
			if sessions, ok = val.(*MapWithMutex); !ok {
				pfxlog.Logger().Panic("could not convert to active sessions")
			}
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

func (sm *StateManagerImpl) RemoveConnectedApiSession(token string, ch channel2.Channel) {
	if val, ok := sm.activeApiSessions.Get(token); ok {
		sessions, ok := val.(*MapWithMutex)

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

func (sm *StateManagerImpl) ActiveSessionTokens() []string {
	return sm.activeApiSessions.Keys()
}

func newMapWithMutex() *MapWithMutex {
	return &MapWithMutex{
		m: map[channel2.Channel]func(){},
	}
}

type MapWithMutex struct {
	sync.Mutex
	m map[channel2.Channel]func()
}

func (self *MapWithMutex) Put(ch channel2.Channel, f func()) {
	self.Lock()
	defer self.Unlock()
	self.m[ch] = f
}
