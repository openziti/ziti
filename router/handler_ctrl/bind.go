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
	"github.com/netfoundry/ziti-fabric/controller/xctrl"
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/netfoundry/ziti-fabric/router/xlink"
	"github.com/netfoundry/ziti-fabric/trace"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type bindHandler struct {
	id           *identity.TokenId
	dialerCfg    map[string]xgress.OptionsData
	xlinkDialers []xlink.Dialer
	ctrl         xgress.CtrlChannel
	forwarder    *forwarder.Forwarder
	xctrls       []xctrl.Xctrl
}

func NewBindHandler(id *identity.TokenId,
	dialerCfg map[string]xgress.OptionsData,
	xlinkDialers []xlink.Dialer,
	ctrl xgress.CtrlChannel,
	forwarder *forwarder.Forwarder,
	xctrls []xctrl.Xctrl) channel2.BindHandler {
	return &bindHandler{
		id:           id,
		dialerCfg:    dialerCfg,
		xlinkDialers: xlinkDialers,
		ctrl:         ctrl,
		forwarder:    forwarder,
		xctrls:       xctrls,
	}
}

func (self *bindHandler) BindChannel(ch channel2.Channel) error {
	ch.AddReceiveHandler(newDialHandler(self.id, self.ctrl, self.xlinkDialers))
	ch.AddReceiveHandler(newRouteHandler(self.id, self.ctrl, self.dialerCfg, self.forwarder))
	ch.AddReceiveHandler(newUnrouteHandler(self.forwarder))
	ch.AddReceiveHandler(newStartXgressHandler(self.forwarder))
	ch.AddReceiveHandler(newTraceHandler(self.id, self.forwarder.TraceController()))
	ch.AddReceiveHandler(newInspectHandler(self.id))
	ch.AddPeekHandler(trace.NewChannelPeekHandler(self.id, ch, self.forwarder.TraceController(), trace.NewChannelSink(ch)))

	for _, x := range self.xctrls {
		if err := ch.Bind(x); err != nil {
			return err
		}
	}

	return nil
}
