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
	"github.com/openziti/metrics"
	sdkinspect "github.com/openziti/sdk-golang/inspect"
	"github.com/openziti/ziti/common/ctrl_msg"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/idgen"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xgress_router"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/sirupsen/logrus"
	"go.uber.org/atomic"
	"time"

	"github.com/openziti/ziti/common/capabilities"
	"github.com/openziti/ziti/common/cert"
	fabricMetrics "github.com/openziti/ziti/common/metrics"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/router/state"
	"github.com/openziti/ziti/router/xgress_common"
)

var peerHeaderRequestMappings = map[uint32]uint32{
	uint32(edge.PublicKeyHeader):        uint32(edge.PublicKeyHeader),
	uint32(edge.CallerIdHeader):         uint32(edge.CallerIdHeader),
	uint32(edge.AppDataHeader):          uint32(edge.AppDataHeader),
	uint32(edge.ConnectionMarkerHeader): uint32(edge.ConnectionMarkerHeader),
	uint32(edge.StickinessTokenHeader):  uint32(ctrl_msg.XtStickinessToken),
}

var peerHeaderRespMappings = map[uint32]uint32{
	ctrl_msg.XtStickinessToken: uint32(edge.StickinessTokenHeader),
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

func (listener *listener) Inspect(key string, timeout time.Duration) any {
	if key == "router-edge-circuits" {
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
	} else if key == "router-sdk-circuits" {
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
					idBase := next.conn.apiSession.IdentityId + "/" +
						next.conn.ch.GetChannel().ConnectionId()
					for _, circuit := range next.circuits.Circuits {
						detail := &inspect.SdkCircuitDetail{
							IdentityId:    next.conn.apiSession.IdentityId,
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
	msg := edge.NewInspectRequest(nil, "circuits")
	reply, err := msg.WithTimeout(4800 * time.Millisecond).SendForReply(conn.ch.GetControlSender())
	if err != nil {
		listener.submitErrResponse(conn, resultCh, fmt.Errorf("unable to get circuits from identity '%s' conn '%v' (%w)",
			conn.apiSession.Id, conn.ch.GetChannel().ConnectionId(), err))
		return
	}

	resp := sdkinspect.SdkInspectResponse{}
	err = json.Unmarshal(reply.Body, &resp)
	if err != nil {
		listener.submitErrResponse(conn, resultCh, fmt.Errorf("unable to unmarshal circuits from identity '%s' conn '%v' (%w)",
			conn.apiSession.Id, conn.ch.GetChannel().ConnectionId(), err))
		return
	}

	if v, ok := resp.Values["circuits"]; ok {
		jsonString, err := json.Marshal(v)
		if err != nil {
			listener.submitErrResponse(conn, resultCh, fmt.Errorf("failed to marshal sdk circuits from identity '%s' conn '%v' (%w)",
				conn.apiSession.Id, conn.ch.GetChannel().ConnectionId(), err))
			return
		}
		circuitsDetails := &xgress.CircuitsDetail{}
		if err = json.Unmarshal(jsonString, &circuitsDetails); err != nil {
			listener.submitErrResponse(conn, resultCh, fmt.Errorf("failed to unmarshal sdk circuits from identity '%s' conn '%v' (%w)",
				conn.apiSession.Id, conn.ch.GetChannel().ConnectionId(), err))
			return
		}

		listener.submitResponse(resultCh, &sdkCircuitResult{
			conn:     conn,
			circuits: circuitsDetails,
		})

	} else {
		listener.submitErrResponse(conn, resultCh, fmt.Errorf("sdk circuit details not returned from identity '%s' conn '%v' (%w)",
			conn.apiSession.Id, conn.ch.GetChannel().ConnectionId(), err))
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
	accepter := NewAcceptor(listener, listener.underlayListener, nil)
	go accepter.Run()

	return nil
}

func (listener *listener) Close() error {
	return listener.underlayListener.Close()
}

type edgeClientConn struct {
	msgMux       edge.MsgMux
	listener     *listener
	fingerprints cert.Fingerprints
	ch           edge.SdkChannel
	idSeq        uint32
	apiSession   *state.ApiSession
	forwarder    env.Forwarder
	xgCircuits   cmap.ConcurrentMap[string, *xgEdgeForwarder]
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
		self.cleanupXgressCircuit(entry.Val, true)
	}
}

func (self *edgeClientConn) cleanupXgressCircuit(edgeForwarder *xgEdgeForwarder, sendEndOfCircuit bool) {
	circuitId := edgeForwarder.circuitId
	log := pfxlog.Logger().WithField("circuitId", circuitId)

	if sendEndOfCircuit {
		self.forwarder.EndCircuit(circuitId)
	}

	self.xgCircuits.Remove(circuitId)

	// Notify the controller of the xgress fault
	fault := &ctrl_pb.Fault{Id: circuitId}
	if edgeForwarder.originator == xgress.Initiator {
		fault.Subject = ctrl_pb.FaultSubject_IngressFault
	} else if edgeForwarder.originator == xgress.Terminator {
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
	return edge.ContentTypeData
}

func (self *edgeClientConn) processConnect(req *channel.Message, ch channel.Channel) {
	sessionToken := string(req.Body)
	log := pfxlog.ContextLogger(ch.Label()).WithField("token", sessionToken).WithFields(edge.GetLoggerFields(req))
	connId, found := req.GetUint32Header(edge.ConnIdHeader)
	if !found {
		pfxlog.Logger().Errorf("connId not set. unable to process connect message")
		return
	}
	ctrlCh := self.apiSession.SelectCtrlCh(self.listener.factory.ctrls)

	if ctrlCh == nil {
		errStr := "no controller available, cannot create circuit"
		log.Error(errStr)
		self.sendStateClosedReply(errStr, req)
		return
	}

	connectCtx := &connectContext{
		sdkConn:      self,
		log:          log,
		req:          req,
		connId:       connId,
		sessionToken: sessionToken,
		ctrlCh:       ctrlCh,
	}

	var handler connectHandler
	if useXgToSdk, _ := req.GetBoolHeader(edge.UseXgressToSdkHeader); useXgToSdk {
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

	terminatorIdentity, _ := req.GetStringHeader(edge.TerminatorIdentityHeader)

	request := &ctrl_msg.CreateCircuitRequest{
		ApiSessionToken:      self.apiSession.Token,
		SessionToken:         sessionToken,
		Fingerprints:         self.fingerprints.Prints(),
		TerminatorInstanceId: terminatorIdentity,
		PeerData:             peerData,
	}

	if xgress_common.IsBearerToken(sessionToken) {
		manager := self.listener.factory.stateManager
		apiSession := manager.GetApiSessionFromCh(ch)

		if apiSession == nil {
			pfxlog.Logger().Errorf("could not find api session for channel, unable to process bind message")
			return
		}

		request.ApiSessionToken = apiSession.Token
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

func (self *edgeClientConn) processBind(manager state.Manager, req *channel.Message, ch channel.Channel) {
	ctrlCh := self.apiSession.SelectCtrlCh(self.listener.factory.ctrls)

	if ctrlCh == nil {
		errStr := "no controller available, cannot create terminator"
		pfxlog.ContextLogger(ch.Label()).
			WithField("token", string(req.Body)).
			WithFields(edge.GetLoggerFields(req)).
			WithField("routerId", self.listener.id.Token).
			Error(errStr)
		self.sendStateClosedReply(errStr, req)
		return
	}

	supportsCreateTerminatorV2 := capabilities.IsCapable(ctrlCh, capabilities.ControllerCreateTerminatorV2)
	if supportsCreateTerminatorV2 {
		self.processBindV2(manager, req, ch, ctrlCh)
	} else {
		self.processBindV1(manager, req, ch, ctrlCh)
	}
}

func (self *edgeClientConn) processBindV1(manager state.Manager, req *channel.Message, ch channel.Channel, ctrlCh channel.Channel) {
	sessionToken := string(req.Body)

	log := pfxlog.ContextLogger(ch.Label()).
		WithField("sessionToken", sessionToken).
		WithFields(edge.GetLoggerFields(req)).
		WithField("routerId", self.listener.id.Token)

	connId, found := req.GetUint32Header(edge.ConnIdHeader)
	if !found {
		pfxlog.Logger().Errorf("connId not set. unable to process bind message")
		return
	}

	log.Debug("binding service")

	hostData := make(map[uint32][]byte)
	pubKey, hasKey := req.Headers[edge.PublicKeyHeader]
	if hasKey {
		hostData[uint32(edge.PublicKeyHeader)] = pubKey
	}

	cost := uint16(0)
	if costBytes, hasCost := req.Headers[edge.CostHeader]; hasCost {
		cost = binary.LittleEndian.Uint16(costBytes)
	}

	precedence := edge_ctrl_pb.TerminatorPrecedence_Default
	if precedenceData, hasPrecedence := req.Headers[edge.PrecedenceHeader]; hasPrecedence && len(precedenceData) > 0 {
		edgePrecedence := edge.Precedence(precedenceData[0])
		if edgePrecedence == edge.PrecedenceRequired {
			precedence = edge_ctrl_pb.TerminatorPrecedence_Required
		} else if edgePrecedence == edge.PrecedenceFailed {
			precedence = edge_ctrl_pb.TerminatorPrecedence_Failed
		}
	}

	assignIds, _ := req.GetBoolHeader(edge.RouterProvidedConnId)
	log.Debugf("client requested router provided connection ids: %v", assignIds)

	useSdkXgress, _ := req.GetBoolHeader(edge.UseXgressToSdkHeader)

	log.Debug("establishing listener")

	terminator := &edgeTerminator{
		MsgChannel:     *edge.NewEdgeMsgChannel(self.ch, connId),
		edgeClientConn: self,
		token:          sessionToken,
		assignIds:      assignIds,
		useSdkXgress:   useSdkXgress,
		createTime:     time.Now(),
	}

	// need to remove session remove listener on close
	terminator.onClose = self.listener.factory.stateManager.AddEdgeSessionRemovedListener(sessionToken, func(token string) {
		terminator.close(self.listener.factory.hostedServices, true, true, "session ended")
	})

	self.listener.factory.hostedServices.PutV1(sessionToken, terminator)

	terminatorIdentity, _ := req.GetStringHeader(edge.TerminatorIdentityHeader)
	var terminatorIdentitySecret []byte
	if terminatorIdentity != "" {
		terminatorIdentitySecret = req.Headers[edge.TerminatorIdentitySecretHeader]
	}

	request := &edge_ctrl_pb.CreateTerminatorRequest{
		SessionToken:   sessionToken,
		Fingerprints:   self.fingerprints.Prints(),
		PeerData:       hostData,
		Cost:           uint32(cost),
		Precedence:     precedence,
		InstanceId:     terminatorIdentity,
		InstanceSecret: terminatorIdentitySecret,
	}

	if xgress_common.IsBearerToken(sessionToken) {
		apiSession := manager.GetApiSessionFromCh(ch)

		if apiSession == nil {
			pfxlog.Logger().Errorf("could not find api session for channel, unable to process bind message")
			return
		}

		request.ApiSessionToken = apiSession.Token
	}

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

	msg := edge.NewStateConnectedMsg(connId)
	msg.ReplyTo(req)

	if assignIds {
		msg.PutBoolHeader(edge.RouterProvidedConnId, true)
	}

	// this needs to go on the data channel to ensure it gets there before data gets there or a state closed msg
	err = msg.WithPriority(channel.High).WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetDefaultSender())
	if err != nil {
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send bind success response")
	}

	log.Info("created terminator")
}

func (self *edgeClientConn) processBindV2(manager state.Manager, req *channel.Message, ch channel.Channel, ctrlCh channel.Channel) {
	sessionToken := string(req.Body)

	log := pfxlog.ContextLogger(ch.Label()).
		WithField("sessionToken", sessionToken).
		WithFields(edge.GetLoggerFields(req)).
		WithField("routerId", self.listener.id.Token)

	if self.listener.factory.stateManager.WasSessionRecentlyRemoved(sessionToken) {
		log.Info("invalid session, not establishing terminator")
		self.sendStateClosedReply("invalid session", req)
		return
	}

	connId, found := req.GetUint32Header(edge.ConnIdHeader)
	if !found {
		pfxlog.Logger().Errorf("connId not set. unable to process bind message")
		return
	}

	terminatorId := idgen.NewUUIDString()
	log = log.WithField("bindConnId", connId).WithField("terminatorId", terminatorId)

	listenerId, _ := req.GetStringHeader(edge.ListenerId)
	if listenerId != "" {
		log = log.WithField("listenerId", listenerId)
	}

	terminatorInstance, _ := req.GetStringHeader(edge.TerminatorIdentityHeader)

	assignIds, _ := req.GetBoolHeader(edge.RouterProvidedConnId)
	log.Debugf("client requested router provided connection ids: %v", assignIds)

	cost := uint16(0)
	if costBytes, hasCost := req.Headers[edge.CostHeader]; hasCost {
		cost = binary.LittleEndian.Uint16(costBytes)
	}

	precedence := edge_ctrl_pb.TerminatorPrecedence_Default
	if precedenceData, hasPrecedence := req.Headers[edge.PrecedenceHeader]; hasPrecedence && len(precedenceData) > 0 {
		edgePrecedence := edge.Precedence(precedenceData[0])
		if edgePrecedence == edge.PrecedenceRequired {
			precedence = edge_ctrl_pb.TerminatorPrecedence_Required
		} else if edgePrecedence == edge.PrecedenceFailed {
			precedence = edge_ctrl_pb.TerminatorPrecedence_Failed
		}
	}

	var terminatorInstanceSecret []byte
	if terminatorInstance != "" {
		terminatorInstanceSecret = req.Headers[edge.TerminatorIdentitySecretHeader]
	}

	hostData := make(map[uint32][]byte)
	if pubKey, hasKey := req.Headers[edge.PublicKeyHeader]; hasKey {
		hostData[uint32(edge.PublicKeyHeader)] = pubKey
	}

	supportsInspect, _ := req.GetBoolHeader(edge.SupportsInspectHeader)
	notifyEstablished, _ := req.GetBoolHeader(edge.SupportsBindSuccessHeader)
	useSdkXgress, _ := req.GetBoolHeader(edge.UseXgressToSdkHeader)

	terminator := &edgeTerminator{
		terminatorId:    terminatorId,
		MsgChannel:      *edge.NewEdgeMsgChannel(self.ch, connId),
		edgeClientConn:  self,
		token:           sessionToken,
		listenerId:      listenerId,
		cost:            cost,
		precedence:      precedence,
		instance:        terminatorInstance,
		instanceSecret:  terminatorInstanceSecret,
		hostData:        hostData,
		assignIds:       assignIds,
		useSdkXgress:    useSdkXgress,
		v2:              true,
		supportsInspect: supportsInspect,
		createTime:      time.Now(),
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

	if checkResult.previous == nil || checkResult.previous.token != sessionToken {
		// need to remove session remove listener on close
		terminator.onClose = self.listener.factory.stateManager.AddEdgeSessionRemovedListener(sessionToken, func(token string) {
			terminator.close(self.listener.factory.hostedServices, true, true, "session ended")
		})
	}

	terminator.establishCallback = func(result edge_ctrl_pb.CreateTerminatorResult) {
		if result == edge_ctrl_pb.CreateTerminatorResult_Success && notifyEstablished {
			notifyMsg := channel.NewMessage(edge.ContentTypeBindSuccess, nil)
			notifyMsg.PutUint32Header(edge.ConnIdHeader, terminator.MsgChannel.Id())

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

	msg := edge.NewStateConnectedMsg(connId)
	msg.ReplyTo(req)

	if assignIds {
		msg.PutBoolHeader(edge.RouterProvidedConnId, true)
	}

	// this needs to go on the data channel to ensure it gets there before data gets there or a state closed msg
	err = msg.WithPriority(channel.High).WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetDefaultSender())
	if err != nil {
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send bind success response")
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

func (self *edgeClientConn) processUnbind(manager state.Manager, req *channel.Message, _ channel.Channel) {
	connId, _ := req.GetUint32Header(edge.ConnIdHeader)
	token := string(req.Body)
	atLeastOneTerminatorRemoved := self.listener.factory.hostedServices.unbindSession(connId, token, self)

	if !atLeastOneTerminatorRemoved {
		pfxlog.Logger().
			WithField("connId", connId).
			WithField("token", token).
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

func (self *edgeClientConn) processUpdateBind(manager state.Manager, req *channel.Message, ch channel.Channel) {
	sessionToken := string(req.Body)

	connId, _ := req.GetUint32Header(edge.ConnIdHeader)
	log := pfxlog.ContextLogger(ch.Label()).WithField("sessionToken", sessionToken).WithFields(edge.GetLoggerFields(req))
	terminators := self.listener.factory.hostedServices.getRelatedTerminators(connId, sessionToken, self)

	if len(terminators) == 0 {
		log.Error("failed to update bind, no listener found")
		return
	}
	ctrlCh := self.apiSession.SelectCtrlCh(self.listener.factory.ctrls)

	if ctrlCh == nil {
		log.Error("no controller available, cannot update terminator")
		return
	}

	for _, terminator := range terminators {
		request := &edge_ctrl_pb.UpdateTerminatorRequest{
			SessionToken: sessionToken,
			Fingerprints: self.fingerprints.Prints(),
			TerminatorId: terminator.terminatorId,
		}

		if xgress_common.IsBearerToken(sessionToken) {
			apiSession := manager.GetApiSessionFromCh(ch)
			request.ApiSessionToken = apiSession.Token
		}

		if costVal, hasCost := req.GetUint16Header(edge.CostHeader); hasCost {
			request.UpdateCost = true
			request.Cost = uint32(costVal)
		}

		if precedenceData, hasPrecedence := req.Headers[edge.PrecedenceHeader]; hasPrecedence && len(precedenceData) > 0 {
			edgePrecedence := edge.Precedence(precedenceData[0])
			request.Precedence = edge_ctrl_pb.TerminatorPrecedence_Default
			request.UpdatePrecedence = true
			if edgePrecedence == edge.PrecedenceRequired {
				request.Precedence = edge_ctrl_pb.TerminatorPrecedence_Required
			} else if edgePrecedence == edge.PrecedenceFailed {
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

func (self *edgeClientConn) processHealthEvent(manager state.Manager, req *channel.Message, ch channel.Channel) {
	sessionToken := string(req.Body)
	log := pfxlog.ContextLogger(ch.Label()).WithField("sessionId", sessionToken).WithFields(edge.GetLoggerFields(req))

	ctrlCh := self.listener.factory.ctrls.AnyCtrlChannel()
	if ctrlCh == nil {
		log.Error("no controller available, cannot forward health event")
		return
	}

	terminator, ok := self.listener.factory.hostedServices.Get(sessionToken)

	if !ok {
		log.Error("failed to update bind, no listener found")
		return
	}

	checkPassed, _ := req.GetBoolHeader(edge.HealthStatusHeader)

	request := &edge_ctrl_pb.HealthEventRequest{
		SessionToken: sessionToken,
		Fingerprints: self.fingerprints.Prints(),
		TerminatorId: terminator.terminatorId,
		CheckPassed:  checkPassed,
	}

	log = log.WithField("terminator", terminator.terminatorId).WithField("checkPassed", checkPassed)

	if xgress_common.IsBearerToken(sessionToken) {
		apiSession := manager.GetApiSessionFromCh(ch)
		request.ApiSessionToken = apiSession.Token
	}

	log = log.WithField("terminator", terminator.terminatorId).WithField("checkPassed", checkPassed)
	log.Debug("sending health event")

	if err := protobufs.MarshalTyped(request).Send(ctrlCh); err != nil {
		log.WithError(err).Error("send of health event failed")
	}
}

func (self *edgeClientConn) processTraceRoute(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).WithFields(edge.GetLoggerFields(msg))

	hops, _ := msg.GetUint32Header(edge.TraceHopCountHeader)
	if hops > 0 {
		hops--
		msg.PutUint32Header(edge.TraceHopCountHeader, hops)
	}

	log.WithField("hops", hops).Debug("traceroute received")
	if hops > 0 {
		self.msgMux.HandleReceive(msg, ch)
	} else {
		ts, _ := msg.GetUint64Header(edge.TimestampHeader)
		connId, _ := msg.GetUint32Header(edge.ConnIdHeader)
		resp := edge.NewTraceRouteResponseMsg(connId, hops, ts, "xgress/edge", "")
		resp.ReplyTo(msg)
		if msgUUID := msg.Headers[edge.UUIDHeader]; msgUUID != nil {
			resp.Headers[edge.UUIDHeader] = msgUUID
		}

		if err := ch.Send(resp); err != nil {
			log.WithError(err).Error("failed to send hop response")
		}
	}
}

func (self *edgeClientConn) sendConnectedReply(req *channel.Message, response *ctrl_msg.CreateCircuitResponse) {
	connId, _ := req.GetUint32Header(edge.ConnIdHeader)

	msg := edge.NewStateConnectedMsg(connId)
	msg.ReplyTo(req)

	if assignIds, _ := req.GetBoolHeader(edge.RouterProvidedConnId); assignIds {
		msg.PutBoolHeader(edge.RouterProvidedConnId, true)
	}

	msg.PutStringHeader(edge.CircuitIdHeader, response.CircuitId)

	self.mapResponsePeerData(response.PeerData)
	for k, v := range response.PeerData {
		msg.Headers[int32(k)] = v
	}

	// this needs to go on the data channel to ensure it gets there before data gets there or a state closed msg
	err := msg.WithPriority(channel.High).WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetDefaultSender())
	if err != nil {
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
		return
	}
}

func (self *edgeClientConn) sendStateClosedReply(message string, req *channel.Message) {
	connId, _ := req.GetUint32Header(edge.ConnIdHeader)
	msg := edge.NewStateClosedMsg(connId, message)
	msg.ReplyTo(req)

	if errorCode, found := req.GetUint32Header(edge.ErrorCodeHeader); found {
		msg.PutUint32Header(edge.ErrorCodeHeader, errorCode)
	}

	err := msg.WithPriority(channel.High).WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetDefaultSender())
	if err != nil {
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
	}
}

func (self *edgeClientConn) processTokenUpdate(manager state.Manager, req *channel.Message, ch channel.Channel) {
	currentApiSession := self.listener.factory.stateManager.GetApiSessionFromCh(ch)

	if currentApiSession == nil || currentApiSession.JwtToken == nil {
		msg := edge.NewUpdateTokenFailedMsg(errors.New("current connection isn't authenticated via JWT beater tokens, unable to switch to them"))
		msg.ReplyTo(req)
		return
	}

	newTokenStr := string(req.Body)

	if !xgress_common.IsBearerToken(newTokenStr) {
		msg := edge.NewUpdateTokenFailedMsg(errors.New("message did not contain a valid JWT bearer token"))
		msg.ReplyTo(req)
		return
	}

	newToken, newClaims, err := self.listener.factory.stateManager.ParseJwt(newTokenStr)

	if err != nil {
		reply := edge.NewUpdateTokenFailedMsg(errors.Wrap(err, "JWT bearer token failed to validate"))
		reply.ReplyTo(req)
		if err := ch.Send(reply); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("error responding to token update request with validation failure")
		}
		return
	}

	newApiSession, err := state.NewApiSessionFromToken(newToken, newClaims)
	if err != nil {
		reply := edge.NewUpdateTokenFailedMsg(errors.Wrap(err, "failed to update a JWT based api session"))
		reply.ReplyTo(req)

		if err := ch.Send(reply); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("error responding to token update request with update failure")
		}
		return
	}

	if err := self.listener.factory.stateManager.UpdateChApiSession(ch, newApiSession); err != nil {
		reply := edge.NewUpdateTokenFailedMsg(errors.Wrap(err, "failed to update a JWT based api session"))
		reply.ReplyTo(req)

		if err := ch.Send(reply); err != nil {
			logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("error responding to token update request with update failure")
		}
		return
	}

	reply := edge.NewUpdateTokenSuccessMsg()
	reply.ReplyTo(req)

	if err := ch.Send(reply); err != nil {
		logrus.WithError(err).WithField("reqSeq", reply.Sequence()).Error("error responding to token update request with success")
	}
}

func (self *edgeClientConn) handleXgClose(msg *channel.Message, _ channel.Channel) {
	circuitId := string(msg.Body)
	if edgeForwarder, ok := self.xgCircuits.Get(circuitId); ok {
		self.cleanupXgressCircuit(edgeForwarder, false)
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
		self.responseToMissingXgress(msg, payload)
		return
	}

	// pfxlog.Logger().Infof("forwarding payload from sdk circuitId: %s, seq: %d", payload.CircuitId, payload.Sequence)

	if err = self.forwarder.ForwardPayload(edgeFwd.address, payload, 0); err != nil {
		if !channel.IsTimeout(err) {
			pfxlog.Logger().WithFields(payload.GetLoggerFields()).WithError(err).Error("unable to forward payload")
			self.forwarder.ReportForwardingFault(payload.CircuitId, "") // ctrlId will be filled in by forwarder, if possible
			return
		} else {
			pfxlog.Logger().WithFields(payload.GetLoggerFields()).WithError(err).Error("failed to forward payload")
		}
	} else {
		if !payload.IsRetransmitFlagSet() {
			edgeFwd.metrics.Rx(edgeFwd, edgeFwd.originator, payload)
		}
	}
}

func (self *edgeClientConn) responseToMissingXgress(req *channel.Message, payload *xgress.Payload) {
	var msg *channel.Message
	connId, _ := req.GetUint32Header(edge.ConnIdHeader)
	if len(payload.Data) == 0 && payload.IsCircuitEndFlagSet() {
		ack := xgress.NewAcknowledgement(payload.CircuitId, payload.GetOriginator().Invert())
		msg = ack.Marshall()
	} else {
		msg = edge.NewStateClosedMsg(connId, "xgress closed")
	}

	msg.PutUint32Header(edge.ConnIdHeader, connId)
	_, _ = self.ch.GetControlSender().TrySend(msg)
}

func (self *edgeClientConn) handleXgAcknowledgement(req *channel.Message, ch channel.Channel) {
	ack, err := xgress.UnmarshallAcknowledgement(req)
	if err != nil {
		// pfxlog.Logger().WithError(err).Error("failed to unmarshal xgress acknowledgement")

		// send a close, since we can't forward anything
		connId, _ := req.GetUint32Header(edge.ConnIdHeader)
		msg := edge.NewStateClosedMsg(connId, "xgress closed")
		msg.PutUint32Header(edge.ConnIdHeader, connId)
		_, _ = self.ch.GetControlSender().TrySend(msg)

		return
	}

	edgeFwd, _ := self.xgCircuits.Get(ack.CircuitId)
	if edgeFwd == nil {
		pfxlog.Logger().WithField("circuitId", ack.CircuitId).Error("no edge forwarder found for edge circuit")
		return
	}

	// pfxlog.Logger().Infof("forwarding ack from sdk circuitId: %s, seq: %d", ack.CircuitId, ack.Sequence)

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
	sdkConn      *edgeClientConn
	log          *logrus.Entry
	req          *channel.Message
	connId       uint32
	sessionToken string
	ctrlCh       channel.Channel
}

type nonXgConnectHandler struct {
	conn *edgeXgressConn
}

func (self *nonXgConnectHandler) Init(ctx *connectContext) bool {
	self.conn = &edgeXgressConn{
		mux:        ctx.sdkConn.msgMux,
		MsgChannel: *edge.NewEdgeMsgChannel(ctx.sdkConn.ch, ctx.connId),
		seq:        NewMsgQueue(4),
	}

	// need to remove session remove listener on close
	stateManager := ctx.sdkConn.listener.factory.stateManager
	self.conn.onClose = stateManager.AddEdgeSessionRemovedListener(ctx.sessionToken, func(token string) {
		self.conn.close(true, "session closed")
	})

	// We can't fix conn id, since it's provided by the client
	if err := ctx.sdkConn.msgMux.AddMsgSink(self.conn); err != nil {
		ctx.log.WithError(err).WithField("token", ctx.sessionToken).Error("error adding to msg mux")
		ctx.sdkConn.sendStateClosedReply(err.Error(), ctx.req)
		return false
	}

	return true
}

func (self *nonXgConnectHandler) FinishConnect(ctx *connectContext, response *ctrl_msg.CreateCircuitResponse, err error) {
	if err != nil {
		ctx.log.WithError(err).Warn("failed to dial fabric")
		ctx.sdkConn.sendStateClosedReply(err.Error(), ctx.req)
		self.conn.close(false, "failed to dial fabric")
		return
	}

	ctx.sdkConn.mapResponsePeerData(response.PeerData)

	xgOptions := &ctx.sdkConn.listener.options.Options
	x := xgress.NewXgress(response.CircuitId, ctx.ctrlCh.Id(), xgress.Address(response.Address), self.conn, xgress.Initiator, xgOptions, response.Tags)
	ctx.sdkConn.listener.bindHandler.HandleXgressBind(x)
	self.conn.ctrlRx = x

	// send the state_connected before starting the xgress. That way we can't get a state_closed before we get state_connected
	ctx.sdkConn.sendConnectedReply(ctx.req, response)

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

func (self *xgEdgeForwarder) GetIntervalId() string {
	return self.circuitId
}

func (self *xgEdgeForwarder) GetTags() map[string]string {
	return self.tags
}

func (self *xgEdgeForwarder) SendPayload(payload *xgress.Payload, timeout time.Duration, payloadType xgress.PayloadType) error {
	// pfxlog.Logger().Infof("forwarding payload to sdk circuitId: %s, seq: %d", payload.CircuitId, payload.Sequence)

	msg := payload.Marshall()
	msg.PutUint32Header(edge.ConnIdHeader, self.connId)
	if timeout == 0 {
		sent, err := self.ch.GetDefaultSender().TrySend(msg)
		if err == nil && !sent {
			self.listener.droppedMsgMeter.Mark(1)
			self.listener.droppedPayloadsMeter.Mark(1)
		}
		if err == nil && sent {
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
	// pfxlog.Logger().Infof("forwarding ack to sdk circuitId: %s, seq: %d", ack.CircuitId, ack.Sequence)

	msg := ack.Marshall()
	msg.PutUint32Header(edge.ConnIdHeader, self.connId)
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
	msg.PutUint32Header(edge.ConnIdHeader, self.connId)
	sent, err := self.ch.GetDefaultSender().TrySend(msg)
	if err == nil && !sent {
		self.listener.droppedMsgMeter.Mark(1)
	}
	return err
}

func (self *xgEdgeForwarder) Init(ctx *connectContext) bool {
	self.connId = ctx.connId
	// TODO: figure out how to handle session removed
	return true
}

func (self *xgEdgeForwarder) RegisterRouting() {
	self.forwarder.RegisterDestination(self.circuitId, self.address, self)
	self.xgCircuits.Set(self.circuitId, self)
}

func (self *xgEdgeForwarder) UnregisterRouting() {
	self.forwarder.EndCircuit(self.circuitId)
	self.xgCircuits.Set(self.circuitId, self)
}

func (self *xgEdgeForwarder) FinishConnect(ctx *connectContext, response *ctrl_msg.CreateCircuitResponse, err error) {
	if err != nil {
		ctx.log.WithError(err).Warn("failed to dial fabric")
		ctx.sdkConn.sendStateClosedReply(err.Error(), ctx.req)
		return
	}

	self.circuitId = response.CircuitId
	self.address = xgress.Address(response.Address)
	self.tags = response.Tags
	self.RegisterRouting()

	// send the state_connected before starting the xgress. That way we can't get a state_closed before we get state_connected
	connId, _ := ctx.req.GetUint32Header(edge.ConnIdHeader)

	msg := edge.NewStateConnectedMsg(connId)
	msg.ReplyTo(ctx.req)

	if assignIds, _ := ctx.req.GetBoolHeader(edge.RouterProvidedConnId); assignIds {
		msg.PutBoolHeader(edge.RouterProvidedConnId, true)
	}

	msg.PutStringHeader(edge.CircuitIdHeader, self.circuitId)
	msg.PutBoolHeader(edge.UseXgressToSdkHeader, true)
	msg.PutStringHeader(edge.XgressCtrlIdHeader, ctx.ctrlCh.Id())
	msg.PutStringHeader(edge.XgressAddressHeader, response.Address)

	self.mapResponsePeerData(response.PeerData)
	for k, v := range response.PeerData {
		msg.Headers[int32(k)] = v
	}

	// this needs to go on the data channel to ensure it gets there before data gets there or a state closed msg
	if err = msg.WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetControlSender()); err != nil {
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
	}
}

func (self *xgEdgeForwarder) Unrouted() {
	self.xgCircuits.Remove(self.circuitId)

	msg := edge.NewStateClosedMsg(self.connId, "xgress unrouted")
	err := msg.WithPriority(channel.High).WithTimeout(5 * time.Second).SendAndWaitForWire(self.ch.GetDefaultSender())
	if err != nil {
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send state closed")
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
	msg := edge.NewInspectRequest(&self.connId, requestedValue)
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
		IdentityId:          self.apiSession.IdentityId,
		CircuitId:           self.circuitId,
		EdgeConnId:          self.connId,
		CtrlId:              self.ctrlId,
		Originator:          self.originator.String(),
		Address:             string(self.address),
		TimeSinceLastLinkRx: (time.Duration(time.Now().UnixMilli()-self.lastRx.Load()) * time.Millisecond).String(),
		Tags:                self.tags,
	}
}
