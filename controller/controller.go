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

package controller

import (
	"bytes"
	"compress/gzip"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/tls"
	"github.com/openziti/xweb/v2"
	"github.com/openziti/ziti/common/capabilities"
	"github.com/openziti/ziti/common/concurrency"
	fabricMetrics "github.com/openziti/ziti/common/metrics"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/common/profiler"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/events"
	"github.com/openziti/ziti/controller/handler_ctrl"
	"github.com/openziti/ziti/controller/handler_peer_ctrl"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/controller/raft"
	"github.com/openziti/ziti/controller/raft/mesh"
	"github.com/openziti/ziti/controller/webapis"
	"github.com/openziti/ziti/controller/xctrl"
	"github.com/openziti/ziti/controller/xmgmt"
	"github.com/openziti/ziti/controller/xt"
	"github.com/openziti/ziti/controller/xt_random"
	"github.com/openziti/ziti/controller/xt_smartrouting"
	"github.com/openziti/ziti/controller/xt_sticky"
	"github.com/openziti/ziti/controller/xt_weighted"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/teris-io/shortid"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
)

type Controller struct {
	config             *config.Config
	env                *env.AppEnv
	network            *network.Network
	raftController     *raft.Controller
	localDispatcher    *command.LocalDispatcher
	ctrlConnectHandler *handler_ctrl.ConnectHandler
	xctrls             []xctrl.Xctrl
	xmgmts             concurrenz.CopyOnWriteSlice[xmgmt.Xmgmt]

	xwebFactoryRegistry xweb.Registry
	xweb                xweb.Instance

	ctrlListener channel.UnderlayListener
	mgmtListener channel.UnderlayListener

	shutdownC         chan struct{}
	isShutdown        atomic.Bool
	agentBindHandlers []channel.BindHandler
	metricsRegistry   metrics.Registry
	versionProvider   versions.VersionProvider
	eventDispatcher   *events.Dispatcher

	apiData      map[string][]event.ApiAddress
	apiDataBytes []byte
	apiDataOnce  sync.Once

	xwebInitialized concurrency.InitState
}

func (c *Controller) GetPeerSigners() []*x509.Certificate {
	if c.raftController == nil || c.raftController.Mesh == nil {
		return nil
	}

	var certs []*x509.Certificate

	for _, peer := range c.raftController.Mesh.GetPeers() {
		certs = append(certs, peer.SigningCerts...)
	}

	return certs
}

func (c *Controller) GetPeerAddresses() []string {
	if c.raftController == nil || c.raftController.Mesh == nil {
		return nil
	}

	var addresses []string

	for _, peer := range c.raftController.Mesh.GetPeers() {
		addresses = append(addresses, peer.Address)
	}

	return addresses

}

func (c *Controller) GetId() *identity.TokenId {
	return c.config.Id
}

func (c *Controller) GetConfig() *config.Config {
	return c.config
}

func (c *Controller) GetMetricsRegistry() metrics.Registry {
	return c.metricsRegistry
}

func (c *Controller) GetOptions() *config.NetworkConfig {
	return c.config.Network
}

func (c *Controller) GetCommandDispatcher() command.Dispatcher {
	if c.raftController == nil {
		if c.localDispatcher != nil {
			return c.localDispatcher
		}
		devVersion := versions.MustParseSemVer("0.0.0")
		version := versions.MustParseSemVer(c.GetVersionProvider().Version())
		c.localDispatcher = &command.LocalDispatcher{
			EncodeDecodeCommands: devVersion.Equals(version),
			Limiter:              command.NewRateLimiter(c.config.CommandRateLimiter, c.metricsRegistry, c.shutdownC),
		}
		return c.localDispatcher
	}
	return c.raftController
}

func (c *Controller) IsRaftEnabled() bool {
	return c.raftController != nil
}

func (c *Controller) IsRaftLeader() bool {
	if c.raftController == nil {
		return false
	}
	return c.raftController.IsLeader()
}

func (c *Controller) GetRaftIndex() uint64 {
	return c.raftController.Raft.LastIndex()
}

