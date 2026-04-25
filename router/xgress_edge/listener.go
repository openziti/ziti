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
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	sdkinspect "github.com/openziti/sdk-golang/inspect"
	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/sdk-golang/xgress"
	sdkedge "github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/cert"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/inspect"
	fabricMetrics "github.com/openziti/ziti/v2/common/metrics"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/state"
	"github.com/openziti/ziti/v2/router/xgress_common"
	"github.com/openziti/ziti/v2/router/xgress_router"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"
)

var peerHeaderRequestMappings = map[uint32]uint32{
	uint32(sdkedge.PublicKeyHeader):        uint32(sdkedge.PublicKeyHeader),
	uint32(sdkedge.CallerIdHeader):         uint32(sdkedge.CallerIdHeader),
	uint32(sdkedge.AppDataHeader):          uint32(sdkedge.AppDataHeader),
	uint32(sdkedge.ConnectionMarkerHeader): uint32(sdkedge.ConnectionMarkerHeader),
	uint32(sdkedge.StickinessTokenHeader):  uint32(ctrl_msg.XtStickinessToken),
}

var peerHeaderRespMappings = map[uint32]uint32{
	ctrl_msg.XtStickinessToken: uint32(sdkedge.StickinessTokenHeader),
}

type listener struct {
	id               *identity.TokenId
	factory          *Factory
	options          *Options
	bindHandler      xgress.BindHandler
	underlayListener channel.UnderlayListener
	headers          map[int32][]byte

	droppedMsgMeter      metrics.Meter
	droppedPayloadsMeter metrics.Meter
	droppedAcksMeter     metrics.Meter
}

func (listener *listener) Inspect(key string, _ time.Duration) any {
	switch key {
	case inspect.RouterEdgeCircuitsKey:
		result := &inspect.EdgeListenerCircuits{
			Circuits: map[string]*inspect.EdgeXgFwdInspectDetail{},
		}

		for entry := range listener.factory.connectionTracker.states.IterBuffered() {
			v := entry.Val
			v.Lock()
			for _, ch := range v.connections {
				if conn, ok := ch.GetUserData().(*edgeClientConn); ok {
					for xgCircuitEntry := range conn.xgCircuits.IterBuffered() {
						xgCircuit := xgCircuitEntry.Val
						result.Circuits[xgCircuit.circuitId+"/"+string(xgCircuit.address)] = xgCircuit.GetCircuitInspectDetail()
					}
				}
			}
			v.Unlock()
		}
		return result

	case inspect.RouterSdkCircuitsKey:
		result := &inspect.SdkCircuits{
			Circuits: map[string]*inspect.SdkCircuitDetail{},
		}

		resultCh := make(chan *sdkCircuitResult, 10)
		expected := 0
		for entry := range listener.factory.connectionTracker.states.IterBuffered() {
			v := entry.Val
			v.Lock()
			for _, ch := range v.connections {
				if conn, ok := ch.GetUserData().(*edgeClientConn); ok {
					go listener.getSdkCircuits(conn, resultCh)
					expected++
				}
			}
			v.Unlock()
		}

		start := time.Now()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for expected > 0 && time.Since(start) < 5*time.Second {
			select {
			case next := <-resultCh:
				if next.err != nil {
					result.Errors = append(result.Errors, next.err.Error())
				}
				if next.circuits != nil {
					idBase := next.conn.apiSessionToken.IdentityId + "/" +
						next.conn.ch.GetChannel().ConnectionId()
					for _, circuit := range next.circuits.Circuits {
						detail := &inspect.SdkCircuitDetail{
							IdentityId:    next.conn.apiSessionToken.IdentityId,
							ChannelConnId: next.conn.ch.GetChannel().ConnectionId(),
							CircuitDetail: circuit,
						}
						result.Circuits[idBase+"/"+detail.CircuitId] = detail
					}
				}
				expected--
			case <-ticker.C:
			}
		}

		return result

	default:
		lc := strings.ToLower(key)
		if strings.HasPrefix(lc, "sdk-context:") {
			identityId := key[len("sdk-context:"):]
			return listener.getSdkContext(identityId)
		}
	}

	return nil
}

type sdkCircuitResult struct {
	conn     *edgeClientConn
	err      error
	circuits *xgress.CircuitsDetail
}

func (listener *listener) getSdkCircuits(conn *edgeClientConn, resultCh chan *sdkCircuitResult) {
	msg := sdkedge.NewInspectRequest(nil, "circuits")
	reply, err := msg.WithTimeout(4800 * time.Millisecond).SendForReply(conn.ch.GetControlSender())
	if err != nil {
		listener.submitErrResponse(conn, resultCh, fmt.Errorf("unable to get circuits from identity '%s' conn '%v' (%w)",
			conn.apiSessionToken.Id, conn.ch.GetChannel().ConnectionId(), err))
		return
	}

	resp := sdkinspect.SdkInspectResponse{}
	err = json.Unmarshal(reply.Body, &resp)
	if err != nil {
		listener.submitErrResponse(conn, resultCh, fmt.Errorf("unable to unmarshal circuits from identity '%s' conn '%v' (%w)",
			conn.apiSessionToken.Id, conn.ch.GetChannel().ConnectionId(), err))
		return
	}

	if v, ok := resp.Values["circuits"]; ok {
		jsonString, err := json.Marshal(v)
		if err != nil {
			listener.submitErrResponse(conn, resultCh, fmt.Errorf("failed to marshal sdk circuits from identity '%s' conn '%v' (%w)",
				conn.apiSessionToken.Id, conn.ch.GetChannel().ConnectionId(), err))
			return
		}
		circuitsDetails := &xgress.CircuitsDetail{}
		if err = json.Unmarshal(jsonString, &circuitsDetails); err != nil {
			listener.submitErrResponse(conn, resultCh, fmt.Errorf("failed to unmarshal sdk circuits from identity '%s' conn '%v' (%w)",
				conn.apiSessionToken.Id, conn.ch.GetChannel().ConnectionId(), err))
			return
		}

		listener.submitResponse(resultCh, &sdkCircuitResult{
			conn:     conn,
			circuits: circuitsDetails,
		})

	} else {
		listener.submitErrResponse(conn, resultCh, fmt.Errorf("sdk circuit details not returned from identity '%s' conn '%v' (%w)",
			conn.apiSessionToken.Id, conn.ch.GetChannel().ConnectionId(), err))
	}
}

func (listener *listener) submitErrResponse(conn *edgeClientConn, resultCh chan *sdkCircuitResult, err error) {
	listener.submitResponse(resultCh, &sdkCircuitResult{
		conn: conn,
		err:  err,
	})
}

func (listener *listener) submitResponse(resultCh chan *sdkCircuitResult, result *sdkCircuitResult) {
	select {
	case resultCh <- result:
	case <-time.After(time.Second):
	}
}

type sdkContextResult struct {
	identityId string
	connId     string
	err        error
	context    any
}

func (listener *listener) getSdkContext(identityId string) any {
	channels := listener.factory.connectionTracker.GetChannelsByIdentityId(identityId)
	if len(channels) == 0 {
		return map[string]any{
			"error": fmt.Sprintf("no connections found for identity %s", identityId),
		}
	}

	resultCh := make(chan *sdkContextResult, len(channels))
	expected := 0
	for _, ch := range channels {
		if conn, ok := ch.GetUserData().(*edgeClientConn); ok {
			expected++
			go func(conn *edgeClientConn) {
				msg := sdkedge.NewInspectRequest(nil, "context")
				reply, err := msg.WithTimeout(4800 * time.Millisecond).SendForReply(conn.ch.GetControlSender())
				if err != nil {
					select {
					case resultCh <- &sdkContextResult{
						identityId: identityId,
						connId:     conn.ch.GetChannel().ConnectionId(),
						err:        err,
					}:
					case <-time.After(time.Second):
					}
					return
				}

				resp := sdkinspect.SdkInspectResponse{}
				if err = json.Unmarshal(reply.Body, &resp); err != nil {
					select {
					case resultCh <- &sdkContextResult{
						identityId: identityId,
						connId:     conn.ch.GetChannel().ConnectionId(),
						err:        fmt.Errorf("failed to unmarshal response: %w", err),
					}:
					case <-time.After(time.Second):
					}
					return
				}

				if v, ok := resp.Values["context"]; ok {
					select {
					case resultCh <- &sdkContextResult{
						identityId: identityId,
						connId:     conn.ch.GetChannel().ConnectionId(),
						context:    v,
					}:
					case <-time.After(time.Second):
					}
				} else {
					select {
					case resultCh <- &sdkContextResult{
						identityId: identityId,
						connId:     conn.ch.GetChannel().ConnectionId(),
						err:        fmt.Errorf("context not returned in inspect response"),
					}:
					case <-time.After(time.Second):
					}
				}
			}(conn)
		}
	}

	if expected == 0 {
		return map[string]any{
			"error": fmt.Sprintf("no edge connections found for identity %s", identityId),
		}
	}

	var results []any
	var errs []string
	deadline := time.After(5 * time.Second)
	for expected > 0 {
		select {
		case next := <-resultCh:
			if next.err != nil {
				errs = append(errs, fmt.Sprintf("conn %s: %v", next.connId, next.err))
			}
			if next.context != nil {
				results = append(results, map[string]any{
					"connId":  next.connId,
					"context": next.context,
				})
			}
			expected--
		case <-deadline:
			expected = 0
		}
	}

	response := map[string]any{
		"identityId": identityId,
		"results":    results,
	}
	if len(errs) > 0 {
		response["errors"] = errs
	}
	return response
}

