/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/metrics"
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/trace"
	"github.com/netfoundry/ziti-fabric/xctrl"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type bindHandler struct {
	id               *identity.TokenId
	dialerCfg        map[string]xgress.OptionsData
	linkOptions      *channel2.Options
	forwarderOptions *forwarder.Options
	ctrl             xgress.CtrlChannel
	forwarder        *forwarder.Forwarder
	xctrls           []xctrl.Xctrl
	metricsRegistry  metrics.Registry
}

func NewBindHandler(id *identity.TokenId,
	dialerCfg map[string]xgress.OptionsData,
	linkOptions *channel2.Options,
	forwarderOptions *forwarder.Options,
	ctrl xgress.CtrlChannel,
	forwarder *forwarder.Forwarder,
	xctrls []xctrl.Xctrl,
	metricsRegistry metrics.Registry) channel2.BindHandler {
	return &bindHandler{
		id:               id,
		dialerCfg:        dialerCfg,
		linkOptions:      linkOptions,
		forwarderOptions: forwarderOptions,
		ctrl:             ctrl,
		forwarder:        forwarder,
		xctrls:           xctrls,
		metricsRegistry:  metricsRegistry,
	}
}

func (bindHandler *bindHandler) BindChannel(ch channel2.Channel) error {
	ch.AddReceiveHandler(newDialHandler(bindHandler.id, bindHandler.ctrl, bindHandler.linkOptions, bindHandler.forwarderOptions, bindHandler.forwarder, bindHandler.metricsRegistry))
	ch.AddReceiveHandler(newRouteHandler(bindHandler.id, bindHandler.ctrl, bindHandler.dialerCfg, bindHandler.forwarder))
	ch.AddReceiveHandler(newUnrouteHandler(bindHandler.forwarder))
	ch.AddReceiveHandler(newStartXgressHandler(bindHandler.forwarder))
	ch.AddReceiveHandler(newTraceHandler(bindHandler.id, bindHandler.forwarder.TraceController()))
	ch.AddReceiveHandler(newInspectHandler(bindHandler.id))
	ch.AddPeekHandler(trace.NewChannelPeekHandler(bindHandler.id, ch, bindHandler.forwarder.TraceController(), trace.NewChannelSink(ch)))

	for _, x := range bindHandler.xctrls {
		if err := ch.Bind(x); err != nil {
			return err
		}
	}

	return nil
}