func (c *Controller) GetRaftInfo() (string, string, string) {
	id := c.config.Id.Token
	addr := c.raftController.Mesh.Addr().String()

	version := c.GetVersionProvider().Version()

	return string(addr), string(id), version
}

func (c *Controller) GetDb() boltz.Db {
	return c.config.Db
}

func (c *Controller) GetVersionProvider() versions.VersionProvider {
	return c.versionProvider
}

func (c *Controller) GetCloseNotify() <-chan struct{} {
	return c.shutdownC
}

func (c *Controller) GetRaftConfig() *config.RaftConfig {
	return c.config.Raft
}

func (c *Controller) GetCommandRateLimiterConfig() command.RateLimiterConfig {
	return c.config.CommandRateLimiter
}

func (c *Controller) RenderJsonConfig() (string, error) {
	return c.config.ToJson()
}

func (c *Controller) GetEnv() *env.AppEnv {
	return c.env
}

func NewController(cfg *config.Config, versionProvider versions.VersionProvider) (*Controller, error) {
	metricRegistry := metrics.NewRegistry(cfg.Id.Token, nil)

	shutdownC := make(chan struct{})

	tlsHandshakeRateLimiter := command.NewAdaptiveRateLimitTracker(cfg.TlsHandshakeRateLimiter, metricRegistry, shutdownC)
	tls.SetSharedListenerRateLimiter(tlsHandshakeRateLimiter)

	log := pfxlog.Logger()

	c := &Controller{
		config:              cfg,
		shutdownC:           shutdownC,
		xwebFactoryRegistry: xweb.NewRegistryMap(),
		metricsRegistry:     metricRegistry,
		versionProvider:     versionProvider,
		eventDispatcher:     events.NewDispatcher(shutdownC),
		xwebInitialized:     concurrency.NewInitState(),
	}

	c.xweb = xweb.NewDefaultInstance(c.xwebFactoryRegistry, c.config.Id)

	if cfg.IsRaftEnabled() {
		c.raftController = raft.NewController(c, c)
		if err := c.raftController.Init(); err != nil {
			log.WithError(err).Panic("error starting raft")
		}

		cfg.Db = c.raftController.GetDb()
	}

	c.registerXts()

	appEnv, err := env.NewAppEnv(c)
	if err != nil {
		return nil, err
	}

	c.env = appEnv

	if n, err := network.NewNetwork(c, appEnv); err == nil {
		c.network = n
	} else {
		return nil, err
	}

	if c.raftController != nil {
		if err = c.raftController.InitEnv(c.env); err != nil {
			return nil, err
		}
	}

	c.initWeb() // need to init web before bootstrapping, so we can provide our endpoints to peers

	if c.raftController != nil && !c.raftController.IsBootstrapped() {
		if err = c.TryInitializeRaftFromBoltDb(); err != nil {
			log.WithError(err).Panic("error bootstrapping raft")
		}
	}

	c.eventDispatcher.InitializeNetworkEvents(c.network)

	if cfg.Ctrl.Options.NewListener != nil {
		c.network.AddRouterPresenceHandler(&OnConnectSettingsHandler{
			config: cfg,
			settings: map[int32][]byte{
				int32(ctrl_pb.SettingTypes_NewCtrlAddress): []byte((*cfg.Ctrl.Options.NewListener).String()),
			},
		})
	}

	if c.raftController != nil {
		logrus.Info("Adding router presence handler to send out ctrl addresses")
		c.network.AddRouterPresenceHandler(
			NewOnConnectCtrlAddressesUpdateHandler(c.config.Ctrl.Listener.String(), c.raftController),
		)
	}

	if err := c.showOptions(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Controller) initWeb() {
	healthChecker, err := c.initializeHealthChecks()
	if err != nil {
		logrus.WithError(err).Fatalf("failed to create health checker")
	}

	if err = c.xweb.GetRegistry().Add(webapis.NewControllerHealthCheckApiFactory(c.env, healthChecker)); err != nil {
		logrus.WithError(err).Fatalf("failed to create health checks api factory")
	}

	fabricManagementFactory := webapis.NewFabricManagementApiFactory(c.config.Id, c.env, c.network, &c.xmgmts)
	if err = c.xweb.GetRegistry().Add(fabricManagementFactory); err != nil {
		logrus.WithError(err).Fatalf("failed to create management api factory")
	}

	if err = c.xweb.GetRegistry().Add(webapis.NewMetricsApiFactory(c.config.Id, c.network)); err != nil {
		logrus.WithError(err).Fatalf("failed to create metrics api factory")
	}

	if err = c.xweb.GetRegistry().Add(webapis.NewZitiAdminConsoleFactory()); err != nil {
		logrus.WithError(err).Fatalf("failed to create single page application factory")
	}

	if c.IsEdgeEnabled() {
		managementApiFactory := webapis.NewManagementApiFactory(c.env)
		clientApiFactory := webapis.NewClientApiFactory(c.env)
		oidcApiFactory := webapis.NewOidcApiFactory(c.env)

		if err = c.xweb.GetRegistry().Add(managementApiFactory); err != nil {
			pfxlog.Logger().Fatalf("failed to create Edge Management API factory: %v", err)
		}

		if err = c.xweb.GetRegistry().Add(clientApiFactory); err != nil {
			pfxlog.Logger().Fatalf("failed to create Edge Client API factory: %v", err)
		}

		if err = c.xweb.GetRegistry().Add(oidcApiFactory); err != nil {
			pfxlog.Logger().Fatalf("failed to create OIDC API factory: %v", err)
		}

		webapis.OverrideRequestWrapper(webapis.NewFabricApiWrapper(c.env))
	} else {
		// if no edge  we need 1 default API, make the fabric api the default
		fabricManagementFactory.MakeDefault = true
	}
	c.xwebInitialized.MarkInitialized()
}

func (c *Controller) IsEdgeEnabled() bool {
	return c.config.Edge.Enabled
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

	capabilityMask := &big.Int{}
	capabilityMask.SetBit(capabilityMask, capabilities.ControllerCreateTerminatorV2, 1)
	capabilityMask.SetBit(capabilityMask, capabilities.ControllerSingleRouterLinkSource, 1)
	capabilityMask.SetBit(capabilityMask, capabilities.ControllerCreateCircuitV2, 1)

	if c.config.RouterDataModel.Enabled || c.raftController != nil {
		capabilityMask.SetBit(capabilityMask, capabilities.RouterDataModel, 1)
	}

	headers := map[int32][]byte{
		channel.HelloVersionHeader:                       versionHeader,
		int32(ctrl_pb.ControlHeaders_CapabilitiesHeader): capabilityMask.Bytes(),
	}

	/**
	 * ctrl listener/accepter.
	 */
	ctrlChannelListenerConfig := channel.ListenerConfig{
		ConnectOptions:   c.config.Ctrl.Options.ConnectOptions,
		PoolConfigurator: fabricMetrics.GoroutinesPoolMetricsConfigF(c.network.GetMetricsRegistry(), "pool.listener.ctrl"),
		Headers:          headers,
		TransportConfig:  transport.Configuration{"protocol": "ziti-ctrl"},
	}

	if c.raftController != nil {
		ctrlChannelListenerConfig.HeadersF = c.raftController.GetListenerHeaders
	}

	ctrlListener := channel.NewClassicListener(c.config.Id, c.config.Ctrl.Listener, ctrlChannelListenerConfig)
	c.ctrlListener = ctrlListener
	if err := c.ctrlListener.Listen(c.ctrlConnectHandler); err != nil {
		panic(err)
	}

	ctrlAccepter := handler_ctrl.NewCtrlAccepter(c.network, c.xctrls, c.config.Ctrl.Options.Options, c.config.Ctrl.Options.RouterHeartbeatOptions, c.config.Trace.Handler)

	ctrlAcceptors := map[string]channel.UnderlayAcceptor{}
	if c.raftController != nil {
		c.raftController.ConfigureMeshHandlers(handler_peer_ctrl.NewBindHandler(c.network, c.raftController, c.config.Ctrl.Options.PeerHeartbeatOptions))
		ctrlAcceptors[mesh.ChannelTypeMesh] = c.raftController.GetMesh()
	}

	underlayDispatcher := channel.NewUnderlayDispatcher(channel.UnderlayDispatcherConfig{
		Listener:        ctrlListener,
		ConnectTimeout:  c.config.Ctrl.Options.ConnectTimeout,
		Acceptors:       ctrlAcceptors,
		DefaultAcceptor: ctrlAccepter,
	})

	go underlayDispatcher.Run()

	if err = c.config.Configure(c.xweb); err != nil {
		panic(err)
	}

	c.xweb.Run()

	for _, helloHeader := range c.GetHelloHeaderProviders() {
		helloHeader.Apply(headers)
	}

	// event handlers
	if err := c.eventDispatcher.WireEventHandlers(c.getEventHandlerConfigs()); err != nil {
		panic(err)
	}

	if c.raftController != nil {
		c.raftController.StartEventGeneration()
	}

	c.network.Run()

	return nil
}

func (c *Controller) getEventHandlerConfigs() []*events.EventHandlerConfig {
	var result []*events.EventHandlerConfig

	if e, ok := c.config.Src["events"]; ok {
		if em, ok := e.(map[interface{}]interface{}); ok {
			for id, v := range em {
				if config, ok := v.(map[interface{}]interface{}); ok {
					result = append(result, &events.EventHandlerConfig{
						Id:     id,
						Config: config,
					})
				}
			}
		}
	}
	return result
}

func (c *Controller) GetCloseNotifyChannel() <-chan struct{} {
	return c.shutdownC
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

		if c.config.Db != nil {
			if err := c.config.Db.Close(); err != nil {
				pfxlog.Logger().WithError(err).Error("failed to close db")
			}
		}

		go c.xweb.Shutdown()
	}
}

