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
	"encoding/binary"
	"encoding/json"
	"fmt"
	gosundheit "github.com/AppsFlyer/go-sundheit"
	"github.com/AppsFlyer/go-sundheit/checks"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/xctrl"
	"github.com/openziti/fabric/health"
	"github.com/openziti/fabric/pb/ctrl_pb"
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
	"github.com/openziti/fabric/xweb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/event"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/profiler"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/foundation/util/info"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"math/rand"
	"plugin"
	"time"
)

type Router struct {
	config          *Config
	ctrl            channel2.Channel
	ctrlOptions     *channel2.Options
	linkOptions     *channel2.Options
	linkListener    channel2.UnderlayListener
	faulter         *forwarder.Faulter
	scanner         *forwarder.Scanner
	forwarder       *forwarder.Forwarder
	xctrls          []xctrl.Xctrl
	xctrlDone       chan struct{}
	xlinkFactories  map[string]xlink.Factory
	xlinkListeners  []xlink.Listener
	xlinkDialers    []xlink.Dialer
	xgressListeners []xgress.Listener
	metricsRegistry metrics.UsageRegistry
	shutdownC       chan struct{}
	shutdownDoneC   chan struct{}
	isShutdown      concurrenz.AtomicBoolean
	eventDispatcher event.Dispatcher
	metricsReporter metrics.Handler
	versionProvider common.VersionProvider
	debugOperations map[byte]func(c *bufio.ReadWriter) error

	xwebs               []xweb.Xweb
	xwebFactoryRegistry xweb.WebHandlerFactoryRegistry
}

func (self *Router) MetricsRegistry() metrics.UsageRegistry {
	return self.metricsRegistry
}

func (self *Router) Channel() channel2.Channel {
	return self.ctrl
}

func (self *Router) DefaultRequestTimeout() time.Duration {
	return self.config.Ctrl.DefaultRequestTimeout
}

