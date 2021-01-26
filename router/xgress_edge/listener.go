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

package xgress_edge

import (
	"encoding/binary"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/edge/router/internal/fabric"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/openziti/sdk-golang/ziti/edge"
	"time"
)

type listener struct {
	id               *identity.TokenId
	factory          *Factory
	options          *Options
	bindHandler      xgress.BindHandler
	underlayListener channel2.UnderlayListener
	headers          map[int32][]byte
}

// newListener creates a new xgress edge listener
func newListener(id *identity.TokenId, factory *Factory, options *Options, headers map[int32][]byte) xgress.Listener {
	return &listener{
		id:      id,
		factory: factory,
		options: options,
		headers: headers,
	}
}

func (listener *listener) Listen(address string, bindHandler xgress.BindHandler) error {
	listener.bindHandler = bindHandler
	addr, err := transport.ParseAddress(address)

	if err != nil {
		return fmt.Errorf("cannot listen on invalid address [%s] (%s)", address, err)
	}

	pfxlog.Logger().WithField("address", addr).Info("starting channel listener")

	listener.underlayListener = channel2.NewClassicListenerWithTransportConfiguration(
		listener.id, addr, listener.options.channelOptions.ConnectOptions, listener.factory.config.Tcfg, listener.headers)

	if err := listener.underlayListener.Listen(); err != nil {
		return err
	}
	accepter := NewAccepter(listener, listener.underlayListener, nil)
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
	ch           channel2.Channel
	idSeq        uint32
}

func (self *edgeClientConn) HandleClose(_ channel2.Channel) {
	log := pfxlog.ContextLogger(self.ch.Label())
	log.Debugf("closing")
	listeners := self.listener.factory.hostedServices.cleanupServices(self)
	for _, listener := range listeners {
		if err := xgress.RemoveTerminator(self.listener.factory, listener.terminatorId); err != nil {
			log.Warnf("failed to remove terminator on service %v for terminator %v on channel close", listener.service, listener.terminatorId)
		}
	}
	self.msgMux.Close()
}

func (self *edgeClientConn) ContentType() int32 {
	return edge.ContentTypeData
}

func (self *edgeClientConn) processConnect(req *channel2.Message, ch channel2.Channel) {
	token := string(req.Body)
	log := pfxlog.ContextLogger(ch.Label()).WithField("token", token).WithFields(edge.GetLoggerFields(req))
	connId, found := req.GetUint32Header(edge.ConnIdHeader)
	if !found {
		pfxlog.Logger().Errorf("connId not set. unable to process connect message")
		return
	}
	log.Debug("validating network session")
	sm := fabric.GetStateManager()
	ns := sm.GetSessionWithTimeout(token, self.listener.options.lookupSessionTimeout)

	if ns == nil || ns.Type != edge_ctrl_pb.SessionType_Dial {
		log.WithField("token", token).Error("session not found")
		self.sendStateClosedReply("Invalid Session", req)
		return
	}

	if _, found := self.fingerprints.HasAny(ns.CertFingerprints); !found {
		log.WithField("token", token).
			WithField("serviceFingerprints", ns.CertFingerprints).
			WithField("clientFingerprints", self.fingerprints.Prints()).
			Error("matching fingerprint not found for connect")
		self.sendStateClosedReply("Invalid Session", req)
		return
	}

	log.Debug("validating connection id")

	conn := &edgeXgressConn{
		mux:        self.msgMux,
		MsgChannel: *edge.NewEdgeMsgChannel(self.ch, connId),
		seq:        NewMsgQueue(4),
	}

	sm.AddSessionRemovedListener(ns.Token, func(token string) {
		conn.close(true, "session closed")
	})

	// We can't fix conn id, since it's provided by the client
	if err := self.msgMux.AddMsgSink(conn); err != nil {
		log.WithField("token", token).Error(err)
		self.sendStateClosedReply(err.Error(), req)
		return
	}

	// fabric connect
	log.Debug("dialing fabric")
	peerData := make(map[uint32][]byte)

	if pk, found := req.Headers[edge.PublicKeyHeader]; found {
		peerData[edge.PublicKeyHeader] = pk
	}

	if callerId, found := req.Headers[edge.CallerIdHeader]; found {
		peerData[edge.CallerIdHeader] = callerId
	}

	if appData, found := req.Headers[edge.AppDataHeader]; found {
		peerData[edge.AppDataHeader] = appData
	}

	if ns.Service.EncryptionRequired && req.Headers[edge.PublicKeyHeader] == nil {
		msg := "encryption required on service, initiator did not send public header"
		self.sendStateClosedReply(msg, req)
		conn.close(false, msg)
		return
	}

	service := ns.Service.Id
	if terminatorIdentity, found := req.GetStringHeader(edge.TerminatorIdentityHeader); found {
		service = terminatorIdentity + "@" + service
	}

	sessionInfo, err := xgress.GetSession(self.listener.factory, ns.Id, service, self.listener.options.Options.GetSessionTimeout, peerData)
	if err != nil {
		log.WithError(err).Warn("failed to dial fabric")
		self.sendStateClosedReply(err.Error(), req)
		conn.close(false, "failed to dial fabric")
		return
	}

	if ns.Service.EncryptionRequired && sessionInfo.SessionId.Data[edge.PublicKeyHeader] == nil {
		msg := "encryption required on service, terminator did not send public header"
		self.sendStateClosedReply(msg, req)
		conn.close(false, msg)
		return
	}

	x := xgress.NewXgress(sessionInfo.SessionId, sessionInfo.Address, conn, xgress.Initiator, &self.listener.options.Options)
	self.listener.bindHandler.HandleXgressBind(x)

	// send the state_connected before starting the xgress. That way we can't get a state_closed before we get state_connected
	self.sendStateConnectedReply(req, sessionInfo.SessionId.Data)
	x.Start()
}