// newListener creates a new xgress edge listener
func newListener(id *identity.TokenId, factory *Factory, options *Options, headers map[int32][]byte) xgress_router.Listener {
	return &listener{
		id:                   id,
		factory:              factory,
		options:              options,
		headers:              headers,
		droppedMsgMeter:      factory.metricsRegistry.Meter("xgress.edge.dropped_msgs"),
		droppedPayloadsMeter: factory.metricsRegistry.Meter("xgress.edge.dropped_payloads"),
		droppedAcksMeter:     factory.metricsRegistry.Meter("xgress.edge.dropped_acks"),
	}
}

func (listener *listener) Listen(address string, bindHandler xgress.BindHandler) error {
	if address == "" {
		return errors.New("address must be specified for edge listeners")
	}
	listener.bindHandler = bindHandler
	addr, err := transport.ParseAddress(address)

	if err != nil {
		return fmt.Errorf("cannot listen on invalid address [%s] (%s)", address, err)
	}

	pfxlog.Logger().WithField("address", addr).Info("starting channel listener")

	listenerConfig := channel.ListenerConfig{
		ConnectOptions:   listener.options.channelOptions.ConnectOptions,
		TransportConfig:  listener.factory.edgeRouterConfig.Tcfg,
		Headers:          listener.headers,
		PoolConfigurator: fabricMetrics.GoroutinesPoolMetricsConfigF(listener.factory.metricsRegistry, "pool.listener.xgress_edge"),
	}

	listener.underlayListener = channel.NewClassicListener(listener.id, addr, listenerConfig)

	if err := listener.underlayListener.Listen(); err != nil {
		return err
	}
	accepter := NewAcceptor(listener, listener.underlayListener)
	go accepter.Run()

	return nil
}

func (listener *listener) Binding() string {
	return common.EdgeBinding
}

func (listener *listener) Close() error {
	return listener.underlayListener.Close()
}

type edgeClientConn struct {
	msgMux          sdkedge.ConnMux[*state.ConnState]
	listener        *listener
	fingerprints    cert.Fingerprints
	ch              sdkedge.SdkChannel
	idSeq           uint32
	apiSessionToken *state.ApiSessionToken
	forwarder       env.Forwarder
	xgCircuits      cmap.ConcurrentMap[string, *xgEdgeForwarder]

	// removeApiSessionListener removes the legacy api-session-removed listener registered in completeBinding.
	// Nil for OIDC sessions (no listener is registered for them). Must be called from HandleClose to avoid
	// pinning the channel via the listener closure on the state manager's event emitter.
	removeApiSessionListener state.RemoveListener

	stateListener struct {
		sync.Mutex
		enabled      atomic.Bool
		lastRequired concurrenz.AtomicValue[time.Time]
	}
}

func (self *edgeClientConn) GetHostedServicesRegistry() *hostedServiceRegistry {
	return self.listener.factory.hostedServices
}

func (self *edgeClientConn) NotifyIdentityEvent(state *common.IdentityState, eventType common.IdentityEventType) {
	log := pfxlog.Logger().WithField("identityId", state.Identity.Id).
		WithField("channel", self.ch.GetChannel().Label()).
		WithField("connectionId", self.ch.GetChannel().ConnectionId())

	closeEvent := false
	if eventType == common.IdentityDeletedEvent {
		log.Info("identity deleted, closing connections")
		closeEvent = true
	} else if state.Identity.Disabled {
		log.Info("identity disabled, closing connections")
		closeEvent = true
	}

	if closeEvent {
		if err := self.ch.GetChannel().Close(); err != nil {
			log.WithError(err).Error("failed to close channel")
		}
	}
}

func (self *edgeClientConn) NotifyServiceChange(_ *common.IdentityState, previousService, service *common.IdentityService, eventType common.ServiceEventType) {
	dialLost := false
	bindLost := false

	if eventType == common.ServiceAccessLostEvent {
		dialLost = service.IsDialAllowed()
		bindLost = service.IsBindAllowed()
	} else if eventType == common.ServiceUpdatedEvent {
		dialLost = previousService.IsDialAllowed() && !service.IsDialAllowed()
		bindLost = previousService.IsBindAllowed() && !service.IsBindAllowed()
	}

	if bindLost {
		self.handleBindAccessLost(service)
	}

	if dialLost {
		self.handleDialAccessLost(service)
	}
}

func (self *edgeClientConn) handleBindAccessLost(service *common.IdentityService) {
	log := pfxlog.Logger().
		WithField("serviceId", service.GetId()).
		WithField("serviceName", service.GetName())

	terminators := self.GetHostedServicesRegistry().getTerminatorsForService(service.GetId())
	for _, terminator := range terminators {
		log.WithField("terminatorId", terminator.terminatorId).
			Info("bind access to service, closing terminator")
		edgeErr := &EdgeError{
			Message:   "bind access lost",
			Code:      sdkedge.ErrorCodeAccessDenied,
			RetryHint: sdkedge.RetryStartOver, // switch back to start over once timing issues are resolved
		}
		reason := "bind access lost"
		self.GetHostedServicesRegistry().ensureSdkCloseSent(terminator, reason, edgeErr)
		terminator.close(self.GetHostedServicesRegistry(), true, reason)
	}
}

func (self *edgeClientConn) handleDialAccessLost(service *common.IdentityService) {
	log := pfxlog.Logger().
		WithField("serviceId", service.GetId()).
		WithField("serviceName", service.GetName())

	var toClose []edgeCircuit
	self.IterateCircuits(func(c edgeCircuit) {
		if c.GetServiceId() == service.GetId() {
			toClose = append(toClose, c)
		}
	})

	for _, c := range toClose {
		log.WithField("circuitId", c.GetCircuitId()).Info("closing circuit, dial access lost")
		c.CloseForDialAccessLoss()
	}
}

type edgeCircuit interface {
	GetCircuitId() string
	GetServiceId() string
	GetApiSessionToken() *state.ApiSessionToken
	CloseForDialAccessLoss()
	IsPostCreateAccessCheckNeeded() bool
	SetPostCreateAccessCheckDone()
}

func (self *edgeClientConn) IterateCircuits(f func(c edgeCircuit)) {
	for _, sink := range self.msgMux.GetSinks() {
		if xgConn, ok := sink.(*edgeXgressConn); ok {
			f(xgConn)
		} else {
			pfxlog.Logger().Errorf("tried to iterate msg mux sink, but it wasn't edgeXgressConn, instead was %T", sink)
		}
	}

	self.xgCircuits.IterCb(func(_ string, v *xgEdgeForwarder) {
		f(v)
	})
}

// getIdentityId safely retrieves the identity ID from the associated API session.
// This method provides a nil-safe way to access the identity ID, which is used
// for logging, security operations, and connection tracking. Returns empty string
// if the API session is not yet established or has been cleared.
//
// Returns:
//   - string: The identity ID if API session exists, empty string otherwise
//
// Usage: Used in security contexts where identity must be determined
// for authorization checks, audit logging, and connection management.
func (self *edgeClientConn) getIdentityId() string {
	if self.apiSessionToken == nil {
		return ""
	}
	return self.apiSessionToken.IdentityId
}

