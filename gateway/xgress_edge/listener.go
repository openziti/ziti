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

package xgress_edge

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/gateway/internal/fabric"
	"github.com/netfoundry/ziti-edge/internal/cert"
	"github.com/netfoundry/ziti-edge/pb/edge_ctrl_pb"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/netfoundry/ziti-foundation/util/sequencer"
	"github.com/netfoundry/ziti-sdk-golang/ziti/edge"
	"time"
)

type listener struct {
	id          *identity.TokenId
	factory     *Factory
	options     *Options
	bindHandler xgress.BindHandler
}

// newListener creates a new xgress edge listener
func newListener(id *identity.TokenId, factory *Factory, options *Options) xgress.XgressListener {
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

	chListener := channel2.NewClassicListener(listener.id, addr)
	if err := chListener.Listen(); err != nil {
		return err
	}
	accepter := NewAccepter(listener, chListener, nil)
	go accepter.Run()

	return nil
}

type ingressProxy struct {
	msgMux       *edge.MsgMux
	listener     *listener
	fingerprints cert.Fingerprints
	ch           channel2.Channel
}

func (proxy *ingressProxy) HandleClose(_ channel2.Channel) {
	proxy.msgMux.Event(&ingressChannelCloseEvent{proxy: proxy})
}

type ingressChannelCloseEvent struct {
	proxy *ingressProxy
}

func (event *ingressChannelCloseEvent) Handle(_ *edge.MsgMux) {
	event.proxy.close()
}

func (proxy *ingressProxy) close() {
	log := pfxlog.ContextLogger(proxy.ch.Label())
	log.Debugf("closing")
	listeners := proxy.listener.factory.hostedServices.cleanupServices(proxy)
	for _, listener := range listeners {
		if err := xgress.UnbindService(proxy.listener.factory, listener.token, listener.service); err != nil {
			log.Warnf("failed to unbind service %v for hostToken %v on channel close", listener.service, listener.token)
		}
	}
	proxy.msgMux.ExecuteClose()
}

func (proxy *ingressProxy) ContentType() int32 {
	return edge.ContentTypeData
}

func (proxy *ingressProxy) processConnect(req *channel2.Message, ch channel2.Channel) {
	token := string(req.Body)
	log := pfxlog.ContextLogger(ch.Label()).WithField("sessionId", token).WithFields(edge.GetLoggerFields(req))
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
		proxy.closeConn(connId)
	})

	conn := &localMessageSink{
		MsgChannel: *edge.NewEdgeMsgChannel(proxy.ch, connId),
		seq:        sequencer.NewSingleWriterSeq(proxy.listener.options.MaxOutOfOrderMsgs),
		closeCB: func(connId uint32) {
			removeListener()
			proxy.closeConn(connId)
		},
	}

	if err := proxy.msgMux.AddMsgSink(conn); err != nil {
		log.WithField("token", token).Error(err)
		proxy.sendStateClosedReply(err.Error(), req)
		return
	}

	// fabric connect
	log.Debug("dialing fabric")
	sessionInfo, err := xgress.GetSession(proxy.listener.factory, ns.Token, ns.Service.Id)
	if err != nil {
		log.Warn("failed to dial fabric ", err)
		proxy.sendStateClosedReply(err.Error(), req)
		return
	}

	proxy.sendStateConnectedReply(req)

	x := xgress.NewXgress(sessionInfo.SessionId, sessionInfo.Address, conn, xgress.Initiator, &proxy.listener.options.Options)
	proxy.listener.bindHandler.HandleXgressBind(sessionInfo.SessionId, sessionInfo.Address, xgress.Initiator, x)
	x.Start()

	if err := sessionInfo.SendStartEgress(); err != nil {
		pfxlog.Logger().WithField("connId", conn.Id()).WithError(err).Error("Failed to send start egress")
		conn.close(true, "egress start failed")
	}
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
	if err := xgress.BindService(proxy.listener.factory, token, ns.Service.Id); err != nil {
		proxy.sendStateClosedReply(err.Error(), req)
		return
	}

	removeListener := sm.AddNetworkSessionRemovedListener(ns.Token, func(token string) {
		proxy.closeConn(connId)
	})

	log.Debug("establishing listener")
	messageSink := &localListener{
		localMessageSink: localMessageSink{
			MsgChannel: *edge.NewEdgeMsgChannel(ch, connId),
			seq:        sequencer.NewSingleWriterSeq(proxy.listener.options.MaxOutOfOrderMsgs),
			closeCB: func(connId uint32) {
				removeListener()
				proxy.closeConn(connId)
			},
			newSinkCB: func(conn *localMessageSink) {
				if err := proxy.msgMux.AddMsgSink(conn); err != nil {
					log.WithError(err).Error("Failed to add sink, duplicate id")
				}
			},
		},
		token:   token,
		service: ns.Service.Id,
		parent:  proxy,
	}

	proxy.listener.factory.hostedServices.Put(token, messageSink)
	log.Debug("returning connection state CONNECTED to client")
	proxy.sendStateConnectedReply(req)
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

	defer proxy.listener.factory.hostedServices.Delete(token)
	if err := xgress.UnbindService(proxy.listener.factory, token, ns.Service.Id); err != nil {
		proxy.sendStateClosedReply(err.Error(), req)
	} else {
		proxy.sendStateClosedReply("unbind successful", req)
	}
}

func (proxy *ingressProxy) sendStateConnectedReply(req *channel2.Message) {
	connId, _ := req.GetUint32Header(edge.ConnIdHeader)
	msg := edge.NewStateConnectedMsg(connId)
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

func (proxy *ingressProxy) closeConn(connId uint32) {
	// This was done in the process loop, but all the relevant data structure are concurrent safe
	// and if the the proxy closed before all the connections could be closed, this would lead to
	// deadlocks
	log := pfxlog.ContextLogger(proxy.ch.Label()).WithField("connId", connId)
	log.Debug("closeConn()")

	// we don't need to close the conn here, it will get closed when the xgress closes its peer
	proxy.msgMux.RemoveMsgSinkById(connId)
}
