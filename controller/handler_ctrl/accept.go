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
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/openziti/foundation/channel2"
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
					if versionValue, found := ch.Underlay().Headers()[channel2.HelloVersionHeader]; found {
						if versionInfo, err := ctrlAccepter.network.VersionProvider.EncoderDecoder().Decode(versionValue); err == nil {
							r.VersionInfo = versionInfo
						} else {
							log.WithError(err).Warn("could not parse version info from router hello, closing router connection")
							_ = ch.Close()
							return
						}
					} else {
						log.Warn("no version info header, closing router connection")
						_ = ch.Close()
						return
					}

					if listenerValue, found := ch.Underlay().Headers()[channel2.HelloRouterAdvertisementsHeader]; found {
						listenerString := string(listenerValue)
						r.AdvertisedListener = listenerString
					} else {
						log.Warn("no advertised listeners")
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
				ctrlAccepter.network.ValidateTerminators(r)

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
