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

package xgress_transport

import (
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xgress"
	"github.com/pkg/errors"
)

type factory struct {
	env  env.RouterEnv
	tcfg transport.Configuration
}

// NewFactory returns a new Transport Xgress factory
func NewFactory(env env.RouterEnv, tcfg transport.Configuration) xgress.Factory {
	return &factory{env: env, tcfg: tcfg}
}

func (self *factory) CreateListener(optionsData xgress.OptionsData) (xgress.Listener, error) {
	options, err := xgress.LoadOptions(optionsData)
	if err != nil {
		return nil, errors.Wrap(err, "error loading options")
	}
	options.AckSender = self.env.GetAckSender()
	return newListener(self.env.GetRouterId(), self.env.GetNetworkControllers(), options, self.tcfg), nil
}

func (self *factory) CreateDialer(optionsData xgress.OptionsData) (xgress.Dialer, error) {
	options, err := xgress.LoadOptions(optionsData)
	if err != nil {
		return nil, errors.Wrap(err, "error loading options")
	}
	return newDialer(self.env.GetRouterId(), self.env.GetNetworkControllers(), options, self.tcfg)
}
