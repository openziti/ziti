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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	sdkinspect "github.com/openziti/sdk-golang/inspect"
	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/sdk-golang/xgress"
	sdkedge "github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/capabilities"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/common/ctrl_msg"
	"github.com/openziti/ziti/common/inspect"
	fabricMetrics "github.com/openziti/ziti/common/metrics"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/idgen"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/state"
	"github.com/openziti/ziti/router/xgress_common"
	"github.com/openziti/ziti/router/xgress_router"
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

// GetConns returns all active connections managed by this edgeClientConn's message multiplexer.
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

func (self *edgeClientConn) HandleClose(ch channel.Channel) {
	log := pfxlog.ContextLogger(self.ch.GetChannel().Label())
	log.Debugf("closing")
	self.listener.factory.hostedServices.cleanupServices(ch)
	self.msgMux.Close()
	self.cleanupXgressCircuits()
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
	ch := controllers.GetCtrlChannel(edgeForwarder.ctrlId)
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

func (self *edgeClientConn) processConnect(req *channel.Message, ch channel.Channel) {
	serviceSessionTokenStr := string(req.Body)

	log := pfxlog.ContextLogger(ch.Label()).WithFields(sdkedge.GetLoggerFields(req))

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
		CtrlCh:  ctrlCh,
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

	if self.apiSessionToken.IsOidc() {
		//if oidc we check on the router, legacy tokens are checked in the controller during circuit creation
		grantingPolicy, err := self.listener.factory.stateManager.HasDialAccess(self.apiSessionToken.IdentityId, self.apiSessionToken.Id, serviceSessionToken.ServiceId)

		if err != nil {
			errStr := err.Error()
			log.Error(err)
			self.sendStateClosedReply(errStr, req)
		}

		if grantingPolicy == nil {
			errStr := "no access to service, failed dial access check"
			log.Error(errStr)
			self.sendStateClosedReply(errStr, req)
			return
		}
	}

	var handler connectHandler
	if useXgToSdk, _ := req.GetBoolHeader(sdkedge.UseXgressToSdkHeader); useXgToSdk {
		log.Debug("use sdk xgress set, setting up sdk flow-control connection")
		handler = &xgEdgeForwarder{
			edgeClientConn: self,
			ctrlId:         ctrlCh.Id(),
			originator:     xgress.Initiator,
			metrics:        self.listener.factory.env.GetXgressMetrics(),
		}
	} else {
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

	terminatorIdentity, _ := req.GetStringHeader(sdkedge.TerminatorIdentityHeader)

	request := &ctrl_msg.CreateCircuitRequest{
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

func (self *edgeClientConn) sendCreateCircuitRequest(req *ctrl_msg.CreateCircuitRequest, ctrlCh channel.Channel) (*ctrl_msg.CreateCircuitResponse, error) {
	if capabilities.IsCapable(ctrlCh, capabilities.ControllerCreateCircuitV2) {
		return self.sendCreateCircuitRequestV2(req, ctrlCh)
	}
	return self.sendCreateCircuitRequestV1(req, ctrlCh)
}

func (self *edgeClientConn) sendCreateCircuitRequestV1(req *ctrl_msg.CreateCircuitRequest, ctrlCh channel.Channel) (*ctrl_msg.CreateCircuitResponse, error) {
	request := &edge_ctrl_pb.CreateCircuitRequest{
		SessionToken:         req.SessionToken,
		ApiSessionToken:      req.ApiSessionToken,
		Fingerprints:         req.Fingerprints,
		TerminatorInstanceId: req.TerminatorInstanceId,
		PeerData:             req.PeerData,
	}

	response := &edge_ctrl_pb.CreateCircuitResponse{}
	timeout := self.listener.options.Options.GetCircuitTimeout
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(timeout).SendForReply(ctrlCh)
	if err = getResultOrFailure(responseMsg, err, response); err != nil {
		return nil, err
	}

	return &ctrl_msg.CreateCircuitResponse{
		CircuitId: response.CircuitId,
		Address:   response.Address,
		PeerData:  response.PeerData,
		Tags:      response.Tags,
	}, nil
}

func (self *edgeClientConn) sendCreateCircuitRequestV2(req *ctrl_msg.CreateCircuitRequest, ctrlCh channel.Channel) (*ctrl_msg.CreateCircuitResponse, error) {
	timeout := self.listener.options.Options.GetCircuitTimeout
	msg, err := req.ToMessage().WithTimeout(timeout).SendForReply(ctrlCh)
	if err != nil {
		return nil, err
	}
	if msg.ContentType == int32(edge_ctrl_pb.ContentType_ErrorType) {
		msg := string(msg.Body)
		if msg == "" {
			msg = "error state returned from controller with no message"
		}
		return nil, errors.New(msg)
	}

	if msg.ContentType != int32(edge_ctrl_pb.ContentType_CreateCircuitV2ResponseType) {
		return nil, errors.Errorf("unexpected response type %v to request. expected %v",
			msg.ContentType, edge_ctrl_pb.ContentType_CreateCircuitV2ResponseType)
	}

	return ctrl_msg.DecodeCreateCircuitResponse(msg)
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

	if self.apiSessionToken.IsOidc() {
		//if oidc we check on the router, legacy tokens are checked in the controller during terminator creation
		grantingPolicy, err := self.listener.factory.stateManager.HasBindAccess(self.apiSessionToken.IdentityId, self.apiSessionToken.Id, serviceSessionToken.ServiceId)

		if err != nil {
			errStr := err.Error()
			log.Error(err)
			self.sendStateClosedReply(errStr, req)
		}

		if grantingPolicy == nil {
			errStr := "no access to service, failed bind access check"
			log.Error(errStr)
			self.sendStateClosedReply(errStr, req)
			return
		}
	}

	supportsCreateTerminatorV2 := capabilities.IsCapable(ctrlCh, capabilities.ControllerCreateTerminatorV2)
	if supportsCreateTerminatorV2 {
		self.processBindV2(serviceSessionToken, req, ch, ctrlCh)
	} else {
		self.processBindV1(serviceSessionToken, req, ch, ctrlCh)
	}
}

func (self *edgeClientConn) processBindV1(serviceSessionToken *state.ServiceSessionToken, req *channel.Message, ch channel.Channel, ctrlCh channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).
		WithFields(sdkedge.GetLoggerFields(req)).
		WithField("routerId", self.listener.id.Token)

	log = serviceSessionToken.AddLoggingFields(log)

	connId, found := req.GetUint32Header(sdkedge.ConnIdHeader)
	if !found {
		pfxlog.Logger().Errorf("connId not set. unable to process bind message")
		return
	}

	log.Debug("binding service")

	hostData := make(map[uint32][]byte)
	pubKey, hasKey := req.Headers[sdkedge.PublicKeyHeader]
	if hasKey {
		hostData[uint32(sdkedge.PublicKeyHeader)] = pubKey
	}

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

	assignIds, _ := req.GetBoolHeader(sdkedge.RouterProvidedConnId)
	log.Debugf("client requested router provided connection ids: %v", assignIds)

	useSdkXgress, _ := req.GetBoolHeader(sdkedge.UseXgressToSdkHeader)

	log.Debug("establishing listener")

	terminator := &edgeTerminator{
		MsgChannel:          *sdkedge.NewEdgeMsgChannel(self.ch, connId),
		edgeClientConn:      self,
		serviceSessionToken: serviceSessionToken,
		assignIds:           assignIds,
		useSdkXgress:        useSdkXgress,
		createTime:          time.Now(),
	}

	// need to remove session remove listener on close
	terminator.onClose = self.listener.factory.stateManager.AddLegacyServiceSessionRemovedListener(serviceSessionToken, func(_ *state.ServiceSessionToken) {
		terminator.close(self.listener.factory.hostedServices, true, true, "session ended")
	})

	self.listener.factory.hostedServices.PutV1(serviceSessionToken, terminator)

	terminatorIdentity, _ := req.GetStringHeader(sdkedge.TerminatorIdentityHeader)
	var terminatorIdentitySecret []byte
	if terminatorIdentity != "" {
		terminatorIdentitySecret = req.Headers[sdkedge.TerminatorIdentitySecretHeader]
	}

	request := &edge_ctrl_pb.CreateTerminatorRequest{
		SessionToken:   serviceSessionToken.JwtToken.Raw,
		Fingerprints:   self.fingerprints.Prints(),
		PeerData:       hostData,
		Cost:           uint32(cost),
		Precedence:     precedence,
		InstanceId:     terminatorIdentity,
		InstanceSecret: terminatorIdentitySecret,
	}

	request.ApiSessionToken = serviceSessionToken.ApiSessionToken.GetToken()

	timeout := self.listener.factory.ctrls.DefaultRequestTimeout()
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(timeout).SendForReply(ctrlCh)
	if err = xgress_common.CheckForFailureResult(responseMsg, err, edge_ctrl_pb.ContentType_CreateTerminatorResponseType); err != nil {
		log.WithError(err).Warn("error creating terminator")
		terminator.close(self.listener.factory.hostedServices, false, false, "") // don't notify here, as we're notifying next line with a response
		self.sendStateClosedReply(err.Error(), req)
		return
	}

	terminatorId := string(responseMsg.Body)
	terminator.lock.Lock()
	terminator.terminatorId = terminatorId
	terminator.lock.Unlock()

	log = log.WithField("terminatorId", terminatorId)

	if terminator.MsgChannel.GetChannel().IsClosed() {
		log.Warn("edge channel closed while setting up terminator. cleaning up terminator now")
		terminator.close(self.listener.factory.hostedServices, false, true, "edge channel closed")
		return
	}

	log.Debug("registered listener for terminator")
	log.Debug("returning connection state CONNECTED to client")

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

	log.Info("created terminator")
}

func (self *edgeClientConn) processBindV2(serviceSessionToken *state.ServiceSessionToken, req *channel.Message, ch channel.Channel, ctrlCh channel.Channel) {

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
		useSdkXgress:        useSdkXgress,
		v2:                  true,
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
			terminator.close(self.listener.factory.hostedServices, true, true, "session ended")
		})
	}

	terminator.establishCallback = func(result edge_ctrl_pb.CreateTerminatorResult) {
		if result == edge_ctrl_pb.CreateTerminatorResult_Success && notifyEstablished {
			notifyMsg := channel.NewMessage(sdkedge.ContentTypeBindSuccess, nil)
			notifyMsg.PutUint32Header(sdkedge.ConnIdHeader, terminator.MsgChannel.Id())

			if err := notifyMsg.WithTimeout(time.Second * 30).Send(terminator.MsgChannel.GetControlSender()); err != nil {
				log.WithError(err).Error("failed to send bind success")
			} else {
				log.Info("sdk notified of terminator creation")
			}
		} else if result == edge_ctrl_pb.CreateTerminatorResult_FailedInvalidSession {
			// TODO: notify of invalid session. Currently handling this using the recently removed sessions
			//       LRU cache in state manager
			log.Trace("invalid session")
		}
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
		terminator.establishCallback(edge_ctrl_pb.CreateTerminatorResult_Success)
		if terminator.supportsInspect {
			go func() {
				if _, err := terminator.inspect(self.listener.factory.hostedServices, true, true); err != nil {
					log.WithError(err).Info("failed to check sdk side of terminator after replace")
				}
			}()
		}
	} else {
		log.Info("establishing terminator")
		self.listener.factory.hostedServices.EstablishTerminator(terminator)
		if listenerId == "" {
			// only removed dupes with a scan if we don't have an sdk provided key
			self.listener.factory.hostedServices.cleanupDuplicates(terminator)
		}
	}
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

func (self *edgeClientConn) removeTerminator(ctrlCh channel.Channel, token, terminatorId string) error {
	request := &edge_ctrl_pb.RemoveTerminatorRequest{
		SessionToken: token,
		Fingerprints: self.fingerprints.Prints(),
		TerminatorId: terminatorId,
	}

	timeout := self.listener.factory.ctrls.DefaultRequestTimeout()
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(timeout).SendForReply(ctrlCh)
	return xgress_common.CheckForFailureResult(responseMsg, err, edge_ctrl_pb.ContentType_RemoveTerminatorResponseType)
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
		responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(timeout).SendForReply(ctrlCh)
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

	ctrlCh := self.listener.factory.ctrls.AnyCtrlChannel()
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
		ts, _ := msg.GetUint64Header(sdkedge.TimestampHeader)
		connId, _ := msg.GetUint32Header(sdkedge.ConnIdHeader)
		resp := sdkedge.NewTraceRouteResponseMsg(connId, hops, ts, "xgress/edge", "")
		resp.ReplyTo(msg)
		if msgUUID := msg.Headers[sdkedge.UUIDHeader]; msgUUID != nil {
			resp.Headers[sdkedge.UUIDHeader] = msgUUID
		}

		if err := ch.Send(resp); err != nil {
			log.WithError(err).Error("failed to send hop response")
		}
	}
}