// handleDataMessage wraps the ConnMux data dispatch with additional diagnostic logging.
// When data arrives for a connId with no registered sink, it logs the channel identity,
// active mux sink count, and xgress circuit count to help diagnose whether a legacy
// (non-xgress) client was incorrectly routed through the xgEdgeForwarder path.
func (self *edgeClientConn) handleDataMessage(msg *channel.Message, ch channel.Channel) {
	connId, found := msg.GetUint32Header(sdkedge.ConnIdHeader)
	if found && !self.msgMux.HasConn(connId) {
		pfxlog.Logger().
			WithField("connId", connId).
			WithField("channel", self.ch.GetChannel().Label()).
			WithField("identityId", self.getIdentityId()).
			WithField("muxSinkCount", self.msgMux.GetConnCount()).
			WithField("xgCircuitCount", self.xgCircuits.Count()).
			Warn("received edge data message for unknown conn id, data may be from a non-xgress client on an xgress circuit")
	}
	self.msgMux.HandleReceive(msg, ch)
}

// GetConnIdToSinks returns all active connections managed by this edgeClientConn's message multiplexer.
// Each connection is identified by a unique connection ID (connId) and represented as a message sink
// that contains connection state information. This method is used by security enforcement systems
// to iterate over all connections when applying policy changes, evaluating access permissions,
// or performing connection-level operations such as forced termination.
//
// Returns:
//   - map[uint32]edge.MsgSink[*state.ConnState]: Map of connection ID to message sink containing
//     connection state. Each sink provides access to connection metadata and state information.
func (self *edgeClientConn) GetConnIdToSinks() map[uint32]sdkedge.MsgSink[*state.ConnState] {
	return self.msgMux.GetSinks()
}

func (self *edgeClientConn) GetApiSessionToken() *state.ApiSessionToken {
	return self.apiSessionToken
}

func (self *edgeClientConn) handleConnInspectResponse(msg *channel.Message) {
	result, err := sdkedge.UnmarshalInspectResult(msg)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshal conn inspect response from sdk client")
		return
	}

	self.GetHostedServicesRegistry().queue(&inspectResponseEvent{
		conn:     self,
		replyFor: msg.ReplyFor(),
		result:   result,
	})
}

func (self *edgeClientConn) HandleClose(ch channel.Channel) {
	log := pfxlog.ContextLogger(self.ch.GetChannel().Label())
	log.Debugf("closing")
	self.listener.factory.hostedServices.cleanupServices(ch)
	self.msgMux.Close()
	self.cleanupXgressCircuits()
	self.listener.factory.stateManager.RouterDataModel().UnsubscribeFromIdentityChanges(self.getIdentityId(), self)
	if self.removeApiSessionListener != nil {
		self.removeApiSessionListener()
	}
}

func (self *edgeClientConn) cleanupXgressCircuits() {
	for entry := range self.xgCircuits.IterBuffered() {
		self.cleanupXgressCircuit(entry.Val)
	}
}

// CloseConn closes a specific connection (connId) on this channel
// Sends StateClosed message to SDK and cleans up any associated circuits
func (self *edgeClientConn) CloseConn(connId uint32, reason string) error {
	log := pfxlog.ContextLogger(self.ch.GetChannel().Label()).WithField("connId", connId)

	// Check if channel is still open
	if self.ch.GetChannel().IsClosed() {
		log.Debug("channel already closed, skipping connection close")
		return nil
	}

	// Find and cleanup any circuits associated with this connId
	var circuitsToCleanup []*xgEdgeForwarder
	for entry := range self.xgCircuits.IterBuffered() {
		edgeForwarder := entry.Val
		if edgeForwarder.connId == connId {
			circuitsToCleanup = append(circuitsToCleanup, edgeForwarder)
		}
	}

	// Clean up circuits for this connection
	for _, edgeForwarder := range circuitsToCleanup {
		log.WithField("circuitId", edgeForwarder.circuitId).Debug("cleaning up circuit for connection")
		self.cleanupXgressCircuit(edgeForwarder)
	}

	// Remove connection from message multiplexer
	// The ConnMux tracks connections and their message handlers
	self.msgMux.RemoveByConnId(connId)

	// Send StateClosed message to SDK
	closeMsg := sdkedge.NewStateClosedMsg(connId, reason)
	closeMsg.PutUint32Header(sdkedge.ConnIdHeader, connId)

	err := closeMsg.WithPriority(channel.High).WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetDefaultSender())
	if err != nil {
		log.WithError(err).WithFields(sdkedge.GetLoggerFields(closeMsg)).Error("failed to send state closed message")
		return err
	}

	log.WithField("reason", reason).Debug("connection closed successfully")
	return nil
}

func (self *edgeClientConn) cleanupXgressCircuit(edgeForwarder *xgEdgeForwarder) {
	circuitId := edgeForwarder.circuitId
	log := pfxlog.Logger().WithField("circuitId", circuitId)

	self.forwarder.EndCircuit(circuitId)
	self.xgCircuits.Remove(circuitId)

	// Notify the controller of the xgress fault
	fault := &ctrl_pb.Fault{Id: circuitId}
	switch edgeForwarder.originator {
	case xgress.Initiator:
		fault.Subject = ctrl_pb.FaultSubject_IngressFault
	case xgress.Terminator:
		fault.Subject = ctrl_pb.FaultSubject_EgressFault
	}

	controllers := self.listener.factory.env.GetNetworkControllers()
	ch := controllers.GetChannel(edgeForwarder.ctrlId)
	if ch == nil {
		log.WithField("ctrlId", edgeForwarder.ctrlId).Error("control channel not available")
	} else {
		log.Debug("notifying controller of fault")
		if err := protobufs.MarshalTyped(fault).Send(ch); err != nil {
			log.WithError(err).Error("error sending fault")
		}
	}
}

func (self *edgeClientConn) ContentType() int32 {
	return sdkedge.ContentTypeData
}

func (self *edgeClientConn) checkForStateListener() {
	// Terminator flow:
	//   1. Check if we have access
	//   2. Add state listener
	//   3. Setup router side terminator structure and start establishing on controller
	//   4. After router-side terminator setup complete, check again, in case access was lost during setup. If lost, close
	//
	// Circuit flow:
	//   1. Check access before circuit setup
	//   2. Add state listener
	//   3. Setup circuit
	//   4. Scan circuit. If circuit is older than interval N, but younger than N * 2, re-check access, in case access was lost in race condition
	//
	if !self.stateListener.enabled.Load() {
		self.stateListener.Lock()
		defer self.stateListener.Unlock()

		if self.stateListener.enabled.CompareAndSwap(false, true) {
			err := self.listener.factory.stateManager.RouterDataModel().SubscribeToIdentityChanges(self.getIdentityId(), self, false)
			if err != nil {
				pfxlog.Logger().
					WithField("identityId", self.getIdentityId()).
					WithError(err).
					Error("failed to subscribe to identity change event")
				self.stateListener.enabled.Store(false)
			}
		}
	}

	self.stateListener.lastRequired.Store(time.Now())
}

func (self *edgeClientConn) IsStateListenerEligibleForRemovalCheck() bool {
	return self.stateListener.enabled.Load() && time.Since(self.stateListener.lastRequired.Load()) > 5*time.Minute
}

func (self *edgeClientConn) removeStateListenerIfEligible() {
	self.stateListener.Lock()
	defer self.stateListener.Unlock()

	if !self.IsStateListenerEligibleForRemovalCheck() {
		return
	}

	if self.msgMux.GetConnCount() == 0 && self.GetHostedServicesRegistry().terminators.Count() == 0 {
		self.listener.factory.stateManager.RouterDataModel().UnsubscribeFromIdentityChanges(self.getIdentityId(), self)
		self.stateListener.enabled.Store(false)
	}
}

