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
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/channel/latency"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/sdk-golang/ziti/edge"
	"math"
)

type Acceptor struct {
	uListener          channel.UnderlayListener
	listener           *listener
	options            *channel.Options
	sessionBindHandler channel.BindHandler
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
		Type:    edge.ContentTypeConnect,
		Handler: proxy.processConnect,
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type:    edge.ContentTypeBind,
		Handler: proxy.processBind,
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type:    edge.ContentTypeUnbind,
		Handler: proxy.processUnbind,
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type:    edge.ContentTypeUpdateBind,
		Handler: proxy.processUpdateBind,
	})

	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type:    edge.ContentTypeHealthEvent,
		Handler: proxy.processHealthEvent,
	})

	binding.AddReceiveHandlerF(edge.ContentTypeStateClosed, proxy.msgMux.HandleReceive)

	binding.AddReceiveHandlerF(edge.ContentTypeTraceRoute, proxy.processTraceRoute)

	binding.AddReceiveHandlerF(edge.ContentTypeTraceRouteResponse, proxy.msgMux.HandleReceive)
	binding.AddTypedReceiveHandler(&latency.LatencyHandler{})

	// Since data is most common type, it gets to dispatch directly
	binding.AddTypedReceiveHandler(proxy.msgMux)
	binding.AddCloseHandler(proxy)

	return self.sessionBindHandler.BindChannel(binding)
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
		if err := channel.AcceptNextChannel("edge", self.uListener, self, self.options, nil); err != nil {
			log.Errorf("error accepting (%v)", err)
			if errors.Is(err, channel.ListenerClosedError) {
				return
			}
		}
	}
}
