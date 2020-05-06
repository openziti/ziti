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
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/controller/xctrl"
	"github.com/netfoundry/ziti-fabric/trace"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type bindHandler struct {
	router  *network.Router
	network *network.Network
	xctrls  []xctrl.Xctrl
}

func newBindHandler(router *network.Router, network *network.Network, xctrls []xctrl.Xctrl) *bindHandler {
	return &bindHandler{router: router, network: network, xctrls: xctrls}
}

func (bindHandler *bindHandler) BindChannel(ch channel2.Channel) error {
	ch.SetLogicalName(bindHandler.router.Id)
	ch.AddReceiveHandler(newSessionRequestHandler(bindHandler.router, bindHandler.network))
	ch.AddReceiveHandler(newCreateTerminatorHandler(bindHandler.network, bindHandler.router))
	ch.AddReceiveHandler(newRemoveTerminatorHandler(bindHandler.network))
	ch.AddReceiveHandler(newUpdateTerminatorHandler(bindHandler.network))
	ch.AddReceiveHandler(newLinkHandler(bindHandler.router, bindHandler.network))
	ch.AddReceiveHandler(newFaultHandler(bindHandler.router, bindHandler.network))
	ch.AddReceiveHandler(newMetricsHandler(bindHandler.network))
	ch.AddReceiveHandler(bindHandler.network.GetTraceEventController())
	ch.AddReceiveHandler(newInspectHandler(bindHandler.network))
	ch.AddPeekHandler(trace.NewChannelPeekHandler(bindHandler.network.GetAppId(), ch, bindHandler.network.GetTraceController(), bindHandler.network.GetTraceEventController()))

	xctrlDone := make(chan struct{})
	for _, x := range bindHandler.xctrls {
		if err := ch.Bind(x); err != nil {
			return err
		}
		if err := x.Run(ch, bindHandler.network.GetDb(), xctrlDone); err != nil {
			return err
		}
	}
	if len(bindHandler.xctrls) > 0 {
		ch.AddCloseHandler(newXctrlCloseHandler(xctrlDone))
	}

	ch.AddCloseHandler(newCloseHandler(bindHandler.router, bindHandler.network))
	return nil
}
