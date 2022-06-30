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
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/transport/v2"
	"github.com/pkg/errors"
)

type factory struct {
	id      *identity.TokenId
	ctrl    xgress.CtrlChannel
	options *xgress.Options
	tcfg    transport.Configuration
}

// NewFactory returns a new Transport Xgress factory
func NewFactory(id *identity.TokenId, ctrl xgress.CtrlChannel, tcfg transport.Configuration) xgress.Factory {
	return &factory{id: id, ctrl: ctrl, tcfg: tcfg}
}

func (factory *factory) CreateListener(optionsData xgress.OptionsData) (xgress.Listener, error) {
	options, err := xgress.LoadOptions(optionsData)
	if err != nil {
		return nil, errors.Wrap(err, "error loading options")
	}
	return newListener(factory.id, factory.ctrl, options, factory.tcfg), nil
}

func (factory *factory) CreateDialer(optionsData xgress.OptionsData) (xgress.Dialer, error) {
	options, err := xgress.LoadOptions(optionsData)
	if err != nil {
		return nil, errors.Wrap(err, "error loading options")
	}
	return newDialer(factory.id, factory.ctrl, options, factory.tcfg)
}
