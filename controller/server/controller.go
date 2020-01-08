/*
	Copyright 2019 Netfoundry, Inc.

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
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/netfoundry/ziti-edge/controller/internal/policy"

	"github.com/gorilla/handlers"
	"github.com/michaelquigley/pfxlog"
	edgeconfig "github.com/netfoundry/ziti-edge/controller/config"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/handler_edge_ctrl"
	_ "github.com/netfoundry/ziti-edge/controller/internal/routes"
	"github.com/netfoundry/ziti-edge/controller/middleware"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/runner"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/common/constants"
	"github.com/netfoundry/ziti-foundation/config"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
)

type Controller struct {
	config          *edgeconfig.Config
	apiServer       *apiServer
	AppEnv          *env.AppEnv
	xmgmt           *submgmt
	xctrl           *subctrl
	policyEngine    runner.Runner
	isLoaded        bool
	initModulesOnce sync.Once
}

const (
	policyMinFreq     = 1 * time.Second
	policyMaxFreq     = 1 * time.Hour
	policyAppWanFreq  = 1 * time.Second
	policySessionFreq = 5 * time.Second
)

func NewController(cfg config.Configurable) (*Controller, error) {
	c := &Controller{}
	if err := cfg.Configure(c); err != nil {
		return nil, fmt.Errorf("failed to load configuration: %s", err)
	}

	if !c.IsEnabled() {
		return c, nil
	}

	ae := env.NewAppEnv(c.config)

	// If SkipClean is false (which is the default), URL cleaning will happen (i.e. double slashes to single slashes)
	// and gorilla will return 301s if cleaning is needed. This causes problems, as redirects are subsequently called
	// with GET (instead of their original HTTP verb) by clients that support redirects.
	// Skipping URL cleaning will result in 404s for poorly constructed URLs unless middleware is introduced to
	// transparently clean the URL. Transparently cleaning URLs can create issues where client logic is written
	// that is never corrected.
	ae.RootRouter.SkipClean(true)

	ae.RootRouter.Use(middleware.UseStatusWriter)
	ae.RootRouter.Use(middleware.RequestDebugLogger)
	ae.RootRouter.Use(middleware.SetResponseTypeToJson)

	corsOpts := []handlers.CORSOption{
		handlers.AllowedOrigins([]string{"*"}),
		handlers.OptionStatusCode(200),
		handlers.AllowedHeaders([]string{
			"Content-Type",
			"Accept",
			constants.ZitiSession,
		}),
		handlers.AllowedMethods([]string{
			http.MethodGet,
			http.MethodHead,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete}),
		handlers.AllowCredentials(),
	}

	as := newApiServer(c.config, ae.RootRouter)

	as.corsOptions = corsOpts
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

	c.apiServer = as
	c.AppEnv = ae
	c.policyEngine = pe

	c.xmgmt = &submgmt{
		parent: c,
	}

	c.xctrl = &subctrl{
		parent: c,
	}

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
	c.AppEnv.HostController.GetNetwork().AddCapability("ziti.edge")
	if err := h.RegisterXctrl(c.xctrl); err != nil {
		panic(err)
	}

	if err := h.RegisterXmgmt(c.xmgmt); err != nil {
		panic(err)
	}
}

func (c *Controller) GetCtrlHandlers() []channel2.ReceiveHandler {
	return []channel2.ReceiveHandler{handler_edge_ctrl.NewSessionHeartbeatHandler(c.AppEnv)}
}

func (c *Controller) GetMgmtHandlers() []channel2.ReceiveHandler {
	return []channel2.ReceiveHandler{}
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
		c.AppEnv.AuthRegistry.Add(model.NewAuthModuleCert(c.AppEnv))

		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleCa(c.AppEnv))
		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleOttCa(c.AppEnv))
		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleOtt(c.AppEnv))
		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleEr(c.AppEnv))
		c.AppEnv.EnrollRegistry.Add(model.NewEnrollModuleUpdb(c.AppEnv))
	})
}

func (c *Controller) Initialize() {
	log := pfxlog.Logger()

	//done before ae.InitPersistence()
	c.initializeAuthModules()

	//should be done after all modules that add migrations have been added (i.e. AuthRegistry)
	if err := c.AppEnv.InitPersistence(); err != nil {
		log.Fatalf("error initializing persistence: %+v", err)
	}

	//after InitPersistence
	c.AppEnv.Broker = env.NewBroker(c.AppEnv)

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
}

func (c *Controller) Run() {
	log := pfxlog.Logger()

	//after InitPersistence
	for _, rf := range env.GetRouters() {
		rf(c.AppEnv)
	}

	admin, err := c.AppEnv.Handlers.Identity.HandleReadDefaultAdmin()

	if err != nil {
		pfxlog.Logger().WithError(err).Panic("could not check if a default admin exists")
	}

	if admin == nil {
		pfxlog.Logger().Fatal("the Ziti Edge has not been initialized via 'ziti-controller edge init', no default admin exists")
	}

	go func() {
		err := c.apiServer.Start()

		if err != nil {
			log.
				WithField("cause", err).
				Fatal("error starting API server", err)
		}
	}()

	go func() {
		err := c.policyEngine.Start()

		if err != nil {
			log.
				WithField("cause", err).
				Fatalf("error starting policy engine")
		}
	}()
}

func (c *Controller) InitAndRun() {
	if !c.config.Enabled {
		return
	}
	log := pfxlog.Logger()
	log.Info("starting controller")

	c.Initialize()
	c.Run()
	c.waitForShutdown()
}

func (c *Controller) waitForShutdown() {
	log := pfxlog.Logger()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	<-ch

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	c.apiServer.Shutdown(ctx)

	log.Info("shutting down")
	os.Exit(0)
}

func (c *Controller) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	c.apiServer.Shutdown(ctx)

	pfxlog.Logger().Info("edge controller shutting down")
}

type subctrl struct {
	parent  *Controller
	channel channel2.Channel
}

func (c *subctrl) LoadConfig(cfgmap map[interface{}]interface{}) error {
	return nil
}

func (c *subctrl) Enabled() bool {
	return c.parent.Enabled()
}

func (c *subctrl) BindChannel(ch channel2.Channel) error {
	for _, h := range c.parent.GetCtrlHandlers() {
		ch.AddReceiveHandler(h)
	}
	c.channel = ch
	return nil
}

func (c *subctrl) Run(ch channel2.Channel, db boltz.Db, done chan struct{}) error {
	return nil
}

type submgmt struct {
	parent  *Controller
	channel channel2.Channel
}

func (m *submgmt) LoadConfig(cfgmap map[interface{}]interface{}) error {
	return nil
}

func (m *submgmt) Enabled() bool {
	return m.parent.Enabled()
}

func (m *submgmt) BindChannel(ch channel2.Channel) error {
	for _, h := range m.parent.GetMgmtHandlers() {
		ch.AddReceiveHandler(h)
	}
	m.channel = ch
	return nil
}

func (m *submgmt) Run(ch channel2.Channel, done chan struct{}) error {
	return nil
}
