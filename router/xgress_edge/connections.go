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
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/metrics"
	edgeSdk "github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/common/spiffehlp"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/state"
	cmap "github.com/orcaman/concurrent-map/v2"
)

type identityConnect struct {
	srcAddr     string
	dstAddr     string
	connectTime int64
}

type reportData struct {
	connects     []identityConnect
	stateChanged bool
}

func (self *reportData) hasReportData() bool {
	return len(self.connects) > 0 || self.stateChanged
}

type identityState struct {
	sync.Mutex
	unreported    reportData
	beingReported reportData
	connections   []channel.Channel
}

func (self *identityState) markConnect(ch channel.Channel, queueEvent bool) {
	self.Lock()
	defer self.Unlock()
	if queueEvent {
		srcAddr := ch.Underlay().GetRemoteAddr().String()
		dstAddr := ch.Underlay().GetLocalAddr().String()

		self.unreported.connects = append(self.unreported.connects, identityConnect{srcAddr: srcAddr, dstAddr: dstAddr, connectTime: time.Now().UnixMilli()})
	}
	self.connections = append(self.connections, ch)
	if len(self.connections) == 1 {
		self.unreported.stateChanged = true
	}
}

func (self *identityState) markDisconnect(ch channel.Channel) {
	self.Lock()
	defer self.Unlock()
	startLen := len(self.connections)
	self.connections = slices.DeleteFunc(self.connections, func(elem channel.Channel) bool {
		return elem == ch
	})
	if startLen > 0 && len(self.connections) == 0 {
		self.unreported.stateChanged = true
	}
}

func (self *identityState) getConnectedStateEvent(id string) *edge_ctrl_pb.ConnectEvents_IdentityConnectEvents {
	self.Lock()
	defer self.Unlock()
	return &edge_ctrl_pb.ConnectEvents_IdentityConnectEvents{
		IdentityId:  id,
		IsConnected: len(self.connections) > 0,
	}
}

func (self *identityState) getStateEvent(id string, fullSync bool) (*edge_ctrl_pb.ConnectEvents_IdentityConnectEvents, bool) {
	self.Lock()
	defer self.Unlock()

	isConnected := len(self.connections) > 0

	if !self.unreported.hasReportData() && !self.beingReported.hasReportData() {
		if fullSync {
			return &edge_ctrl_pb.ConnectEvents_IdentityConnectEvents{
				IdentityId:  id,
				IsConnected: isConnected,
			}, isConnected
		}
		return nil, isConnected
	}

	result := &edge_ctrl_pb.ConnectEvents_IdentityConnectEvents{
		IdentityId:  id,
		IsConnected: isConnected,
	}

	for _, t := range self.beingReported.connects {
		result.ConnectTimes = append(result.ConnectTimes, &edge_ctrl_pb.ConnectEvents_ConnectDetails{
			ConnectTime: t.connectTime,
			SrcAddr:     t.srcAddr,
			DstAddr:     t.dstAddr,
		})
	}

	for _, t := range self.unreported.connects {
		result.ConnectTimes = append(result.ConnectTimes, &edge_ctrl_pb.ConnectEvents_ConnectDetails{
			ConnectTime: t.connectTime,
			SrcAddr:     t.srcAddr,
			DstAddr:     t.dstAddr,
		})
	}

	self.beingReported.connects = append(self.beingReported.connects, self.unreported.connects...)
	self.beingReported.stateChanged = self.beingReported.stateChanged || self.unreported.stateChanged
	self.unreported.connects = nil
	self.unreported.stateChanged = false

	return result, result.IsConnected
}

func (self *identityState) clearReported() int {
	self.Lock()
	defer self.Unlock()
	count := len(self.beingReported.connects)
	self.beingReported.connects = nil
	self.beingReported.stateChanged = false
	return count
}

type connectionTracker struct {
	enabled            bool
	lock               sync.Mutex
	controllers        env.NetworkControllers
	states             cmap.ConcurrentMap[string, *identityState]
	needsFullSync      map[string]channel.Channel
	notifyFullSync     chan struct{}
	batchInterval      time.Duration
	fullSyncInterval   time.Duration
	maxQueuedEvents    int64
	lastFullSync       time.Time
	queuedEventCounter atomic.Int64
}

