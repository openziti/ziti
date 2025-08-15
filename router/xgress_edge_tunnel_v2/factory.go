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

package xgress_edge_tunnel_v2

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/state"
	"github.com/openziti/ziti/router/xgress_router"
	"github.com/pkg/errors"
)

const (
	DefaultMode              = "tproxy"
	DefaultServicePollRate   = 15 * time.Second
	DefaultDnsResolver       = "udp://127.0.0.1:53"
	DefaultDnsServiceIpRange = "100.64.0.1/10"
)

type Factory struct {
	id                *identity.TokenId
	ctrls             env.NetworkControllers
	routerConfig      *env.Config
	stateManager      state.Manager
	tunneler          *tunneler
	metricsRegistry   metrics.UsageRegistry
	env               env.RouterEnv
	hostedServices    *hostedServiceRegistry
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

func (self *Factory) HandleCreateTunnelTerminatorResponse(msg *channel.Message, ch channel.Channel) {
	self.hostedServices.HandleCreateTerminatorResponse(msg, ch)
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
	return self.routerConfig.Ctrl.DefaultRequestTimeout
}

// NewFactory constructs a new Edge Xgress Tunnel Factory instance
func NewFactory(env env.RouterEnv, routerConfig *env.Config, stateManager state.Manager) *Factory {
	factory := &Factory{
		id:              env.GetRouterId(),
		routerConfig:    routerConfig,
		stateManager:    stateManager,
		metricsRegistry: env.GetMetricsRegistry(),
		env:             env,
		hostedServices:  newHostedServicesRegistry(env, stateManager),
	}
	factory.tunneler = newTunneler(factory)
	return factory
}

// CreateListener creates a new Edge Tunnel Xgress listener
func (self *Factory) CreateListener(optionsData xgress.OptionsData) (xgress_router.Listener, error) {
	self.env.MarkRouterDataModelRequired()
	self.hostedServices.Start()

	options := &Options{}
	if err := options.load(optionsData); err != nil {
		return nil, err
	}
	self.tunneler.listenOptions = options

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
	buf.WriteString(fmt.Sprintf("mtu=%v\n", options.Mtu))
	buf.WriteString(fmt.Sprintf("randomDrops=%v\n", options.RandomDrops))
	buf.WriteString(fmt.Sprintf("drop1InN=%v\n", options.Drop1InN))
	buf.WriteString(fmt.Sprintf("txQueueSize=%v\n", options.TxQueueSize))
	buf.WriteString(fmt.Sprintf("txPortalStartSize=%v\n", options.TxPortalStartSize))
	buf.WriteString(fmt.Sprintf("txPortalMaxSize=%v\n", options.TxPortalMaxSize))
	buf.WriteString(fmt.Sprintf("txPortalMinSize=%v\n", options.TxPortalMinSize))
	buf.WriteString(fmt.Sprintf("txPortalIncreaseThresh=%v\n", options.TxPortalIncreaseThresh))
	buf.WriteString(fmt.Sprintf("txPortalIncreaseScale=%v\n", options.TxPortalIncreaseScale))
	buf.WriteString(fmt.Sprintf("txPortalRetxThresh=%v\n", options.TxPortalRetxThresh))
	buf.WriteString(fmt.Sprintf("txPortalRetxScale=%v\n", options.TxPortalRetxScale))
	buf.WriteString(fmt.Sprintf("txPortalDupAckThresh=%v\n", options.TxPortalDupAckThresh))
	buf.WriteString(fmt.Sprintf("txPortalDupAckScale=%v\n", options.TxPortalDupAckScale))
	buf.WriteString(fmt.Sprintf("rxBufferSize=%v\n", options.RxBufferSize))
	buf.WriteString(fmt.Sprintf("retxStartMs=%v\n", options.RetxStartMs))
	buf.WriteString(fmt.Sprintf("retxScale=%v\n", options.RetxScale))
	buf.WriteString(fmt.Sprintf("retxAddMs=%v\n", options.RetxAddMs))
	buf.WriteString(fmt.Sprintf("maxCloseWait=%v\n", options.MaxCloseWait))
	buf.WriteString(fmt.Sprintf("getCircuitTimeout=%v\n", options.GetCircuitTimeout))

	return buf.String()
}