func (c *Controller) showOptions() error {
	if ctrl, err := json.MarshalIndent(c.config.Ctrl.Options, "", "  "); err == nil {
		pfxlog.Logger().Infof("ctrl = %s", string(ctrl))
	} else {
		return err
	}
	return nil
}

func (c *Controller) startProfiling() {
	if c.config.Profile.Memory.Path != "" {
		go profiler.NewMemoryWithShutdown(c.config.Profile.Memory.Path, c.config.Profile.Memory.Interval, c.shutdownC).Run()
	}
	if c.config.Profile.CPU.Path != "" {
		if cpu, err := profiler.NewCPUWithShutdown(c.config.Profile.CPU.Path, c.shutdownC); err == nil {
			go cpu.Run()
		} else {
			logrus.Errorf("unexpected error launching cpu profiling (%v)", err)
		}
	}
}

func (c *Controller) registerXts() {
	xt.GlobalRegistry().RegisterFactory(xt_smartrouting.NewFactory())
	xt.GlobalRegistry().RegisterFactory(xt_random.NewFactory())
	xt.GlobalRegistry().RegisterFactory(xt_weighted.NewFactory())
	xt.GlobalRegistry().RegisterFactory(xt_sticky.NewFactory())
}

func (c *Controller) registerComponents() error {
	c.ctrlConnectHandler = handler_ctrl.NewConnectHandler(c.config.Id, c.network)
	c.eventDispatcher.AddClusterEventHandler(event.ClusterEventHandlerF(c.routerDispatchCallback))
	return nil
}

