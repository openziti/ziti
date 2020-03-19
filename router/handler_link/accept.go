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

package handler_link

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/metrics"
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type Accepter struct {
	id               *identity.TokenId
	ctrl             xgress.CtrlChannel
	forwarder        *forwarder.Forwarder
	listener         channel2.UnderlayListener
	options          *channel2.Options
	forwarderOptions *forwarder.Options
}

func NewAccepter(id *identity.TokenId,
	ctrl xgress.CtrlChannel,
	forwarder *forwarder.Forwarder,
	listener channel2.UnderlayListener,
	options *channel2.Options,
	forwarderOptions *forwarder.Options) *Accepter {
	return &Accepter{
		id:               id,
		ctrl:             ctrl,
		forwarder:        forwarder,
		listener:         listener,
		options:          options,
		forwarderOptions: forwarderOptions,
	}
}

func (accepter *Accepter) Run() {
	log := pfxlog.Logger()
	log.Info("started")
	defer log.Warn("exited")

	for {
		ch, err := channel2.NewChannel("link", accepter.listener, accepter.options)
		if err == nil {
			link := forwarder.NewLink(ch.Id(), ch)

			if err := link.Channel.Bind(NewBindHandler(accepter.id, link, accepter.ctrl, accepter.forwarder)); err == nil {
				accepter.forwarder.RegisterLink(link)

				go metrics.ProbeLatency(
					link.Channel,
					accepter.forwarder.MetricsRegistry().Histogram("link."+link.Id.Token+".latency"),
					accepter.forwarderOptions.LatencyProbeInterval,
				)

				log.Infof("accepted new link [l/%s]", link.Channel.Id().Token)
			} else {
				log.Infof("error binding new link (%s)", err)
			}
		} else {
			log.Errorf("error accepting (%s)", err)
		}
	}
}
