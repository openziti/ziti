/*
	(c) Copyright NetFoundry Inc.

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

	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xlink"
)

type channelType byte

const (
	LinkHeaderConnId        = 0
	LinkHeaderType          = 1
	LinkHeaderRouterId      = 2
	LinkHeaderRouterVersion = 3
	LinkHeaderBinding       = 4
	LinkHeaderIteration     = 5

	PayloadChannel channelType = 1
	AckChannel     channelType = 2
)

func (self channelType) String() string {
	if self == PayloadChannel {
		return "payload"
	}
	if self == AckChannel {
		return "ack"
	}
	return "invalid"
}

type LinkEnv interface {
	GetMetricsRegistry() metrics.UsageRegistry
	GetXLinkRegistry() xlink.Registry
	GetNetworkControllers() env.NetworkControllers
	GetRateLimiterPool() goroutines.Pool
	GetCloseNotify() <-chan struct{}
}

func NewFactory(accepter xlink.Acceptor,
	bindHandlerFactory BindHandlerFactory,
	tcfg transport.Configuration,
	env LinkEnv) xlink.Factory {

	tcfg[transport.KeyProtocol] = append(tcfg.Protocols(), "ziti-link")

	return &factory{
		acceptor:           accepter,
		bindHandlerFactory: bindHandlerFactory,
		transportConfig:    tcfg,
		env:                env,
	}
}

func (self *factory) CreateListener(id *identity.TokenId, configData transport.Configuration) (xlink.Listener, error) {
	config, err := loadListenerConfig(configData)

	if err != nil {
		return nil, fmt.Errorf("error loading listener configuration (%w)", err)
	}

	if config.options == nil {
		config.options = channel.DefaultOptions()
	}

	if config.options.OutQueueSize == channel.DefaultOutQueueSize {
		config.options.OutQueueSize = 64
	}

	return &listener{
		id:                 id,
		config:             config,
		accepter:           self.acceptor,
		bindHandlerFactory: self.bindHandlerFactory,
		tcfg:               self.transportConfig,
		pendingLinks:       map[string]*pendingLink{},
		xlinkRegistery:     self.env.GetXLinkRegistry(),
		env:                self.env,
	}, nil
}

func (self *factory) CreateDialer(id *identity.TokenId, configData transport.Configuration) (xlink.Dialer, error) {
	config, err := loadDialerConfig(configData)
	if err != nil {
		return nil, fmt.Errorf("error loading dialer configuration (%w)", err)
	}

	if config.options == nil {
		config.options = channel.DefaultOptions()
	}

	if config.options.OutQueueSize == channel.DefaultOutQueueSize {
		config.options.OutQueueSize = 64
	}

	return &dialer{
		id:                 id,
		config:             config,
		acceptor:           self.acceptor,
		bindHandlerFactory: self.bindHandlerFactory,
		transportConfig:    self.transportConfig,
		env:                self.env,
	}, nil
}

type factory struct {
	acceptor           xlink.Acceptor
	bindHandlerFactory BindHandlerFactory
	transportConfig    transport.Configuration
	env                LinkEnv
}
