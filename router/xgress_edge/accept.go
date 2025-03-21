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
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/latency"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/cert"
	"math"
	"sync/atomic"
	"time"
)

type Acceptor struct {
	uListener           channel.UnderlayListener
	listener            *listener
	options             *channel.Options
	sessionBindHandler  *sessionConnectionHandler
	connectFailureMeter metrics.Meter
	connectSuccessMeter metrics.Meter
	disconnectMeter     metrics.Meter
	connectionCount     atomic.Int64
	connStateTracker    *connectionTracker
}

func (self *Acceptor) BindChannel(binding channel.Binding) error {
	log := pfxlog.Logger()
	log.WithField("token", binding.GetChannel().Id()).Debug("accepting edge connection")

	fpg := cert.NewFingerprintGenerator()

	conn := &edgeClientConn{
		msgMux:       edge.NewCowMapMsgMux(),
		listener:     self.listener,
		fingerprints: fpg.FromCerts(binding.GetChannel().Certificates()),
		ch:           binding.GetChannel(),
		idSeq:        math.MaxUint32 / 2,
	}

	log.Debug("peer fingerprints ", conn.fingerprints)

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeConnect,
		Handler: func(m *channel.Message, ch channel.Channel) {
			conn.processConnect(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeBind,
		Handler: func(m *channel.Message, ch channel.Channel) {
			conn.processBind(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeUnbind,
		Handler: func(m *channel.Message, ch channel.Channel) {
			conn.processUnbind(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeUpdateBind,
		Handler: func(m *channel.Message, ch channel.Channel) {
			conn.processUpdateBind(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeHealthEvent,
		Handler: func(m *channel.Message, ch channel.Channel) {
			conn.processHealthEvent(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeUpdateToken,
		Handler: func(m *channel.Message, ch channel.Channel) {
			conn.processTokenUpdate(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddReceiveHandlerF(edge.ContentTypeStateClosed, conn.msgMux.HandleReceive)

	binding.AddReceiveHandlerF(edge.ContentTypeTraceRoute, conn.processTraceRoute)

	binding.AddReceiveHandlerF(edge.ContentTypeTraceRouteResponse, conn.msgMux.HandleReceive)
	binding.AddTypedReceiveHandler(&latency.LatencyHandler{})

	// Since data is the most common type, it gets to dispatch directly
	if self.listener.factory.routerConfig.Metrics.EnableDataDelayMetric {
		delayTimer := self.listener.factory.env.GetMetricsRegistry().Timer("xgress_edge.long_data_queue_time")
		binding.AddReceiveHandlerF(conn.msgMux.ContentType(), func(m *channel.Message, ch channel.Channel) {
			start := time.Now()
			conn.msgMux.HandleReceive(m, ch)
			if processingTime := time.Since(start); processingTime > 5*time.Millisecond {
				delayTimer.Update(processingTime)
			}
		})
	} else {
		binding.AddTypedReceiveHandler(conn.msgMux)
	}
	binding.AddCloseHandler(conn)
	binding.AddPeekHandler(debugPeekHandler{})

	if err := self.sessionBindHandler.validateApiSession(binding, conn); err != nil {
		self.connectFailureMeter.Mark(1)
		return err
	}

	identityId := conn.apiSession.ApiSession.IdentityId
	self.connStateTracker.markConnected(identityId, conn.ch)

	binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
		self.connectionCount.Add(-1)
		self.disconnectMeter.Mark(1)
		self.connStateTracker.markDisconnected(identityId, ch)
	}))

	self.connectSuccessMeter.Mark(1)
	self.connectionCount.Add(1)

	self.sessionBindHandler.completeBinding(binding, conn)
	return nil
}

type debugPeekHandler struct{}

func (d debugPeekHandler) Connect(ch channel.Channel, remoteAddress string) {
}

func (d debugPeekHandler) Rx(m *channel.Message, ch channel.Channel) {
	if m.ContentType == edge.ContentTypeDialSuccess || m.ContentType == edge.ContentTypeDialFailed {
		connId, _ := m.GetUint32Header(edge.ConnIdHeader)
		result, err := edge.UnmarshalDialResult(m)
		if err != nil {
			pfxlog.Logger().WithError(err).Infof("err unmarshalling dial result, seq: %d , replyTo: %d, connId: %d",
				m.Sequence(), m.ReplyFor(), connId)
		} else {
			pfxlog.Logger().Infof("received dial result: seq: %d , replyTo: %d, connId: %d, newConnId: %d, success: %v, msg: %s",
				m.Sequence(), m.ReplyFor(), connId, result.NewConnId, result.Success, result.Message)
		}
	}
}

func (d debugPeekHandler) Tx(m *channel.Message, ch channel.Channel) {
	if m.ContentType == edge.ContentTypeDial {
		connId, _ := m.GetUint32Header(edge.ConnIdHeader)
		newConnId, _ := m.GetUint32Header(edge.RouterProvidedConnId)
		circuitId, _ := m.GetStringHeader(edge.CircuitIdHeader)

		pfxlog.Logger().Infof("sending dial: seq: %d , connId: %d, newConnId: %d, circuitId: %s",
			m.Sequence(), connId, newConnId, circuitId)
	}
}

func (d debugPeekHandler) Close(ch channel.Channel) {
}

func NewAcceptor(listener *listener, uListener channel.UnderlayListener, options *channel.Options) *Acceptor {
	sessionHandler := newSessionConnectHandler(listener.factory.stateManager, listener.options, listener.factory.metricsRegistry)

	optionsWithBind := options
	if optionsWithBind == nil {
		optionsWithBind = channel.DefaultOptions()
	}

	result := &Acceptor{
		listener:            listener,
		uListener:           uListener,
		options:             optionsWithBind,
		sessionBindHandler:  sessionHandler,
		connStateTracker:    listener.factory.connectionTracker,
		connectFailureMeter: listener.factory.metricsRegistry.Meter("edge.connect.failures"),
		connectSuccessMeter: listener.factory.metricsRegistry.Meter("edge.connect.successes"),
		disconnectMeter:     listener.factory.metricsRegistry.Meter("edge.disconnects"),
	}

	listener.factory.metricsRegistry.FuncGauge("edge.connections", func() int64 {
		return result.connectionCount.Load()
	})

	return result
}

func (self *Acceptor) Run() {
	log := pfxlog.Logger()
	log.Info("starting")
	defer log.Warn("exiting")

	for {
		if err := channel.AcceptNextChannel("edge", self.uListener, self, self.options); err != nil {
			log.Errorf("error accepting (%v)", err)
			if errors.Is(err, channel.ListenerClosedError) {
				return
			}
		}
	}
}
