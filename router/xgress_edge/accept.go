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
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/cert"
	"math"
)

type Acceptor struct {
	uListener          channel.UnderlayListener
	listener           *listener
	options            *channel.Options
	sessionBindHandler *sessionConnectionHandler
}

func (self *Acceptor) BindChannel(binding channel.Binding) error {
	log := pfxlog.Logger()
	log.WithField("token", binding.GetChannel().Id()).Debug("accepting edge connection")

	fpg := cert.NewFingerprintGenerator()

	proxy := &edgeClientConn{
		msgMux:       edge.NewCowMapMsgMux(),
		listener:     self.listener,
		fingerprints: fpg.FromCerts(binding.GetChannel().Certificates()),
		ch:           binding.GetChannel(),
		idSeq:        math.MaxUint32 / 2,
	}

	log.Debug("peer fingerprints ", proxy.fingerprints)

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeConnect,
		Handler: func(m *channel.Message, ch channel.Channel) {
			proxy.processConnect(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeBind,
		Handler: func(m *channel.Message, ch channel.Channel) {
			proxy.processBind(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeUnbind,
		Handler: func(m *channel.Message, ch channel.Channel) {
			proxy.processUnbind(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeUpdateBind,
		Handler: func(m *channel.Message, ch channel.Channel) {
			proxy.processUpdateBind(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeHealthEvent,
		Handler: func(m *channel.Message, ch channel.Channel) {
			proxy.processHealthEvent(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type: edge.ContentTypeUpdateToken,
		Handler: func(m *channel.Message, ch channel.Channel) {
			proxy.processTokenUpdate(self.listener.factory.stateManager, m, ch)
		},
	})

	binding.AddReceiveHandlerF(edge.ContentTypeStateClosed, proxy.msgMux.HandleReceive)

	binding.AddReceiveHandlerF(edge.ContentTypeTraceRoute, proxy.processTraceRoute)

	binding.AddReceiveHandlerF(edge.ContentTypeTraceRouteResponse, proxy.msgMux.HandleReceive)
	binding.AddTypedReceiveHandler(&latency.LatencyHandler{})

	// Since data is most common type, it gets to dispatch directly
	binding.AddTypedReceiveHandler(proxy.msgMux)
	binding.AddCloseHandler(proxy)
	binding.AddPeekHandler(debugPeekHandler{})
	return self.sessionBindHandler.BindChannel(binding, proxy)
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

	return &Acceptor{
		listener:           listener,
		uListener:          uListener,
		options:            optionsWithBind,
		sessionBindHandler: sessionHandler,
	}
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
