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
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xctrl"
	metrics2 "github.com/openziti/fabric/router/metrics"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/metrics"
	"time"
)

type bindHandler struct {
	router  *network.Router
	network *network.Network
	xctrls  []xctrl.Xctrl
}

func newBindHandler(router *network.Router, network *network.Network, xctrls []xctrl.Xctrl) *bindHandler {
	return &bindHandler{router: router, network: network, xctrls: xctrls}
}

func (self *bindHandler) BindChannel(ch channel2.Channel) error {
	traceDispatchWrapper := trace.NewDispatchWrapper(self.network.GetEventDispatcher().Dispatch)
	ch.SetLogicalName(self.router.Id)
	ch.AddReceiveHandler(newSessionRequestHandler(self.router, self.network))
	ch.AddReceiveHandler(newRouteResultHandler(self.network, self.router))
	ch.AddReceiveHandler(newSessionConfirmationHandler(self.network, self.router))
	ch.AddReceiveHandler(newCreateTerminatorHandler(self.network, self.router))
	ch.AddReceiveHandler(newRemoveTerminatorHandler(self.network))
	ch.AddReceiveHandler(newUpdateTerminatorHandler(self.network))
	ch.AddReceiveHandler(newLinkHandler(self.router, self.network))
	ch.AddReceiveHandler(newFaultHandler(self.router, self.network))
	ch.AddReceiveHandler(newMetricsHandler(self.network))
	ch.AddReceiveHandler(newTraceHandler(traceDispatchWrapper))
	ch.AddReceiveHandler(newInspectHandler(self.network))
	ch.AddReceiveHandler(newPingHandler())
	ch.AddPeekHandler(trace.NewChannelPeekHandler(self.network.GetAppId().Token, ch, self.network.GetTraceController(), traceDispatchWrapper))
	ch.AddPeekHandler(metrics2.NewCtrlChannelPeekHandler(self.router.Id, self.network.GetMetricsRegistry()))

	if self.router.VersionInfo.HasMinimumVersion("0.18.7") {
		latencyHandler := &ctrlChannelLatencyHandler{
			histogram: self.network.GetMetricsRegistry().Histogram("ctrl." + self.router.Id + ".latency"),
		}
		metrics.AddLatencyProbe(ch, self.network.GetOptions().CtrlChanLatencyInterval, latencyHandler)
	}

	xctrlDone := make(chan struct{})
	for _, x := range self.xctrls {
		if err := ch.Bind(x); err != nil {
			return err
		}
		if err := x.Run(ch, self.network.GetDb(), xctrlDone); err != nil {
			return err
		}
	}
	if len(self.xctrls) > 0 {
		ch.AddCloseHandler(newXctrlCloseHandler(xctrlDone))
	}

	ch.AddCloseHandler(newCloseHandler(self.router, self.network))
	return nil
}

type ctrlChannelLatencyHandler struct {
	histogram metrics.Histogram
}

func (self *ctrlChannelLatencyHandler) LatencyReported(latency time.Duration) {
	self.histogram.Update(latency.Nanoseconds())
}

func (self *ctrlChannelLatencyHandler) ChannelClosed() {
	self.histogram.Dispose()
}
