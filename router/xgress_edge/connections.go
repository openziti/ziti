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

package xgress_edge

import (
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/common/spiffehlp"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/state"
	cmap "github.com/orcaman/concurrent-map/v2"
	"strings"
	"sync"
	"time"
)

type identityConnect struct {
	srcAddr     string
	dstAddr     string
	connectTime int64
}

type identityState struct {
	sync.Mutex
	connects     []identityConnect
	disconnects  []time.Time
	connectCount int
}

func (self *identityState) markConnect(srcAddr, dstAddr string) {
	self.Lock()
	defer self.Unlock()
	self.connects = append(self.connects, identityConnect{srcAddr: srcAddr, dstAddr: dstAddr, connectTime: time.Now().UnixMilli()})
	self.connectCount++
}

func (self *identityState) markDisconnect() {
	self.Lock()
	defer self.Unlock()
	self.disconnects = append(self.disconnects, time.Now())
	self.connectCount--
}

func (self *identityState) getConnectedStateEvent(id string) *edge_ctrl_pb.ConnectEvents_IdentityConnectEvents {
	self.Lock()
	defer self.Unlock()
	return &edge_ctrl_pb.ConnectEvents_IdentityConnectEvents{
		IdentityId:  id,
		IsConnected: self.connectCount > 0,
	}
}

func (self *identityState) getStateEvent(id string) (*edge_ctrl_pb.ConnectEvents_IdentityConnectEvents, bool) {
	self.Lock()
	defer self.Unlock()

	if self.connects == nil && self.disconnects == nil {
		return nil, self.connectCount > 0
	}

	result := &edge_ctrl_pb.ConnectEvents_IdentityConnectEvents{
		IdentityId:  id,
		IsConnected: self.connectCount > 0,
	}

	for _, t := range self.connects {
		result.ConnectTimes = append(result.ConnectTimes, &edge_ctrl_pb.ConnectEvents_ConnectDetails{
			ConnectTime: t.connectTime,
			SrcAddr:     t.srcAddr,
			DstAddr:     t.dstAddr,
		})
	}

	self.connects = nil
	self.disconnects = nil

	return result, result.IsConnected
}

type connectionTracker struct {
	lock           sync.Mutex
	controllers    env.NetworkControllers
	states         cmap.ConcurrentMap[string, *identityState]
	needsFullSync  map[string]channel.Channel
	notifyFullSync chan struct{}
	queued         []*edge_ctrl_pb.ConnectEvents
	maxQueueSize   int
}

func newConnectionTracker(env env.RouterEnv) *connectionTracker {
	result := &connectionTracker{
		controllers:    env.GetNetworkControllers(),
		states:         cmap.New[*identityState](),
		needsFullSync:  map[string]channel.Channel{},
		notifyFullSync: make(chan struct{}, 1),
		maxQueueSize:   10,
	}

	go result.runLoop(env.GetCloseNotify())

	return result
}

func (self *connectionTracker) runLoop(closeNotify <-chan struct{}) {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			self.report()
			self.sendFullSync()
		case <-self.notifyFullSync:
			self.sendFullSync()
		case <-closeNotify:
			return
		}
	}
}

func (self *connectionTracker) notifyNeedsFullSync() {
	select {
	case self.notifyFullSync <- struct{}{}:
	default:
	}
}

func (self *connectionTracker) markConnected(identityId string, srcAddr string, dstAddr string) {
	self.states.Upsert(identityId, nil, func(exist bool, valueInMap *identityState, newValue *identityState) *identityState {
		if valueInMap == nil {
			valueInMap = &identityState{}
		}
		valueInMap.markConnect(srcAddr, dstAddr)
		return valueInMap
	})
}

func (self *connectionTracker) markDisconnected(identityId string) {
	self.states.Upsert(identityId, nil, func(exist bool, valueInMap *identityState, newValue *identityState) *identityState {
		if valueInMap == nil {
			valueInMap = &identityState{}
		}
		valueInMap.markDisconnect()
		return valueInMap
	})
}

func (self *connectionTracker) report() {
	self.lock.Lock()
	defer self.lock.Unlock()

	var removeCheck []string
	evts := &edge_ctrl_pb.ConnectEvents{
		FullState: false,
	}

	self.states.IterCb(func(key string, v *identityState) {
		evt, connected := v.getStateEvent(key)
		if !connected {
			removeCheck = append(removeCheck, key)
		}
		if evt != nil {
			evts.Events = append(evts.Events, evt)
		}
	})

	for _, k := range removeCheck {
		self.states.RemoveCb(k, func(key string, v *identityState, exists bool) bool {
			if v == nil {
				return true
			}

			v.Lock()
			defer v.Unlock()
			return v.connectCount == 0 && v.connects == nil && v.disconnects == nil
		})
	}

	if len(evts.Events) > 0 {
		self.queued = append(self.queued, evts)
		if len(self.queued) > self.maxQueueSize {
			self.queued = self.queued[1:]
		}
	}

	self.sendQueued()
}

func (self *connectionTracker) sendQueued() {
	for len(self.queued) > 0 {
		if self.sendEvents(self.queued[0]) {
			self.queued = self.queued[1:]
		} else {
			return
		}
	}
}

func (self *connectionTracker) sendEvents(evts *edge_ctrl_pb.ConnectEvents) bool {
	successfulSend := false
	self.controllers.ForEach(func(ctrlId string, ch channel.Channel) {
		if err := protobufs.MarshalTyped(evts).WithTimeout(time.Second).Send(ch); err != nil {
			pfxlog.Logger().WithField("ctrlId", ctrlId).WithError(err).Error("error sending connect events")
			self.needsFullSync[ctrlId] = ch
			self.notifyNeedsFullSync()
		} else {
			successfulSend = true
		}
	})
	return successfulSend
}

