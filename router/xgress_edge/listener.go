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
	"fmt"
	"github.com/openziti/ziti/common/ctrl_msg"
	"github.com/openziti/ziti/controller/idgen"
	"github.com/openziti/ziti/router/xgress_router"
	"github.com/sirupsen/logrus"
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
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/router/state"
	"github.com/openziti/ziti/router/xgress"
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
}

// newListener creates a new xgress edge listener
func newListener(id *identity.TokenId, factory *Factory, options *Options, headers map[int32][]byte) xgress_router.Listener {
	return &listener{
		id:      id,
		factory: factory,
		options: options,
		headers: headers,
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
}

func (self *edgeClientConn) HandleClose(ch channel.Channel) {
	log := pfxlog.ContextLogger(self.ch.GetChannel().Label())
	log.Debugf("closing")
	self.listener.factory.hostedServices.cleanupServices(ch)
	self.msgMux.Close()
}

func (self *edgeClientConn) ContentType() int32 {
	return edge.ContentTypeData
}

func (self *edgeClientConn) processConnect(manager state.Manager, req *channel.Message, ch channel.Channel) {
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

	conn := &edgeXgressConn{
		mux:        self.msgMux,
		MsgChannel: *edge.NewEdgeMsgChannel(self.ch, connId),
		seq:        NewMsgQueue(4),
	}

	// need to remove session remove listener on close
	conn.onClose = self.listener.factory.stateManager.AddEdgeSessionRemovedListener(sessionToken, func(token string) {
		conn.close(true, "session closed")
	})

	// We can't fix conn id, since it's provided by the client
	if err := self.msgMux.AddMsgSink(conn); err != nil {
		log.WithError(err).WithField("token", sessionToken).Error("error adding to msg mux")
		self.sendStateClosedReply(err.Error(), req)
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
		apiSession := manager.GetApiSessionFromCh(ch)

		if apiSession == nil {
			pfxlog.Logger().Errorf("could not find api session for channel, unable to process bind message")
			return
		}

		request.ApiSessionToken = apiSession.Token
	}

	response, err := self.sendCreateCircuitRequest(request, ctrlCh)
	if err != nil {
		log.WithError(err).Warn("failed to dial fabric")
		self.sendStateClosedReply(err.Error(), req)
		conn.close(false, "failed to dial fabric")
		return
	}

	self.mapResponsePeerData(response.PeerData)

	x := xgress.NewXgress(response.CircuitId, ctrlCh.Id(), xgress.Address(response.Address), conn, xgress.Initiator, &self.listener.options.Options, response.Tags)
	self.listener.bindHandler.HandleXgressBind(x)
	conn.ctrlRx = x
	// send the state_connected before starting the xgress. That way we can't get a state_closed before we get state_connected
	self.sendStateConnectedReply(req, response.PeerData, response.CircuitId)
	x.Start()
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

	log.Debug("establishing listener")

	terminator := &edgeTerminator{
		MsgChannel:     *edge.NewEdgeMsgChannel(self.ch, connId),
		edgeClientConn: self,
		token:          sessionToken,
		assignIds:      assignIds,
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
	self.sendStateConnectedReply(req, nil, "")

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

	self.sendStateConnectedReply(req, nil, "")

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

func (self *edgeClientConn) sendStateConnectedReply(req *channel.Message, hostData map[uint32][]byte, circuitId string) {
	connId, _ := req.GetUint32Header(edge.ConnIdHeader)
	msg := edge.NewStateConnectedMsg(connId)

	if assignIds, _ := req.GetBoolHeader(edge.RouterProvidedConnId); assignIds {
		msg.PutBoolHeader(edge.RouterProvidedConnId, true)
	}

	if circuitId != "" {
		msg.PutStringHeader(edge.CircuitIdHeader, circuitId)
	}

	for k, v := range hostData {
		msg.Headers[int32(k)] = v
	}
	msg.ReplyTo(req)

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
