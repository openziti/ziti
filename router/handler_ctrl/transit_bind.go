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
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/metrics"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/forwarder"
	"github.com/sirupsen/logrus"
)

// transitBindHandler is a control channel bind handler for client network connections.
// It registers only the transit-relevant handlers (route, unroute, peer state change)
// and configures heartbeats, without any edge or terminator validation functionality.
type transitBindHandler struct {
	networkId    uint16
	env          env.RouterEnv
	forwarder    *forwarder.Forwarder
	xgDialerPool goroutines.Pool
	ctrls        env.NetworkControllers
}

// NewTransitBindHandler creates a bind handler for a transit-only control channel on the
// given client network. It sets up an xgress dialer pool and returns a handler that
// registers only route, unroute, and peer state change handlers on bind.
func NewTransitBindHandler(networkId uint16, routerEnv env.RouterEnv, forwarder *forwarder.Forwarder, ctrls env.NetworkControllers) (channel.BindHandler, error) {
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

	metrics.ConfigureGoroutinesPoolMetrics(&xgDialerPoolConfig, routerEnv.GetMetricsRegistry(), fmt.Sprintf("pool.route.handler.net_%d", networkId))

	xgDialerPool, err := goroutines.NewPool(xgDialerPoolConfig)
	if err != nil {
		return nil, fmt.Errorf("error creating xgress route handler pool for network %d (%w)", networkId, err)
	}

	return &transitBindHandler{
		networkId:    networkId,
		env:          routerEnv,
		forwarder:    forwarder,
		xgDialerPool: xgDialerPool,
		ctrls:        ctrls,
	}, nil
}

func (self *transitBindHandler) BindChannel(binding channel.Binding) error {
	ctrlCh := binding.GetChannel().(channel.MultiChannel).GetUnderlayHandler().(ctrlchan.CtrlChannel)

	binding.AddTypedReceiveHandler(newRouteHandler(ctrlCh, self.env, self.forwarder, self.xgDialerPool, self.networkId))
	binding.AddTypedReceiveHandler(newUnrouteHandler(self.forwarder, self.networkId))
	binding.AddTypedReceiveHandler(newPeerStateChangeHandler(self.env))

	ctrl := self.ctrls.GetNetworkController(binding.GetChannel().Id())
	if ctrl == nil {
		return fmt.Errorf("controller [%v] not registered, cannot configure heartbeat for network %d", binding.GetChannel().Id(), self.networkId)
	}

	channel.ConfigureHeartbeat(binding, self.env.GetHeartbeatOptions().SendInterval, self.env.GetHeartbeatOptions().CheckInterval, ctrl.HeartbeatCallback())

	return nil
}
