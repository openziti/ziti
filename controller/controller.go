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
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/handler_ctrl"
	"github.com/netfoundry/ziti-fabric/controller/handler_mgmt"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/controller/xctrl"
	"github.com/netfoundry/ziti-fabric/controller/xctrl_example"
	"github.com/netfoundry/ziti-fabric/controller/xmgmt"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/profiler"
	"github.com/netfoundry/ziti-foundation/util/concurrenz"
)

type Controller struct {
	config             *Config
	network            *network.Network
	ctrlConnectHandler *handler_ctrl.ConnectHandler
	mgmtConnectHandler *handler_mgmt.ConnectHandler
	xctrls             []xctrl.Xctrl
	xmgmts             []xmgmt.Xmgmt

	ctrlListener channel2.UnderlayListener
	mgmtListener channel2.UnderlayListener

	shutdownC  chan struct{}
	isShutdown concurrenz.AtomicBoolean
}

func NewController(cfg *Config) (*Controller, error) {
	c := &Controller{
		config:    cfg,
		shutdownC: make(chan struct{}),
	}

	if n, err := network.NewNetwork(cfg.Id, cfg.Network, cfg.Db, cfg.Metrics); err == nil {
		c.network = n
	} else {
		return nil, err
	}

	if err := c.showOptions(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Controller) Run() error {
	c.startProfiling()

	if err := c.registerComponents(); err != nil {
		return fmt.Errorf("error registering component: %s", err)
	}

	/**
	 * ctrl listener/accepter.
	 */
	ctrlListener := channel2.NewClassicListener(c.config.Id, c.config.Ctrl.Listener, c.config.Ctrl.Options.ConnectOptions)
	c.ctrlListener = ctrlListener
	if err := c.ctrlListener.Listen(c.ctrlConnectHandler); err != nil {
		panic(err)
	}
	ctrlAccepter := handler_ctrl.NewCtrlAccepter(c.network, c.xctrls, c.ctrlListener, c.config.Ctrl.Options)
	go ctrlAccepter.Run()
	/* */

	/**
	 * mgmt listener/accepter.
	 */
	mgmtListener := channel2.NewClassicListener(c.config.Id, c.config.Mgmt.Listener, c.config.Mgmt.Options.ConnectOptions)
	c.mgmtListener = mgmtListener
	if err := c.mgmtListener.Listen(c.mgmtConnectHandler); err != nil {
		panic(err)
	}
	mgmtAccepter := handler_mgmt.NewMgmtAccepter(c.mgmtListener, c.config.Mgmt.Options)
	go mgmtAccepter.Run()
	/* */

	c.network.Run()

	return nil
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

		c.network.Shutdown()

		if c.config.Db != nil {
			if err := c.config.Db.Close(); err != nil {
				pfxlog.Logger().WithError(err).Error("failed to close db")
			}
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
}

func (c *Controller) registerComponents() error {
	c.ctrlConnectHandler = handler_ctrl.NewConnectHandler(c.network, c.xctrls)
	c.mgmtConnectHandler = handler_mgmt.NewConnectHandler(c.network)

	c.config.Mgmt.Options.BindHandlers = []channel2.BindHandler{handler_mgmt.NewBindHandler(c.network, c.xmgmts)}

	if err := c.RegisterXctrl(xctrl_example.NewExample()); err != nil {
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

func (c *Controller) GetNetwork() *network.Network {
	return c.network
}
