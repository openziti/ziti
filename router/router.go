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

package router

import (
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/metrics"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/router/handler_ctrl"
	"github.com/netfoundry/ziti-fabric/router/handler_link"
	"github.com/netfoundry/ziti-fabric/router/handler_xgress"
	"github.com/netfoundry/ziti-fabric/xctrl"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-fabric/xgress_proxy"
	"github.com/netfoundry/ziti-fabric/xgress_proxy_udp"
	"github.com/netfoundry/ziti-fabric/xgress_transport"
	"github.com/netfoundry/ziti-fabric/xgress_transport_udp"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/profiler"
	"github.com/netfoundry/ziti-foundation/util/info"
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
	metricsRegistry metrics.Registry
}

func (router *Router) Channel() channel2.Channel {
	return router.ctrl
}

func Create(config *Config) *Router {
	metricsRegistry := metrics.NewRegistry(ctrl_pb.MetricsSourceType_Internal, config.Id.Token, make(map[string]string), time.Second*15, nil)

	return &Router{
		config:          config,
		forwarder:       forwarder.NewForwarder(metricsRegistry),
		metricsRegistry: metricsRegistry,
	}
}

func (router *Router) RegisterXctrl(x xctrl.Xctrl) error {
	if err := router.config.Configure(x); err != nil {
		return err
	}
	if x.Enabled() {
		router.xctrls = append(router.xctrls, x)
	}
	return nil
}

func (r *Router) Start() error {
	rand.Seed(info.NowInMilliseconds())

	r.showOptions()

	r.startProfiling()

	if err := r.registerComponents(); err != nil {
		return err
	}

	r.instantiateListeners()

	r.startLinkListener()

	if err := r.startControlPlane(); err != nil {
		return err
	}
	return nil
}

func (r *Router) Run() error {
	if err := r.Start(); err != nil {
		return err
	}
	for {
		time.Sleep(1 * time.Hour)
	}
}

func (router *Router) showOptions() {
	if ctrl, err := json.Marshal(router.config.Ctrl.Options); err == nil {
		pfxlog.Logger().Infof("ctrl = %s", string(ctrl))
	} else {
		logrus.Fatalf("unable to display options (%w)", err)
	}
	if link, err := json.Marshal(router.config.Link.Options); err == nil {
		pfxlog.Logger().Infof("link = %s", string(link))
	} else {
		logrus.Fatalf("unable to display options (%w)", err)
	}
}

func (router *Router) startProfiling() {
	if router.config.Profile.Memory.Path != "" {
		go profiler.NewMemory(router.config.Profile.Memory.Path, router.config.Profile.Memory.Interval).Run()
	}
	if router.config.Profile.CPU.Path != "" {
		if cpu, err := profiler.NewCPU(router.config.Profile.CPU.Path); err == nil {
			go cpu.Run()
		} else {
			pfxlog.Logger().Errorf("unexpected error launching cpu profiling (%s)", err)
		}
	}
	go newRouterMonitor(router.forwarder).Monitor()
}

func (router *Router) registerComponents() error {
	xgress.GlobalRegistry().Register("proxy", xgress_proxy.NewFactory(router.config.Id, router))
	xgress.GlobalRegistry().Register("proxy_udp", xgress_proxy_udp.NewFactory(router))
	xgress.GlobalRegistry().Register("transport", xgress_transport.NewFactory(router.config.Id, router))
	xgress.GlobalRegistry().Register("transport_udp", xgress_transport_udp.NewFactory(router.config.Id, router))

	return nil
}

func (router *Router) instantiateListeners() {
	for _, binding := range router.config.Listeners {
		factory, err := xgress.GlobalRegistry().Factory(binding.name)
		if err != nil {
			logrus.Fatalf("error getting xgress factory [%s] (%s)", binding.name, err)
		}
		listener, err := factory.CreateListener(binding.options)
		if err != nil {
			logrus.Fatalf("error creating xgress listener [%s] (%s)", binding.name, err)
		}
		if address, found := binding.options["address"]; found {
			err = listener.Listen(address.(string),
				handler_xgress.NewBindHandler(
					handler_xgress.NewReceiveHandler(router, router.forwarder),
					handler_xgress.NewCloseHandler(router, router.forwarder),
					router.forwarder,
				),
			)
			if err != nil {
				logrus.Fatalf("error listening [%s] (%s)", binding.name, err)
			}
			logrus.Infof("created xgress listener [%s] at [%s]", binding.name, address)
		}
	}
}

func (router *Router) startLinkListener() {
	if router.config.Link.Advertise != nil && router.config.Link.Listener != nil {
		router.linkListener = channel2.NewClassicListener(router.config.Id, router.config.Link.Listener)
		if err := router.linkListener.Listen(); err != nil {
			panic(err)
		}
		linkAccepter := handler_link.NewAccepter(router.config.Id, router, router.forwarder, router.linkListener, router.config.Link.Options, router.config.Forwarder)
		go linkAccepter.Run()
	} else {
		pfxlog.Logger().Warn("skipping link listener due to configuration")
	}
}

func (router *Router) startControlPlane() error {
	var attributes map[int32][]byte
	if router.config.Link.Listener != nil && router.config.Link.Advertise != nil {
		attributes = make(map[int32][]byte)
		attributes[channel2.HelloListenerHeader] = []byte(router.config.Link.Advertise.String())
	}

	dialer := channel2.NewReconnectingDialer(router.config.Id, router.config.Ctrl.Endpoint, attributes)

	bindHandler := handler_ctrl.NewBindHandler(
		router.config.Id,
		router.config.Dialers,
		router.config.Link.Options,
		router.config.Forwarder,
		router,
		router.forwarder,
		router.xctrls,
		router.metricsRegistry,
	)

	router.config.Ctrl.Options.BindHandlers = append(router.config.Ctrl.Options.BindHandlers, bindHandler)

	ch, err := channel2.NewChannel("ctrl", dialer, router.config.Ctrl.Options)
	if err != nil {
		return fmt.Errorf("error connecting ctrl (%s)", err)
	}

	router.ctrl = ch

	router.xctrlDone = make(chan struct{})
	for _, x := range router.xctrls {
		if err := x.Run(router.ctrl, nil, router.xctrlDone); err != nil {
			return err
		}
	}

	router.metricsRegistry.EventController().AddHandler(metrics.NewChannelReporter(router.ctrl))

	return nil
}
