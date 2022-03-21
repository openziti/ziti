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

package server

import "C"
import (
	"fmt"
	"github.com/openziti/channel"
	sync2 "github.com/openziti/edge/controller/sync_strats"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/controller/api_impl"
	"io/ioutil"
	"sync"
	"time"

	"github.com/openziti/edge/controller/internal/policy"

	"github.com/michaelquigley/pfxlog"
	edgeconfig "github.com/openziti/edge/controller/config"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/handler_edge_ctrl"
	_ "github.com/openziti/edge/controller/internal/routes"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/runner"
	"github.com/openziti/foundation/config"
	"github.com/openziti/foundation/storage/boltz"
)

type Controller struct {
	config          *edgeconfig.Config
	AppEnv          *env.AppEnv
	xmgmt           *submgmt
	xctrl           *subctrl
	policyEngine    runner.Runner
	isLoaded        bool
	initModulesOnce sync.Once
	initialized     bool
}

const (
	policyMinFreq     = 1 * time.Second
	policyMaxFreq     = 1 * time.Hour
	policyAppWanFreq  = 1 * time.Second
	policySessionFreq = 5 * time.Second

	ZitiInstanceId = "ziti-instance-id"
)

func NewController(cfg config.Configurable, host env.HostController) (*Controller, error) {
	c := &Controller{}

	if err := cfg.Configure(c); err != nil {
		return nil, fmt.Errorf("failed to load configuration: %s", err)
	}

	if !c.IsEnabled() {
		return c, nil
	}

	c.AppEnv = env.NewAppEnv(c.config, host)

	c.AppEnv.TraceManager = env.NewTraceManager(host.GetCloseNotifyChannel())
	c.AppEnv.HostController.GetNetwork().AddCapability("ziti.edge")

	pfxlog.Logger().Infof("edge controller instance id: %s", c.AppEnv.InstanceId)

	pe, err := runner.NewRunner(policyMinFreq, policyMaxFreq, func(e error, enforcer runner.Operation) {
		pfxlog.Logger().
			WithField("cause", e).
			WithField("enforcerName", enforcer.GetName()).
			WithField("enforcerId", enforcer.GetId()).
			Errorf("error running policy enforcer")
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create policy runner: %s", err)
	}

	c.policyEngine = pe

	c.xmgmt = &submgmt{
		parent: c,
	}

	c.xctrl = &subctrl{
		parent: c,
	}

	// Add the root host controller's identity's CAs to the ca's served by well-known urls
	if caCerts, err := ioutil.ReadFile(c.AppEnv.HostController.Identity().GetConfig().CA); err == nil {
		c.config.AddCaPems(caCerts)
	} else {
		pfxlog.Logger().Fatalf("could not read controller identity CA file: %s: %v", c.AppEnv.HostController.Identity().GetConfig().CA, err)
	}

	if err := host.RegisterXctrl(c.xctrl); err != nil {
		panic(err)
	}

	if err := host.RegisterXmgmt(c.xmgmt); err != nil {
		panic(err)
	}

	api_impl.OverrideRequestWrapper(&fabricWrapper{ae: c.AppEnv})

	return c, nil
}

func (c *Controller) IsEnabled() bool {
	return c.config != nil && c.config.Enabled
}

func (c *Controller) SetHostController(h env.HostController) {
	if !c.IsEnabled() {
		return
	}

	c.AppEnv.HostController = h
	c.AppEnv.TraceManager = env.NewTraceManager(h.GetCloseNotifyChannel())
	c.AppEnv.HostController.GetNetwork().AddCapability("ziti.edge")
	if err := h.RegisterXctrl(c.xctrl); err != nil {
		panic(err)
	}

	if err := h.RegisterXmgmt(c.xmgmt); err != nil {
		panic(err)
	}
}

func (c *Controller) GetCtrlHandlers(binding channel.Binding) []channel.TypedReceiveHandler {
	ch := binding.GetChannel()
	tunnelState := handler_edge_ctrl.NewTunnelState()

	result := []channel.TypedReceiveHandler{
		handler_edge_ctrl.NewSessionHeartbeatHandler(c.AppEnv),
		handler_edge_ctrl.NewCreateCircuitHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewCreateTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewUpdateTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewRemoveTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewValidateSessionsHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewHealthEventHandler(c.AppEnv, ch),

		handler_edge_ctrl.NewCreateApiSessionHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewCreateCircuitForTunnelHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewCreateTunnelTerminatorHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewUpdateTunnelTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewRemoveTunnelTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewListTunnelServicesHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewTunnelHealthEventHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewExtendEnrollmentHandler(c.AppEnv),
		handler_edge_ctrl.NewExtendEnrollmentVerifyHandler(c.AppEnv),
	}

	result = append(result, c.AppEnv.Broker.GetReceiveHandlers()...)

	return result
}

func (c *Controller) GetMgmtHandlers() []channel.TypedReceiveHandler {
	return []channel.TypedReceiveHandler{}
}

func (c *Controller) LoadConfig(cfgmap map[interface{}]interface{}) error {
	if c.isLoaded {
		return nil
	}

	parsedConfig, err := edgeconfig.LoadFromMap(cfgmap)
	if err != nil {
		return fmt.Errorf("error loading edge controller configuration: %s", err.Error())
	}

	c.config = parsedConfig

	return nil
}

func (c *Controller) Enabled() bool {
	return c.config.Enabled
}

func (c *Controller) initializeAuthModules() {
	c.initModulesOnce.Do(func() {
		c.AppEnv.AuthRegistry.Add(model.NewAuthModuleUpdb(c.AppEnv))
		c.AppEnv.AuthRegistry.Add(model.NewAuthModuleCert(c.AppEnv, c.AppEnv.GetConfig().CaPems()))
		c.AppEnv.AuthRegistry.Add(model.NewAuthModuleExtJwt(c.AppEnv))

		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleCa(c.AppEnv))
		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleOttCa(c.AppEnv))
		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleOtt(c.AppEnv))
		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleEdgeRouterOtt(c.AppEnv))
		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleTransitRouterOtt(c.AppEnv))
		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleUpdb(c.AppEnv))
	})
}

