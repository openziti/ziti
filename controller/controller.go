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

package controller

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/api_impl"
	"github.com/openziti/fabric/controller/handler_ctrl"
	"github.com/openziti/fabric/controller/handler_mgmt"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/openziti/fabric/controller/xmgmt"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/controller/xt_random"
	"github.com/openziti/fabric/controller/xt_smartrouting"
	"github.com/openziti/fabric/controller/xt_weighted"
	"github.com/openziti/fabric/events"
	"github.com/openziti/fabric/health"
	fabricMetrics "github.com/openziti/fabric/metrics"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/profiler"
	"github.com/openziti/fabric/xweb"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/sirupsen/logrus"
)

type Controller struct {
	config             *Config
	network            *network.Network
	ctrlConnectHandler *handler_ctrl.ConnectHandler
	mgmtConnectHandler *handler_mgmt.ConnectHandler
	xctrls             []xctrl.Xctrl
	xmgmts             []xmgmt.Xmgmt

	xwebs               []xweb.Xweb
	xwebFactoryRegistry xweb.WebHandlerFactoryRegistry

	ctrlListener channel.UnderlayListener
	mgmtListener channel.UnderlayListener

	shutdownC     chan struct{}
	isShutdown    concurrenz.AtomicBoolean
	agentHandlers map[byte]func(c *bufio.ReadWriter) error
}

func NewController(cfg *Config, versionProvider common.VersionProvider) (*Controller, error) {
	c := &Controller{
		config:              cfg,
		shutdownC:           make(chan struct{}),
		xwebFactoryRegistry: xweb.NewWebHandlerFactoryRegistryImpl(),
		agentHandlers:       map[byte]func(c *bufio.ReadWriter) error{},
	}

	c.registerXts()
	c.loadEventHandlers()

	if n, err := network.NewNetwork(cfg.Id.Token, cfg.Network, cfg.Db, cfg.Metrics, versionProvider, c.shutdownC); err == nil {
		c.network = n
	} else {
		return nil, err
	}

	events.InitTerminatorEventRouter(c.network)
	events.InitRouterEventRouter(c.network)

	if cfg.Ctrl.Options.NewListener != nil {
		c.network.AddRouterPresenceHandler(&OnConnectSettingsHandler{
			config: cfg,
			settings: map[int32][]byte{
				int32(ctrl_pb.SettingTypes_NewCtrlAddress): []byte((*cfg.Ctrl.Options.NewListener).String()),
			},
		})
	}

	if err := c.showOptions(); err != nil {
		return nil, err
	}

	c.initWeb()

	return c, nil
}

func (c *Controller) initWeb() {
	healthChecker, err := c.initializeHealthChecks()
	if err != nil {
		logrus.WithError(err).Fatalf("failed to create health checker")
	}

	if err := c.RegisterXWebHandlerFactory(health.NewHealthCheckApiFactory(healthChecker)); err != nil {
		logrus.WithError(err).Fatalf("failed to create health checks api factory")
	}

	if err := c.RegisterXWebHandlerFactory(api_impl.NewManagementApiFactory(c.config.Id, c.network, c.xmgmts)); err != nil {
		logrus.WithError(err).Fatalf("failed to create management api factory")
	}
}

func (c *Controller) Run() error {
	c.startProfiling()

	if err := c.registerComponents(); err != nil {
		return fmt.Errorf("error registering component: %s", err)
	}
	versionInfo := c.network.VersionProvider.AsVersionInfo()
	versionHeader, err := c.network.VersionProvider.EncoderDecoder().Encode(versionInfo)

	if err != nil {
		pfxlog.Logger().Panicf("could not prepare version headers: %v", err)
	}
	headers := map[int32][]byte{
		channel.HelloVersionHeader: versionHeader,
	}

	/**
	 * ctrl listener/accepter.
	 */
	ctrlChannelListenerConfig := channel.ListenerConfig{
		ConnectOptions:   c.config.Ctrl.Options.ConnectOptions,
		PoolConfigurator: fabricMetrics.ConfigureGoroutinesPoolMetrics(c.network.GetMetricsRegistry(), "pool.listener.ctrl"),
		Headers:          headers,
	}
	ctrlListener := channel.NewClassicListener(c.config.Id, c.config.Ctrl.Listener, ctrlChannelListenerConfig)
	c.ctrlListener = ctrlListener
	if err := c.ctrlListener.Listen(c.ctrlConnectHandler); err != nil {
		panic(err)
	}

	ctrlAccepter := handler_ctrl.NewCtrlAccepter(c.network, c.xctrls, c.ctrlListener, c.config.Ctrl.Options.Options, c.config.Trace.Handler)
	go ctrlAccepter.Run()
	/* */

	/**
	 * mgmt listener/accepter.
	 */
	mgmtChannelListenerConfig := channel.ListenerConfig{
		ConnectOptions:   c.config.Mgmt.Options.ConnectOptions,
		PoolConfigurator: fabricMetrics.ConfigureGoroutinesPoolMetrics(c.network.GetMetricsRegistry(), "pool.listener.mgmt"),
		Headers:          headers,
	}
	mgmtListener := channel.NewClassicListener(c.config.Id, c.config.Mgmt.Listener, mgmtChannelListenerConfig)
	c.mgmtListener = mgmtListener
	if err := c.mgmtListener.Listen(c.mgmtConnectHandler); err != nil {
		panic(err)
	}
	mgmtAccepter := handler_mgmt.NewMgmtAccepter(c.network, c.xmgmts, c.mgmtListener, c.config.Mgmt.Options)
	go mgmtAccepter.Run()

	/*
	 * start xweb for http/web API listening
	 */
	for _, web := range c.xwebs {
		go web.Run()
	}

	// event handlers
	if err := events.WireEventHandlers(c.network.InitServiceCounterDispatch); err != nil {
		panic(err)
	}

	events.InitNetwork(c.network)

	c.network.Run()

	return nil
}

