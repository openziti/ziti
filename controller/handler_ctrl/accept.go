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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/openziti/ziti/v2/controller/xctrl"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
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

// NewMultiListener returns an acceptor that handles both grouped (multi-underlay) and
// ungrouped (single underlay) connections from routers.
func (self *CtrlAccepter) NewMultiListener() channel.UnderlayAcceptor {
	multiListener := channel.NewMultiListener(self.HandleGroupedUnderlay, self.AcceptUnderlay)
	return &multiListenerAcceptor{multiListener: multiListener}
}

// multiListenerAcceptor wraps MultiListener to implement UnderlayAcceptor
type multiListenerAcceptor struct {
	multiListener *channel.MultiListener
}

func (self *multiListenerAcceptor) AcceptUnderlay(underlay channel.Underlay) error {
	self.multiListener.AcceptUnderlay(underlay)
	return nil
}

// HandleGroupedUnderlay handles incoming grouped connections from routers that support
// multi-underlay control channels. It creates a MultiChannel with ListenerCtrlChannel.
func (self *CtrlAccepter) HandleGroupedUnderlay(underlay channel.Underlay, closeCallback func()) (channel.MultiChannel, error) {
	listenerCtrlChan := ctrlchan.NewListenerCtrlChannel()
	multiConfig := channel.MultiChannelConfig{
		LogicalName:     "ctrl/" + underlay.Id(),
		Options:         self.options,
		UnderlayHandler: listenerCtrlChan,
		BindHandler: channel.BindHandlerF(func(binding channel.Binding) error {
			binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
				closeCallback()
			}))
			return self.Bind(binding)
		}),
		Underlay: underlay,
	}
	mc, err := channel.NewMultiChannel(&multiConfig)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("failure accepting ctrl channel %v with multi-underlay", underlay.Label())
		return nil, err
	}
	return mc, nil
}

// AcceptUnderlay handles incoming ungrouped connections from routers that don't support
// multi-underlay control channels (backward compatibility).
func (self *CtrlAccepter) AcceptUnderlay(underlay channel.Underlay) error {
	_, err := self.HandleGroupedUnderlay(underlay, func() {})
	return err
}

func (self *CtrlAccepter) Bind(binding channel.Binding) error {
	binding.GetChannel().SetLogicalName(binding.GetChannel().Id())
	ch := binding.GetChannel()

	log := pfxlog.Logger().WithField("routerId", ch.Id())
	// Use a new copy of the router instance each time we connect. That way we can tell on disconnect
	// if we're working with the right connection, in case connects and disconnects happen quickly.
	// It also means that the channel and connected time fields don't change and we don't have to protect them
	r, err := self.network.GetReloadedRouter(ch.Id())
	if err != nil {
		return err
	}
	if r == nil {
		return errors.Errorf("no router with id [%v] found, closing connection", ch.Id())
	}

	if ch.Underlay().Headers() != nil {
		if versionValue, found := ch.Underlay().Headers()[channel.HelloVersionHeader]; found {
			if versionInfo, err := self.network.VersionProvider.EncoderDecoder().Decode(versionValue); err == nil {
				r.VersionInfo = versionInfo
				log = log.WithField("version", r.VersionInfo.Version).
					WithField("revision", r.VersionInfo.Revision).
					WithField("buildDate", r.VersionInfo.BuildDate).
					WithField("os", r.VersionInfo.OS).
					WithField("arch", r.VersionInfo.Arch)
			} else {
				return errors.Wrap(err, "could not parse version info from router hello, not accepting router connection")
			}
		} else {
			return errors.New("no version info header, not accepting router connection")
		}

		r.Listeners = nil
		headers := ch.Underlay().Headers()

		// Determine header locations based on router capabilities. 2.0+ routers
		// send a CapabilitiesHeader with RouterMultiChannel set and use header IDs
		// in the 1000+ range. Pre-2.0 routers use legacy IDs (10-12) and don't
		// send a CapabilitiesHeader.
		r.Capabilities = capabilities.GetCapabilities(headers)
		useNewHeaders := capabilities.IsSet(r.Capabilities, capabilities.RouterMultiChannel)

		listenersHeaderId := ctrl_pb.LegacyListenersHeader
		if useNewHeaders {
			listenersHeaderId = int32(ctrl_pb.ControlHeaders_ListenersHeader)
		}

		if val, found := headers[listenersHeaderId]; found {
			listeners := &ctrl_pb.Listeners{}
			if err = proto.Unmarshal(val, listeners); err != nil {
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
			log.Debug("no advertised listeners")
		}

		if val, found := ch.Underlay().Headers()[int32(ctrl_pb.ControlHeaders_CtrlChanListenersHeader)]; found {
			ctrlListeners := &ctrl_pb.CtrlChanListeners{}
			if err = proto.Unmarshal(val, ctrlListeners); err != nil {
				log.WithError(err).Error("unable to unmarshal ctrl chan listeners value")
			} else {
				result := make(map[string][]string, len(ctrlListeners.Listeners))
				for _, listener := range ctrlListeners.Listeners {
					result[listener.Address] = listener.Groups
				}
				r.CtrlChanListeners = result
			}
		} else {
			r.CtrlChanListeners = nil
		}
	} else {
		return errors.New("channel provided no headers, not accepting router connection as version info not provided")
	}

	r.Control = ch.(channel.MultiChannel).GetUnderlayHandler().(ctrlchan.CtrlChannel)
	r.ConnectTime = time.Now()
	if err := binding.Bind(newBindHandler(self.heartbeatOptions, r, self.network, self.xctrls)); err != nil {
		return errors.Wrap(err, "error binding router")
	}

	if self.traceHandler != nil {
		binding.AddPeekHandler(self.traceHandler)
	}

	log.Info("accepted new router connection")

	self.network.ConnectRouter(r)

	changeCtx := change.NewControlChannelChange(r.Id, r.Name, "router.connect", ch)
	if err := self.network.Router.UpdateCtrlChanListeners(r.Id, r.CtrlChanListeners, changeCtx); err != nil {
		log.WithError(err).Error("failed to update ctrl chan listeners")
	}

	return nil
}