func newConnectionTracker(env env.RouterEnv) *connectionTracker {
	result := &connectionTracker{
		enabled:          env.GetConnectEventsConfig().Enabled,
		controllers:      env.GetNetworkControllers(),
		states:           cmap.New[*identityState](),
		needsFullSync:    map[string]channel.Channel{},
		notifyFullSync:   make(chan struct{}, 1),
		batchInterval:    env.GetConnectEventsConfig().BatchInterval,
		fullSyncInterval: env.GetConnectEventsConfig().FullSyncInterval,
		maxQueuedEvents:  env.GetConnectEventsConfig().MaxQueuedEvents,
	}

	go result.runLoop(env.GetCloseNotify())

	return result
}

func (self *connectionTracker) runLoop(closeNotify <-chan struct{}) {
	ticker := time.NewTicker(self.batchInterval)
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

// GetChannels returns a snapshot of all active channels grouped by identity ID.
// This method is primarily used by security monitoring systems that need to iterate
// over all connected identities and their associated channels for compliance checks,
// threat detection, or administrative operations. The returned map allows efficient
// bulk operations while maintaining thread safety through concurrent iteration and
// per-identity locking.
//
// Returns:
//   - map[string][]channel.Channel: Map where keys are identity IDs and values are
//     slices of all active channels for that identity. Empty identities are included
//     with empty slices to maintain a complete view of the connection state.
//
// Thread Safety: Safe for concurrent use. Uses buffered iteration over the concurrent
// map and individual locks per identity state to ensure consistent snapshots.
func (self *connectionTracker) GetChannels() map[string][]channel.Channel {
	result := map[string][]channel.Channel{}
	for entry := range self.states.IterBuffered() {
		identityId := entry.Key
		idState := entry.Val

		idState.Lock()
		result[identityId] = []channel.Channel{}

		result[identityId] = append(result[identityId], idState.connections...)

		idState.Unlock()
	}

	return result
}

// GetChannelsByIdentityId retrieves all active channels for a specific identity.
// This method is used for security enforcement operations where the router needs to
// take action on all connections belonging to a particular identity, such as when
// posture checks fail, access policies change, or MFA requirements trigger.
//
// Parameters:
//   - identityId: The unique identity ID to look up connections for
//
// Returns:
//   - []channel.Channel: Slice of all active channels for the identity. Returns empty
//     slice if identity has no connections or doesn't exist.
func (self *connectionTracker) GetChannelsByIdentityId(identityId string) []channel.Channel {
	idState, _ := self.states.Get(identityId)

	var result []channel.Channel
	if idState == nil {
		return result
	}

	idState.Lock()

	result = append(result, idState.connections...)

	defer idState.Unlock()

	return result
}

func (self *connectionTracker) notifyNeedsFullSync() {
	select {
	case self.notifyFullSync <- struct{}{}:
	default:
	}
}

func (self *connectionTracker) markConnected(identityId string, ch channel.Channel) {
	pfxlog.Logger().WithField("identityId", identityId).Trace("marking connected")
	queueEvent := self.enabled && self.queuedEventCounter.Load() < self.maxQueuedEvents
	self.states.Upsert(identityId, nil, func(exist bool, valueInMap *identityState, newValue *identityState) *identityState {
		if valueInMap == nil {
			valueInMap = &identityState{}
		}
		valueInMap.markConnect(ch, queueEvent)
		return valueInMap
	})

	if queueEvent {
		self.queuedEventCounter.Add(1)
	}
}

func (self *connectionTracker) markDisconnected(identityId string, ch channel.Channel) {
	pfxlog.Logger().WithField("identityId", identityId).Trace("marking disconnected")
	self.states.Upsert(identityId, nil, func(exist bool, valueInMap *identityState, newValue *identityState) *identityState {
		if valueInMap == nil {
			valueInMap = &identityState{}
		}
		valueInMap.markDisconnect(ch)
		return valueInMap
	})
}

func (self *connectionTracker) report() {
	self.lock.Lock()
	defer self.lock.Unlock()

	startTime := time.Now()
	fullSync := time.Since(self.lastFullSync) > self.fullSyncInterval

	var removeCheck []string
	evts := &edge_ctrl_pb.ConnectEvents{
		FullState: fullSync,
	}

	self.states.IterCb(func(key string, v *identityState) {
		evt, connected := v.getStateEvent(key, fullSync)
		if !connected && evt == nil {
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
			return len(v.connections) == 0 && !v.unreported.hasReportData() && !v.beingReported.hasReportData()
		})
	}

	if len(evts.Events) > 0 || evts.FullState {
		if self.sendEvents(evts) {
			if fullSync {
				self.lastFullSync = startTime
			}

			self.states.IterCb(func(key string, v *identityState) {
				clearedCount := v.clearReported()
				if clearedCount > 0 {
					self.queuedEventCounter.Add(int64(-clearedCount))
				}
			})
		}
	} else if fullSync {
		self.lastFullSync = startTime
	}
}

func (self *connectionTracker) sendEvents(evts *edge_ctrl_pb.ConnectEvents) bool {
	successfulSend := false
	self.controllers.ForEach(func(ctrlId string, ch channel.Channel) {
		pfxlog.Logger().WithField("ctrlId", ch.Id()).WithField("fullSync", evts.FullState).Trace("sending connect events")

		if err := protobufs.MarshalTyped(evts).WithTimeout(time.Second).Send(ch); err != nil {
			pfxlog.Logger().WithField("ctrlId", ctrlId).WithError(err).Error("error sending connect events")
			self.needsFullSync[ctrlId] = ch
			self.notifyNeedsFullSync()
		} else {
			successfulSend = true
			if evts.FullState {
				delete(self.needsFullSync, ctrlId)
			}
		}
	})
	return successfulSend
}

func (self *connectionTracker) sendFullSync() {
	self.lock.Lock()
	defer self.lock.Unlock()

	ctrls := map[string]channel.Channel{}
	for k := range self.needsFullSync {
		ctrl := self.controllers.GetNetworkController(k)
		if ctrl == nil {
			delete(self.needsFullSync, k)
		} else if ctrl.IsConnected() {
			ctrls[k] = ctrl.Channel()
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
		pfxlog.Logger().WithField("ctrlId", ch.Id()).Trace("doing full connection state sync")
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

	pfxlog.Logger().WithField("ctrlId", ch.Id()).Debug("sending full sync of connection state after reconnect")
	self.needsFullSync[ch.Id()] = ch
	self.notifyNeedsFullSync()
}

func (self *connectionTracker) Inspect(_ string, _ time.Duration) any {
	self.lock.Lock()
	result := &inspect.RouterIdentityConnections{
		IdentityConnections: map[string]*inspect.RouterIdentityConnectionDetail{},
		LastFullSync:        self.lastFullSync.Format(time.RFC3339),
		QueuedEventCount:    self.queuedEventCounter.Load(),
		MaxQueuedEvents:     self.maxQueuedEvents,
		BatchInterval:       self.batchInterval.String(),
		FullSyncInterval:    self.fullSyncInterval.String(),
	}
	for ctrlId := range self.needsFullSync {
		result.NeedFullSync = append(result.NeedFullSync, ctrlId)
	}
	self.lock.Unlock()

	for entry := range self.states.IterBuffered() {
		identityId := entry.Key
		states := entry.Val
		states.Lock()
		identityDetail := &inspect.RouterIdentityConnectionDetail{
			UnreportedCount:           uint64(len(states.unreported.connects)),
			UnreportedStateChanged:    states.unreported.stateChanged,
			BeingReportedCount:        uint64(len(states.beingReported.connects)),
			BeingReportedStateChanged: states.beingReported.stateChanged,
		}
		for _, ch := range states.connections {
			identityDetail.Connections = append(identityDetail.Connections, &inspect.RouterConnectionDetail{
				Id:      ch.Id(),
				Closed:  ch.IsClosed(),
				SrcAddr: ch.Underlay().GetRemoteAddr().String(),
				DstAddr: ch.Underlay().GetLocalAddr().String(),
			})
		}
		entry.Val.Unlock()
		result.IdentityConnections[identityId] = identityDetail
	}
	return result
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

// validateApiSession performs security validation of an incoming SDK connection by verifying
// the API session token and client certificate authenticity. The validation process includes
// session token verification, certificate chain validation, fingerprint matching, and SPIFFE
// ID verification when present.
//
// Failed validation results in connection termination and security event logging.
//
// Parameters:
//   - binding: The channel binding containing connection details and certificates
//   - edgeConn: The edgeClientConn being validated and populated with session info
//
// Returns:
//   - error: nil if validation succeeds, descriptive error if validation fails
func (handler *sessionConnectionHandler) validateApiSession(binding channel.Binding, edgeConn *edgeClientConn) error {
	ch := binding.GetChannel()
	binding.AddCloseHandler(handler)

	byteToken, ok := ch.Headers()[edgeSdk.SessionTokenHeader]

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

	apiSession := handler.stateManager.GetApiSessionTokenWithTimeout(token, handler.options.lookupApiSessionTimeout)

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

	edgeConn.apiSessionToken = apiSession

	isValid := handler.validateBySpiffeId(apiSession, certificates[0])

	if !isValid {
		isValid = handler.validateByFingerprint(apiSession, fingerprint)
	}

	if isValid {
		return nil
	}

	_ = ch.Close()
	return errors.New("invalid client certificate for api session")
}

// completeBinding finalizes the connection setup after successful validation by registering
// the connection for lifecycle management. This includes setting up cleanup handlers for
// API session removal, adding the connection to active tracking systems, and ensuring
// proper cleanup coordination to prevent resource leaks.
//
// Parameters:
//   - binding: The validated channel binding to complete setup for
//   - edgeConn: The validated edgeClientConn to register for operation
func (handler *sessionConnectionHandler) completeBinding(binding channel.Binding, edgeConn *edgeClientConn) {
	ch := binding.GetChannel()
	apiSession := edgeConn.apiSessionToken

	_ = handler.stateManager.AddApiSessionRemovedListener(apiSession, func(_ *state.ApiSessionToken) {
		if !ch.IsClosed() {
			if err := ch.Close(); err != nil {
				pfxlog.Logger().WithError(err).Error("could not close channel during api session removal")
			}
		}
	})
}

func (handler *sessionConnectionHandler) validateByFingerprint(apiSession *state.ApiSessionToken, clientFingerprint string) bool {
	for _, fingerprint := range apiSession.CertFingerprints {
		if clientFingerprint == fingerprint {
			return true
		}
	}

	return false
}

func (handler *sessionConnectionHandler) HandleClose(ch channel.Channel) {
	//no work, interface fulfillment
}

func (handler *sessionConnectionHandler) validateBySpiffeId(apiSession *state.ApiSessionToken, clientCert *x509.Certificate) bool {
	spiffeId, err := spiffehlp.GetSpiffeIdFromCert(clientCert)

	if err != nil {
		return false
	}

	if spiffeId == nil {
		return false
	}

	return verifySpiffId(spiffeId, apiSession.Id)
}

func verifySpiffId(spiffeId *url.URL, expectedApiSessionId string) bool {
	if spiffeId.Scheme != "spiffe" {
		return false
	}

	path := strings.TrimPrefix(spiffeId.Path, "/")
	parts := strings.Split(path, "/")

	// /identity/<id>/apiSession/<id> or /identity/<id>/apiSession/<id>/apiSessionCertificate/<id>
	if len(parts) != 4 && len(parts) != 6 {
		return false
	}

	if parts[0] != "identity" {
		return false
	}

	if parts[2] != "apiSession" {
		return false
	}

	if len(parts) == 6 {
		if parts[4] != "apiSessionCertificate" {
			return false
		}
	}

	return parts[3] == expectedApiSessionId
}