func (c *Controller) GetCloseNotifyChannel() <-chan struct{} {
	return c.shutdownC
}

func (c *Controller) Shutdown() {
	if c.isShutdown.CompareAndSwap(false, true) {
		close(c.shutdownC)

		if c.ctrlListener != nil {
			if err := c.ctrlListener.Close(); err != nil {
				pfxlog.Logger().WithError(err).Error("failed to close ctrl channel listener")
			}
		}

		if c.mgmtListener != nil {
			if err := c.mgmtListener.Close(); err != nil {
				pfxlog.Logger().WithError(err).Error("failed to close mgmt channel listener")
			}
		}

		if c.config.Db != nil {
			if err := c.config.Db.Close(); err != nil {
				pfxlog.Logger().WithError(err).Error("failed to close db")
			}
		}

		for _, web := range c.xwebs {
			go web.Shutdown()
		}
	}
}

func (c *Controller) showOptions() error {
	if ctrl, err := json.MarshalIndent(c.config.Ctrl.Options, "", "  "); err == nil {
		pfxlog.Logger().Infof("ctrl = %s", string(ctrl))
	} else {
		return err
	}
	if mgmt, err := json.MarshalIndent(c.config.Mgmt.Options, "", "  "); err == nil {
		pfxlog.Logger().Infof("mgmt = %s", string(mgmt))
	} else {
		return err
	}
	return nil
}

func (c *Controller) startProfiling() {
	if c.config.Profile.Memory.Path != "" {
		go profiler.NewMemoryWithShutdown(c.config.Profile.Memory.Path, c.config.Profile.Memory.Interval, c.shutdownC).Run()
	}
	if c.config.Profile.CPU.Path != "" {
		if cpu, err := profiler.NewCPUWithShutdown(c.config.Profile.CPU.Path, c.shutdownC); err == nil {
			go cpu.Run()
		} else {
			logrus.Errorf("unexpected error launching cpu profiling (%v)", err)
		}
	}
}

func (c *Controller) loadEventHandlers() {
	if e, ok := c.config.src["events"]; ok {
		if em, ok := e.(map[interface{}]interface{}); ok {
			for k, v := range em {
				if handlerConfig, ok := v.(map[interface{}]interface{}); ok {
					events.RegisterEventHandler(k, handlerConfig)
				}
			}
		}
	}
}

func (c *Controller) registerXts() {
	xt.GlobalRegistry().RegisterFactory(xt_smartrouting.NewFactory())
	xt.GlobalRegistry().RegisterFactory(xt_random.NewFactory())
	xt.GlobalRegistry().RegisterFactory(xt_weighted.NewFactory())
}

func (c *Controller) registerComponents() error {
	c.ctrlConnectHandler = handler_ctrl.NewConnectHandler(c.config.Id, c.network)
	c.mgmtConnectHandler = handler_mgmt.NewConnectHandler(c.config.Id, c.network)

	//add default REST XWeb
	if err := c.RegisterXweb(xweb.NewXwebImpl(c.xwebFactoryRegistry, c.config.Id)); err != nil {
		return err
	}

	return nil
}

func (c *Controller) RegisterXctrl(x xctrl.Xctrl) error {
	if err := c.config.Configure(x); err != nil {
		return err
	}
	if x.Enabled() {
		c.xctrls = append(c.xctrls, x)
		if c.config.Trace.Handler != nil {
			for _, decoder := range x.GetTraceDecoders() {
				c.config.Trace.Handler.AddDecoder(decoder)
			}
		}
	}
	return nil
}

func (c *Controller) RegisterXmgmt(x xmgmt.Xmgmt) error {
	if err := c.config.Configure(x); err != nil {
		return err
	}
	if x.Enabled() {
		c.xmgmts = append(c.xmgmts, x)
	}
	return nil
}

func (c *Controller) RegisterXweb(x xweb.Xweb) error {
	if err := c.config.Configure(x); err != nil {
		return err
	}
	if x.Enabled() {
		c.xwebs = append(c.xwebs, x)
	}
	return nil
}

func (c *Controller) RegisterXWebHandlerFactory(x xweb.WebHandlerFactory) error {
	return c.xwebFactoryRegistry.Add(x)
}

func (c *Controller) GetNetwork() *network.Network {
	return c.network
}

func (c *Controller) Identity() identity.Identity {
	return c.config.Id
}