func Create(config *Config, versionProvider common.VersionProvider) *Router {
	closeNotify := make(chan struct{})

	eventDispatcher := event.NewDispatcher(closeNotify)
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
		eventDispatcher:     eventDispatcher,
		versionProvider:     versionProvider,
		debugOperations:     map[byte]func(c *bufio.ReadWriter) error{},
		xwebFactoryRegistry: xweb.NewWebHandlerFactoryRegistryImpl(),
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

func (self *Router) GetMetricsRegistry() metrics.UsageRegistry {
	return self.metricsRegistry
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
		if self.ctrl != nil {
			if err := self.ctrl.Close(); err != nil {
				errors = append(errors, err)
			}
		}

		close(self.shutdownC)
		close(self.xctrlDone)

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

	if err := self.RegisterXweb(xweb.NewXwebImpl(self.xwebFactoryRegistry, self.config.Id)); err != nil {
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

	attributes[channel2.HelloVersionHeader] = version

	if len(self.xlinkListeners) == 1 {
		attributes[channel2.HelloRouterAdvertisementsHeader] = []byte(self.xlinkListeners[0].GetAdvertisement())
	}

	reconnectHandler := func() {
		for _, x := range self.xctrls {
			x.NotifyOfReconnect()
		}
	}

	dialer := channel2.NewReconnectingDialerWithHandler(self.config.Id, self.config.Ctrl.Endpoint, attributes, reconnectHandler)

	bindHandler := handler_ctrl.NewBindHandler(
		self.config.Id,
		self.config.Dialers,
		self.xlinkDialers,
		self,
		self.forwarder,
		self.xctrls,
		self.shutdownC,
	)

	self.config.Ctrl.Options.BindHandlers = append(self.config.Ctrl.Options.BindHandlers, bindHandler)

	ch, err := channel2.NewChannel("ctrl", dialer, self.config.Ctrl.Options)
	if err != nil {
		return fmt.Errorf("error connecting ctrl (%v)", err)
	}

	self.ctrl = ch
	self.faulter.SetCtrl(ch)
	self.scanner.SetCtrl(ch)

	self.xctrlDone = make(chan struct{})
	for _, x := range self.xctrls {
		if err := x.Run(self.ctrl, nil, self.xctrlDone); err != nil {
			return err
		}
	}

	self.metricsReporter = metrics.NewChannelReporter(self.ctrl)
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

const (
	DumpForwarderTables byte = 1
	UpdateRoute         byte = 2
	CloseControlChannel byte = 3
	OpenControlChannel  byte = 4
)

func (self *Router) RegisterDefaultDebugOps() {
	self.debugOperations[DumpForwarderTables] = self.debugOpWriteForwarderTables
	self.debugOperations[UpdateRoute] = self.debugOpUpdateRouter
	self.debugOperations[CloseControlChannel] = self.debugOpCloseControlChannel
	self.debugOperations[OpenControlChannel] = self.debugOpOpenControlChannel
}

func (self *Router) RegisterDebugOp(opId byte, f func(c *bufio.ReadWriter) error) {
	self.debugOperations[opId] = f
}

func (self *Router) debugOpWriteForwarderTables(c *bufio.ReadWriter) error {
	tables := self.forwarder.Debug()
	_, err := c.Write([]byte(tables))
	return err
}

func (self *Router) debugOpUpdateRouter(c *bufio.ReadWriter) error {
	logrus.Error("received debug operation to update routes")
	sizeBuf := make([]byte, 4)
	if _, err := c.Read(sizeBuf); err != nil {
		return err
	}
	size := binary.LittleEndian.Uint32(sizeBuf)
	messageBuf := make([]byte, size)

	if _, err := c.Read(messageBuf); err != nil {
		return err
	}

	route := &ctrl_pb.Route{}
	if err := proto.Unmarshal(messageBuf, route); err != nil {
		return err
	}

	logrus.Errorf("updating with route: %+v", route)
	logrus.Errorf("updating with route: %v", route)

	self.forwarder.Route(route)
	_, _ = c.WriteString("route added")
	return nil
}

func (self *Router) debugOpCloseControlChannel(c *bufio.ReadWriter) error {
	logrus.Warn("control channel: closing")
	_, _ = c.WriteString("control channel: closing\n")
	if toggleable, ok := self.ctrl.Underlay().(connectionToggle); ok {
		if err := toggleable.Disconnect(); err != nil {
			logrus.WithError(err).Error("control channel: failed to close")
			_, _ = c.WriteString(fmt.Sprintf("control channel: failed to close (%v)\n", err))
		} else {
			logrus.Warn("control channel: closed")
			_, _ = c.WriteString("control channel: closed")
		}
	} else {
		logrus.Warn("control channel: error not toggleable")
		_, _ = c.WriteString("control channel: error not toggleable")
	}
	return nil
}

func (self *Router) debugOpOpenControlChannel(c *bufio.ReadWriter) error {
	logrus.Warn("control channel: reconnecting")
	if togglable, ok := self.ctrl.Underlay().(connectionToggle); ok {
		if err := togglable.Reconnect(); err != nil {
			logrus.WithError(err).Error("control channel: failed to reconnect")
			_, _ = c.WriteString(fmt.Sprintf("control channel: failed to reconnect (%v)\n", err))
		} else {
			logrus.Warn("control channel: reconnected")
			_, _ = c.WriteString("control channel: reconnected")
		}
	} else {
		logrus.Warn("control channel: error not toggleable")
		_, _ = c.WriteString("control channel: error not toggleable")
	}
	return nil
}

func (self *Router) HandleDebug(conn io.ReadWriter) error {
	bconn := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	op, err := bconn.ReadByte()
	if err != nil {
		return err
	}

	if opF, ok := self.debugOperations[op]; ok {
		return opF(bconn)
	}
	return errors.Errorf("invalid operation %v", op)
}

func (self *Router) RegisterXweb(x xweb.Xweb) error {
	if err := self.config.Configure(x); err != nil {
		return err
	}
	if x.Enabled() {
		self.xwebs = append(self.xwebs, x)
	}
	return nil
}

func (self *Router) RegisterXWebHandlerFactory(x xweb.WebHandlerFactory) error {
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
	ch := self.router.ctrl
	if ch == nil {
		return errors.Errorf("control channel not yet established")
	}

	if ch.IsClosed() {
		return errors.Errorf("control channel not yet established")
	}

	msg := channel2.NewMessage(channel2.ContentTypePingType, nil)
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	timeout := deadline.Sub(time.Now())
	_, err := self.router.ctrl.SendAndWaitWithTimeout(msg, timeout)
	return err
}