func (self *edgeClientConn) processBind(req *channel2.Message, ch channel2.Channel) {
	token := string(req.Body)

	log := pfxlog.ContextLogger(ch.Label()).WithField("sessionId", token).WithFields(edge.GetLoggerFields(req))
	connId, found := req.GetUint32Header(edge.ConnIdHeader)
	if !found {
		pfxlog.Logger().Errorf("connId not set. unable to process bind message")
		return
	}
	log.Debug("validating network session")
	sm := fabric.GetStateManager()
	ns := sm.GetSessionWithTimeout(token, self.listener.options.lookupSessionTimeout)

	if ns == nil || ns.Type != edge_ctrl_pb.SessionType_Bind {
		log.WithField("token", token).Error("session not found")
		self.sendStateClosedReply("Invalid Session", req)
		return
	}

	if _, found := self.fingerprints.HasAny(ns.CertFingerprints); !found {
		log.WithField("token", token).
			WithField("serviceFingerprints", ns.CertFingerprints).
			WithField("clientFingerprints", self.fingerprints.Prints()).
			Error("matching fingerprint not found for bind")
		self.sendStateClosedReply("Invalid Session", req)
		return
	}

	log.Debug("binding service")

	hostData := make(map[uint32][]byte)
	pubKey, hasKey := req.Headers[edge.PublicKeyHeader]
	if hasKey {
		hostData[edge.PublicKeyHeader] = pubKey
	}

	cost := uint16(0)
	if costBytes, hasCost := req.Headers[edge.CostHeader]; hasCost {
		cost = binary.LittleEndian.Uint16(costBytes)
	}

	precedence := ctrl_pb.TerminatorPrecedence_Default
	if precedenceData, hasPrecedence := req.Headers[edge.PrecedenceHeader]; hasPrecedence && len(precedenceData) > 0 {
		edgePrecedence := precedenceData[0]
		if edgePrecedence == edge.PrecedenceRequired {
			precedence = ctrl_pb.TerminatorPrecedence_Required
		} else if edgePrecedence == edge.PrecedenceFailed {
			precedence = ctrl_pb.TerminatorPrecedence_Failed
		}
	}

	assignIds, _ := req.GetBoolHeader(edge.RouterProvidedConnId)
	log.Debugf("client requested router provided connection ids: %v", assignIds)

	log.Debug("establishing listener")
	messageSink := &edgeTerminator{
		MsgChannel:     *edge.NewEdgeMsgChannel(self.ch, connId),
		edgeClientConn: self,
		token:          token,
		service:        ns.Service.Id,
		assignIds:      assignIds,
	}

	sm.AddSessionRemovedListener(ns.Token, func(token string) {
		messageSink.close(true, "session ended")
	})

	self.listener.factory.hostedServices.Put(token, messageSink)

	terminatorIdentity, _ := req.GetStringHeader(edge.TerminatorIdentityHeader)
	var terminatorIdentitySecret []byte
	if terminatorIdentity != "" {
		terminatorIdentitySecret, _ = req.Headers[edge.TerminatorIdentitySecretHeader]
	}

	terminatorId, err := xgress.AddTerminator(self.listener.factory, ns.Service.Id, "edge", "hosted:"+token, terminatorIdentity, terminatorIdentitySecret, hostData, cost, precedence)
	messageSink.terminatorId = terminatorId

	log.Debugf("registered listener for terminator %v, token: %v", terminatorId, token)

	if err != nil {
		messageSink.close(false, "") // don't notify here, as we're notifying next line with a response
		self.sendStateClosedReply(err.Error(), req)
		return
	}

	log.Debug("returning connection state CONNECTED to client")
	self.sendStateConnectedReply(req, nil)
}

