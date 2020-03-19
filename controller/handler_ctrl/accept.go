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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/xctrl"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/transport"
)

type CtrlAccepter struct {
	network  *network.Network
	xctrls   []xctrl.Xctrl
	listener channel2.UnderlayListener
	options  *channel2.Options
}

func NewCtrlAccepter(network *network.Network,
	xctrls []xctrl.Xctrl,
	listener channel2.UnderlayListener,
	options *channel2.Options) *CtrlAccepter {
	return &CtrlAccepter{
		network:  network,
		xctrls:   xctrls,
		listener: listener,
		options:  options,
	}
}

func (ctrlAccepter *CtrlAccepter) Run() {
	log := pfxlog.Logger()
	log.Info("started")
	defer log.Warn("exited")

	for {
		ch, err := channel2.NewChannel("ctrl", ctrlAccepter.listener, ctrlAccepter.options)
		if err == nil {
			if r, err := ctrlAccepter.network.GetRouter(ch.Id().Token); err == nil {
				if ch.Underlay().Headers() != nil {
					if listenerValue, found := ch.Underlay().Headers()[channel2.HelloListenerHeader]; found {
						listenerString := string(listenerValue)
						if listener, err := transport.ParseAddress(listenerString); err == nil {
							r.AdvertisedListener = listener
						} else {
							log.Errorf("error parsing listener address (%s)", err)
							_ = ch.Close()
							continue
						}
					} else {
						log.Warn("missing listener attribute")
					}
				} else {
					log.Warn("no attributes provided")
				}

				r.Control = ch
				if err := r.Control.Bind(newBindHandler(r, ctrlAccepter.network, ctrlAccepter.xctrls)); err != nil {
					log.Errorf("error binding router (%s)", err)
					_ = ch.Close()
					continue
				}
				ctrlAccepter.network.ConnectRouter(r)

				log.Infof("accepted new router connection [r/%s]", r.Id)
			}

		} else {
			log.Errorf("error accepting (%s)", err)
			if err.Error() == "closed" {
				return
			}
		}
	}
}
