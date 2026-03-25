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

package xgress_edge

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/internal/apiproxy"
	"github.com/openziti/ziti/v2/router/state"
	"github.com/openziti/ziti/v2/router/xgress_router"
	"github.com/pkg/errors"
)

type reconnectionHandler interface {
	NotifyOfReconnect(ch channel.Channel)
}

type Factory struct {
	ctrls                env.NetworkControllers
	enabled              bool
	routerConfig         *env.Config
	edgeRouterConfig     *env.EdgeConfig
	hostedServices       *hostedServiceRegistry
	stateManager         state.Manager
	versionProvider      versions.VersionProvider
	metricsRegistry      metrics.Registry
	env                  env.RouterEnv
	reconnectionHandlers concurrenz.CopyOnWriteSlice[reconnectionHandler]
	connectionTracker    *connectionTracker
}

func (factory *Factory) Inspect(key string, timeout time.Duration) any {
	if key == inspect.RouterIdentityConnectionStatusesKey {
		return factory.connectionTracker.Inspect(key, timeout)
	}
	return nil
}

func (factory *Factory) GetNetworkControllers() env.NetworkControllers {
	return factory.ctrls
}

func (factory *Factory) Enabled() bool {
	return factory.enabled
}

func (factory *Factory) BindChannel(binding channel.Binding) error {
	binding.AddTypedReceiveHandler(&channel.AsyncFunctionReceiveAdapter{
		Type:    int32(edge_ctrl_pb.ContentType_CreateTerminatorV2ResponseType),
		Handler: factory.hostedServices.HandleCreateTerminatorResponse,
	})

	return nil
}

func (factory *Factory) NotifyOfReconnect(ch channel.Channel) {
	pfxlog.Logger().Info("control channel reconnected, re-establishing hosted services")
	factory.hostedServices.HandleReconnect()

	go factory.stateManager.ValidateSessions(ch, factory.edgeRouterConfig.SessionValidateChunkSize, factory.edgeRouterConfig.SessionValidateMinInterval, factory.edgeRouterConfig.SessionValidateMaxInterval)

	for _, handler := range factory.reconnectionHandlers.Value() {
		go handler.NotifyOfReconnect(ch)
	}
}

func (factory *Factory) addReconnectionHandler(h reconnectionHandler) {
	factory.reconnectionHandlers.Append(h)
}

func (factory *Factory) Run(env env.RouterEnv) error {
	factory.stateManager.StartHeartbeat(env, factory.edgeRouterConfig.HeartbeatIntervalSeconds, env.GetCloseNotify())
	return nil
}

func (factory *Factory) LoadConfig(configMap map[interface{}]interface{}) error {
	factory.enabled = factory.routerConfig.Edge.Enabled

	if !factory.enabled {
		return nil
	}

	edgeConfig := factory.routerConfig.Edge
	if edgeConfig.Tcfg == nil {
		edgeConfig.Tcfg = make(transport.Configuration)
	}

	if !stringz.Contains(edgeConfig.Tcfg.Protocols(), "ziti-edge") {
		edgeConfig.Tcfg[transport.KeyProtocol] = append(edgeConfig.Tcfg.Protocols(), "ziti-edge")
	}
	if !stringz.Contains(edgeConfig.Tcfg.Protocols(), "") {
		edgeConfig.Tcfg[transport.KeyProtocol] = append(edgeConfig.Tcfg.Protocols(), "")
	}

	factory.edgeRouterConfig = factory.routerConfig.Edge
	factory.env.MarkRouterDataModelRequired()

	go apiproxy.Start(edgeConfig)

	return nil
}

// NewFactory constructs a new Edge Xgress Factory instance
func NewFactory(routerConfig *env.Config, env env.RouterEnv, stateManager state.Manager) *Factory {
	factory := &Factory{
		ctrls:             env.GetNetworkControllers(),
		hostedServices:    newHostedServicesRegistry(env, stateManager),
		stateManager:      stateManager,
		versionProvider:   env.GetVersionInfo(),
		routerConfig:      routerConfig,
		metricsRegistry:   env.GetMetricsRegistry(),
		env:               env,
		connectionTracker: newConnectionTracker(env, stateManager),
	}

	factory.stateManager.SetConnectionTracker(factory.connectionTracker)

	factory.addReconnectionHandler(factory.connectionTracker)
	return factory
}

// CreateListener creates a new Edge Xgress listener
func (factory *Factory) CreateListener(optionsData xgress.OptionsData) (xgress_router.Listener, error) {
	if !factory.enabled {
		return nil, errors.New("edge listener enabled but required configuration section [edge] is missing")
	}

	options := &Options{}
	if err := options.load(optionsData); err != nil {
		return nil, err
	}

	pfxlog.Logger().Infof("xgress edge listener options: %v", options.ToLoggableString())

	versionInfo := factory.versionProvider.AsVersionInfo()
	versionHeader, err := factory.versionProvider.EncoderDecoder().Encode(versionInfo)

	if err != nil {
		return nil, fmt.Errorf("could not generate version header: %v", err)
	}

	capMask := &big.Int{}
	capMask.SetBit(capMask, edge.RouterCapabilityConnectV2, 1)

	headers := map[int32][]byte{
		channel.HelloVersionHeader:       versionHeader,
		edge.SupportsBindSuccessHeader:   {1},
		edge.SupportsPostureChecksHeader: {1},
		edge.RouterCapabilitiesHeader:    capMask.Bytes(),
	}

	return newListener(factory.env.GetRouterId(), factory, options, headers), nil
}

