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

package handler_mgmt

import (
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xmgmt"
	"github.com/openziti/fabric/trace"
)

type BindHandler struct {
	network *network.Network
	xmgmts  []xmgmt.Xmgmt
}

func NewBindHandler(network *network.Network, xmgmts []xmgmt.Xmgmt) channel.BindHandler {
	return &BindHandler{network: network, xmgmts: xmgmts}
}

func (bindHandler *BindHandler) BindChannel(binding channel.Binding) error {
	network := bindHandler.network
	binding.AddTypedReceiveHandler(newCreateRouterHandler(network))
	binding.AddTypedReceiveHandler(newCreateServiceHandler(network))
	binding.AddTypedReceiveHandler(newGetServiceHandler(network))
	binding.AddTypedReceiveHandler(newInspectHandler(network))
	binding.AddTypedReceiveHandler(newListLinksHandler(network))
	binding.AddTypedReceiveHandler(newListRoutersHandler(network))
	binding.AddTypedReceiveHandler(newListServicesHandler(network))
	binding.AddTypedReceiveHandler(newListCircuitsHandler(network))
	binding.AddTypedReceiveHandler(newRemoveRouterHandler(network))
	binding.AddTypedReceiveHandler(newRemoveServiceHandler(network))
	binding.AddTypedReceiveHandler(newRemoveCircuitHandler(network))
	binding.AddTypedReceiveHandler(newSetLinkCostHandler(network))
	binding.AddTypedReceiveHandler(newSetLinkDownHandler(network))

	binding.AddTypedReceiveHandler(newCreateTerminatorHandler(network))
	binding.AddTypedReceiveHandler(newRemoveTerminatorHandler(network))
	binding.AddTypedReceiveHandler(newGetTerminatorHandler(network))
	binding.AddTypedReceiveHandler(newListTerminatorsHandler(network))
	binding.AddTypedReceiveHandler(newSetTerminatorCostHandler(network))

	streamMetricHandler := newStreamMetricsHandler(network)
	binding.AddTypedReceiveHandler(streamMetricHandler)
	binding.AddCloseHandler(streamMetricHandler)

	streamCircuitsHandler := newStreamCircuitsHandler(network)
	binding.AddTypedReceiveHandler(streamCircuitsHandler)
	binding.AddCloseHandler(streamCircuitsHandler)

	streamTracesHandler := newStreamTracesHandler(network)
	binding.AddTypedReceiveHandler(streamTracesHandler)
	binding.AddCloseHandler(streamTracesHandler)

	binding.AddTypedReceiveHandler(newTogglePipeTracesHandler(network))

	traceDispatchWrapper := trace.NewDispatchWrapper(network.GetEventDispatcher().Dispatch)
	binding.AddPeekHandler(trace.NewChannelPeekHandler(network.GetAppId(), binding.GetChannel(), network.GetTraceController(), traceDispatchWrapper))

	binding.AddTypedReceiveHandler(newSnapshotDbHandler(network))

	xmgmtDone := make(chan struct{})
	for _, x := range bindHandler.xmgmts {
		if err := binding.Bind(x); err != nil {
			return err
		}
		if err := x.Run(binding.GetChannel(), xmgmtDone); err != nil {
			return err
		}
	}
	if len(bindHandler.xmgmts) > 0 {
		binding.AddCloseHandler(newXmgmtCloseHandler(xmgmtDone))
	}

	return nil
}
