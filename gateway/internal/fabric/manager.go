/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-edge/pb/edge_ctrl_pb"
	"github.com/netfoundry/ziti-edge/runner"
	"github.com/netfoundry/ziti-foundation/channel2"
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
	AddSession(ns *edge_ctrl_pb.Session)
	UpdateSession(ns *edge_ctrl_pb.Session)
	RemoveSession(token string)
	RemoveMissingSessions(knownSessions []*edge_ctrl_pb.Session)
	AddApiSession(ns *edge_ctrl_pb.ApiSession)
	UpdateApiSession(ns *edge_ctrl_pb.ApiSession)
	RemoveApiSession(token string)
	RemoveMissingApiSessions(knownSessions []*edge_ctrl_pb.ApiSession)
	GetNetworkSession(token string) *edge_ctrl_pb.Session
	GetNetworkSessionWithTimeout(token string, timeout time.Duration) *edge_ctrl_pb.Session
	GetSessionByFingerprint(fingerprint string) chan *edge_ctrl_pb.ApiSession
	GetSession(token string) chan *edge_ctrl_pb.ApiSession
	AddNetworkSessionRemovedListener(token string, callBack func(token string)) RemoveListener
	AddSessionRemovedListener(token string, callBack func(token string)) RemoveListener
	StartHeartbeat(channel channel2.Channel)
	AddConnectedSession(token string, removeCB func(), ch channel2.Channel)
	RemoveConnectedSession(token string, underlay channel2.Channel)
}

type ProtocolEndpointMap map[string]string

type StateManagerImpl struct {
	networkSessionsByToken *sync.Map
	sessionsByToken        *sync.Map
	supportedFabrics       *sync.Map
	activeSessions         *sync.Map
	ProtocolEndpoint       ProtocolEndpointMap
	Hostname               string
	ControllerAddr         string
	ClusterId              string
	NodeId                 string
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
		EventEmmiter:           events.New(),
		networkSessionsByToken: &sync.Map{},
		sessionsByToken:        &sync.Map{},
		supportedFabrics:       &sync.Map{},
		activeSessions:         &sync.Map{},
	}

	return singleStateManager
}

func (sm *StateManagerImpl) AddApiSession(session *edge_ctrl_pb.ApiSession) {
	pfxlog.Logger().Debugf("adding session [%s] fingerprints [%s]", session.Token, session.CertFingerprints)
	sm.sessionsByToken.Store(session.Token, session)
	sm.Emit(EventAddedSession, session)
}

func (sm *StateManagerImpl) UpdateApiSession(session *edge_ctrl_pb.ApiSession) {
	pfxlog.Logger().Debugf("updating session [%s] fingerprints [%s]", session.Token, session.CertFingerprints)
	sm.sessionsByToken.Store(session.Token, session)
	sm.Emit(EventUpdatedSession, session)
}