func (self *connectionTracker) sendFullSync() {
	self.lock.Lock()
	defer self.lock.Unlock()

	ctrls := map[string]channel.Channel{}
	for k, _ := range self.needsFullSync {
		ch, exists := self.controllers.GetIfResponsive(k)
		if !exists {
			delete(self.needsFullSync, k)
		} else if ch != nil {
			ctrls[k] = ch
		}
	}

	if len(ctrls) == 0 {
		return
	}

	evts := &edge_ctrl_pb.ConnectEvents{
		FullState: true,
	}

	self.states.IterCb(func(key string, v *identityState) {
		evt := v.getConnectedStateEvent(key)
		if evt.IsConnected {
			evts.Events = append(evts.Events, evt)
		}
	})

	for ctrlId, ch := range ctrls {
		if err := protobufs.MarshalTyped(evts).WithTimeout(time.Second).Send(ch); err != nil {
			pfxlog.Logger().WithField("ctrlId", ctrlId).WithError(err).Error("error sending connect events")
		} else {
			delete(self.needsFullSync, ctrlId)
		}
	}
}

func (self *connectionTracker) NotifyOfReconnect(ch channel.Channel) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.needsFullSync[ch.Id()] = ch
	self.notifyNeedsFullSync()
}

type sessionConnectionHandler struct {
	stateManager                     state.Manager
	options                          *Options
	invalidApiSessionToken           metrics.Meter
	invalidApiSessionTokenDuringSync metrics.Meter
}

func newSessionConnectHandler(stateManager state.Manager, options *Options, metricsRegistry metrics.Registry) *sessionConnectionHandler {
	return &sessionConnectionHandler{
		stateManager:                     stateManager,
		options:                          options,
		invalidApiSessionToken:           metricsRegistry.Meter("edge.invalid_api_tokens"),
		invalidApiSessionTokenDuringSync: metricsRegistry.Meter("edge.invalid_api_tokens_during_sync"),
	}
}

func (handler *sessionConnectionHandler) BindChannel(binding channel.Binding, edgeConn *edgeClientConn) error {
	ch := binding.GetChannel()
	binding.AddCloseHandler(handler)

	byteToken, ok := ch.Underlay().Headers()[edge.SessionTokenHeader]

	if !ok {
		_ = ch.Close()
		return errors.New("no token attribute provided")
	}

	certificates := ch.Certificates()

	if len(certificates) == 0 {
		return errors.New("no client certificates provided")
	}

	fpg := cert.NewFingerprintGenerator()
	fingerprint := fpg.FromCert(certificates[0])

	token := string(byteToken)

	apiSession := handler.stateManager.GetApiSessionWithTimeout(token, handler.options.lookupApiSessionTimeout)

	if apiSession == nil {
		_ = ch.Close()

		var subjects []string

		for _, curCert := range certificates {
			subjects = append(subjects, curCert.Subject.String())
		}

		handler.invalidApiSessionToken.Mark(1)
		if handler.stateManager.IsSyncInProgress() {
			handler.invalidApiSessionTokenDuringSync.Mark(1)
		}

		return fmt.Errorf("no api session found for token [%s], fingerprint: [%v], subjects [%v]", token, fingerprint, subjects)
	}

	edgeConn.apiSession = apiSession

	isValid := handler.validateBySpiffeId(apiSession, certificates[0])

	if !isValid {
		isValid = handler.validateByFingerprint(apiSession, fingerprint)
	}

	if isValid {
		if apiSession.Claims != nil {
			token = apiSession.Claims.ApiSessionId
		}

		removeListener := handler.stateManager.AddApiSessionRemovedListener(token, func(token string) {
			if !ch.IsClosed() {
				if err := ch.Close(); err != nil {
					pfxlog.Logger().WithError(err).Error("could not close channel during api session removal")
				}
			}

			handler.stateManager.RemoveActiveChannel(ch)
		})

		handler.stateManager.AddActiveChannel(ch, apiSession)
		handler.stateManager.AddConnectedApiSessionWithChannel(token, removeListener, ch)

		return nil
	}

	_ = ch.Close()
	return errors.New("invalid client certificate for api session")
}

func (handler *sessionConnectionHandler) validateByFingerprint(apiSession *state.ApiSession, clientFingerprint string) bool {
	for _, fingerprint := range apiSession.CertFingerprints {
		if clientFingerprint == fingerprint {
			return true
		}
	}

	return false
}

func (handler *sessionConnectionHandler) HandleClose(ch channel.Channel) {
	token := ""
	if byteToken, ok := ch.Underlay().Headers()[edge.SessionTokenHeader]; ok {
		token = string(byteToken)

		handler.stateManager.RemoveConnectedApiSessionWithChannel(token, ch)
	} else {
		pfxlog.Logger().
			WithField("id", ch.Id()).
			Error("session connection handler encountered a HandleClose that did not have a SessionTokenHeader")
	}
}

func (handler *sessionConnectionHandler) validateBySpiffeId(apiSession *state.ApiSession, clientCert *x509.Certificate) bool {
	spiffeId, err := spiffehlp.GetSpiffeIdFromCert(clientCert)

	if err != nil {
		return false
	}

	if spiffeId == nil {
		return false
	}

	parts := strings.Split(spiffeId.Path, "/")

	if len(parts) != 6 {
		return false
	}

	if parts[0] != "identity" {
		return false
	}

	if parts[2] != "apiSession" {
		return false
	}

	if parts[4] != "apiSessionCertificate" {
		return false
	}

	if apiSession.Id == parts[3] {
		return true
	}

	return false
}