func (self *edgeClientConn) processUnbind(req *channel2.Message, ch channel2.Channel) {
	token := string(req.Body)
	log := pfxlog.ContextLogger(ch.Label()).WithField("sessionId", token).WithFields(edge.GetLoggerFields(req))

	sm := fabric.GetStateManager()
	ns := sm.GetSession(token)

	if ns == nil {
		log.WithField("token", token).Error("session not found")
		self.sendStateClosedReply("Invalid Session", req)
		return
	}

	if _, found := self.fingerprints.HasAny(ns.CertFingerprints); !found {
		log.WithField("token", token).
			WithField("serviceFingerprints", ns.CertFingerprints).
			WithField("clientFingerprints", self.fingerprints.Prints()).
			Error("matching fingerprint not found for unbind")
		self.sendStateClosedReply("Invalid Session", req)
		return
	}

	localListener, ok := self.listener.factory.hostedServices.Get(token)
	if ok {
		defer self.listener.factory.hostedServices.Delete(token)

		log.Debugf("removing terminator %v for token: %v", localListener.terminatorId, token)
		if err := xgress.RemoveTerminator(self.listener.factory, localListener.terminatorId); err != nil {
			self.sendStateClosedReply(err.Error(), req)
		} else {
			self.sendStateClosedReply("unbind successful", req)
		}
	} else {
		self.sendStateClosedReply("unbind successful", req)
	}
}

func (self *edgeClientConn) processUpdateBind(req *channel2.Message, ch channel2.Channel) {
	token := string(req.Body)
	log := pfxlog.ContextLogger(ch.Label()).WithField("sessionId", token).WithFields(edge.GetLoggerFields(req))

	localListener, ok := self.listener.factory.hostedServices.Get(token)

	if !ok {
		log.Error("failed to update bind, no listener found")
		return
	}

	sm := fabric.GetStateManager()
	ns := sm.GetSession(token)

	if ns == nil {
		log.WithField("token", token).Error("session not found")
		self.sendStateClosedReply("Invalid Session", req)
		return
	}

	if _, found := self.fingerprints.HasAny(ns.CertFingerprints); !found {
		log.WithField("token", token).
			WithField("serviceFingerprints", ns.CertFingerprints).
			WithField("clientFingerprints", self.fingerprints.Prints()).
			Error("matching fingerprint not found for update bind")
		self.sendStateClosedReply("Invalid Session", req)
		return
	}

	var cost *uint16
	if costVal, hasCost := req.GetUint16Header(edge.CostHeader); hasCost {
		cost = &costVal
	}

	var precedence *ctrl_pb.TerminatorPrecedence
	if precedenceData, hasPrecedence := req.Headers[edge.PrecedenceHeader]; hasPrecedence && len(precedenceData) > 0 {
		edgePrecedence := precedenceData[0]
		updatedPrecedence := ctrl_pb.TerminatorPrecedence_Default
		if edgePrecedence == edge.PrecedenceRequired {
			updatedPrecedence = ctrl_pb.TerminatorPrecedence_Required
		} else if edgePrecedence == edge.PrecedenceFailed {
			updatedPrecedence = ctrl_pb.TerminatorPrecedence_Failed
		}
		precedence = &updatedPrecedence
	}

	log.Debugf("updating terminator %v to precedence %v and cost %v", localListener.terminatorId, precedence, cost)
	if err := xgress.UpdateTerminator(self.listener.factory, localListener.terminatorId, cost, precedence); err != nil {
		log.WithError(err).Error("failed to update bind")
	}
}

func (self *edgeClientConn) sendStateConnectedReply(req *channel2.Message, hostData map[uint32][]byte) {
	connId, _ := req.GetUint32Header(edge.ConnIdHeader)
	msg := edge.NewStateConnectedMsg(connId)

	if assignIds, _ := req.GetBoolHeader(edge.RouterProvidedConnId); assignIds {
		msg.PutBoolHeader(edge.RouterProvidedConnId, true)
	}

	for k, v := range hostData {
		msg.Headers[int32(k)] = v
	}
	msg.ReplyTo(req)

	err := self.ch.SendPrioritizedWithTimeout(msg, channel2.High, time.Second*5)
	if err != nil {
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
		return
	}
}

func (self *edgeClientConn) sendStateClosedReply(message string, req *channel2.Message) {
	connId, _ := req.GetUint32Header(edge.ConnIdHeader)
	msg := edge.NewStateClosedMsg(connId, message)
	msg.ReplyTo(req)

	syncC, err := self.ch.SendAndSyncWithPriority(msg, channel2.High)
	if err != nil {
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
		return
	}

	select {
	case err = <-syncC:
		if err != nil {
			pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
		}
	case <-time.After(time.Second * 5):
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("timed out sending state response")
	}
}

func (self *edgeClientConn) sendStateClosed(connId uint32, message string) {
	msg := edge.NewStateClosedMsg(connId, message)
	pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Debug("sending state closed message")

	syncC, err := self.ch.SendAndSyncWithPriority(msg, channel2.High)
	if err != nil {
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
		return
	}

	select {
	case err = <-syncC:
		if err != nil {
			pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("failed to send state response")
		}
	case <-time.After(time.Second * 5):
		pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).WithError(err).Error("timed out sending state response")
	}
}
