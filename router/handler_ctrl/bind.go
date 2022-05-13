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
	"github.com/openziti/channel/latency"
	"github.com/openziti/fabric/metrics"
	"github.com/openziti/fabric/router/env"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/util/goroutines"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"time"
)

type bindHandler struct {
	env                env.RouterEnv
	forwarder          *forwarder.Forwarder
	ctrlAddressChanger CtrlAddressChanger
}

func NewBindHandler(routerEnv env.RouterEnv, forwarder *forwarder.Forwarder, ctrlAddressChanger CtrlAddressChanger) channel.BindHandler {
	return &bindHandler{
		env:                routerEnv,
		forwarder:          forwarder,
		ctrlAddressChanger: ctrlAddressChanger,
	}
}

func (self *bindHandler) BindChannel(binding channel.Binding) error {
	linkDialerPoolConfig := goroutines.PoolConfig{
		QueueSize:   uint32(self.forwarder.Options.LinkDial.QueueLength),
		MinWorkers:  0,
		MaxWorkers:  uint32(self.forwarder.Options.LinkDial.WorkerCount),
		IdleTime:    30 * time.Second,
		CloseNotify: self.env.GetCloseNotify(),
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().WithField(logrus.ErrorKey, err).Error("panic during link dial")
		},
	}

	metrics.ConfigureGoroutinesPoolMetrics(self.env.GetMetricsRegistry(), "pool.link.dialer")

	linkDialerPool, err := goroutines.NewPool(linkDialerPoolConfig)
	if err != nil {
		return errors.Wrap(err, "error creating link dialer pool")
	}

	xgDialerPoolConfig := goroutines.PoolConfig{
		QueueSize:   uint32(self.forwarder.Options.XgressDial.QueueLength),
		MinWorkers:  0,
		MaxWorkers:  uint32(self.forwarder.Options.XgressDial.WorkerCount),
		IdleTime:    30 * time.Second,
		CloseNotify: self.env.GetCloseNotify(),
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().WithField(logrus.ErrorKey, err).Error("panic during xgress dial")
		},
	}

	metrics.ConfigureGoroutinesPoolMetrics(self.env.GetMetricsRegistry(), "pool.route.handler")

	xgDialerPool, err := goroutines.NewPool(xgDialerPoolConfig)
	if err != nil {
		return errors.Wrap(err, "error creating xgress route handler pool")
	}

	binding.AddTypedReceiveHandler(newDialHandler(self.env, linkDialerPool))
	binding.AddTypedReceiveHandler(newRouteHandler(self.env, self.forwarder, xgDialerPool))
	binding.AddTypedReceiveHandler(newValidateTerminatorsHandler(self.env))
	binding.AddTypedReceiveHandler(newUnrouteHandler(self.forwarder))
	binding.AddTypedReceiveHandler(newTraceHandler(self.env.GetRouterId(), self.forwarder.TraceController()))
	binding.AddTypedReceiveHandler(newInspectHandler(self.env.GetRouterId(), self.env.GetXlinkRegistry(), self.forwarder))
	binding.AddTypedReceiveHandler(newSettingsHandler(self.ctrlAddressChanger))
	binding.AddTypedReceiveHandler(newFaultHandler(self.env.GetXlinkRegistry()))

	binding.AddPeekHandler(trace.NewChannelPeekHandler(self.env.GetRouterId().Token, binding.GetChannel(), self.forwarder.TraceController(), trace.NewChannelSink(binding.GetChannel())))
	latency.AddLatencyProbeResponder(binding)

	cb := &heartbeatCallback{}

	channel.ConfigureHeartbeat(binding, 10*time.Second, time.Second, cb)

	if self.env.GetTraceHandler() != nil {
		binding.AddPeekHandler(self.env.GetTraceHandler())
	}

	for _, x := range self.env.GetXtrls() {
		if err := binding.Bind(x); err != nil {
			return err
		}
	}

	return nil
}

type heartbeatCallback struct{}

func (self *heartbeatCallback) HeartbeatTx(int64) {}

func (self *heartbeatCallback) HeartbeatRx(int64) {}

func (self *heartbeatCallback) HeartbeatRespTx(int64) {}

func (self *heartbeatCallback) HeartbeatRespRx(ts int64) {}

func (self *heartbeatCallback) CheckHeartBeat() {}
