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

package xgress_edge_tunnel

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/state"
	"github.com/openziti/ziti/v2/router/xgress_router"
	"github.com/pkg/errors"
)

const (
	DefaultMode              = "tproxy"
	DefaultServicePollRate   = 15 * time.Second
	DefaultDnsResolver       = "udp://127.0.0.1:53"
	DefaultDnsServiceIpRange = "100.64.0.1/10"
	DefaultDnsUnanswerable   = "refused"
)

var fabricProviderF concurrenz.AtomicValue[func(env.RouterEnv, *HostedServiceRegistry) TunnelFabricProvider]

// OverrideFabricProviderF allows overriding the default TunnelFabricProvider factory function
// used by the edge tunnel v2 xgress factory. This is primarily useful for wrapping the fabric
// provider to intercept hosting/tunnel calls for router embedders wishing to customize tunnel
// behavior.
//
// The provided function will be called during CreateListener to create a TunnelFabricProvider
// instance. If no override is set, the factory will use NewTunnelFabricProvider as the default.
//
// Parameters:
//   - f: A factory function that takes a RouterEnv and HostedServiceRegistry and returns
//     a TunnelFabricProvider instance. Pass nil to clear any existing override.
//
// This function is thread-safe and can be called concurrently with CreateListener.
func OverrideFabricProviderF(f func(env.RouterEnv, *HostedServiceRegistry) TunnelFabricProvider) {
	fabricProviderF.Store(f)
}

type Factory struct {
	id                *identity.TokenId
	ctrls             env.NetworkControllers
	stateManager      state.Manager
	tunneler          *tunneler
	metricsRegistry   metrics.UsageRegistry
	env               env.RouterEnv
	hostedServices    *HostedServiceRegistry
	dialerInitialized atomic.Bool
}

func (self *Factory) NotifyOfReconnect(channel.Channel) {
}

func (self *Factory) Enabled() bool {
	return true
}

func (self *Factory) BindChannel(binding channel.Binding) error {
	binding.AddReceiveHandlerF(int32(edge_ctrl_pb.ContentType_CreateTunnelTerminatorResponseV2Type), self.hostedServices.HandleCreateTerminatorResponse)
	return nil
}

func (self *Factory) Run(env env.RouterEnv) error {
	self.ctrls = env.GetNetworkControllers()
	if self.tunneler.listenOptions != nil {
		return self.tunneler.Start()
	} else {
		self.tunneler.initialized.Store(true)
	}
	return nil
}

func (self *Factory) LoadConfig(map[interface{}]interface{}) error {
	return nil
}

func (self *Factory) DefaultRequestTimeout() time.Duration {
	return self.env.DefaultRequestTimeout()
}

// NewFactory constructs a new Edge Xgress Tunnel Factory instance
func NewFactory(env env.RouterEnv, stateManager state.Manager) *Factory {
	factory := &Factory{
		id:              env.GetRouterId(),
		stateManager:    stateManager,
		metricsRegistry: env.GetMetricsRegistry(),
		env:             env,
	}
	factory.tunneler = newTunneler(factory)
	factory.hostedServices = newHostedServicesRegistry(env, stateManager)
	factory.tunneler.hostedServices = factory.hostedServices
	return factory
}

// CreateListener creates a new Edge Tunnel Xgress listener
func (self *Factory) CreateListener(optionsData xgress.OptionsData) (xgress_router.Listener, error) {
	self.env.MarkRouterDataModelRequired()
	self.hostedServices.Start()

	if fabricProviderFunc := fabricProviderF.Load(); fabricProviderFunc != nil {
		self.tunneler.fabricProvider = fabricProviderFunc(self.env, self.hostedServices)
	} else {
		self.tunneler.fabricProvider = NewTunnelFabricProvider(self.env, self.hostedServices)
	}

	options := &Options{}
	if err := options.load(optionsData); err != nil {
		return nil, err
	}

	self.tunneler.listenOptions = options
	self.tunneler.fabricProvider.SetXgressOptions(options.Options)

	pfxlog.Logger().Debugf("xgress edge tunnel options: %v", options.ToLoggableString())

	return self.tunneler, nil
}

// CreateDialer creates a new Edge Xgress dialer
func (self *Factory) CreateDialer(optionsData xgress.OptionsData) (xgress_router.Dialer, error) {
	if self.dialerInitialized.CompareAndSwap(false, true) {
		self.env.MarkRouterDataModelRequired()
		options := &Options{}
		if err := options.load(optionsData); err != nil {
			return nil, err
		}

		self.tunneler.dialOptions = options
	}

	return self.tunneler, nil
}

