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
	"context"
	"fmt"
	"github.com/openziti/edge/controller"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/response"
	sync2 "github.com/openziti/edge/controller/sync_strats"
	"github.com/openziti/edge/controller/timeout"
	"github.com/openziti/edge/edge_common"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/edge/rest_server"
	"github.com/openziti/fabric/controller/xtv"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/openziti/edge/controller/internal/policy"

	"github.com/gorilla/handlers"
	"github.com/michaelquigley/pfxlog"
	edgeconfig "github.com/openziti/edge/controller/config"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/handler_edge_ctrl"
	_ "github.com/openziti/edge/controller/internal/routes"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/runner"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/common/constants"
	"github.com/openziti/foundation/config"
	"github.com/openziti/foundation/storage/boltz"
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
	initialized     bool
}

const (
	policyMinFreq     = 1 * time.Second
	policyMaxFreq     = 1 * time.Hour
	policyAppWanFreq  = 1 * time.Second
	policySessionFreq = 5 * time.Second

	ZitiInstanceId = "ziti-instance-id"
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

	pfxlog.Logger().Infof("edge controller instance id: %s", ae.InstanceId)

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

func (c *Controller) GetCtrlHandlers(ch channel2.Channel) []channel2.ReceiveHandler {
	tunnelState := &handler_edge_ctrl.TunnelState{}
	return []channel2.ReceiveHandler{
		handler_edge_ctrl.NewSessionHeartbeatHandler(c.AppEnv),
		handler_edge_ctrl.NewCreateCircuitHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewCreateTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewUpdateTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewRemoveTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewValidateSessionsHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewCreateApiSessionHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewCreateCircuitForTunnelHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewCreateTunnelTerminatorHandler(c.AppEnv, ch, tunnelState),
		handler_edge_ctrl.NewUpdateTunnelTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewRemoveTunnelTerminatorHandler(c.AppEnv, ch),
		handler_edge_ctrl.NewListTunnelServicesHandler(c.AppEnv, ch, tunnelState),
	}
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
		c.AppEnv.AuthRegistry.Add(model.NewAuthModuleCert(c.AppEnv, c.AppEnv.GetConfig().CaPems()))

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

	//done before ae.InitPersistence()
	c.initializeAuthModules()

	//should be done after all modules that add migrations have been added (i.e. AuthRegistry)
	if err := c.AppEnv.InitPersistence(); err != nil {
		log.Fatalf("error initializing persistence: %+v", err)
	}

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

	xtv.RegisterValidator(edge_common.EdgeBinding, env.NewEdgeTerminatorValidator(c.AppEnv))
	if err := xtv.InitializeMappings(); err != nil {
		log.Fatalf("error initializing xtv: %+v", err)
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

	corsOpts := []handlers.CORSOption{
		handlers.AllowedOrigins([]string{"*"}),
		handlers.OptionStatusCode(200),
		handlers.AllowedHeaders([]string{
			"content-type",
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
	c.AppEnv.Api.Context()
	apiHandler := c.AppEnv.Api.Serve(func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rc := c.AppEnv.CreateRequestContext(rw, r)

			env.AddRequestContextToHttpContext(r, rc)

			err := c.AppEnv.FillRequestContext(rc)
			if err != nil {
				rc.RespondWithError(err)
				return
			}

			//after request context is filled so that api session is present for session expiration headers
			response.AddHeaders(rc)

			handler.ServeHTTP(rw, r)
		})
	})

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set(ZitiInstanceId, c.AppEnv.InstanceId)
		//if not edge prefix, translate to "/edge/v<latest>"
		if !strings.HasPrefix(request.URL.Path, controller.RestApiBase) {
			request.URL.Path = controller.RestApiBaseUrlLatest + request.URL.Path
		}

		if request.URL.Path == controller.RestApiSpecUrl {
			writer.Header().Set("content-type", "application/json")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write(rest_server.SwaggerJSON)
			return
		}
		//let the OpenApi http router take over
		apiHandler.ServeHTTP(writer, request)
	})

	timeoutHandler := timeout.TimeoutHandler(handler, 10*time.Second, apierror.NewTimeoutError())

	as := newApiServer(c.config, timeoutHandler)

	as.corsOptions = corsOpts
	c.apiServer = as

	admin, err := c.AppEnv.Handlers.Identity.ReadDefaultAdmin()

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
		err := c.policyEngine.Start(c.AppEnv.HostController.GetCloseNotifyChannel())

		if err != nil {
			log.
				WithField("cause", err).
				Fatalf("error starting policy engine")
		}
	}()
}

// should be called as a go routine, blocks
func (c *Controller) RunAndWait() {
	c.Run()
	c.waitForShutdown()
}

func (c *Controller) waitForShutdown() {

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	<-ch
	c.Shutdown()
}

func (c *Controller) Shutdown() {
	log := pfxlog.Logger()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	pfxlog.Logger().Info("edge controller: shutting down...")
	c.apiServer.Shutdown(ctx)

	c.AppEnv.Broker.Stop()

	c.AppEnv.GetHandlers().ApiSession.HeartbeatCollector.Stop()

	pfxlog.Logger().Info("edge controller: stopped")

	pfxlog.Logger().Info("fabric controller: shutting down...")

	c.AppEnv.GetHostController().Shutdown()

	pfxlog.Logger().Info("fabric controller: stopped")

	log.Info("shutdown complete")

	//if we reach here and the controller is still running, something hasn't been stopped
}

type subctrl struct {
	parent  *Controller
	channel channel2.Channel
}

func (c *subctrl) GetTraceDecoders() []channel2.TraceMessageDecoder {
	return []channel2.TraceMessageDecoder{
		&edge_ctrl_pb.Decoder{},
	}
}

func (c *subctrl) NotifyOfReconnect() {
}

func (c *subctrl) LoadConfig(cfgmap map[interface{}]interface{}) error {
	return nil
}

func (c *subctrl) Enabled() bool {
	return c.parent.Enabled()
}

func (c *subctrl) BindChannel(ch channel2.Channel) error {
	for _, h := range c.parent.GetCtrlHandlers(ch) {
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