func (self *edgeClientConn) sendConnectedReply(req *channel.Message, response *ctrl_msg.CreateCircuitResponse) {
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

func (self *edgeClientConn) sendStateClosedReply(message string, req *channel.Message) {
	connId, _ := req.GetUint32Header(sdkedge.ConnIdHeader)
	msg := sdkedge.NewStateClosedMsg(connId, message)
	msg.ReplyTo(req)

	if errorCode, found := req.GetUint32Header(sdkedge.ErrorCodeHeader); found {
		msg.PutUint32Header(sdkedge.ErrorCodeHeader, errorCode)
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
		reply.ReplyTo(req)

		if err := ch.Send(reply); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("failed to send error: " + err.Error())
		}
		return
	}

	newTokenStr := string(req.Body)

	if !xgress_common.IsBearerToken(newTokenStr) {
		retErr := NewInvalidApiSessionTokenError("message did not contain a valid JWT bearer token")
		reply := sdkedge.NewUpdateTokenFailedMsg(retErr)

		retErr.ApplyToMsg(reply)
		reply.ReplyTo(req)

		if err := ch.Send(reply); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("failed to send error: " + err.Error())
		}
		return
	}

	newApiSessionToken, err := self.listener.factory.stateManager.ParseApiSessionJwt(newTokenStr)

	if err != nil {
		retErr := NewInvalidApiSessionTokenError("api session JWT bearer token failed to parse or validate")
		reply := sdkedge.NewUpdateTokenFailedMsg(retErr)

		retErr.ApplyToMsg(reply)
		reply.ReplyTo(req)

		if err := ch.Send(reply); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("failed to send error: " + err.Error())
		}
		return
	}

	if newApiSessionToken.Claims.ApiSessionId != currentApiSession.Claims.ApiSessionId {
		retErr := NewInvalidApiSessionTokenError("api session JWT bearer token does not match current connection's api session id")
		reply := sdkedge.NewUpdateTokenFailedMsg(retErr)

		retErr.ApplyToMsg(reply)
		reply.ReplyTo(req)

		if err := ch.Send(reply); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("failed to send error: " + err.Error())
		}
	}

	if err := self.listener.factory.stateManager.HandleClientApiSessionTokenUpdate(newApiSessionToken); err != nil {
		retErr := NewInvalidApiSessionTokenError(err.Error())
		reply := sdkedge.NewUpdateTokenFailedMsg(errors.Wrap(err, ""))

		retErr.ApplyToMsg(reply)
		reply.ReplyTo(req)

		if err := ch.Send(reply); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("failed to send error: " + err.Error())
		}
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
		self.cleanupXgressCircuit(edgeForwarder)
	} else {
		log.Debug("received close request from sdk, but no edge forwarder found")
	}
}