func (c *Controller) Initialize() {
	if !c.config.Enabled {
		return
	}

	log := pfxlog.Logger()

	log.Info("initializing edge")
	
	//should be done after all modules that add migrations have been added (i.e. AuthRegistry)
	if err := c.AppEnv.InitPersistence(); err != nil {
		log.Fatalf("error initializing persistence: %+v", err)
	}

	c.initializeAuthModules()

	//after InitPersistence
	c.AppEnv.Broker = env.NewBroker(c.AppEnv, sync2.NewInstantStrategy(c.AppEnv, sync2.InstantStrategyOptions{
		MaxQueuedRouterConnects:  100,
		MaxQueuedClientHellos:    100,
		RouterConnectWorkerCount: 10,
		SyncWorkerCount:          10,
		RouterTxBufferSize:       100,
		HelloSendTimeout:         10 * time.Second,
		SessionChunkSize:         100,
	}))

	servicePolicyEnforcer := policy.NewServicePolicyEnforcer(c.AppEnv, policyAppWanFreq)
	if err := c.policyEngine.AddOperation(servicePolicyEnforcer); err != nil {
		log.WithField("cause", err).
			WithField("enforcerName", servicePolicyEnforcer.GetName()).
			WithField("enforcerId", servicePolicyEnforcer.GetId()).
			Fatalf("could not add service policy enforcer")
	}

	sessionEnforcer := policy.NewSessionEnforcer(c.AppEnv, policySessionFreq, c.config.SessionTimeoutDuration())
	if err := c.policyEngine.AddOperation(sessionEnforcer); err != nil {
		log.WithField("cause", err).
			WithField("enforcerName", sessionEnforcer.GetName()).
			WithField("enforcerId", sessionEnforcer.GetId()).
			Errorf("could not add session enforcer")

	}

	if err := c.AppEnv.GetStores().EventualEventer.Start(c.AppEnv.GetHostController().GetCloseNotifyChannel()); err != nil {
		log.WithError(err).Panic("could not start EventualEventer")
	}

	c.initialized = true
}

func (c *Controller) Run() {
	if !c.config.Enabled {
		return
	}
	log := pfxlog.Logger()

	if !c.initialized {
		log.Panic("edge not initialized")
	}

	log.Info("starting edge")

	//after InitPersistence
	for _, rf := range env.GetRouters() {
		rf(c.AppEnv)
	}

	admin, err := c.AppEnv.Handlers.Identity.ReadDefaultAdmin()

	if err != nil {
		pfxlog.Logger().WithError(err).Panic("could not check if a default admin exists")
	}

	if admin == nil {
		pfxlog.Logger().Fatal("the Ziti Edge has not been initialized via 'ziti-controller edge init', no default admin exists")
	}

	managementApiFactory := NewManagementApiFactory(c.AppEnv)
	clientApiFactory := NewClientApiFactory(c.AppEnv)

	if err := c.AppEnv.HostController.RegisterXWebHandlerFactory(managementApiFactory); err != nil {
		pfxlog.Logger().Fatalf("failed to create Edge Management API factory: %v", err)
	}

	if err := c.AppEnv.HostController.RegisterXWebHandlerFactory(clientApiFactory); err != nil {
		pfxlog.Logger().Fatalf("failed to create Edge Client API factory: %v", err)
	}

	if err = c.policyEngine.Start(c.AppEnv.HostController.GetCloseNotifyChannel()); err != nil {
		log.WithError(err).Fatalf("error starting policy engine")
	}
}

func (c *Controller) Shutdown() {
	if c.config.Enabled {
		log := pfxlog.Logger()

		pfxlog.Logger().Info("edge controller: shutting down...")

		c.AppEnv.Broker.Stop()

		c.AppEnv.GetHandlers().ApiSession.HeartbeatCollector.Stop()

		pfxlog.Logger().Info("edge controller: stopped")

		pfxlog.Logger().Info("fabric controller: shutting down...")

		c.AppEnv.GetHostController().Shutdown()

		pfxlog.Logger().Info("fabric controller: stopped")

		log.Info("shutdown complete")
	}
}

type subctrl struct {
	parent *Controller
}

func (c *subctrl) GetTraceDecoders() []channel.TraceMessageDecoder {
	return []channel.TraceMessageDecoder{
		edge_ctrl_pb.Decoder{},
	}
}

func (c *subctrl) NotifyOfReconnect() {
}

func (c *subctrl) LoadConfig(map[interface{}]interface{}) error {
	return nil
}

func (c *subctrl) Enabled() bool {
	return c.parent.Enabled()
}

func (c *subctrl) BindChannel(binding channel.Binding) error {
	for _, h := range c.parent.GetCtrlHandlers(binding) {
		binding.AddTypedReceiveHandler(h)
	}
	return nil
}

func (c *subctrl) Run(channel.Channel, boltz.Db, chan struct{}) error {
	return nil
}

type submgmt struct {
	parent *Controller
}

func (m *submgmt) LoadConfig(map[interface{}]interface{}) error {
	return nil
}

func (m *submgmt) Enabled() bool {
	return m.parent.Enabled()
}

func (m *submgmt) BindChannel(binding channel.Binding) error {
	for _, h := range m.parent.GetMgmtHandlers() {
		binding.AddTypedReceiveHandler(h)
	}
	return nil
}

func (m *submgmt) Run(channel.Channel, chan struct{}) error {
	return nil
}
