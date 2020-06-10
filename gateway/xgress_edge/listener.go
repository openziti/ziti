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
	"github.com/openziti/edge/gateway/internal/fabric"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/sequencer"
	"github.com/openziti/sdk-golang/ziti/edge"
	"time"
)

type listener struct {
	id               *identity.TokenId
	factory          *Factory
	options          *Options
	bindHandler      xgress.BindHandler
	underlayListener channel2.UnderlayListener
}

// newListener creates a new xgress edge listener
func newListener(id *identity.TokenId, factory *Factory, options *Options) xgress.Listener {
	return &listener{
		id:      id,
		factory: factory,
		options: options,
	}
}

func (listener *listener) Listen(address string, bindHandler xgress.BindHandler) error {
	listener.bindHandler = bindHandler
	addr, err := transport.ParseAddress(address)

	if err != nil {
		return fmt.Errorf("cannot listen on invalid address [%s] (%s)", address, err)
	}

	pfxlog.Logger().WithField("address", addr).Info("starting channel listener")

	listener.underlayListener = channel2.NewClassicListener(listener.id, addr, listener.options.channelOptions.ConnectOptions)
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

type ingressProxy struct {
	msgMux       *edge.MsgMux
	listener     *listener
	fingerprints cert.Fingerprints
	ch           channel2.Channel
}

func (proxy *ingressProxy) HandleClose(_ channel2.Channel) {
	log := pfxlog.ContextLogger(proxy.ch.Label())
	log.Debugf("closing")
	listeners := proxy.listener.factory.hostedServices.cleanupServices(proxy)
	for _, listener := range listeners {
		if err := xgress.RemoveTerminator(proxy.listener.factory, listener.terminatorIdRef.Get()); err != nil {
			log.Warnf("failed to remove terminator on service %v for terminator %v on channel close", listener.service, listener.terminatorIdRef.Get())
		}
	}
	proxy.msgMux.Close()
}

func (proxy *ingressProxy) ContentType() int32 {
	return edge.ContentTypeData
}

func (proxy *ingressProxy) processConnect(req *channel2.Message, ch channel2.Channel) {
	token := string(req.Body)
	log := pfxlog.ContextLogger(ch.Label()).WithField("token", token).WithFields(edge.GetLoggerFields(req))
	connId, found := req.GetUint32Header(edge.ConnIdHeader)
	if !found {
		pfxlog.Logger().Errorf("connId not set. unable to process connect message")
		return
	}
	log.Debug("validating network session")
	sm := fabric.GetStateManager()
	ns := sm.GetNetworkSessionWithTimeout(token, time.Second*5)

	if ns == nil || ns.Type != edge_ctrl_pb.SessionType_Dial {
		log.WithField("token", token).Error("session not found")
		proxy.sendStateClosedReply("Invalid Session", req)
		return
	}

	if _, found := proxy.fingerprints.HasAny(ns.CertFingerprints); !found {
		log.WithField("token", token).
			WithField("serviceFingerprints", ns.CertFingerprints).
			WithField("clientFingerprints", proxy.fingerprints.Prints()).
			Error("matching fingerprint not found")
		proxy.sendStateClosedReply("Invalid Session", req)
		return
	}

	log.Debug("validating connection id")

	removeListener := sm.AddNetworkSessionRemovedListener(ns.Token, func(token string) {
		proxy.sendStateClosed(connId, "session closed")
		proxy.closeConn(connId)
	})

	conn := &localMessageSink{
		MsgChannel: *edge.NewEdgeMsgChannel(proxy.ch, connId),
		seq:        sequencer.NewSingleWriterSeq(proxy.listener.options.MaxOutOfOrderMsgs),
		closeCB: func(connId uint32) {
			removeListener()
		},
	}

	if err := proxy.msgMux.AddMsgSink(conn); err != nil {
		log.WithField("token", token).Error(err)
		proxy.sendStateClosedReply(err.Error(), req)
		return
	}

	// fabric connect
	log.Debug("dialing fabric")
	peerData := make(map[uint32][]byte)
	peerData[edge.PublicKeyHeader] = req.Headers[edge.PublicKeyHeader]
	sessionInfo, err := xgress.GetSession(proxy.listener.factory, ns.Token, ns.Service.Id, peerData)
	if err != nil {
		log.Warn("failed to dial fabric ", err)
		proxy.sendStateClosedReply(err.Error(), req)
		return
	}

	x := xgress.NewXgress(sessionInfo.SessionId, sessionInfo.Address, conn, xgress.Initiator, &proxy.listener.options.Options)
	proxy.listener.bindHandler.HandleXgressBind(sessionInfo.SessionId, sessionInfo.Address, xgress.Initiator, x)
	x.Start()

	if err := sessionInfo.SendStartEgress(); err != nil {
		pfxlog.Logger().WithField("connId", conn.Id()).WithError(err).Error("Failed to send start egress")
		conn.close(true, "egress start failed")
	}

	proxy.sendStateConnectedReply(req, sessionInfo.SessionId.Data)
}

func (proxy *ingressProxy) processBind(req *channel2.Message, ch channel2.Channel) {
	token := string(req.Body)

	log := pfxlog.ContextLogger(ch.Label()).WithField("sessionId", token).WithFields(edge.GetLoggerFields(req))
	connId, found := req.GetUint32Header(edge.ConnIdHeader)
	if !found {
		pfxlog.Logger().Errorf("connId not set. unable to process bind message")
		return
	}
	log.Debug("validating network session")
	sm := fabric.GetStateManager()
	ns := sm.GetNetworkSessionWithTimeout(token, time.Second*5)

	if ns == nil || ns.Type != edge_ctrl_pb.SessionType_Bind {
		log.WithField("token", token).Error("session not found")
		proxy.sendStateClosedReply("Invalid Session", req)
		return
	}

	if _, found := proxy.fingerprints.HasAny(ns.CertFingerprints); !found {
		log.WithField("token", token).
			WithField("serviceFingerprints", ns.CertFingerprints).
			WithField("clientFingerprints", proxy.fingerprints.Prints()).
			Error("matching fingerprint not found")
		proxy.sendStateClosedReply("Invalid Session", req)
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

	terminatorIdRef := &concurrenz.AtomicString{}

	removeListener := sm.AddNetworkSessionRemovedListener(ns.Token, func(token string) {
		terminatorId := terminatorIdRef.Get()
		defer proxy.listener.factory.hostedServices.Delete(token)
		if err := xgress.RemoveTerminator(proxy.listener.factory, terminatorId); err != nil {
			log.Errorf("failed to remove terminator %v (%v)", terminatorId, err)
		}
		proxy.sendStateClosed(connId, "session closed")
		proxy.closeConn(connId)
	})

	log.Debug("establishing listener")
	messageSink := &localListener{
		localMessageSink: localMessageSink{
			MsgChannel: *edge.NewEdgeMsgChannel(ch, connId),
			seq:        sequencer.NewSingleWriterSeq(proxy.listener.options.MaxOutOfOrderMsgs),
			closeCB: func(connId uint32) {
				removeListener()
			},
			newSinkCB: func(conn *localMessageSink) {
				if err := proxy.msgMux.AddMsgSink(conn); err != nil {
					log.WithError(err).Error("Failed to add sink, duplicate id")
				}
			},
		},
		terminatorIdRef: terminatorIdRef,
		service:         ns.Service.Id,
		parent:          proxy,
	}

	proxy.listener.factory.hostedServices.Put(token, messageSink)

	terminatorId, err := xgress.AddTerminator(proxy.listener.factory, ns.Service.Id, "edge", "hosted:"+token, hostData, cost, precedence)
	messageSink.terminatorIdRef.Set(terminatorId)

	if err != nil {
		messageSink.closeCB(messageSink.Id())
		proxy.sendStateClosedReply(err.Error(), req)
		return
	}

	log.Debug("returning connection state CONNECTED to client")
	proxy.sendStateConnectedReply(req, nil)
}

func (proxy *ingressProxy) processUnbind(req *channel2.Message, ch channel2.Channel) {
	token := string(req.Body)
	log := pfxlog.ContextLogger(ch.Label()).WithField("sessionId", token).WithFields(edge.GetLoggerFields(req))

	sm := fabric.GetStateManager()
	ns := sm.GetNetworkSession(token)

	if ns == nil {
		log.WithField("token", token).Error("session not found")
		proxy.sendStateClosedReply("Invalid Session", req)
		return
	}

	if _, found := proxy.fingerprints.HasAny(ns.CertFingerprints); !found {
		log.WithField("token", token).
			WithField("serviceFingerprints", ns.CertFingerprints).
			WithField("clientFingerprints", proxy.fingerprints.Prints()).
			Error("matching fingerprint not found")
		proxy.sendStateClosedReply("Invalid Session", req)
		return
	}

	localListener, ok := proxy.listener.factory.hostedServices.Get(token)
	if ok {
		defer proxy.listener.factory.hostedServices.Delete(token)
		if err := xgress.RemoveTerminator(proxy.listener.factory, localListener.terminatorIdRef.Get()); err != nil {
			proxy.sendStateClosedReply(err.Error(), req)
		} else {
			proxy.sendStateClosedReply("unbind successful", req)
		}
	} else {
		proxy.sendStateClosedReply("unbind successful", req)
	}
}

func (proxy *ingressProxy) processUpdateBind(req *channel2.Message, ch channel2.Channel) {
	token := string(req.Body)
	log := pfxlog.ContextLogger(ch.Label()).WithField("sessionId", token).WithFields(edge.GetLoggerFields(req))

	localListener, ok := proxy.listener.factory.hostedServices.Get(token)

	if !ok {
		log.Error("failed to update bind, no listener found")
		return
	}

	sm := fabric.GetStateManager()
	ns := sm.GetNetworkSession(token)

	if ns == nil {
		log.WithField("token", token).Error("session not found")
		proxy.sendStateClosedReply("Invalid Session", req)
		return
	}

	if _, found := proxy.fingerprints.HasAny(ns.CertFingerprints); !found {
		log.WithField("token", token).
			WithField("serviceFingerprints", ns.CertFingerprints).
			WithField("clientFingerprints", proxy.fingerprints.Prints()).
			Error("matching fingerprint not found")
		proxy.sendStateClosedReply("Invalid Session", req)
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

	log.Debugf("updating terminator %v to precedence %v and cost %v", localListener.terminatorIdRef.Get(), precedence, cost)
	if err := xgress.UpdateTerminator(proxy.listener.factory, localListener.terminatorIdRef.Get(), cost, precedence); err != nil {
		log.WithError(err).Error("failed to update bind")
	}
}

func (proxy *ingressProxy) sendStateConnectedReply(req *channel2.Message, hostData map[uint32][]byte) {
	connId, _ := req.GetUint32Header(edge.ConnIdHeader)
	msg := edge.NewStateConnectedMsg(connId)
	for k, v := range hostData {
		msg.Headers[int32(k)] = v
	}
	msg.ReplyTo(req)

	syncC, err := proxy.ch.SendAndSyncWithPriority(msg, channel2.High)
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

func (proxy *ingressProxy) sendStateClosedReply(message string, req *channel2.Message) {
	connId, _ := req.GetUint32Header(edge.ConnIdHeader)
	msg := edge.NewStateClosedMsg(connId, message)
	msg.ReplyTo(req)

	syncC, err := proxy.ch.SendAndSyncWithPriority(msg, channel2.High)
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

func (proxy *ingressProxy) sendStateClosed(connId uint32, message string) {
	msg := edge.NewStateClosedMsg(connId, message)
	pfxlog.Logger().WithFields(edge.GetLoggerFields(msg)).Debug("sending state closed message")

	syncC, err := proxy.ch.SendAndSyncWithPriority(msg, channel2.High)
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

func (proxy *ingressProxy) closeConn(connId uint32) {
	// This was done in the process loop, but all the relevant data structure are concurrent safe
	// and if the the proxy closed before all the connections could be closed, this would lead to
	// deadlocks
	log := pfxlog.ContextLogger(proxy.ch.Label()).WithField("connId", connId)
	log.Debug("closeConn()")

	// we don't need to close the conn here, it will get closed when the xgress closes its peer
	proxy.msgMux.RemoveMsgSinkById(connId)
}
