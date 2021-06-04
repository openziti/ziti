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
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
)

const (
	LinkHeaderConnId   = 0
	LinkHeaderType     = 1
	LinkHeaderRouterId = 2

	PayloadChannel = 1
	AckChannel     = 2
)

func NewFactory(accepter xlink.Accepter, chAccepter ChannelAccepter, c transport.Configuration) xlink.Factory {
	return &factory{accepter: accepter, chAccepter: chAccepter, tcfg: c}
}

func (self *factory) CreateListener(id *identity.TokenId, _ xlink.Forwarder, configData transport.Configuration) (xlink.Listener, error) {
	config, err := loadListenerConfig(configData)
	if err != nil {
		return nil, fmt.Errorf("error loading listener configuration (%w)", err)
	}
	return &listener{
		id:              id,
		config:          config,
		accepter:        self.accepter,
		chAccepter:      self.chAccepter,
		tcfg:            self.tcfg,
		eventC:          make(chan linkEvent, 2),
		pendingChannels: map[string]*newChannelEvent{},
	}, nil
}

func (self *factory) CreateDialer(id *identity.TokenId, _ xlink.Forwarder, configData transport.Configuration) (xlink.Dialer, error) {
	config, err := loadDialerConfig(configData)
	if err != nil {
		return nil, fmt.Errorf("error loading dialer configuration (%w)", err)
	}
	return &dialer{
		id:         id,
		config:     config,
		accepter:   self.accepter,
		chAccepter: self.chAccepter,
		tcfg:       self.tcfg,
	}, nil
}

type factory struct {
	accepter   xlink.Accepter
	chAccepter ChannelAccepter
	tcfg       transport.Configuration
}
