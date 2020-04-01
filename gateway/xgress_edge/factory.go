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

package xgress_edge

import (
	"github.com/netfoundry/ziti-edge/gateway/handler_edge_ctrl"
	"github.com/netfoundry/ziti-edge/gateway/internal/apiproxy"
	"github.com/netfoundry/ziti-edge/gateway/internal/fabric"
	"github.com/netfoundry/ziti-edge/gateway/internal/gateway"
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"math"
)

const (
	DefaultMaxOutOfOrderMsgs = 1000
)

type Factory struct {
	id             *identity.TokenId
	ctrl           channel2.Channel
	enabled        bool
	config         *gateway.Config
	hostedServices *hostedServiceRegistry
	stateManager   fabric.StateManager
}

func (factory *Factory) Channel() channel2.Channel {
	return factory.ctrl
}

func (factory *Factory) Enabled() bool {
	return factory.enabled
}

func (factory *Factory) BindChannel(ch channel2.Channel) error {
	factory.ctrl = ch
	ch.AddReceiveHandler(handler_edge_ctrl.NewHelloHandler(factory.config.Advertise, []string{"tls"}))

	ch.AddReceiveHandler(handler_edge_ctrl.NewSessionAddedHandler(factory.stateManager))
	ch.AddReceiveHandler(handler_edge_ctrl.NewSessionRemovedHandler(factory.stateManager))
	ch.AddReceiveHandler(handler_edge_ctrl.NewSessionUpdatedHandler(factory.stateManager))

	ch.AddReceiveHandler(handler_edge_ctrl.NewApiSessionAddedHandler(factory.stateManager))
	ch.AddReceiveHandler(handler_edge_ctrl.NewApiSessionRemovedHandler(factory.stateManager))
	ch.AddReceiveHandler(handler_edge_ctrl.NewApiSessionUpdatedHandler(factory.stateManager))
	return nil
}

func (factory *Factory) Run(ctrl channel2.Channel, _ boltz.Db, done chan struct{}) error {
	factory.ctrl = ctrl
	factory.stateManager.StartHeartbeat(ctrl)
	return nil
}

func (factory *Factory) LoadConfig(configMap map[interface{}]interface{}) error {
	_, factory.enabled = configMap["edge"]

	if !factory.enabled {
		return nil
	}

	var err error
	config := gateway.NewConfig()
	if err = config.LoadConfigFromMap(configMap); err != nil {
		return err
	}

	if id, err := config.LoadIdentity(); err == nil {
		factory.id = identity.NewIdentity(id)
	} else {
		return err
	}

	factory.config = config
	go apiproxy.Start(config)

	return nil
}

// NewFactory constructs a new Edge Xgress Factory instance
func NewFactory() *Factory {
	factory := &Factory{
		hostedServices: &hostedServiceRegistry{},
		stateManager:   fabric.GetStateManager(),
	}
	return factory
}

// CreateListener creates a new Edge Xgress listener
func (factory *Factory) CreateListener(optionsData xgress.OptionsData) (xgress.Listener, error) {
	if !factory.enabled {
		return nil, errors.New("edge listener enabled but required configuration section [edge] is missing")
	}

	options := &Options{}
	if err := options.load(optionsData); err != nil {
		return nil, err
	}

	return newListener(factory.id, factory, options), nil
}

// CreateDialer creates a new Edge Xgress dialer
func (factory *Factory) CreateDialer(optionsData xgress.OptionsData) (xgress.Dialer, error) {
	if !factory.enabled {
		return nil, errors.New("edge listener enabled but required configuration section [edge] is missing")
	}

	options := &Options{}
	if err := options.load(optionsData); err != nil {
		return nil, err
	}

	return newDialer(factory, options), nil
}

type Options struct {
	xgress.Options
	MaxOutOfOrderMsgs uint32
}

func (options *Options) load(data xgress.OptionsData) error {
	options.Options = *xgress.LoadOptions(data)
	options.MaxOutOfOrderMsgs = DefaultMaxOutOfOrderMsgs

	if value, found := data["options"]; found {
		data = value.(map[interface{}]interface{})

		if value, found := data["maxOutOfOrderMsgs"]; found {
			iVal, ok := value.(int)
			if !ok || iVal < 0 || iVal > math.MaxInt32 {
				return errors.Errorf("maxOutOfOrderMsgs must be int value between 0 and %v", math.MaxInt32)
			}
			options.MaxOutOfOrderMsgs = uint32(iVal)
		}
	}
	return nil
}
