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

package federation

import (
	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/router/env"
)

// ClientDialEnv implements env.DialEnv for a federated client network connection.
// It adapts the host router's configuration to present the client network's identity
// when dialing the client network's controllers.
type ClientDialEnv struct {
	hostConfig  *env.Config
	clientId    *identity.TokenId
	bindHandler channel.BindHandler
}

// NewClientDialEnv creates a ClientDialEnv that uses the given host configuration,
// client identity, and bind handler for connecting to client network controllers.
func NewClientDialEnv(hostConfig *env.Config, clientId *identity.TokenId, bindHandler channel.BindHandler) *ClientDialEnv {
	return &ClientDialEnv{
		hostConfig:  hostConfig,
		clientId:    clientId,
		bindHandler: bindHandler,
	}
}

// GetConfig returns a shallow copy of the host configuration with the Id replaced
// by the client network's identity.
func (self *ClientDialEnv) GetConfig() *env.Config {
	cfg := *self.hostConfig
	cfg.Id = self.clientId
	return &cfg
}

// GetChannelHeaders returns the channel headers to send when dialing a client network
// controller. For the prototype this returns empty headers; a production implementation
// would include version and capability information.
func (self *ClientDialEnv) GetChannelHeaders() (channel.Headers, error) {
	return channel.Headers{}, nil
}

// GetCtrlChannelBindHandler returns the bind handler to use when a control channel
// is established to a client network controller.
func (self *ClientDialEnv) GetCtrlChannelBindHandler() channel.BindHandler {
	return self.bindHandler
}

// NotifyOfReconnect is a no-op for the prototype. A production implementation would
// propagate reconnection events to synchronize state with the client network controller.
func (self *ClientDialEnv) NotifyOfReconnect(ch ctrlchan.CtrlChannel) {
}