func (self *edgeClientConn) handleXgPayload(msg *channel.Message, ch channel.Channel) {
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

func (self *edgeClientConn) handleXgAcknowledgement(req *channel.Message, ch channel.Channel) {
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

func getResultOrFailure(msg *channel.Message, err error, result protobufs.TypedMessage) error {
	if err != nil {
		return err
	}

	if msg.ContentType == int32(edge_ctrl_pb.ContentType_ErrorType) {
		msg := string(msg.Body)
		if msg == "" {
			msg = "error state returned from controller with no message"
		}
		return errors.New(msg)
	}

	if msg.ContentType != result.GetContentType() {
		return errors.Errorf("unexpected response type %v to request. expected %v", msg.ContentType, result.GetContentType())
	}

	return proto.Unmarshal(msg.Body, result)
}

type connectHandler interface {
	Init(ctx *connectContext) bool
	FinishConnect(ctx *connectContext, response *ctrl_msg.CreateCircuitResponse, err error)
}

type connectContext struct {
	SdkConn             *edgeClientConn
	Log                 *logrus.Entry
	Req                 *channel.Message
	ConnId              uint32
	CtrlCh              channel.Channel
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

func (self *nonXgConnectHandler) FinishConnect(ctx *connectContext, response *ctrl_msg.CreateCircuitResponse, err error) {
	if err != nil {
		ctx.Log.WithError(err).Warn("failed to dial fabric")
		ctx.SdkConn.sendStateClosedReply(err.Error(), ctx.Req)
		self.conn.close(false, "failed to dial fabric")
		return
	}

	ctx.SdkConn.mapResponsePeerData(response.PeerData)

	xgOptions := &ctx.SdkConn.listener.options.Options
	x := xgress.NewXgress(response.CircuitId, ctx.CtrlCh.Id(), xgress.Address(response.Address), self.conn, xgress.Initiator, xgOptions, response.Tags)
	ctx.SdkConn.listener.bindHandler.HandleXgressBind(x)
	self.conn.ctrlRx = x

	// send the state_connected before starting the xgress. That way we can't get a state_closed before we get state_connected
	ctx.SdkConn.sendConnectedReply(ctx.Req, response)

	x.Start()
}

type xgEdgeForwarder struct {
	*edgeClientConn
	circuitId  string
	originator xgress.Originator
	ctrlId     string
	address    xgress.Address
	lastRx     atomic.Int64
	connId     uint32
	metrics    env.XgressMetrics
	tags       map[string]string
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
	// TODO: figure out how to handle session removed
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
	self.xgCircuits.Set(self.circuitId, self)
}

func (self *xgEdgeForwarder) FinishConnect(ctx *connectContext, response *ctrl_msg.CreateCircuitResponse, err error) {
	if err != nil {
		ctx.Log.WithError(err).Warn("failed to dial fabric")
		ctx.SdkConn.sendStateClosedReply(err.Error(), ctx.Req)
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
	msg.PutStringHeader(sdkedge.XgressCtrlIdHeader, ctx.CtrlCh.Id())
	msg.PutStringHeader(sdkedge.XgressAddressHeader, response.Address)

	self.mapResponsePeerData(response.PeerData)
	for k, v := range response.PeerData {
		msg.Headers[int32(k)] = v
	}

	// this needs to go on the data channel to ensure it gets there before data gets there or a state closed msg
	if err = msg.WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetControlSender()); err != nil {
		pfxlog.Logger().WithFields(sdkedge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
	}
}

// msg from controller that a circuit is unrouted
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