func (c *Controller) RegisterXctrl(x xctrl.Xctrl) error {
	if err := c.config.Configure(x); err != nil {
		return err
	}
	if x.Enabled() {
		c.xctrls = append(c.xctrls, x)
		if c.config.Trace.Handler != nil {
			for _, decoder := range x.GetTraceDecoders() {
				c.config.Trace.Handler.AddDecoder(decoder)
			}
		}
	}
	return nil
}

func (c *Controller) RegisterXmgmt(x xmgmt.Xmgmt) error {
	if err := c.config.Configure(x); err != nil {
		return err
	}
	pfxlog.Logger().Infof("adding xmgmt %T, enabled? %v", x, x.Enabled())
	if x.Enabled() {
		c.xmgmts.Append(x)
	}
	return nil
}

func (c *Controller) GetXWebInstance() xweb.Instance {
	return c.xweb
}

func (c *Controller) GetNetwork() *network.Network {
	return c.network
}

func (c *Controller) Identity() identity.Identity {
	return c.config.Id
}

func (c *Controller) GetEventDispatcher() event.Dispatcher {
	return c.eventDispatcher
}

func (c *Controller) routerDispatchCallback(evt *event.ClusterEvent) {
	if evt.EventType == event.ClusterLeadershipGained {
		req := &ctrl_pb.UpdateClusterLeader{
			Index: evt.Index,
		}

		for _, r := range c.network.AllConnectedRouters() {
			log := pfxlog.Logger().WithFields(map[string]interface{}{
				"index": evt.Index,
			})

			if err := protobufs.MarshalTyped(req).Send(r.Control); err != nil {

				pfxlog.Logger().WithError(err).WithField("routerId", r.Id).Error("unable to update cluster leader on router")
			} else {
				log.WithField("routerId", r.Id).WithField("routerName", r.Name).Info("router updated with info on new leader")
			}
		}
	}

	if evt.EventType == event.ClusterMembersChanged {
		var endpoints []string
		for _, peer := range evt.Peers {
			endpoints = append(endpoints, peer.Addr)
		}

		updMsg := &ctrl_pb.UpdateCtrlAddresses{
			Addresses: endpoints,
			IsLeader:  c.raftController.IsLeader(),
			Index:     evt.Index,
		}

		log := pfxlog.Logger().WithFields(map[string]interface{}{
			"addresses": endpoints,
			"index":     evt.Index,
		})

		log.Info("syncing updated ctrl addresses to connected routers")

		for _, r := range c.network.AllConnectedRouters() {
			if err := protobufs.MarshalTyped(updMsg).Send(r.Control); err != nil {
				pfxlog.Logger().WithError(err).WithField("routerId", r.Id).Error("unable to update controller endpoints on router")
			} else {
				log.WithField("routerId", r.Id).WithField("routerName", r.Name).Info("router updated with latest ctrl addresses")
			}
		}
	}
}

