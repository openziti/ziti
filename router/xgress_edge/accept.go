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
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/sdk-golang/ziti/edge"
	"math"
)

type Accepter struct {
	uListener channel2.UnderlayListener
	listener  *listener
	options   *channel2.Options
}

type edgeBindHandler struct {
	listener *listener
}

func (handler edgeBindHandler) BindChannel(ch channel2.Channel) error {
	log := pfxlog.Logger()
	log.WithField("token", ch.Id()).Debug("accepting edge connection")

	fpg := cert.NewFingerprintGenerator()
	proxy := &edgeClientConn{
		msgMux:       edge.NewCowMapMsgMux(),
		listener:     handler.listener,
		fingerprints: fpg.FromCerts(ch.Certificates()),
		ch:           ch,
		idSeq:        math.MaxUint32 / 2,
	}

	log.Debug("peer fingerprints ", proxy.fingerprints)

	ch.AddReceiveHandler(&edge.AsyncFunctionReceiveAdapter{
		Type:    edge.ContentTypeConnect,
		Handler: proxy.processConnect,
	})

	ch.AddReceiveHandler(&edge.AsyncFunctionReceiveAdapter{
		Type:    edge.ContentTypeBind,
		Handler: proxy.processBind,
	})

	ch.AddReceiveHandler(&edge.AsyncFunctionReceiveAdapter{
		Type:    edge.ContentTypeUnbind,
		Handler: proxy.processUnbind,
	})

	ch.AddReceiveHandler(&edge.AsyncFunctionReceiveAdapter{
		Type:    edge.ContentTypeUpdateBind,
		Handler: proxy.processUpdateBind,
	})

	ch.AddReceiveHandler(&edge.AsyncFunctionReceiveAdapter{
		Type:    edge.ContentTypeHealthEvent,
		Handler: proxy.processHealthEvent,
	})

	ch.AddReceiveHandler(&edge.FunctionReceiveAdapter{
		Type:    edge.ContentTypeStateClosed,
		Handler: proxy.msgMux.HandleReceive,
	})

	ch.AddReceiveHandler(&channel2.LatencyHandler{})

	// Since data is most common type, it gets to dispatch directly
	ch.AddReceiveHandler(proxy.msgMux)
	ch.AddCloseHandler(proxy)

	return nil
}

func NewAccepter(listener *listener, uListener channel2.UnderlayListener, options *channel2.Options) *Accepter {
	edgeBindHandler := &edgeBindHandler{listener: listener}
	sessionHandler := newSessionConnectHandler(listener.factory.stateManager, listener.options, listener.factory.metricsRegistry)

	optionsWithBind := options
	if optionsWithBind == nil {
		optionsWithBind = channel2.DefaultOptions()
	}

	optionsWithBind.BindHandlers = append(optionsWithBind.BindHandlers, edgeBindHandler, sessionHandler)

	return &Accepter{
		listener:  listener,
		uListener: uListener,
		options:   optionsWithBind,
	}
}

func (accepter *Accepter) Run() {
	log := pfxlog.Logger()
	log.Info("starting")
	defer log.Warn("exiting")

	for {
		if err := channel2.AcceptNextChannel("edge", accepter.uListener, accepter.options, nil); err != nil {
			log.Errorf("error accepting (%v)", err)
			if errors.Is(err, channel2.ListenerClosedError) {
				return
			}
		}
	}

}
