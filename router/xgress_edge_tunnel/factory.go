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

package xgress_edge_tunnel

import (
	"github.com/openziti/channel"
	"github.com/openziti/edge/router/fabric"
	"github.com/openziti/edge/router/handler_edge_ctrl"
	"github.com/openziti/fabric/router"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/stringz"
	"github.com/pkg/errors"
	"time"
)

const (
	DefaultMode              = "tproxy"
	DefaultServicePollRate   = 15 * time.Second
	DefaultDnsResolver       = "udp://127.0.0.1:53"
	DefaultDnsServiceIpRange = "100.64.0.1/10"
)

type Factory struct {
	id                 identity.Identity
	ctrl               channel.Channel
	routerConfig       *router.Config
	stateManager       fabric.StateManager
	serviceListHandler *handler_edge_ctrl.ServiceListHandler
	tunneler           *tunneler
}

func (self *Factory) NotifyOfReconnect() {
}

func (self *Factory) GetTraceDecoders() []channel.TraceMessageDecoder {
	return nil
}

func (self *Factory) Channel() channel.Channel {
	return self.ctrl
}

func (self *Factory) Enabled() bool {
	return true
}

func (self *Factory) BindChannel(binding channel.Binding) error {
	self.ctrl = binding.GetChannel()
	self.serviceListHandler = handler_edge_ctrl.NewServiceListHandler(self.tunneler.servicePoller.handleServiceListUpdate)
	binding.AddTypedReceiveHandler(self.serviceListHandler)
	return nil
}

func (self *Factory) Run(ctrl channel.Channel, _ boltz.Db, notifyClose chan struct{}) error {
	self.ctrl = ctrl
	if self.tunneler.listenOptions != nil {
		return self.tunneler.Start(notifyClose)
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
func NewFactory(routerConfig *router.Config, stateManager fabric.StateManager) *Factory {
	factory := &Factory{
		id:           routerConfig.Id,
		routerConfig: routerConfig,
		stateManager: stateManager,
	}
	factory.tunneler = newTunneler(factory, stateManager)
	return factory
}

// CreateListener creates a new Edge Tunnel Xgress listener
func (self *Factory) CreateListener(optionsData xgress.OptionsData) (xgress.Listener, error) {
	options := &Options{}
	if err := options.load(optionsData); err != nil {
		return nil, err
	}
	self.tunneler.listenOptions = options
	return self.tunneler, nil
}

// CreateDialer creates a new Edge Xgress dialer
func (self *Factory) CreateDialer(optionsData xgress.OptionsData) (xgress.Dialer, error) {
	options := &Options{}
	if err := options.load(optionsData); err != nil {
		return nil, err
	}
	self.tunneler.dialOptions = options
	return self.tunneler, nil
}

type Options struct {
	*xgress.Options
	mode          string
	svcPollRate   time.Duration
	resolver      string
	dnsSvcIpRange string
	lanIf         string
	services      []string
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
			if strVal, ok := value.(string); ok && stringz.Contains([]string{"tproxy", "host", "proxy"}, strVal) {
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

	}

	return nil
}