// CreateDialer creates a new Edge Xgress dialer
func (factory *Factory) CreateDialer(optionsData xgress.OptionsData) (xgress_router.Dialer, error) {
	if !factory.enabled {
		return nil, errors.New("edge listener enabled but required configuration section [edge] is missing")
	}

	options := &Options{}
	if err := options.load(optionsData); err != nil {
		return nil, err
	}

	// CreateDialer is called for every egress route and for inspect and validations
	// can't Log this every time.
	// pfxlog.Logger().Infof("xgress edge dialer options: %v", options.ToLoggableString())

	return newDialer(factory, options), nil
}

type Options struct {
	xgress.Options
	channelOptions          *channel.Options
	lookupApiSessionTimeout time.Duration
	lookupSessionTimeout    time.Duration
}

func (options *Options) ToLoggableString() string {
	buf := strings.Builder{}
	fmt.Fprintf(&buf, "mtu=%v\n", options.Mtu)
	fmt.Fprintf(&buf, "randomDrops=%v\n", options.RandomDrops)
	fmt.Fprintf(&buf, "drop1InN=%v\n", options.Drop1InN)
	fmt.Fprintf(&buf, "txQueueSize=%v\n", options.TxQueueSize)
	fmt.Fprintf(&buf, "txPortalStartSize=%v\n", options.TxPortalStartSize)
	fmt.Fprintf(&buf, "txPortalMaxSize=%v\n", options.TxPortalMaxSize)
	fmt.Fprintf(&buf, "txPortalMinSize=%v\n", options.TxPortalMinSize)
	fmt.Fprintf(&buf, "txPortalIncreaseThresh=%v\n", options.TxPortalIncreaseThresh)
	fmt.Fprintf(&buf, "txPortalIncreaseScale=%v\n", options.TxPortalIncreaseScale)
	fmt.Fprintf(&buf, "txPortalRetxThresh=%v\n", options.TxPortalRetxThresh)
	fmt.Fprintf(&buf, "txPortalRetxScale=%v\n", options.TxPortalRetxScale)
	fmt.Fprintf(&buf, "txPortalDupAckThresh=%v\n", options.TxPortalDupAckThresh)
	fmt.Fprintf(&buf, "txPortalDupAckScale=%v\n", options.TxPortalDupAckScale)
	fmt.Fprintf(&buf, "rxBufferSize=%v\n", options.RxBufferSize)
	fmt.Fprintf(&buf, "retxStartMs=%v\n", options.RetxStartMs)
	fmt.Fprintf(&buf, "retxScale=%v\n", options.RetxScale)
	fmt.Fprintf(&buf, "retxAddMs=%v\n", options.RetxAddMs)
	fmt.Fprintf(&buf, "retxMaxMs=%v\n", options.RetxMaxMs)
	fmt.Fprintf(&buf, "maxRttScale=%v\n", options.MaxRttScale)
	fmt.Fprintf(&buf, "maxCloseWait=%v\n", options.MaxCloseWait)
	fmt.Fprintf(&buf, "getCircuitTimeout=%v\n", options.GetCircuitTimeout)

	fmt.Fprintf(&buf, "lookupApiSessionTimeout=%v\n", options.lookupApiSessionTimeout)
	fmt.Fprintf(&buf, "lookupSessionTimeout=%v\n", options.lookupSessionTimeout)

	fmt.Fprintf(&buf, "channel.outQueueSize=%v\n", options.channelOptions.OutQueueSize)
	fmt.Fprintf(&buf, "channel.connectTimeout=%v\n", options.channelOptions.ConnectTimeout)
	fmt.Fprintf(&buf, "channel.maxOutstandingConnects=%v\n", options.channelOptions.MaxOutstandingConnects)
	fmt.Fprintf(&buf, "channel.maxQueuedConnects=%v\n", options.channelOptions.MaxQueuedConnects)

	return buf.String()
}

func (options *Options) load(data xgress.OptionsData) error {
	o, err := xgress.LoadOptions(data)
	if err != nil {
		return errors.Wrap(err, "error loading options")
	}
	options.Options = *o
	options.lookupSessionTimeout = 5 * time.Second
	options.lookupApiSessionTimeout = 5 * time.Second

	if value, found := data["options"]; found {
		data = value.(map[interface{}]interface{})

		var err error
		options.channelOptions, err = channel.LoadOptions(data)
		if err != nil {
			return err
		}
		if err := options.channelOptions.Validate(); err != nil {
			return fmt.Errorf("error loading options for [edge/options]: %v", err)
		}

		if value, found := data["lookupSessionTimeout"]; found {
			timeout, err := time.ParseDuration(value.(string))
			if err != nil {
				return errors.Wrap(err, "invalid 'lookupSessionTimeout' value")
			}
			options.lookupSessionTimeout = timeout
		}

		if value, found := data["lookupApiSessionTimeout"]; found {
			timeout, err := time.ParseDuration(value.(string))
			if err != nil {
				return errors.Wrap(err, "invalid 'lookupApiSessionTimeout' value")
			}
			options.lookupApiSessionTimeout = timeout
		}
	} else {
		options.channelOptions = channel.DefaultOptions()
	}

	if options.channelOptions.OutQueueSize == channel.DefaultOutQueueSize {
		options.channelOptions.OutQueueSize = 64
	}

	return nil
}