func (c *Controller) getMigrationDb() (*string, error) {
	val, found := c.config.Src["db"]
	if !found {
		return nil, nil
	}

	path := fmt.Sprintf("%v", val)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "source db not found at [%v], either remove 'db' config setting or fix path ", path)
		}
		return nil, errors.Wrapf(err, "invalid db path [%v]", path)
	}

	return &path, nil
}

func (c *Controller) ValidateMigrationEnvironment() error {
	_, err := c.getMigrationDb()
	return err
}

func (c *Controller) TryInitializeRaftFromBoltDb() error {
	path, err := c.getMigrationDb()
	if err != nil || path == nil {
		return err
	}
	return c.InitializeRaftFromBoltDb(*path)
}

func (c *Controller) InitializeRaftFromBoltDb(sourceDbPath string) error {
	log := pfxlog.Logger()

	if c.raftController == nil {
		return errors.New("can't initialize non-raft controller using initialize from db")
	}

	if c.raftController.IsBootstrapped() {
		return errors.New("raft is already bootstrapped, must start with a uninitialized controller")
	}

	if _, err := os.Stat(sourceDbPath); err != nil {
		if os.IsNotExist(err) {
			return errors.Wrapf(err, "source db not found at [%v]", sourceDbPath)
		}
		return errors.Wrapf(err, "invalid db path [%v]", sourceDbPath)
	}

	sourceDb, err := db.Open(sourceDbPath)
	if err != nil {
		return err
	}
	defer func() {
		if err = sourceDb.Close(); err != nil {
			log.WithError(err).Error("error closing migration source bolt db")
		}
	}()

	timelineId, err := sourceDb.GetTimelineId(boltz.TimelineModeForceReset, shortid.Generate)
	if err != nil {
		return err
	}
	log.WithField("timelineId", timelineId).WithField("path", sourceDbPath).Info("initializing from bolt db")

	buf := &bytes.Buffer{}
	gzWriter := gzip.NewWriter(buf)
	if err = sourceDb.StreamToWriter(gzWriter); err != nil {
		return err
	}

	if err = gzWriter.Close(); err != nil {
		return errors.Wrap(err, "error finishing gz compression of migration snapshot")
	}

	c.env.InitTimelineId(timelineId)

	cmd := &command.SyncSnapshotCommand{
		TimelineId:   timelineId,
		Snapshot:     buf.Bytes(),
		SnapshotSink: c.network.RestoreSnapshot,
	}

	if err = c.raftController.Bootstrap(); err != nil {
		return fmt.Errorf("unable to bootstrap cluster (%w)", err)
	}

	return c.raftController.Dispatch(cmd)
}

