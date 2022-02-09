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
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/pkg/errors"
)

type CtrlAccepter struct {
	network      *network.Network
	xctrls       []xctrl.Xctrl
	listener     channel.UnderlayListener
	options      *channel.Options
	traceHandler *channel.TraceHandler
}

func NewCtrlAccepter(network *network.Network,
	xctrls []xctrl.Xctrl,
	listener channel.UnderlayListener,
	options *channel.Options,
	traceHandler *channel.TraceHandler) *CtrlAccepter {
	return &CtrlAccepter{
		network:      network,
		xctrls:       xctrls,
		listener:     listener,
		options:      options,
		traceHandler: traceHandler,
	}
}

func (self *CtrlAccepter) Run() {
	log := pfxlog.Logger()
	log.Info("started")
	defer log.Warn("exited")

	for {
		ch, err := channel.NewChannel("ctrl", self.listener, channel.BindHandlerF(self.Bind), self.options)
		if err != nil {
			log.WithError(err).Error("error accepting control channel connection")
			if err.Error() == "closed" {
				return
			}
		} else if r, err := self.network.GetRouter(ch.Id().Token); err == nil {
			go self.network.ConnectRouter(r)
		} else {
			log.WithError(err).Error("error getting router for control channel")
		}
	}
}

func (self *CtrlAccepter) Bind(binding channel.Binding) error {
	log := pfxlog.Logger()

	binding.GetChannel().SetLogicalName(binding.GetChannel().Id().Token)
	ch := binding.GetChannel()

	if r, err := self.network.GetRouter(ch.Id().Token); err == nil {
		if ch.Underlay().Headers() != nil {
			if versionValue, found := ch.Underlay().Headers()[channel.HelloVersionHeader]; found {
				if versionInfo, err := self.network.VersionProvider.EncoderDecoder().Decode(versionValue); err == nil {
					r.VersionInfo = versionInfo
				} else {
					return errors.Wrap(err, "could not parse version info from router hello, closing router connection")
				}
			} else {
				return errors.New("no version info header, closing router connection")
			}

			if listenerValue, found := ch.Underlay().Headers()[channel.HelloRouterAdvertisementsHeader]; found {
				listenerString := string(listenerValue)
				r.AdvertisedListener = listenerString
			} else {
				log.Warn("no advertised listeners")
			}
		} else {
			log.Warn("no attributes provided")
		}

		r.Control = ch
		if err := binding.Bind(newBindHandler(r, self.network, self.xctrls)); err != nil {
			return errors.Wrap(err, "error binding router")
		}

		if self.traceHandler != nil {
			binding.AddPeekHandler(self.traceHandler)
		}

		log.Infof("accepted new router connection [r/%s]", r.Id)
	}
	return nil
}
