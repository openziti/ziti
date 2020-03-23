/*
	(c) Copyright NetFoundry, Inc.

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

package xlink_transport

import (
	"fmt"
	"github.com/netfoundry/ziti-fabric/router/xlink"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

func NewFactory(accepter xlink.Accepter, chAccepter ChannelAccepter) xlink.Factory {
	return &factory{accepter: accepter, chAccepter: chAccepter}
}

func (self *factory) CreateListener(id *identity.TokenId, configData map[interface{}]interface{}) (xlink.Listener, error) {
	c, err := loadListenerConfig(configData)
	if err != nil {
		return nil, fmt.Errorf("error loading configuration (%w)", err)
	}
	return &listenerImpl{
		id:         id,
		config:     c,
		accepter:   self.accepter,
		chAccepter: self.chAccepter,
	}, nil
}

func (self *factory) CreateDialer(id *identity.TokenId, configData map[interface{}]interface{}) (xlink.Dialer, error) {
	c, err := loadDialerConfig(configData)
	if err != nil {
		return nil, fmt.Errorf("error loading configuration (%w)", err)
	}
	return &dialerImpl{
		id:         id,
		config:     c,
		accepter:   self.accepter,
		chAccepter: self.chAccepter,
	}, nil
}

type factory struct {
	accepter   xlink.Accepter
	chAccepter ChannelAccepter
}
