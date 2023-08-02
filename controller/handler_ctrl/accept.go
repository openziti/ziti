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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"time"
)

type CtrlAccepter struct {
	network          *network.Network
	xctrls           []xctrl.Xctrl
	options          *channel.Options
	heartbeatOptions *channel.HeartbeatOptions
	traceHandler     *channel.TraceHandler
}

func NewCtrlAccepter(network *network.Network,
	xctrls []xctrl.Xctrl,
	options *channel.Options,
	heartbeatOptions *channel.HeartbeatOptions,
	traceHandler *channel.TraceHandler) *CtrlAccepter {
	return &CtrlAccepter{
		network:          network,
		xctrls:           xctrls,
		options:          options,
		heartbeatOptions: heartbeatOptions,
		traceHandler:     traceHandler,
	}
}

func (self *CtrlAccepter) AcceptUnderlay(underlay channel.Underlay) error {
	_, err := channel.NewChannelWithUnderlay("ctrl", underlay, channel.BindHandlerF(self.Bind), self.options)
	return err
}

func (self *CtrlAccepter) Bind(binding channel.Binding) error {
	binding.GetChannel().SetLogicalName(binding.GetChannel().Id())
	ch := binding.GetChannel()

	log := pfxlog.Logger().WithField("routerId", ch.Id())

	// Use a new copy of the router instance each time we connect. That way we can tell on disconnect
	// if we're working with the right connection, in case connects and disconnects happen quickly.
	// It also means that the channel and connected time fields don't change and we don't have to protect them
	if r, err := self.network.GetReloadedRouter(ch.Id()); err == nil {
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
			r.Listeners = nil
			if val, found := ch.Underlay().Headers()[int32(ctrl_pb.ContentType_ListenersHeader)]; found {
				log.Debug("router reported listeners using listeners header")
				listeners := &ctrl_pb.Listeners{}
				if err := proto.Unmarshal(val, listeners); err != nil {
					log.WithError(err).Error("unable to unmarshall listeners value")
				} else {
					r.SetLinkListeners(listeners.Listeners)
					for _, listener := range listeners.Listeners {
						log.WithField("address", listener.GetAddress()).
							WithField("protocol", listener.GetProtocol()).
							WithField("costTags", listener.GetCostTags()).
							Debug("router listener")
					}
				}
			} else {
				log.Warn("no advertised listeners")
			}
			if val, found := ch.Underlay().Headers()[int32(ctrl_pb.ContentType_RouterMetadataHeader)]; found {
				log.Debug("router reported listeners using listeners header")
				routerMetadata := &ctrl_pb.RouterMetadata{}
				if err = proto.Unmarshal(val, routerMetadata); err != nil {
					log.WithError(err).Error("unable to unmarshall router metadata value")
				}
				r.SetMetadata(routerMetadata)
			}
		} else {
			log.Warn("no attributes provided")
		}

		r.Control = ch
		r.ConnectTime = time.Now()
		if err := binding.Bind(newBindHandler(self.heartbeatOptions, r, self.network, self.xctrls)); err != nil {
			return errors.Wrap(err, "error binding router")
		}

		if self.traceHandler != nil {
			binding.AddPeekHandler(self.traceHandler)
		}

		log.Infof("accepted new router connection [r/%s]", r.Id)

		self.network.ConnectRouter(r)
	}
	return nil
}