func (c *Controller) RaftRestoreFromBoltDb(sourceDbPath string) error {
	log := pfxlog.Logger()

	if c.raftController == nil {
		return errors.New("can't initialize non-raft controller using initialize from db")
	}

	if _, err := os.Stat(sourceDbPath); err != nil {
		if os.IsNotExist(err) {
			return errors.Wrapf(err, "source db not found at [%v]", sourceDbPath)
		}
		return errors.Wrapf(err, "invalid db path [%v]", sourceDbPath)
	}

	sourceDb, err := db.Open(sourceDbPath)
	if err != nil {
		return err
	}
	defer func() {
		if err = sourceDb.Close(); err != nil {
			log.WithError(err).Error("error closing migration source bolt db")
		}
	}()

	timelineId, err := sourceDb.GetTimelineId(boltz.TimelineModeForceReset, shortid.Generate)
	if err != nil {
		return err
	}
	log.WithField("timelineId", timelineId).WithField("path", sourceDbPath).Info("restoring from bolt db")

	buf := &bytes.Buffer{}
	gzWriter := gzip.NewWriter(buf)
	if err = sourceDb.StreamToWriter(gzWriter); err != nil {
		return err
	}

	if err = gzWriter.Close(); err != nil {
		return errors.Wrap(err, "error finishing gz compression of migration snapshot")
	}

	cmd := &command.SyncSnapshotCommand{
		TimelineId:   timelineId,
		Snapshot:     buf.Bytes(),
		SnapshotSink: c.network.RestoreSnapshot,
	}

	if err = c.raftController.Bootstrap(); err != nil {
		return fmt.Errorf("unable to bootstrap cluster (%w)", err)
	}

	return c.raftController.Dispatch(cmd)
}

// TODO: this functions is a temporary hack and should be provided by xweb
func getApiPath(binding string) string {
	switch binding {
	case "edge-client":
		return "/edge/client/v1"
	case "edge-management":
		return "/edge/management/v1"
	case "fabric":
		return "/fabric/v1"
	case "health-checks":
		return "/health-checks"
	case "edge-oidc":
		return "/oidc"
	}

	return ""
}

func (c *Controller) GetApiAddresses() (map[string][]event.ApiAddress, []byte) {
	c.apiDataOnce.Do(func() {
		c.xwebInitialized.WaitTillInitialized()
		xwebConfig := c.xweb.GetConfig()

		apiData := map[string][]event.ApiAddress{}
		for _, serverConfig := range xwebConfig.ServerConfigs {
			for _, bindPoint := range serverConfig.BindPoints {
				for _, api := range serverConfig.APIs {
					apiData[api.Binding()] = append(apiData[api.Binding()], event.ApiAddress{
						Url:     "https://" + bindPoint.Address + getApiPath(api.Binding()), //TODO: temp till xweb support reporting API paths
						Version: "v1",                                                       //TODO: temp till xweb supports reporting versions via api.Version()
					})
				}
			}
		}

		c.apiData = apiData
		c.apiDataBytes, _ = json.Marshal(apiData)
	})

	return c.apiData, c.apiDataBytes
}

func (c *Controller) GetHelloHeaderProviders() []mesh.HeaderProvider {
	providerFunc := func(headers map[int32][]byte) {
		_, apiDataBytes := c.GetApiAddresses()
		headers[mesh.ApiAddressesHeader] = apiDataBytes
	}

	provider := mesh.HeaderProviderFunc(providerFunc)
	return []mesh.HeaderProvider{provider}
}