func (self *edgeClientConn) processConnect(req *channel.Message, ch channel.Channel) {
	serviceSessionTokenStr := string(req.Body)

	log := pfxlog.ContextLogger(ch.Label()).
		WithFields(sdkedge.GetLoggerFields(req)).
		WithField("identityId", self.getIdentityId())

	connId, found := req.GetUint32Header(sdkedge.ConnIdHeader)
	if !found {
		pfxlog.Logger().Errorf("connId not set. unable to process connect message")
		errStr := "connId not set, required"
		self.sendStateClosedReply(errStr, req)
		return
	}

	ctrlCh := self.apiSessionToken.SelectCtrlCh(self.listener.factory.ctrls)

	if ctrlCh == nil {
		errStr := "no controller available, cannot create circuit"
		log.Error(errStr)
		self.sendStateClosedReply(errStr, req)
		return
	}

	connectCtx := &connectContext{
		SdkConn: self,
		Log:     log,
		Req:     req,
		ConnId:  connId,
		CtrlId:  ctrlCh.PeerId(),
	}

	serviceSessionToken, err := self.listener.factory.stateManager.GetServiceSessionToken(serviceSessionTokenStr, self.apiSessionToken)

	if err != nil {
		errStr := err.Error()
		log.Error(err)
		self.sendStateClosedReply(errStr, req)
		return
	}

	if serviceSessionToken == nil {
		errStr := "no such service session token"
		log.Error(errStr)
		self.sendStateClosedReply(errStr, req)
		return
	}

	connectCtx.ServiceSessionToken = serviceSessionToken

	if err = self.checkAccess(serviceSessionToken.ServiceId, edge_ctrl_pb.PolicyType_DialPolicy); err != nil {
		log.WithError(err).Error("access denied")
		self.sendStateClosedReply(err.Error(), req)
		return
	}
	self.checkForStateListener()

	var handler connectHandler
	useXgToSdk, _ := req.GetBoolHeader(sdkedge.UseXgressToSdkHeader)
	if useXgToSdk {
		log.WithField("useXgress", true).Info("client requested sdk xgress flow-control")
		handler = &xgEdgeForwarder{
			edgeClientConn: self,
			serviceId:      serviceSessionToken.ServiceId,
			ctrlId:         ctrlCh.PeerId(),
			originator:     xgress.Initiator,
			metrics:        self.listener.factory.env.GetXgressMetrics(),
		}
	} else {
		log.WithField("useXgress", false).Info("client using legacy edge data flow")
		handler = &nonXgConnectHandler{}
	}

	if !handler.Init(connectCtx) {
		return
	}

	// fabric connect
	log.Debug("dialing fabric")
	peerData := make(map[uint32][]byte)

	for k, v := range peerHeaderRequestMappings {
		if pk, found := req.Headers[int32(k)]; found {
			peerData[v] = pk
		}
	}

	if identityId := self.getIdentityId(); identityId != "" {
		peerData[ctrl_msg.DialerIdentityIdHeader] = []byte(identityId)
		if identity, found := self.listener.factory.stateManager.RouterDataModel().Identities.Get(identityId); found {
			peerData[ctrl_msg.DialerIdentityNameHeader] = []byte(identity.Name)
		}
	}

	terminatorIdentity, _ := req.GetStringHeader(sdkedge.TerminatorIdentityHeader)

	request := &ctrl_msg.CreateCircuitV2Request{
		ApiSessionToken:      self.apiSessionToken.Token(),
		SessionToken:         serviceSessionTokenStr,
		Fingerprints:         self.fingerprints.Prints(),
		TerminatorInstanceId: terminatorIdentity,
		PeerData:             peerData,
	}

	response, err := self.sendCreateCircuitRequest(request, ctrlCh)

	handler.FinishConnect(connectCtx, response, err)
}

func (self *edgeClientConn) mapResponsePeerData(m map[uint32][]byte) {
	for k, v := range peerHeaderRespMappings {
		if val, ok := m[k]; ok {
			delete(m, k)
			m[v] = val
		}
	}
}

func (self *edgeClientConn) sendCreateCircuitRequest(req *ctrl_msg.CreateCircuitV2Request, ctrlCh ctrlchan.CtrlChannel) (*ctrl_msg.CreateCircuitV2Response, error) {
	timeout := self.listener.options.Options.GetCircuitTimeout
	msg, err := req.ToMessage().WithTimeout(timeout).SendForReply(ctrlCh.GetHighPrioritySender())
	if err != nil {
		return nil, err
	}
	if msg.ContentType == int32(edge_ctrl_pb.ContentType_ErrorType) {
		errMsg := string(msg.Body)
		if errMsg == "" {
			errMsg = "error state returned from controller with no message"
		}
		var resp *ctrl_msg.CreateCircuitV2Response
		if circuitId, found := msg.GetStringHeader(sdkedge.CircuitIdHeader); found {
			resp = &ctrl_msg.CreateCircuitV2Response{CircuitId: circuitId}
		}
		return resp, errors.New(errMsg)
	}

	if msg.ContentType != int32(edge_ctrl_pb.ContentType_CreateCircuitV2ResponseType) {
		return nil, errors.Errorf("unexpected response type %v to request. expected %v",
			msg.ContentType, edge_ctrl_pb.ContentType_CreateCircuitV2ResponseType)
	}

	return ctrl_msg.DecodeCreateCircuitV2Response(msg)
}

func (self *edgeClientConn) processBind(req *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).
		WithFields(sdkedge.GetLoggerFields(req)).
		WithField("routerId", self.listener.id.Token)

	ctrlCh := self.apiSessionToken.SelectCtrlCh(self.listener.factory.ctrls)

	if ctrlCh == nil {
		errStr := "no controller available, cannot create terminator"
		pfxlog.ContextLogger(ch.Label()).
			WithField("token", string(req.Body)).
			WithFields(sdkedge.GetLoggerFields(req)).
			WithField("routerId", self.listener.id.Token).
			Error(errStr)
		self.sendStateClosedReply(errStr, req)
		return
	}

	sessionTokenStr := string(req.Body)

	serviceSessionToken, err := self.listener.factory.stateManager.ParseServiceSessionJwt(sessionTokenStr, self.apiSessionToken)

	if err != nil {
		log.WithError(err).Error("unable to verify service session token")
		self.sendStateClosedReply(err.Error(), req)
		return
	}

	log = serviceSessionToken.AddLoggingFields(log)

	if serviceSessionToken.Claims.Type != common.ServiceSessionTypeBind {
		msg := fmt.Sprintf("rejecting bind, invalid session type. expected %v, got %v", common.ServiceSessionTypeBind, serviceSessionToken.Claims.Type)
		log.Error(msg)
		self.sendStateClosedReply(msg, req)
		return
	}

	if err = self.checkAccess(serviceSessionToken.ServiceId, edge_ctrl_pb.PolicyType_BindPolicy); err != nil {
		log.Error(err.Error())
		self.sendStateClosedReply(err.Error(), req)
		return
	}

	self.checkForStateListener()
	self.processBindV2(serviceSessionToken, req, ch)
}