func (sm *StateManagerImpl) RemoveApiSession(token string) {
	if ns, ok := sm.sessionsByToken.Load(token); ok {
		pfxlog.Logger().Debugf("removing session [%s]", token)
		sm.sessionsByToken.Delete(token)
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
	sm.sessionsByToken.Range(func(key, _ interface{}) bool {
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
	pfxlog.Logger().Debugf("adding network session [%s] fingerprints [%s] hosting? [%v]", ns.Token, ns.CertFingerprints, ns.Hosting)
	sm.networkSessionsByToken.Store(ns.Token, ns)
	sm.Emit(EventAddedNetworkSession, ns)
}

func (sm *StateManagerImpl) UpdateSession(ns *edge_ctrl_pb.Session) {
	pfxlog.Logger().Debugf("updating network session [%s] fingerprints [%s]", ns.Token, ns.CertFingerprints)
	sm.networkSessionsByToken.Store(ns.Token, ns)
	sm.Emit(EventUpdatedNetworkSession, ns)
}

func (sm *StateManagerImpl) RemoveMissingSessions(knownSessions []*edge_ctrl_pb.Session) {
	validTokens := map[string]bool{}
	for _, ns := range knownSessions {
		validTokens[ns.Token] = true
	}

	var tokensToRemove []string
	sm.networkSessionsByToken.Range(func(key, _ interface{}) bool {
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
	if ns, ok := sm.networkSessionsByToken.Load(token); ok {
		pfxlog.Logger().Debugf("removing network session [%s]", token)
		sm.networkSessionsByToken.Delete(token)
		sm.Emit(EventRemovedNetworkSession, ns)
		eventName := sm.getNetworkSessionRemovedEventName(token)
		sm.Emit(eventName)
		sm.RemoveAllListeners(eventName)
	} else {
		pfxlog.Logger().Debugf("could not remove network session [%s]; not found", token)
	}
}

func (sm *StateManagerImpl) GetSession(token string) chan *edge_ctrl_pb.ApiSession {
	ch := make(chan *edge_ctrl_pb.ApiSession)
	go func() {
		if val, ok := sm.sessionsByToken.Load(token); ok {
			if session, ok := val.(*edge_ctrl_pb.ApiSession); ok {
				ch <- session
				return
			}
		}
		ch <- nil
	}()
	return ch
}

func (sm *StateManagerImpl) GetSessionByFingerprint(fingerprint string) chan *edge_ctrl_pb.ApiSession {
	ch := make(chan *edge_ctrl_pb.ApiSession)
	go func() {
		found := false
		sm.sessionsByToken.Range(func(_, value interface{}) bool {
			if session, ok := value.(*edge_ctrl_pb.ApiSession); ok {
				for _, curPrint := range session.CertFingerprints {
					if fingerprint == curPrint {
						ch <- session
						found = true
						return false
					}
				}
			}
			return true
		})

		if !found {
			ch <- nil
		}
	}()

	return ch
}

func (sm *StateManagerImpl) GetNetworkSession(token string) *edge_ctrl_pb.Session {
	if obj, ok := sm.networkSessionsByToken.Load(token); ok {
		if ns, ok := obj.(*edge_ctrl_pb.Session); ok {
			return ns
		}
		pfxlog.Logger().Panic("encountered non-network session in network session map")
	}

	return nil
}

func (sm *StateManagerImpl) GetNetworkSessionWithTimeout(token string, timeout time.Duration) *edge_ctrl_pb.Session {
	deadline := time.Now().Add(timeout)
	session := sm.GetNetworkSession(token)

	if session == nil {
		//convert this to return a channel instead of sleeping
		waitTime := time.Millisecond
		for time.Now().Before(deadline) {
			session = sm.GetNetworkSession(token)
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

func (sm *StateManagerImpl) AddNetworkSessionRemovedListener(token string, callBack func(token string)) RemoveListener {
	eventName := sm.getNetworkSessionRemovedEventName(token)
	listener := func(args ...interface{}) {
		callBack(token)
	}
	sm.AddListener(eventName, listener)

	return func() {
		sm.RemoveListener(eventName, listener)
	}
}

func (sm *StateManagerImpl) AddSessionRemovedListener(token string, callBack func(token string)) RemoveListener {
	eventName := sm.getSessionRemovedEventName(token)
	listener := func(args ...interface{}) {
		callBack(token)
	}
	sm.AddListener(eventName, listener)

	return func() {
		sm.RemoveListener(eventName, listener)
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

func (sm *StateManagerImpl) StartHeartbeat(ctrl channel2.Channel) {
	sm.heartbeatOperation = newHeartbeatOperation(ctrl, 5*time.Second, sm)

	var err error
	sm.heartbeatRunner, err = runner.NewRunner(1*time.Second, 30*time.Second, func(e error, operation runner.Operation) {
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

func (sm *StateManagerImpl) AddConnectedSession(token string, removeCB func(), ch channel2.Channel) {
	var sessions map[channel2.Channel]func()

	if val, ok := sm.activeSessions.Load(token); ok {
		if sessions, ok = val.(map[channel2.Channel]func()); !ok {
			pfxlog.Logger().Panic("could not convert to active sessions")
		}
	} else {
		sessions = map[channel2.Channel]func(){}
	}
	sessions[ch] = removeCB

	sm.activeSessions.Store(token, sessions)
}

func (sm *StateManagerImpl) RemoveConnectedSession(token string, ch channel2.Channel) {
	if val, ok := sm.activeSessions.Load(token); ok {
		sessions, ok := val.(map[channel2.Channel]func())

		if !ok {
			pfxlog.Logger().Panic("could not convert active sessions to map")
		}

		if removeCB, found := sessions[ch]; found {
			removeCB()
			delete(sessions, ch)
		}

		if len(sessions) == 0 {
			sm.activeSessions.Delete(token)
		}
	}
}

func (sm *StateManagerImpl) ActiveSessionTokens() []string {
	var tokens []string

	sm.activeSessions.Range(func(key, _ interface{}) bool {
		if token, ok := key.(string); ok {
			tokens = append(tokens, token)
		} else {
			pfxlog.Logger().WithField("value", key).Panic("could not convert key to token")
		}
		return true
	})

	return tokens
}
