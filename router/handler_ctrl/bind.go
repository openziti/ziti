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
	"fmt"
	"runtime/debug"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/metrics"
	"github.com/openziti/ziti/v2/common/trace"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/forwarder"
	"github.com/openziti/ziti/v2/router/xgress_router"
	"github.com/sirupsen/logrus"
)

type InspectRouterEnv interface {
	env.RouterEnv
	GetXgressListeners() []xgress_router.Listener
}

type bindHandler struct {
	env                        InspectRouterEnv
	forwarder                  *forwarder.Forwarder
	xgDialerPool               goroutines.Pool
	terminatorValidationPool   goroutines.Pool
	ctrlAddrChangeHandler      channel.TypedReceiveHandler
	clusterLeaderChangeHandler channel.TypedReceiveHandler
}

func XgressDialerWorker(_ uint32, f func()) {
	f()
}

func NewBindHandler(routerEnv InspectRouterEnv, forwarder *forwarder.Forwarder) (channel.BindHandler, error) {
	xgDialerPoolConfig := goroutines.PoolConfig{
		QueueSize:   uint32(forwarder.Options.XgressDial.QueueLength),
		MinWorkers:  0,
		MaxWorkers:  uint32(forwarder.Options.XgressDial.WorkerCount),
		IdleTime:    30 * time.Second,
		CloseNotify: routerEnv.GetCloseNotify(),
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().WithField(logrus.ErrorKey, err).WithField("backtrace", string(debug.Stack())).Error("panic during xgress dial")
		},
		WorkerFunction: XgressDialerWorker,
	}

	metrics.ConfigureGoroutinesPoolMetrics(&xgDialerPoolConfig, routerEnv.GetMetricsRegistry(), "pool.route.handler")

	xgDialerPool, err := goroutines.NewPool(xgDialerPoolConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating xgress route handler pool (%w)", err)
	}

	terminatorValidatorPoolConfig := goroutines.PoolConfig{
		QueueSize:   uint32(1),
		MinWorkers:  0,
		MaxWorkers:  uint32(50),
		IdleTime:    30 * time.Second,
		CloseNotify: routerEnv.GetCloseNotify(),
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().WithField(logrus.ErrorKey, err).WithField("backtrace", string(debug.Stack())).Error("panic during terminator validation operation")
		},
		WorkerFunction: terminatorValidatorWorker,
	}

	metrics.ConfigureGoroutinesPoolMetrics(&terminatorValidatorPoolConfig, routerEnv.GetMetricsRegistry(), "pool.terminator_validation")

	terminatorValidationPool, err := goroutines.NewPool(terminatorValidatorPoolConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating terminator validation pool (%w)", err)
	}

	return &bindHandler{
		env:                        routerEnv,
		forwarder:                  forwarder,
		xgDialerPool:               xgDialerPool,
		terminatorValidationPool:   terminatorValidationPool,
		ctrlAddrChangeHandler:      newUpdateCtrlAddressesHandler(routerEnv),
		clusterLeaderChangeHandler: newUpdateClusterLeaderHandler(routerEnv),
	}, nil
}

func terminatorValidatorWorker(_ uint32, f func()) {
	f()
}

func (self *bindHandler) BindChannel(binding channel.Binding) error {
	if !capabilities.IsCapable(binding.GetChannel().Underlay(), capabilities.ControllerSupportsJWTLegacySessions) {
		pfxlog.Logger().WithField("ctrlId", binding.GetChannel().Id()).
			Error("controller does not support JWT format legacy sessions, use with controller versions 2.0+")
		return fmt.Errorf("controller %s does not support JWT format legacy sessions", binding.GetChannel().Id())
	}

	ctrlCh := binding.GetChannel().(channel.MultiChannel).GetUnderlayHandler().(ctrlchan.CtrlChannel)
	binding.AddTypedReceiveHandler(newPeerStateChangeHandler(self.env))
	binding.AddTypedReceiveHandler(newRouteHandler(ctrlCh, self.env, self.forwarder, self.xgDialerPool))
	binding.AddTypedReceiveHandler(newValidateTerminatorsHandler(self.env))
	binding.AddTypedReceiveHandler(newValidateTerminatorsV2Handler(self.env, self.terminatorValidationPool))
	binding.AddTypedReceiveHandler(newUnrouteHandler(self.forwarder))
	binding.AddTypedReceiveHandler(newTraceHandler(self.env.GetRouterId(), self.forwarder.TraceController(), binding.GetChannel()))
	binding.AddTypedReceiveHandler(newInspectHandler(self.env, self.forwarder))
	binding.AddTypedReceiveHandler(newSettingsHandler(self.env))
	binding.AddTypedReceiveHandler(newFaultHandler(self.env.GetXlinkRegistry()))
	binding.AddTypedReceiveHandler(self.ctrlAddrChangeHandler)
	binding.AddTypedReceiveHandler(self.clusterLeaderChangeHandler)

	binding.AddPeekHandler(trace.NewChannelPeekHandler(self.env.GetRouterId().Token, binding.GetChannel(), self.forwarder.TraceController()))

	ctrl := self.env.GetNetworkControllers().GetNetworkController(binding.GetChannel().Id())
	if ctrl == nil {
		return fmt.Errorf("controller [%v] not registered, cannot configure", binding.GetChannel().Id())
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