func (self *edgeClientConn) processBindV2(serviceSessionToken *state.ServiceSessionToken, req *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).
		WithFields(sdkedge.GetLoggerFields(req)).
		WithField("routerId", self.listener.id.Token)

	log = serviceSessionToken.AddLoggingFields(log)

	if serviceSessionToken.Claims.IsLegacy && self.listener.factory.stateManager.WasLegacyServiceSessionRecentlyRemoved(serviceSessionToken.Token()) {
		log.Info("invalid session, not establishing terminator")
		self.sendStateClosedReply("invalid session", req)
		return
	}

	connId, found := req.GetUint32Header(sdkedge.ConnIdHeader)
	if !found {
		pfxlog.Logger().Errorf("connId not set. unable to process bind message")
		return
	}

	terminatorId := idgen.MustNewUUIDString()
	log = log.WithField("bindConnId", connId).WithField("terminatorId", terminatorId)

	listenerId, _ := req.GetStringHeader(sdkedge.ListenerId)
	if listenerId != "" {
		log = log.WithField("listenerId", listenerId)
	}

	terminatorInstance, _ := req.GetStringHeader(sdkedge.TerminatorIdentityHeader)

	assignIds, _ := req.GetBoolHeader(sdkedge.RouterProvidedConnId)
	log.Debugf("client requested router provided connection ids: %v", assignIds)

	cost := uint16(0)
	if costBytes, hasCost := req.Headers[sdkedge.CostHeader]; hasCost {
		cost = binary.LittleEndian.Uint16(costBytes)
	}

	precedence := edge_ctrl_pb.TerminatorPrecedence_Default
	if precedenceData, hasPrecedence := req.Headers[sdkedge.PrecedenceHeader]; hasPrecedence && len(precedenceData) > 0 {
		edgePrecedence := sdkedge.Precedence(precedenceData[0])
		switch edgePrecedence {
		case sdkedge.PrecedenceRequired:
			precedence = edge_ctrl_pb.TerminatorPrecedence_Required
		case sdkedge.PrecedenceFailed:
			precedence = edge_ctrl_pb.TerminatorPrecedence_Failed
		}
	}

	var terminatorInstanceSecret []byte
	if terminatorInstance != "" {
		terminatorInstanceSecret = req.Headers[sdkedge.TerminatorIdentitySecretHeader]
	}

	hostData := make(map[uint32][]byte)
	if pubKey, hasKey := req.Headers[sdkedge.PublicKeyHeader]; hasKey {
		hostData[uint32(sdkedge.PublicKeyHeader)] = pubKey
	}

	supportsInspect, _ := req.GetBoolHeader(sdkedge.SupportsInspectHeader)
	notifyEstablished, _ := req.GetBoolHeader(sdkedge.SupportsBindSuccessHeader)
	useSdkXgress, _ := req.GetBoolHeader(sdkedge.UseXgressToSdkHeader)

	terminator := &edgeTerminator{
		terminatorId:        terminatorId,
		MsgChannel:          *sdkedge.NewEdgeMsgChannel(self.ch, connId),
		edgeClientConn:      self,
		serviceSessionToken: serviceSessionToken,
		listenerId:          listenerId,
		cost:                cost,
		precedence:          precedence,
		instance:            terminatorInstance,
		instanceSecret:      terminatorInstanceSecret,
		hostData:            hostData,
		assignIds:           assignIds,
		notifyEstablished:   notifyEstablished,
		useSdkXgress:        useSdkXgress,
		supportsInspect:     supportsInspect,
		createTime:          time.Now(),
	}

	terminator.state.Store(xgress_common.TerminatorStateEstablishing)

	checkResult, err := self.listener.factory.hostedServices.checkForExistingListenerId(terminator)
	if err != nil {
		log.WithError(err).Error("error, cancelling processing")
		return
	}

	terminator = checkResult.terminator
	if terminator.state.Load() == xgress_common.TerminatorStateDeleting {
		return
	}

	if checkResult.previous == nil || checkResult.previous.serviceSessionToken != serviceSessionToken {
		// need to remove session remove listener on close
		terminator.onClose = self.listener.factory.stateManager.AddLegacyServiceSessionRemovedListener(serviceSessionToken, func(_ *state.ServiceSessionToken) {
			edgeErr := &EdgeError{
				Message:   "service session removed",
				Code:      sdkedge.ErrorCodeInvalidSession,
				RetryHint: sdkedge.RetryStartOver,
			}
			reason := "session ended"
			self.listener.factory.hostedServices.ensureSdkCloseSent(terminator, reason, edgeErr)
			terminator.close(self.listener.factory.hostedServices, true, reason)
		})
	}

	msg := sdkedge.NewStateConnectedMsg(connId)
	msg.ReplyTo(req)

	if assignIds {
		msg.PutBoolHeader(sdkedge.RouterProvidedConnId, true)
	}

	// this needs to go on the data channel to ensure it gets there before data gets there or a state closed msg
	err = msg.WithPriority(channel.High).WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetDefaultSender())
	if err != nil {
		pfxlog.Logger().WithFields(sdkedge.GetLoggerFields(msg)).WithError(err).Error("failed to send bind success response")
	}

	if checkResult.replaceExisting {
		log.Info("sending replacement terminator success to sdk")
		terminator.NotifyEstablished(edge_ctrl_pb.CreateTerminatorResult_Success)
		if terminator.supportsInspect {
			// Use fire-and-forget inspect instead of blocking SendForReply.
			// The registry run loop will send the inspect request via TrySend,
			// retry on interval, and close the terminator if no valid response
			// is received within the timeout.
			// Queue in a goroutine to avoid blocking the bind response, which
			// could cause the SDK to time out waiting — the very problem we're
			// trying to avoid.
			go self.listener.factory.hostedServices.queuePostCreateInspect(terminator)
		}
	} else {
		log.Info("establishing terminator")
		self.listener.factory.hostedServices.EstablishTerminator(terminator)
		if terminator.supportsInspect {
			go self.listener.factory.hostedServices.queuePostCreateInspect(terminator)
		}
		if listenerId == "" {
			// only removed dupes with a scan if we don't have an sdk provided key
			self.listener.factory.hostedServices.cleanupDuplicates(terminator)
		}
	}

	if err = self.checkAccess(serviceSessionToken.ServiceId, edge_ctrl_pb.PolicyType_BindPolicy); err != nil {
		log.WithError(err).Error("bind access lost while terminator setup, closing")
		edgeErr := &EdgeError{
			Message:   "bind access lost",
			Code:      sdkedge.ErrorCodeAccessDenied,
			RetryHint: sdkedge.RetryStartOver, // switch back to NotRetriable once timing issues are sorted out
			Cause:     err,
		}
		reason := "bind access lost"
		self.GetHostedServicesRegistry().ensureSdkCloseSent(terminator, reason, edgeErr)
		terminator.close(self.GetHostedServicesRegistry(), true, reason)
	}
}

func (self *edgeClientConn) checkAccess(serviceId string, policyType edge_ctrl_pb.PolicyType) error {
	if self.apiSessionToken.IsOidc() {
		stateManager := self.listener.factory.stateManager
		// if oidc we check on the router, legacy tokens are checked in the controller during terminator creation
		grantingPolicy, err := stateManager.HasAccess(self.apiSessionToken.IdentityId, self.apiSessionToken.Id, serviceId, policyType)

		if err != nil {
			return err
		}

		if grantingPolicy == nil {
			policyTypeDescriptor := "dial"
			if policyType == edge_ctrl_pb.PolicyType_BindPolicy {
				policyTypeDescriptor = "bind"
			}
			return errors.Errorf("no access to service, failed %s access check", policyTypeDescriptor)
		}
	}

	return nil
}

func (self *edgeClientConn) processUnbind(req *channel.Message, ch channel.Channel) {
	connId, _ := req.GetUint32Header(sdkedge.ConnIdHeader)
	sessionTokenStr := string(req.Body)

	apiSessionToken := state.GetApiSessionTokenFromCh(ch)

	log := pfxlog.ContextLogger(ch.Label()).
		WithFields(sdkedge.GetLoggerFields(req)).
		WithField("routerId", self.listener.id.Token)

	serviceSessionToken, err := self.listener.factory.stateManager.ParseServiceSessionJwt(sessionTokenStr, apiSessionToken)

	if err != nil {
		log.WithError(err).Warn("unable to verify service session token")
		self.sendStateClosedReply(err.Error(), req)
		return
	}

	if serviceSessionToken.Claims.Type != common.ServiceSessionTypeBind {
		msg := fmt.Sprintf("rejecting bind, invalid session type. expected %v, got %v", common.ServiceSessionTypeBind, serviceSessionToken.Claims.Type)
		log.Error(msg)
		self.sendStateClosedReply(msg, req)
		return
	}

	log = serviceSessionToken.AddLoggingFields(log)

	atLeastOneTerminatorRemoved := self.listener.factory.hostedServices.unbindSession(connId, serviceSessionToken, self)

	if !atLeastOneTerminatorRemoved {
		log.
			WithField("connId", connId).
			Info("no terminator found to unbind for token")
	}
}

func (self *edgeClientConn) processUpdateBind(req *channel.Message, ch channel.Channel) {
	connId, _ := req.GetUint32Header(sdkedge.ConnIdHeader)
	sessionTokenStr := string(req.Body)

	apiSessionToken := state.GetApiSessionTokenFromCh(ch)

	log := pfxlog.ContextLogger(ch.Label()).
		WithFields(sdkedge.GetLoggerFields(req)).
		WithField("routerId", self.listener.id.Token)

	serviceSessionToken, err := self.listener.factory.stateManager.ParseServiceSessionJwt(sessionTokenStr, apiSessionToken)

	if err != nil {
		log.WithError(err).Warn("unable to verify service session token")
		self.sendStateClosedReply(err.Error(), req)
		return
	}

	if serviceSessionToken.Claims.Type != common.ServiceSessionTypeBind {
		msg := fmt.Sprintf("rejecting bind, invalid session type. expected %v, got %v", common.ServiceSessionTypeBind, serviceSessionToken.Claims.Type)
		log.Error(msg)
		self.sendStateClosedReply(msg, req)
		return
	}

	log = serviceSessionToken.AddLoggingFields(log)

	terminators := self.listener.factory.hostedServices.getRelatedTerminators(connId, serviceSessionToken, self)

	if len(terminators) == 0 {
		log.Error("failed to update bind, no listener found")
		return
	}
	ctrlCh := self.apiSessionToken.SelectCtrlCh(self.listener.factory.ctrls)

	if ctrlCh == nil {
		log.Error("no controller available, cannot update terminator")
		return
	}

	for _, terminator := range terminators {
		request := &edge_ctrl_pb.UpdateTerminatorRequest{
			SessionToken:    serviceSessionToken.Token(),
			Fingerprints:    self.fingerprints.Prints(),
			TerminatorId:    terminator.terminatorId,
			ApiSessionToken: serviceSessionToken.ApiSessionToken.GetToken(),
		}

		if costVal, hasCost := req.GetUint16Header(sdkedge.CostHeader); hasCost {
			request.UpdateCost = true
			request.Cost = uint32(costVal)
		}

		if precedenceData, hasPrecedence := req.Headers[sdkedge.PrecedenceHeader]; hasPrecedence && len(precedenceData) > 0 {
			edgePrecedence := sdkedge.Precedence(precedenceData[0])
			request.Precedence = edge_ctrl_pb.TerminatorPrecedence_Default
			request.UpdatePrecedence = true
			if edgePrecedence == sdkedge.PrecedenceRequired {
				request.Precedence = edge_ctrl_pb.TerminatorPrecedence_Required
			} else if edgePrecedence == sdkedge.PrecedenceFailed {
				request.Precedence = edge_ctrl_pb.TerminatorPrecedence_Failed
			}
		}

		log = log.WithField("terminator", terminator.terminatorId).
			WithField("precedence", request.Precedence).
			WithField("cost", request.Cost).
			WithField("updatingPrecedence", request.UpdatePrecedence).
			WithField("updatingCost", request.UpdateCost)

		log.Debug("updating terminator")

		timeout := self.listener.factory.ctrls.DefaultRequestTimeout()
		responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(timeout).SendForReply(ctrlCh.GetDefaultSender())
		if err := xgress_common.CheckForFailureResult(responseMsg, err, edge_ctrl_pb.ContentType_UpdateTerminatorResponseType); err != nil {
			log.WithError(err).Error("terminator update failed")
		} else {
			log.Debug("terminator updated successfully")
		}
	}
}

