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
	"runtime/debug"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/latency"
	"github.com/openziti/fabric/common/metrics"
	"github.com/openziti/fabric/router/env"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/common/trace"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type bindHandler struct {
	env                env.RouterEnv
	forwarder          *forwarder.Forwarder
	xgDialerPool       goroutines.Pool
	ctrlAddressUpdater CtrlAddressUpdater
}

func NewBindHandler(routerEnv env.RouterEnv, forwarder *forwarder.Forwarder, ctrlAddressUpdater CtrlAddressUpdater) (channel.BindHandler, error) {
	xgDialerPoolConfig := goroutines.PoolConfig{
		QueueSize:   uint32(forwarder.Options.XgressDial.QueueLength),
		MinWorkers:  0,
		MaxWorkers:  uint32(forwarder.Options.XgressDial.WorkerCount),
		IdleTime:    30 * time.Second,
		CloseNotify: routerEnv.GetCloseNotify(),
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().WithField(logrus.ErrorKey, err).WithField("backtrace", string(debug.Stack())).Error("panic during xgress dial")
		},
	}

	metrics.ConfigureGoroutinesPoolMetrics(&xgDialerPoolConfig, routerEnv.GetMetricsRegistry(), "pool.route.handler")

	xgDialerPool, err := goroutines.NewPool(xgDialerPoolConfig)
	if err != nil {
		return nil, errors.Wrap(err, "error creating xgress route handler pool")
	}

	return &bindHandler{
		env:                routerEnv,
		forwarder:          forwarder,
		xgDialerPool:       xgDialerPool,
		ctrlAddressUpdater: ctrlAddressUpdater,
	}, nil
}

func (self *bindHandler) BindChannel(binding channel.Binding) error {
	binding.AddTypedReceiveHandler(newPeerStateChangeHandler(self.env))
	binding.AddTypedReceiveHandler(newDialHandler(self.env))
	binding.AddTypedReceiveHandler(newRouteHandler(binding.GetChannel(), self.env, self.forwarder, self.xgDialerPool))
	binding.AddTypedReceiveHandler(newValidateTerminatorsHandler(self.env))
	binding.AddTypedReceiveHandler(newUnrouteHandler(self.forwarder))
	binding.AddTypedReceiveHandler(newTraceHandler(self.env.GetRouterId(), self.forwarder.TraceController(), binding.GetChannel()))
	binding.AddTypedReceiveHandler(newInspectHandler(self.env, self.forwarder))
	binding.AddTypedReceiveHandler(newSettingsHandler(self.ctrlAddressUpdater))
	binding.AddTypedReceiveHandler(newFaultHandler(self.env.GetXlinkRegistry()))
	binding.AddTypedReceiveHandler(newUpdateCtrlAddressesHandler(self.ctrlAddressUpdater))

	binding.AddPeekHandler(trace.NewChannelPeekHandler(self.env.GetRouterId().Token, binding.GetChannel(), self.forwarder.TraceController()))
	latency.AddLatencyProbeResponder(binding)

	ctrl := self.env.GetNetworkControllers().GetNetworkController(binding.GetChannel().Id())
	if ctrl == nil {
		return errors.Errorf("controller [%v] not registered, cannot configure", binding.GetChannel().Id())
	}

	channel.ConfigureHeartbeat(binding, self.env.GetHeartbeatOptions().SendInterval, self.env.GetHeartbeatOptions().CheckInterval, ctrl.HeartbeatCallback())

	if self.env.GetTraceHandler() != nil {
		binding.AddPeekHandler(self.env.GetTraceHandler())
	}

	for _, x := range self.env.GetXrctrls() {
		if err := binding.Bind(x); err != nil {
			return err
		}
	}

	return nil
}
