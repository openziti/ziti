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

package server

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	runner2 "github.com/openziti/ziti/common/runner"
	edgeconfig "github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/handler_edge_ctrl"
	"github.com/openziti/ziti/controller/internal/policy"
	_ "github.com/openziti/ziti/controller/internal/routes"
	"github.com/openziti/ziti/controller/model"
	sync2 "github.com/openziti/ziti/controller/sync_strats"
	"os"
	"sync"
	"time"
)

type Controller struct {
	config          *edgeconfig.EdgeConfig
	AppEnv          *env.AppEnv
	xctrl           *subctrl
	policyEngine    runner2.Runner
	initModulesOnce sync.Once
	initialized     bool
}

const (
	policyMinFreq     = 1 * time.Second
	policyMaxFreq     = 1 * time.Hour
	policyAppWanFreq  = 1 * time.Second
	policySessionFreq = 5 * time.Second
)

func NewController(host env.HostController) (*Controller, error) {
	c := &Controller{
		config: host.GetConfig().Edge,
		AppEnv: host.GetEnv(),
	}

	if !c.Enabled() {
		return c, nil
	}

	c.AppEnv.HostController.GetNetwork().AddCapability("ziti.edge")

	pfxlog.Logger().Infof("edge controller instance id: %s", c.AppEnv.InstanceId)

	pe, err := runner2.NewRunner(policyMinFreq, policyMaxFreq, func(e error, enforcer runner2.Operation) {
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

	c.xctrl = &subctrl{
		parent: c,
	}

	// Add the root host controller's identity's CAs to the ca's served by well-known urls
	if caCerts, err := os.ReadFile(c.AppEnv.HostController.Identity().GetConfig().CA); err == nil {
		c.config.AddCaPems(caCerts)
	} else {
		pfxlog.Logger().Fatalf("could not read controller identity CA file: %s: %v", c.AppEnv.HostController.Identity().GetConfig().CA, err)
	}

	if err := host.RegisterXctrl(c.xctrl); err != nil {
		panic(err)
	}

	return c, nil
}

func (c *Controller) GetCtrlHandlers(binding channel.Binding) []channel.TypedReceiveHandler {
	ch := binding.GetChannel()
	tunnelState := handler_edge_ctrl.NewTunnelState()

	result := []channel.TypedReceiveHandler{
		handler_edge_ctrl.NewSessionHeartbeatHandler(c.AppEnv),
		handler_edge_ctrl.NewCreateCircuitHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewCreateCircuitV2Handler(c.AppEnv, ch),
		handler_edge_ctrl.NewCreateTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewCreateTerminatorV2Handler(c.AppEnv, ch),
		handler_edge_ctrl.NewUpdateTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewRemoveTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewValidateSessionsHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewHealthEventHandler(c.AppEnv, ch),

		handler_edge_ctrl.NewCreateApiSessionHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewCreateCircuitForTunnelHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewCreateTunnelCircuitV2Handler(c.AppEnv, ch),
		handler_edge_ctrl.NewCreateTunnelTerminatorHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewCreateTunnelTerminatorV2Handler(c.AppEnv, ch),
		handler_edge_ctrl.NewUpdateTunnelTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewRemoveTunnelTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewListTunnelServicesHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewTunnelHealthEventHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewExtendEnrollmentHandler(c.AppEnv),
		handler_edge_ctrl.NewExtendEnrollmentVerifyHandler(c.AppEnv),
		handler_edge_ctrl.NewConnectEventsHandler(c.AppEnv),
	}

	result = append(result, c.AppEnv.Broker.GetReceiveHandlers()...)

	return result
}

func (c *Controller) Enabled() bool {
	return c.AppEnv.HostController.GetConfig().Edge.Enabled
}

func (c *Controller) initializeAuthModules() {
	c.initModulesOnce.Do(func() {
		c.AppEnv.AuthRegistry.Add(model.NewAuthModuleUpdb(c.AppEnv))
		c.AppEnv.AuthRegistry.Add(model.NewAuthModuleCert(c.AppEnv))
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
	if !c.Enabled() {
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
		MaxQueuedClientHellos:    1000,
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
	if !c.Enabled() {
		return
	}

	log := pfxlog.Logger()

	if !c.initialized {
		log.Panic("edge not initialized")
	}

	log.Info("starting edge")

	//after InitPersistence
	for _, router := range env.GetRouters() {
		router.Register(c.AppEnv)
	}
	// middleware has to be added after all routes are added due to caching mechanisms in the generated code
	for _, router := range env.GetRouters() {
		if apiRouterMiddleware, ok := router.(env.ApiRouterMiddleware); ok {
			apiRouterMiddleware.AddMiddleware(c.AppEnv)
		}
	}

	go c.checkEdgeInitialized()

	if err := c.policyEngine.Start(c.AppEnv.HostController.GetCloseNotifyChannel()); err != nil {
		log.WithError(err).Fatalf("error starting policy engine")
	}
}

func (c *Controller) checkEdgeInitialized() {
	log := pfxlog.Logger()
	defaultAdminFound := false
	lastWarn := time.Time{}
	first := true

	for !defaultAdminFound {
		admin, err := c.AppEnv.Managers.Identity.ReadDefaultAdmin()

		if err != nil {
			log.WithError(err).Panic("could not check if a default admin exists")
		}

		if admin == nil {
			if !c.AppEnv.GetHostController().IsRaftEnabled() {
				log.Fatal("the Ziti Edge has not been initialized via 'ziti controller edge init', no default admin exists")
			}

			if first {
				time.Sleep(3 * time.Second)
				first = false
			}

			now := time.Now()
			if now.Sub(lastWarn) > time.Minute {
				log.Warnf("the controller has not been initialized, no default admin exists. Add this node to a cluster using "+
					"'ziti agent cluster add %s' against an existing cluster member, or if this is the bootstrap node, run "+
					"'ziti agent cluster init' to configure the default admin and bootstrap the cluster",
					(*c.AppEnv.HostController.GetEnv().GetConfig().Ctrl.Options.AdvertiseAddress).String())
				lastWarn = now
			}
			time.Sleep(time.Second)
		} else {
			defaultAdminFound = true
		}
	}
	log.Info("edge initialized")
}

func (c *Controller) Shutdown() {
	if c.Enabled() {
		log := pfxlog.Logger()

		pfxlog.Logger().Info("edge controller: shutting down...")

		c.AppEnv.Broker.Stop()

		c.AppEnv.GetManagers().ApiSession.HeartbeatCollector.Stop()

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

func (c *subctrl) NotifyOfReconnect(channel.Channel) {
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