type Options struct {
	*xgress.Options
	mode             string
	svcPollRate      time.Duration
	resolver         string
	dnsSvcIpRange    string
	dnsUpstreams     []string
	dnsUnanswerable  string
	lanIf            string
	services         []string
	udpIdleTimeout   time.Duration
	udpCheckInterval time.Duration
}

func (options *Options) load(data xgress.OptionsData) error {
	options.mode = DefaultMode
	options.svcPollRate = DefaultServicePollRate
	options.resolver = DefaultDnsResolver
	options.dnsSvcIpRange = DefaultDnsServiceIpRange
	options.dnsUnanswerable = DefaultDnsUnanswerable

	var err error
	options.Options, err = xgress.LoadOptions(data)
	if err != nil {
		return err
	}

	if value, found := data["options"]; found {
		data = value.(map[interface{}]interface{})

		if value, found := data["svcPollRate"]; found {
			if strVal, ok := value.(string); ok {
				dur, err := time.ParseDuration(strVal)
				if err != nil {
					return errors.Wrapf(err, "invalid value '%v' for svcPollRate, must be string duration (ex: 1m or 30s)", value)
				}
				options.svcPollRate = dur
			} else {
				return errors.Errorf("invalid value '%v' for svcPollRate, must be string duration (ex: 1m or 30s)", value)
			}
		}

		if value, found := data["resolver"]; found {
			if strVal, ok := value.(string); ok {
				options.resolver = strVal
			} else {
				return errors.Errorf("invalid value '%v' for resolver, must be string value", value)
			}
		}

		if value, found := data["dnsSvcIpRange"]; found {
			if strVal, ok := value.(string); ok {
				options.dnsSvcIpRange = strVal
			} else {
				return errors.Errorf("invalid value '%v' for dnsSvcIpRange, must be string value", value)
			}
		}

		if value, found := data["dnsUpstream"]; found {
			switch v := value.(type) {
			case string:
				if v != "" {
					options.dnsUpstreams = []string{v}
				}
			case []interface{}:
				for _, item := range v {
					strVal, ok := item.(string)
					if !ok {
						return errors.Errorf("invalid value '%v' in dnsUpstream list, must be a string", item)
					}
					if strVal != "" {
						options.dnsUpstreams = append(options.dnsUpstreams, strVal)
					}
				}
			default:
				return errors.Errorf("invalid value '%v' for dnsUpstream, must be string or list of strings", value)
			}
		}

		if value, found := data["dnsUnanswerable"]; found {
			if strVal, ok := value.(string); ok {
				options.dnsUnanswerable = strVal
			} else {
				return errors.Errorf("invalid value '%v' for dnsUnanswerable, must be string value", value)
			}
		}

		if value, found := data["mode"]; found {
			if strVal, ok := value.(string); ok && stringz.Contains([]string{"tproxy", "host", "proxy"}, strVal) ||
				strings.HasPrefix(strVal, "tproxy:") {
				options.mode = strVal
			} else {
				return errors.Errorf(`invalid value '%v' for mode, must be one of ["tproxy", "host", "proxy"']`, value)
			}
		}

		if value, found := data["services"]; found {
			if slice, ok := value.([]interface{}); ok {
				for _, value := range slice {
					if strVal, ok := value.(string); ok {
						options.services = append(options.services, strVal)
					} else {
						return errors.Errorf(`invalid value '%v' for services, must be list of strings`, value)
					}
				}
			} else {
				return errors.New(`invalid value for services, must be list of strings']`)
			}
		}

		if value, found := data["lanIf"]; found {
			if strVal, ok := value.(string); ok {
				options.lanIf = strVal
			} else {
				return errors.Errorf(`invalid value '%v' for lanIf, must be a string value`, value)
			}
		}

		if value, found := data["udpIdleTimeout"]; found {
			if strVal, ok := value.(string); ok {
				dur, err := time.ParseDuration(strVal)
				if err != nil {
					return errors.Wrapf(err, "invalid value '%v' for udpIdleTimeout, must be string duration (ex: 1m or 30s)", value)
				}
				options.udpIdleTimeout = dur
			} else {
				return errors.Errorf("invalid value '%v' for udpIdleTimeout, must be string duration (ex: 1m or 30s)", value)
			}
		}

		if value, found := data["udpCheckInterval"]; found {
			if strVal, ok := value.(string); ok {
				dur, err := time.ParseDuration(strVal)
				if err != nil {
					return errors.Wrapf(err, "invalid value '%v' for udpCheckInterval, must be string duration (ex: 1m or 30s)", value)
				}
				options.udpCheckInterval = dur
			} else {
				return errors.Errorf("invalid value '%v' for udpCheckInterval, must be string duration (ex: 1m or 30s)", value)
			}
		}

	}

	return nil
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

	return buf.String()
}
