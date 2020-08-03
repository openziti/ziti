/*
	(c) Copyright NetFoundry, Inc.

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

package router

import (
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/handler_ctrl"
	"github.com/openziti/fabric/router/handler_link"
	"github.com/openziti/fabric/router/handler_xgress"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xgress_proxy"
	"github.com/openziti/fabric/router/xgress_proxy_udp"
	"github.com/openziti/fabric/router/xgress_transport"
	"github.com/openziti/fabric/router/xgress_transport_udp"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/fabric/router/xlink_transport"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/profiler"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/info"
	"github.com/sirupsen/logrus"
	"math/rand"
	"time"
)

type Router struct {
	config          *Config
	ctrl            channel2.Channel
	ctrlOptions     *channel2.Options
	linkOptions     *channel2.Options
	linkListener    channel2.UnderlayListener
	forwarder       *forwarder.Forwarder
	xctrls          []xctrl.Xctrl
	xctrlDone       chan struct{}
	xlinkFactories  map[string]xlink.Factory
	xlinkListeners  []xlink.Listener
	xlinkDialers    []xlink.Dialer
	xgressListeners []xgress.Listener
	metricsRegistry metrics.Registry
	shutdownC       chan struct{}
	isShutdown      concurrenz.AtomicBoolean
}

func (self *Router) Channel() channel2.Channel {
	return self.ctrl
}

func Create(config *Config) *Router {
	metricsRegistry := metrics.NewRegistry(config.Id.Token, make(map[string]string), time.Second*15, nil)

	return &Router{
		config:          config,
		forwarder:       forwarder.NewForwarder(metricsRegistry, config.Forwarder),
		metricsRegistry: metricsRegistry,
		shutdownC:       make(chan struct{}),
	}
}

func (self *Router) RegisterXctrl(x xctrl.Xctrl) error {
	if err := self.config.Configure(x); err != nil {
		return err
	}
	if x.Enabled() {
		self.xctrls = append(self.xctrls, x)
	}
	return nil
}

func (self *Router) Start() error {
	rand.Seed(info.NowInMilliseconds())

	self.showOptions()

	self.startProfiling()

	if err := self.registerComponents(); err != nil {
		return err
	}

	self.startXlinkDialers()
	self.startXlinkListeners()
	self.startXgressListeners()

	if err := self.startControlPlane(); err != nil {
		return err
	}
	return nil
}

func (self *Router) Shutdown() error {
	var errors []error
	if self.isShutdown.CompareAndSwap(false, true) {
		if err := self.ctrl.Close(); err != nil {
			errors = append(errors, err)
		}

		close(self.shutdownC)

		for _, xlinkListener := range self.xlinkListeners {
			if err := xlinkListener.Close(); err != nil {
				errors = append(errors, err)
			}
		}

		for _, xgressListener := range self.xgressListeners {
			if err := xgressListener.Close(); err != nil {
				errors = append(errors, err)
			}
		}
	}
	if len(errors) == 0 {
		return nil
	}
	if len(errors) == 1 {
		return errors[0]
	}
	return network.MultipleErrors(errors)
}

func (self *Router) Run() error {
	if err := self.Start(); err != nil {
		return err
	}
	for {
		time.Sleep(1 * time.Hour)
	}
}

func (self *Router) showOptions() {
	if ctrl, err := json.Marshal(self.config.Ctrl.Options); err == nil {
		pfxlog.Logger().Infof("ctrl = %s", string(ctrl))
	} else {
		logrus.Fatalf("unable to display options (%v)", err)
	}
}

func (self *Router) startProfiling() {
	if self.config.Profile.Memory.Path != "" {
		go profiler.NewMemoryWithShutdown(self.config.Profile.Memory.Path, self.config.Profile.Memory.Interval, self.shutdownC).Run()
	}
	if self.config.Profile.CPU.Path != "" {
		if cpu, err := profiler.NewCPUWithShutdown(self.config.Profile.CPU.Path, self.shutdownC); err == nil {
			go cpu.Run()
		} else {
			logrus.Errorf("unexpected error launching cpu profiling (%v)", err)
		}
	}
	go newRouterMonitor(self.forwarder).Monitor()
}

func (self *Router) registerComponents() error {
	self.xlinkFactories = make(map[string]xlink.Factory)
	xlinkAccepter := newXlinkAccepter(self.forwarder)
	xlinkChAccepter := handler_link.NewChannelAccepter(self,
		self.forwarder,
		self.config.Forwarder,
		self.metricsRegistry,
	)
	self.xlinkFactories["transport"] = xlink_transport.NewFactory(xlinkAccepter, xlinkChAccepter, self.config.Transport)

	xgress.GlobalRegistry().Register("proxy", xgress_proxy.NewFactory(self.config.Id, self, self.config.Transport))
	xgress.GlobalRegistry().Register("proxy_udp", xgress_proxy_udp.NewFactory(self))
	xgress.GlobalRegistry().Register("transport", xgress_transport.NewFactory(self.config.Id, self, self.config.Transport))
	xgress.GlobalRegistry().Register("transport_udp", xgress_transport_udp.NewFactory(self.config.Id, self))

	return nil
}

func (self *Router) startXlinkDialers() {
	for _, lmap := range self.config.Link.Dialers {
		binding := lmap["binding"].(string)
		if factory, found := self.xlinkFactories[binding]; found {
			dialer, err := factory.CreateDialer(self.config.Id, self.forwarder, lmap)
			if err != nil {
				logrus.Fatalf("error creating Xlink dialer (%v)", err)
			}
			self.xlinkDialers = append(self.xlinkDialers, dialer)
			logrus.Infof("started Xlink dialer with binding [%s]", binding)
		}
	}
}

func (self *Router) startXlinkListeners() {
	for _, lmap := range self.config.Link.Listeners {
		binding := lmap["binding"].(string)
		if factory, found := self.xlinkFactories[binding]; found {
			listener, err := factory.CreateListener(self.config.Id, self.forwarder, lmap)
			if err != nil {
				logrus.Fatalf("error creating Xlink listener (%v)", err)
			}
			if err := listener.Listen(); err != nil {
				logrus.Fatalf("error listening on Xlink (%v)", err)
			}
			self.xlinkListeners = append(self.xlinkListeners, listener)
			logrus.Infof("started Xlink listener with binding [%s] advertising [%s]", binding, listener.GetAdvertisement())
		}
	}
}

func (self *Router) startXgressListeners() {
	for _, binding := range self.config.Listeners {
		factory, err := xgress.GlobalRegistry().Factory(binding.name)
		if err != nil {
			logrus.Fatalf("error getting xgress factory [%s] (%v)", binding.name, err)
		}
		listener, err := factory.CreateListener(binding.options)
		if err != nil {
			logrus.Fatalf("error creating xgress listener [%s] (%v)", binding.name, err)
		}
		self.xgressListeners = append(self.xgressListeners, listener)
		if address, found := binding.options["address"]; found {
			err = listener.Listen(address.(string),
				handler_xgress.NewBindHandler(
					handler_xgress.NewReceiveHandler(self, self.forwarder),
					handler_xgress.NewCloseHandler(self, self.forwarder),
					self.forwarder,
				),
			)
			if err != nil {
				logrus.Fatalf("error listening [%s] (%v)", binding.name, err)
			}
			logrus.Infof("created xgress listener [%s] at [%s]", binding.name, address)
		}
	}
}

func (self *Router) startControlPlane() error {
	var attributes map[int32][]byte
	if len(self.xlinkListeners) == 1 {
		attributes = make(map[int32][]byte)
		attributes[channel2.HelloRouterAdvertisementsHeader] = []byte(self.xlinkListeners[0].GetAdvertisement())
	}

	dialer := channel2.NewReconnectingDialer(self.config.Id, self.config.Ctrl.Endpoint, attributes)

	bindHandler := handler_ctrl.NewBindHandler(
		self.config.Id,
		self.config.Dialers,
		self.xlinkDialers,
		self,
		self.forwarder,
		self.xctrls,
	)

	self.config.Ctrl.Options.BindHandlers = append(self.config.Ctrl.Options.BindHandlers, bindHandler)

	ch, err := channel2.NewChannel("ctrl", dialer, self.config.Ctrl.Options)
	if err != nil {
		return fmt.Errorf("error connecting ctrl (%v)", err)
	}

	self.ctrl = ch

	self.xctrlDone = make(chan struct{})
	for _, x := range self.xctrls {
		if err := x.Run(self.ctrl, nil, self.xctrlDone); err != nil {
			return err
		}
	}

	self.metricsRegistry.EventController().AddHandler(metrics.NewChannelReporter(self.ctrl))

	return nil
}