func (self *edgeClientConn) processHealthEvent(req *channel.Message, ch channel.Channel) {
	sessionTokenStr := string(req.Body)

	apiSessionToken := state.GetApiSessionTokenFromCh(ch)

	log := pfxlog.ContextLogger(ch.Label()).
		WithFields(sdkedge.GetLoggerFields(req)).
		WithField("routerId", self.listener.id.Token)

	serviceSessionToken, err := self.listener.factory.stateManager.ParseServiceSessionJwt(sessionTokenStr, apiSessionToken)

	if err != nil {
		log.WithError(err).Warn("unable to verify service session token")
		self.sendStateClosedReply(err.Error(), req)
		return
	}

	log = serviceSessionToken.AddLoggingFields(log)

	ctrlCh := self.listener.factory.ctrls.AnyChannel()
	if ctrlCh == nil {
		log.Error("no controller available, cannot forward health event")
		return
	}

	terminator, ok := self.listener.factory.hostedServices.Get(serviceSessionToken.TokenId())

	if !ok {
		log.Error("failed to update bind, no listener found")
		return
	}

	checkPassed, _ := req.GetBoolHeader(sdkedge.HealthStatusHeader)

	request := &edge_ctrl_pb.HealthEventRequest{
		SessionToken:    serviceSessionToken.Token(),
		ApiSessionToken: serviceSessionToken.ApiSessionToken.GetToken(),
		Fingerprints:    self.fingerprints.Prints(),
		TerminatorId:    terminator.terminatorId,
		CheckPassed:     checkPassed,
	}

	log = log.WithField("terminator", terminator.terminatorId).WithField("checkPassed", checkPassed)
	log.Debug("sending health event")

	if err := protobufs.MarshalTyped(request).Send(ctrlCh); err != nil {
		log.WithError(err).Error("send of health event failed")
	}
}

func (self *edgeClientConn) processTraceRoute(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).WithFields(sdkedge.GetLoggerFields(msg))

	hops, _ := msg.GetUint32Header(sdkedge.TraceHopCountHeader)
	if hops > 0 {
		hops--
		msg.PutUint32Header(sdkedge.TraceHopCountHeader, hops)
	}

	log.WithField("hops", hops).Debug("traceroute received")
	if hops > 0 {
		self.msgMux.HandleReceive(msg, ch)
	} else {
		go self.sendTraceRouteResponse(msg, ch)
	}
}

func (self *edgeClientConn) sendTraceRouteResponse(msg *channel.Message, ch channel.Channel) {
	ts, _ := msg.GetUint64Header(sdkedge.TimestampHeader)
	connId, _ := msg.GetUint32Header(sdkedge.ConnIdHeader)
	hops, _ := msg.GetUint32Header(sdkedge.TraceHopCountHeader)
	resp := sdkedge.NewTraceRouteResponseMsg(connId, hops, ts, "xgress/edge", "")
	resp.ReplyTo(msg)
	if msgUUID := msg.Headers[sdkedge.UUIDHeader]; msgUUID != nil {
		resp.Headers[sdkedge.UUIDHeader] = msgUUID
	}

	if err := ch.Send(resp); err != nil {
		pfxlog.ContextLogger(ch.Label()).WithFields(sdkedge.GetLoggerFields(msg)).
			WithError(err).Error("failed to send hop response")
	}
}

func (self *edgeClientConn) sendConnectedReply(req *channel.Message, response *ctrl_msg.CreateCircuitV2Response) {
	connId, _ := req.GetUint32Header(sdkedge.ConnIdHeader)

	msg := sdkedge.NewStateConnectedMsg(connId)
	msg.ReplyTo(req)

	if assignIds, _ := req.GetBoolHeader(sdkedge.RouterProvidedConnId); assignIds {
		msg.PutBoolHeader(sdkedge.RouterProvidedConnId, true)
	}

	msg.PutStringHeader(sdkedge.CircuitIdHeader, response.CircuitId)

	self.mapResponsePeerData(response.PeerData)
	for k, v := range response.PeerData {
		msg.Headers[int32(k)] = v
	}

	// this needs to go on the data channel to ensure it gets there before data gets there or a state closed msg
	err := msg.WithPriority(channel.High).WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetDefaultSender())
	if err != nil {
		pfxlog.Logger().WithFields(sdkedge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
		return
	}
}

func (self *edgeClientConn) sendStateClosedReply(message string, req *channel.Message, headers ...channel.Headers) {
	connId, _ := req.GetUint32Header(sdkedge.ConnIdHeader)
	msg := sdkedge.NewStateClosedMsg(connId, message)
	msg.ReplyTo(req)

	if errorCode, found := req.GetUint32Header(sdkedge.ErrorCodeHeader); found {
		msg.PutUint32Header(sdkedge.ErrorCodeHeader, errorCode)
	}

	for _, h := range headers {
		for k, v := range h {
			msg.Headers[k] = v
		}
	}

	err := msg.WithPriority(channel.High).WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetDefaultSender())
	if err != nil {
		pfxlog.Logger().WithFields(sdkedge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
	}
}

func (self *edgeClientConn) processPostureResponse(msg *channel.Message, ch channel.Channel) {
	if msg.ContentType == int32(edge_client_pb.ContentType_PostureResponseType) {
		postureResponses := &edge_client_pb.PostureResponses{}

		if err := proto.Unmarshal(msg.Body, postureResponses); err != nil {
			pfxlog.Logger().WithError(err).Error("failed to unmarshal posture responses")
		}

		go self.listener.factory.stateManager.ProcessPostureResponses(ch, postureResponses)

	}
}

func (self *edgeClientConn) processTokenUpdate(req *channel.Message, ch channel.Channel) {
	currentApiSession := state.GetApiSessionTokenFromCh(ch)

	if currentApiSession == nil || currentApiSession.JwtToken == nil || currentApiSession.Claims.ApiSessionId == "" {
		retErr := NewInvalidApiSessionType("current connection isn't authenticated via JWT beater tokens, unable to switch to them")
		reply := sdkedge.NewUpdateTokenFailedMsg(retErr)

		retErr.ApplyToMsg(reply)

		if err := reply.ReplyTo(req).WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("failed to send error: " + err.Error())
		}
		_ = ch.Close()
		return
	}

	newTokenStr := string(req.Body)

	if !xgress_common.IsBearerToken(newTokenStr) {
		retErr := NewInvalidApiSessionTokenError("invalid token, could not be parsed")
		reply := sdkedge.NewUpdateTokenFailedMsg(retErr)

		retErr.ApplyToMsg(reply)

		if err := reply.ReplyTo(req).WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("failed to send error: " + err.Error())
		}
		_ = ch.Close()
		return
	}

	newApiSessionToken, err := self.listener.factory.stateManager.ParseApiSessionJwt(newTokenStr)

	if err != nil {
		retErr := NewInvalidApiSessionTokenError("invalid token, invalid signature")
		reply := sdkedge.NewUpdateTokenFailedMsg(retErr)

		retErr.ApplyToMsg(reply)

		if err := reply.ReplyTo(req).WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("failed to send error: " + err.Error())
		}
		_ = ch.Close()
		return
	}

	if newApiSessionToken.Claims.ApiSessionId != currentApiSession.Claims.ApiSessionId {
		retErr := NewInvalidApiSessionTokenError("invalid token, API session id does not match")
		reply := sdkedge.NewUpdateTokenFailedMsg(retErr)

		retErr.ApplyToMsg(reply)

		if err := reply.ReplyTo(req).WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("failed to send error: " + err.Error())
		}
		_ = ch.Close()
		return
	}

	if err := self.listener.factory.stateManager.HandleClientApiSessionTokenUpdate(newApiSessionToken); err != nil {
		retErr := NewInvalidApiSessionTokenError(err.Error())
		reply := sdkedge.NewUpdateTokenFailedMsg(errors.Wrap(err, ""))

		retErr.ApplyToMsg(reply)

		if err := reply.ReplyTo(req).WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("failed to send error: " + err.Error())
		}

		_ = ch.Close()
		return
	}

	reply := sdkedge.NewUpdateTokenSuccessMsg()
	reply.ReplyTo(req)

	if err := ch.Send(reply); err != nil {
		logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("error responding to token update request with success")
	}
}

