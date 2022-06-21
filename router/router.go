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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	gosundheit "github.com/AppsFlyer/go-sundheit"
	"github.com/AppsFlyer/go-sundheit/checks"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/openziti/fabric/health"
	fabricMetrics "github.com/openziti/fabric/metrics"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/profiler"
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
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/foundation/util/info"
	"github.com/openziti/xweb/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"math/rand"
	"plugin"
	"time"
)

type Router struct {
	config          *Config
	ctrl            concurrenz.AtomicValue[channel.Channel]
	faulter         *forwarder.Faulter
	scanner         *forwarder.Scanner
	forwarder       *forwarder.Forwarder
	xctrls          []xctrl.Xctrl
	xlinkFactories  map[string]xlink.Factory
	xlinkListeners  []xlink.Listener
	xlinkDialers    []xlink.Dialer
	xlinkRegistry   xlink.Registry
	xgressListeners []xgress.Listener
	metricsRegistry metrics.UsageRegistry
	shutdownC       chan struct{}
	shutdownDoneC   chan struct{}
	isShutdown      concurrenz.AtomicBoolean
	metricsReporter metrics.Handler
	versionProvider common.VersionProvider
	debugOperations map[byte]func(c *bufio.ReadWriter) error

	xwebs               []xweb.Instance
	xwebFactoryRegistry xweb.Registry
	agentBindHandlers   []channel.BindHandler
}

func (self *Router) GetRouterId() *identity.TokenId {
	return self.config.Id
}

func (self *Router) GetDialerCfg() map[string]xgress.OptionsData {
	return self.config.Dialers
}

func (self *Router) GetXlinkDialer() []xlink.Dialer {
	return self.xlinkDialers
}

func (self *Router) GetXtrls() []xctrl.Xctrl {
	return self.xctrls
}

func (self *Router) GetTraceHandler() *channel.TraceHandler {
	return self.config.Trace.Handler
}

func (self *Router) GetXlinkRegistry() xlink.Registry {
	return self.xlinkRegistry
}

func (self *Router) GetCloseNotify() <-chan struct{} {
	return self.shutdownC
}

func (self *Router) GetMetricsRegistry() metrics.UsageRegistry {
	return self.metricsRegistry
}

func (self *Router) Channel() channel.Channel {
	// if we're just starting up, we may be nil. wait till initialized
	// The initial control channel connect has a timeout, so if that timeouts the process will exit
	// Once connected the control channel will never get set back to nil. Reconnects happen under the hood
	ch := self.ctrl.Load()
	for ch == nil {
		time.Sleep(50 * time.Millisecond)
		ch = self.ctrl.Load()
	}
	return self.ctrl.Load()
}

func (self *Router) DefaultRequestTimeout() time.Duration {
	return self.config.Ctrl.DefaultRequestTimeout
}

