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
	"github.com/openziti/fabric/controller/handler_ctrl"
	"github.com/openziti/fabric/controller/handler_mgmt"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/openziti/fabric/controller/xctrl_example"
	"github.com/openziti/fabric/controller/xmgmt"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/controller/xt_ha"
	"github.com/openziti/fabric/controller/xt_random"
	"github.com/openziti/fabric/controller/xt_smartrouting"
	"github.com/openziti/fabric/controller/xt_weighted"
	"github.com/openziti/fabric/controller/xtv"
	"github.com/openziti/fabric/events"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/profiler"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/pkg/errors"
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

func NewController(cfg *Config, versionProvider common.VersionProvider) (*Controller, error) {
	c := &Controller{
		config:    cfg,
		shutdownC: make(chan struct{}),
	}

	c.registerXts()
	if err := c.loadXtvMappings(); err != nil {
		return nil, err
	}

	c.loadEventHandlers()

	if n, err := network.NewNetwork(cfg.Id, cfg.Network, cfg.Db, cfg.Metrics, versionProvider); err == nil {
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
	versionInfo := c.network.VersionProvider.AsVersionInfo()
	versionHeader, err := c.network.VersionProvider.EncoderDecoder().Encode(versionInfo)

	if err != nil {
		pfxlog.Logger().Panicf("could not prepare version headers: %v", err)
	}
	headers := map[int32][]byte{
		channel2.HelloVersionHeader: versionHeader,
	}

	/**
	 * ctrl listener/accepter.
	 */
	ctrlListener := channel2.NewClassicListener(c.config.Id, c.config.Ctrl.Listener, c.config.Ctrl.Options.ConnectOptions, headers)
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
	mgmtListener := channel2.NewClassicListener(c.config.Id, c.config.Mgmt.Listener, c.config.Mgmt.Options.ConnectOptions, headers)
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

func (c *Controller) loadXtvMappings() error {
	if t, ok := c.config.src["terminator"]; ok {
		if tm, ok := t.(map[interface{}]interface{}); ok {
			if vals, ok := tm["validators"]; ok {
				if valMap, ok := vals.(map[interface{}]interface{}); ok {
					for k, v := range valMap {
						binding, ok := k.(string)
						if !ok {
							return errors.Errorf("invalid binding in terminator.validators configuration: %v", binding)
						}
						validatorId, ok := v.(string)
						if !ok {
							return errors.Errorf("invalid validator id in terminator.validators configuration: %v", binding)
						}
						xtv.GetRegistry().RegisterBinding(binding, validatorId)
					}
				}
			}
		}
	}
	return nil
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
	xt.GlobalRegistry().RegisterFactory(xt_ha.NewFactory())
	xt.GlobalRegistry().RegisterFactory(xt_random.NewFactory())
	xt.GlobalRegistry().RegisterFactory(xt_weighted.NewFactory())
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