func (self *edgeClientConn) handleXgClose(msg *channel.Message, _ channel.Channel) {
	circuitId := string(msg.Body)
	log := pfxlog.Logger().WithField("circuitId", circuitId)
	if edgeForwarder, ok := self.xgCircuits.Get(circuitId); ok {
		log.Debug("received close request from sdk, closing sdk-xg circuit")
		go self.cleanupXgressCircuit(edgeForwarder)
	} else {
		log.Debug("received close request from sdk, but no edge forwarder found")
	}
}

func (self *edgeClientConn) handleXgPayload(msg *channel.Message, _ channel.Channel) {
	payload, err := xgress.UnmarshallPayload(msg)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to unmarshal xgress payload")
		return
	}

	edgeFwd, _ := self.xgCircuits.Get(payload.CircuitId)
	if edgeFwd == nil {
		if !payload.IsCircuitEndFlagSet() && !payload.IsFlagEOFSet() {
			pfxlog.Logger().WithFields(payload.GetLoggerFields()).Debug("no xgress edge forwarder for circuit")
		}
		return
	}

	if err = self.forwarder.ForwardPayload(edgeFwd.address, payload, 0); err != nil {
		if !payload.IsCircuitEndFlagSet() && !payload.IsFlagEOFSet() {
			pfxlog.Logger().WithFields(payload.GetLoggerFields()).WithError(err).Debug("failed to forward payload")
		}

		if !channel.IsTimeout(err) {
			self.forwarder.ReportForwardingFault(payload.CircuitId, "") // ctrlId will be filled in by forwarder, if possible
		}
	} else {
		if !payload.IsRetransmitFlagSet() {
			edgeFwd.metrics.Rx(edgeFwd, edgeFwd.originator, payload)
		}
	}
}

func (self *edgeClientConn) handleXgAcknowledgement(req *channel.Message, _ channel.Channel) {
	ack, err := xgress.UnmarshallAcknowledgement(req)
	if err != nil {
		// pfxlog.Logger().WithError(err).Error("failed to unmarshal xgress acknowledgement")

		// send a close, since we can't forward anything
		connId, _ := req.GetUint32Header(sdkedge.ConnIdHeader)
		msg := sdkedge.NewStateClosedMsg(connId, "xgress closed")
		msg.PutUint32Header(sdkedge.ConnIdHeader, connId)
		_, _ = self.ch.GetControlSender().TrySend(msg)

		return
	}

	edgeFwd, _ := self.xgCircuits.Get(ack.CircuitId)
	if edgeFwd == nil {
		pfxlog.Logger().WithField("circuitId", ack.CircuitId).Error("no edge forwarder found for edge circuit")
		return
	}

	if err = self.forwarder.ForwardAcknowledgement(edgeFwd.address, ack); err != nil {
		pfxlog.Logger().WithFields(ack.GetLoggerFields()).WithError(err).Error("failed to forward acknowledgement")
	}
}

type connectHandler interface {
	Init(ctx *connectContext) bool
	FinishConnect(ctx *connectContext, response *ctrl_msg.CreateCircuitV2Response, err error)
}

type connectContext struct {
	SdkConn             *edgeClientConn
	Log                 *logrus.Entry
	Req                 *channel.Message
	ConnId              uint32
	CtrlId              string
	PolicyType          edge_ctrl_pb.PolicyType
	ServiceSessionToken *state.ServiceSessionToken
}

type nonXgConnectHandler struct {
	conn *edgeXgressConn
}

func (self *nonXgConnectHandler) Init(ctx *connectContext) bool {
	self.conn = &edgeXgressConn{
		mux:        ctx.SdkConn.msgMux,
		MsgChannel: *sdkedge.NewEdgeMsgChannel(ctx.SdkConn.ch, ctx.ConnId),
		seq:        NewMsgQueue(4),
	}

	self.conn.SetData(&state.ConnState{
		ServiceSessionToken: ctx.ServiceSessionToken,
		ApiSessionToken:     ctx.SdkConn.apiSessionToken,
		PolicyType:          edge_ctrl_pb.PolicyType_DialPolicy,
	})

	// need to remove session remove listener on close
	stateManager := ctx.SdkConn.listener.factory.stateManager

	self.conn.onClose = stateManager.AddLegacyServiceSessionRemovedListener(ctx.ServiceSessionToken, func(_ *state.ServiceSessionToken) {
		self.conn.close(true, "session closed")
	})

	// We can't fix conn id, since it's provided by the client
	if err := ctx.SdkConn.msgMux.Add(self.conn); err != nil {
		ctx.Log.WithError(err).Error("error adding to msg mux")
		ctx.SdkConn.sendStateClosedReply(err.Error(), ctx.Req)
		return false
	}

	return true
}

func (self *nonXgConnectHandler) FinishConnect(ctx *connectContext, response *ctrl_msg.CreateCircuitV2Response, err error) {
	if err != nil {
		ctx.Log.WithError(err).Warn("failed to dial fabric")
		var headers channel.Headers
		if response != nil && response.CircuitId != "" {
			headers = channel.Headers{}
			headers.PutStringHeader(sdkedge.CircuitIdHeader, response.CircuitId)
		}
		ctx.SdkConn.sendStateClosedReply(err.Error(), ctx.Req, headers)
		self.conn.close(false, "failed to dial fabric")
		return
	}

	ctx.SdkConn.mapResponsePeerData(response.PeerData)

	xgOptions := &ctx.SdkConn.listener.options.Options
	x := xgress.NewXgress(response.CircuitId, ctx.CtrlId, xgress.Address(response.Address), self.conn, xgress.Initiator, xgOptions, response.Tags)
	ctx.SdkConn.listener.bindHandler.HandleXgressBind(x)
	self.conn.x.Store(x)

	// send the state_connected before starting the xgress. That way we can't get a state_closed before we get state_connected
	ctx.Log.WithField("circuitId", response.CircuitId).
		WithField("connId", self.conn.Id()).
		Info("sending connected response with legacy edge data flow")
	ctx.SdkConn.sendConnectedReply(ctx.Req, response)

	x.Start()
}

type xgEdgeForwarder struct {
	*edgeClientConn
	circuitId       string
	serviceId       string
	originator      xgress.Originator
	ctrlId          string
	address         xgress.Address
	lastRx          atomic.Int64
	connId          uint32
	metrics         env.XgressMetrics
	tags            map[string]string
	accessCheckDone atomic.Bool
}

func (self *xgEdgeForwarder) GetDestinationType() string {
	return "xg-edge-fwd"
}

func (self *xgEdgeForwarder) GetIntervalId() string {
	return self.circuitId
}

func (self *xgEdgeForwarder) GetTags() map[string]string {
	return self.tags
}

func (self *xgEdgeForwarder) GetCircuitId() string {
	return self.circuitId
}

func (self *xgEdgeForwarder) GetServiceId() string {
	return self.serviceId
}

func (self *xgEdgeForwarder) GetApiSessionId() string {
	return self.edgeClientConn.GetApiSessionToken().Id
}

func (self *xgEdgeForwarder) CloseForDialAccessLoss() {
	self.cleanupXgressCircuit(self)
}