func Create(config *Config, versionProvider common.VersionProvider) *Router {
	closeNotify := make(chan struct{})

	metricsRegistry := metrics.NewUsageRegistry(config.Id.Token, map[string]string{}, closeNotify)
	xgress.InitMetrics(metricsRegistry)

	faulter := forwarder.NewFaulter(config.Forwarder.FaultTxInterval, closeNotify)
	scanner := forwarder.NewScanner(config.Forwarder, closeNotify)
	fwd := forwarder.NewForwarder(metricsRegistry, faulter, scanner, config.Forwarder, closeNotify)

	xgress.InitPayloadIngester(closeNotify)
	xgress.InitAcker(fwd, metricsRegistry, closeNotify)
	xgress.InitRetransmitter(fwd, fwd, metricsRegistry, closeNotify)

	return &Router{
		config:              config,
		faulter:             faulter,
		scanner:             scanner,
		forwarder:           fwd,
		metricsRegistry:     metricsRegistry,
		shutdownC:           closeNotify,
		shutdownDoneC:       make(chan struct{}),
		versionProvider:     versionProvider,
		debugOperations:     map[byte]func(c *bufio.ReadWriter) error{},
		xwebFactoryRegistry: xweb.NewRegistryMap(),
		xlinkRegistry:       NewLinkRegistry(),
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

func (self *Router) GetVersionInfo() common.VersionProvider {
	return self.versionProvider
}

func (self *Router) GetConfig() *Config {
	return self.config
}

func (self *Router) Start() error {
	rand.Seed(info.NowInMilliseconds())

	self.showOptions()

	self.startProfiling()

	healthChecker, err := self.initializeHealthChecks()
	if err != nil {
		logrus.WithError(err).Fatalf("failed to create health checker")
	}

	if err := self.RegisterXWebHandlerFactory(health.NewHealthCheckApiFactory(healthChecker)); err != nil {
		logrus.WithError(err).Fatalf("failed to create health checks api factory")
	}

	if err := self.registerComponents(); err != nil {
		return err
	}

	if err := self.registerPlugins(); err != nil {
		return err
	}

	self.startXlinkDialers()
	self.startXlinkListeners()
	self.startXgressListeners()

	for _, web := range self.xwebs {
		go web.Run()
	}

	if err := self.startControlPlane(); err != nil {
		return err
	}
	return nil
}

func (self *Router) Shutdown() error {
	var errors []error
	if self.isShutdown.CompareAndSwap(false, true) {
		if ch := self.ctrl.Load(); ch != nil {
			if err := ch.Close(); err != nil {
				errors = append(errors, err)
			}
		}

		close(self.shutdownC)

		for _, xlinkListener := range self.xlinkListeners {
			if err := xlinkListener.Close(); err != nil {
				errors = append(errors, err)
			}
		}

		self.xlinkRegistry.Shutdown()

		for _, xgressListener := range self.xgressListeners {
			if err := xgressListener.Close(); err != nil {
				errors = append(errors, err)
			}
		}

		for _, web := range self.xwebs {
			go web.Shutdown()
		}

		close(self.shutdownDoneC)
	}
	if len(errors) == 0 {
		return nil
	}
	if len(errors) == 1 {
		return errors[0]
	}
	return errorz.MultipleErrors(errors)
}

func (self *Router) Run() error {
	if err := self.Start(); err != nil {
		return err
	}

	<-self.shutdownDoneC
	return nil
}

func (self *Router) showOptions() {
	if output, err := json.Marshal(self.config.Ctrl.Options); err == nil {
		pfxlog.Logger().Infof("ctrl = %s", string(output))
	} else {
		logrus.Fatalf("unable to display options (%v)", err)
	}

	if output, err := json.Marshal(self.config.Metrics); err == nil {
		pfxlog.Logger().Infof("metrics = %s", string(output))
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
	go newRouterMonitor(self.forwarder, self.shutdownC).Monitor()
}

func (self *Router) registerComponents() error {
	self.xlinkFactories = make(map[string]xlink.Factory)
	xlinkAccepter := newXlinkAccepter(self.forwarder)
	xlinkChAccepter := handler_link.NewBindHandlerFactory(
		self,
		self.forwarder,
		self.config.Forwarder,
		self.metricsRegistry,
		self.xlinkRegistry,
	)
	self.xlinkFactories["transport"] = xlink_transport.NewFactory(xlinkAccepter, xlinkChAccepter, self.config.Transport, self.xlinkRegistry, self.metricsRegistry)

	xgress.GlobalRegistry().Register("proxy", xgress_proxy.NewFactory(self.config.Id, self, self.config.Transport))
	xgress.GlobalRegistry().Register("proxy_udp", xgress_proxy_udp.NewFactory(self))
	xgress.GlobalRegistry().Register("transport", xgress_transport.NewFactory(self.config.Id, self, self.config.Transport))
	xgress.GlobalRegistry().Register("transport_udp", xgress_transport_udp.NewFactory(self.config.Id, self))

	if err := self.RegisterXweb(xweb.NewDefaultInstance(self.xwebFactoryRegistry, self.config.Id)); err != nil {
		return err
	}

	if err := self.RegisterXctrl(self.xlinkRegistry); err != nil {
		return err
	}

	return nil
}

func (self *Router) registerPlugins() error {
	for _, pluginPath := range self.config.Plugins {
		goPlugin, err := plugin.Open(pluginPath)
		if err != nil {
			return errors.Wrapf(err, "router unable to load plugin at path %v", pluginPath)
		}
		initializeSymbol, err := goPlugin.Lookup("Initialize")
		if err != nil {
			return errors.Wrapf(err, "router plugin at %v does not contain Initialize symbol", pluginPath)
		}
		initialize, ok := initializeSymbol.(func(*Router) error)
		if !ok {
			return errors.Errorf("router plugin at %v exports Initialize symbol, but it is not of type 'func(router *router.Router) error'", pluginPath)
		}
		if err := initialize(self); err != nil {
			return errors.Wrapf(err, "error initializing router plugin at %v", pluginPath)
		}
	}
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

		var address string
		if addressVal, found := binding.options["address"]; found {
			address = addressVal.(string)
		}

		err = listener.Listen(address,
			handler_xgress.NewBindHandler(
				handler_xgress.NewReceiveHandler(self.forwarder),
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

func (self *Router) startControlPlane() error {
	attributes := map[int32][]byte{}

	version, err := self.versionProvider.EncoderDecoder().Encode(self.versionProvider.AsVersionInfo())

	if err != nil {
		return fmt.Errorf("error with version header information value: %v", err)
	}

	attributes[channel.HelloVersionHeader] = version

	if len(self.xlinkListeners) == 1 {
		attributes[channel.HelloRouterAdvertisementsHeader] = []byte(self.xlinkListeners[0].GetAdvertisement())
	}

	listeners := &ctrl_pb.Listeners{}
	for _, listener := range self.xlinkListeners {
		listeners.Listeners = append(listeners.Listeners, &ctrl_pb.Listener{
			Address:  listener.GetAdvertisement(),
			Protocol: listener.GetLinkProtocol(),
			CostTags: listener.GetLinkCostTags(),
		})
	}

	if len(listeners.Listeners) > 0 {
		if buf, err := proto.Marshal(listeners); err != nil {
			return errors.Wrap(err, "unable to marshal Listeners")
		} else {
			attributes[int32(ctrl_pb.ContentType_ListenersHeader)] = buf
		}
	}

	reconnectHandler := func() {
		for _, x := range self.xctrls {
			go x.NotifyOfReconnect()
		}
	}

	if "" != self.config.Ctrl.LocalBinding {
		logrus.Debugf("Using local interface %s to dial controller", self.config.Ctrl.LocalBinding)
	}
	dialer := channel.NewReconnectingDialerWithHandlerAndLocalBinding(self.config.Id, self.config.Ctrl.Endpoint, self.config.Ctrl.LocalBinding, attributes, reconnectHandler)

	bindHandler := handler_ctrl.NewBindHandler(self, self.forwarder, self.config)

	ch, err := channel.NewChannel("ctrl", dialer, bindHandler, self.config.Ctrl.Options)
	if err != nil {
		return fmt.Errorf("error connecting ctrl (%v)", err)
	}

	self.ctrl.Store(ch)
	self.faulter.SetCtrl(ch)
	self.scanner.SetCtrl(ch)

	for _, x := range self.xctrls {
		if err := x.Run(ch, nil, self.shutdownC); err != nil {
			return err
		}
	}

	self.metricsReporter = fabricMetrics.NewChannelReporter(ch)
	self.metricsRegistry.StartReporting(self.metricsReporter, self.config.Metrics.ReportInterval, self.config.Metrics.MessageQueueSize)

	return nil
}

func (self *Router) initializeHealthChecks() (gosundheit.Health, error) {
	checkConfig := self.config.HealthChecks
	logrus.Infof("starting health check with ctrl ping initially after %v, then every %v, timing out after %v",
		checkConfig.CtrlPingCheck.InitialDelay, checkConfig.CtrlPingCheck.Interval, checkConfig.CtrlPingCheck.Timeout)

	h := gosundheit.New()
	ctrlPinger := &controllerPinger{
		router: self,
	}
	ctrlPingCheck, err := checks.NewPingCheck("controllerPing", ctrlPinger)
	if err != nil {
		return nil, err
	}

	err = h.RegisterCheck(ctrlPingCheck,
		gosundheit.ExecutionPeriod(checkConfig.CtrlPingCheck.InitialDelay),
		gosundheit.ExecutionTimeout(checkConfig.CtrlPingCheck.Timeout),
		gosundheit.InitiallyPassing(true),
		gosundheit.InitialDelay(checkConfig.CtrlPingCheck.InitialDelay),
	)

	if err != nil {
		return nil, err
	}

	return h, nil
}

func (self *Router) RegisterXweb(x xweb.Instance) error {
	if err := self.config.Configure(x); err != nil {
		return err
	}
	if x.Enabled() {
		self.xwebs = append(self.xwebs, x)
	}
	return nil
}

func (self *Router) RegisterXWebHandlerFactory(x xweb.ApiHandlerFactory) error {
	return self.xwebFactoryRegistry.Add(x)
}

type connectionToggle interface {
	Disconnect() error
	Reconnect() error
}

type controllerPinger struct {
	router *Router
}

func (self *controllerPinger) PingContext(ctx context.Context) error {
	ch := self.router.Channel()
	if ch == nil {
		return errors.Errorf("control channel not yet established")
	}

	if ch.IsClosed() {
		return errors.Errorf("control channel not yet established")
	}

	msg := channel.NewMessage(channel.ContentTypePingType, nil)
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	timeout := deadline.Sub(time.Now())
	_, err := msg.WithTimeout(timeout).SendForReply(ch)
	return err
}