func (self *xgEdgeForwarder) IsPostCreateAccessCheckNeeded() bool {
	// Only OIDC sessions need post-create access checks on the router.
	// Legacy sessions have their access verified by the controller.
	isOidc := self.apiSessionToken != nil && self.apiSessionToken.IsOidc()
	return self.originator == xgress.Initiator && !self.accessCheckDone.Load() && isOidc
}

func (self *xgEdgeForwarder) SetPostCreateAccessCheckDone() {
	self.accessCheckDone.Store(true)
}

func (self *xgEdgeForwarder) SendPayload(payload *xgress.Payload, timeout time.Duration, _ xgress.PayloadType) error {
	msg := payload.Marshall()
	msg.PutUint32Header(sdkedge.ConnIdHeader, self.connId)
	if timeout == 0 {
		sent, err := self.ch.GetDefaultSender().TrySend(msg)
		if err == nil && !sent {
			self.listener.droppedMsgMeter.Mark(1)
			self.listener.droppedPayloadsMeter.Mark(1)

			pfxlog.Logger().WithField("circuitId", payload.CircuitId).Debug("payload to xgress sdk dropped")
			if !payload.IsRetransmitFlagSet() {
				self.metrics.Tx(self, self.originator, payload)
			}
		}
		return err
	}

	self.lastRx.Store(time.Now().UnixMilli())

	if err := msg.WithTimeout(timeout).Send(self.ch.GetDefaultSender()); err != nil {
		self.listener.droppedMsgMeter.Mark(1)
		self.listener.droppedPayloadsMeter.Mark(1)
		return err
	}

	if !payload.IsRetransmitFlagSet() {
		self.metrics.Tx(self, self.originator, payload)
	}

	return nil
}

func (self *xgEdgeForwarder) SendAcknowledgement(ack *xgress.Acknowledgement) error {
	msg := ack.Marshall()
	msg.PutUint32Header(sdkedge.ConnIdHeader, self.connId)
	sent, err := self.ch.GetDefaultSender().TrySend(msg)
	if err == nil && !sent {
		self.listener.droppedMsgMeter.Mark(1)
		self.listener.droppedAcksMeter.Mark(1)
	}

	self.lastRx.Store(time.Now().UnixMilli())

	return err
}

func (self *xgEdgeForwarder) SendControl(ctrl *xgress.Control) error {
	msg := ctrl.Marshall()
	msg.PutUint32Header(sdkedge.ConnIdHeader, self.connId)
	sent, err := self.ch.GetDefaultSender().TrySend(msg)
	if err == nil && !sent {
		self.listener.droppedMsgMeter.Mark(1)
	}
	return err
}

func (self *xgEdgeForwarder) Init(ctx *connectContext) bool {
	self.connId = ctx.ConnId
	return true
}

func (self *xgEdgeForwarder) RegisterRouting() {
	pfxlog.Logger().WithField("circuitId", self.circuitId).Debug("routing registered")
	self.forwarder.RegisterDestination(self.circuitId, self.address, self)
	self.xgCircuits.Set(self.circuitId, self)
}

func (self *xgEdgeForwarder) UnregisterRouting() {
	pfxlog.Logger().WithField("circuitId", self.circuitId).Debug("routing unregistered")
	self.forwarder.EndCircuit(self.circuitId)
}

func (self *xgEdgeForwarder) FinishConnect(ctx *connectContext, response *ctrl_msg.CreateCircuitV2Response, err error) {
	if err != nil {
		ctx.Log.WithError(err).Warn("failed to dial fabric")
		var headers channel.Headers
		if response != nil && response.CircuitId != "" {
			headers = channel.Headers{}
			headers.PutStringHeader(sdkedge.CircuitIdHeader, response.CircuitId)
		}
		ctx.SdkConn.sendStateClosedReply(err.Error(), ctx.Req, headers)
		return
	}

	self.circuitId = response.CircuitId
	self.address = xgress.Address(response.Address)
	self.tags = response.Tags
	self.RegisterRouting()

	connId, _ := ctx.Req.GetUint32Header(sdkedge.ConnIdHeader)

	msg := sdkedge.NewStateConnectedMsg(connId)
	msg.ReplyTo(ctx.Req)

	if assignIds, _ := ctx.Req.GetBoolHeader(sdkedge.RouterProvidedConnId); assignIds {
		msg.PutBoolHeader(sdkedge.RouterProvidedConnId, true)
	}

	msg.PutStringHeader(sdkedge.CircuitIdHeader, self.circuitId)
	msg.PutBoolHeader(sdkedge.UseXgressToSdkHeader, true)
	msg.PutStringHeader(sdkedge.XgressCtrlIdHeader, ctx.CtrlId)
	msg.PutStringHeader(sdkedge.XgressAddressHeader, response.Address)

	ctx.Log.WithField("circuitId", self.circuitId).
		WithField("useXgress", true).
		Info("sending connected response with xgress flow-control")

	self.mapResponsePeerData(response.PeerData)
	for k, v := range response.PeerData {
		msg.Headers[int32(k)] = v
	}

	// this needs to go on the data channel to ensure it gets there before data gets there or a state closed msg
	if err = msg.WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetControlSender()); err != nil {
		pfxlog.Logger().WithFields(sdkedge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
	}
}

// Unrouted signals that a circuit is unrouted, either due to a message from the controller or a policy change
func (self *xgEdgeForwarder) Unrouted() {
	pfxlog.Logger().WithField("circuitId", self.circuitId).Debug("unroute: start")
	defer pfxlog.Logger().WithField("circuitId", self.circuitId).Debug("unroute: complete")
	self.xgCircuits.Remove(self.circuitId)

	msg := sdkedge.NewStateClosedMsg(self.connId, "xgress unrouted")
	err := msg.WithPriority(channel.High).WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetDefaultSender())
	if err != nil {
		pfxlog.Logger().WithField("circuitId", self.circuitId).
			WithFields(sdkedge.GetLoggerFields(msg)).WithError(err).Error("failed to send state closed")
	}
}

func (self *xgEdgeForwarder) GetTimeOfLastRxFromLink() int64 {
	return self.lastRx.Load()
}

func (self *xgEdgeForwarder) InspectCircuit(detail *xgress.CircuitInspectDetail) {
	detail.AddRelatedEntity("edge", fmt.Sprintf("%v", self.connId), self.GetCircuitInspectDetail())

	var requestedValue string
	if detail.IncludeGoroutines() {
		requestedValue = "circuitandstacks:" + self.circuitId
	} else {
		requestedValue = "circuit:" + detail.CircuitId
	}
	msg := sdkedge.NewInspectRequest(&self.connId, requestedValue)
	reply, err := msg.WithTimeout(2 * time.Second).SendForReply(self.ch.GetControlSender())
	if err != nil {
		detail.AddError(fmt.Errorf("failed to get sdk xgress response, originator: %s, (%w)", self.originator.String(), err))
		return
	}

	resp := &sdkinspect.SdkInspectResponse{}
	if err = json.Unmarshal(reply.Body, &resp); err != nil {
		detail.AddError(fmt.Errorf("failed to unmarshall sdk xgress response, originator: %s, (%w)", self.originator.String(), err))
		return
	}

	if v, ok := resp.Values[requestedValue]; ok {
		jsonString, err := json.Marshal(v)
		if err != nil {
			detail.AddError(fmt.Errorf("failed to marshall sdk xgress detail, originator: %s, (%w)", self.originator.String(), err))
			return
		}
		sdkXgDetail := &xgress.InspectDetail{}
		if err = json.Unmarshal(jsonString, &sdkXgDetail); err != nil {
			detail.AddError(fmt.Errorf("failed to unmarshall sdk xgress detail, originator: %s, (%w)", self.originator.String(), err))
			return
		}
		detail.AddXgressDetail(sdkXgDetail)
	} else {
		detail.AddError(fmt.Errorf("sdk xgress state not returned, originator: %s", self.originator.String()))
	}
}

func (self *xgEdgeForwarder) GetCircuitInspectDetail() *inspect.EdgeXgFwdInspectDetail {
	return &inspect.EdgeXgFwdInspectDetail{
		ChannelConnId:       self.ch.GetChannel().ConnectionId(),
		IdentityId:          self.apiSessionToken.IdentityId,
		CircuitId:           self.circuitId,
		EdgeConnId:          self.connId,
		CtrlId:              self.ctrlId,
		Originator:          self.originator.String(),
		Address:             string(self.address),
		TimeSinceLastLinkRx: (time.Duration(time.Now().UnixMilli()-self.lastRx.Load()) * time.Millisecond).String(),
		Tags:                self.tags,
	}
}
