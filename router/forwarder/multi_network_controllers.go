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

package forwarder

import (
	"sync"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/router/env"
)

// MultiNetworkControllers wraps the host network's controllers and any client network
// controllers added via federation. It provides dispatching by (networkId, ctrlId) for
// the faulter, scanner, and other components that need to reach the right controller.
type MultiNetworkControllers struct {
	hostCtrls env.NetworkControllers // networkId 0
	mu        sync.RWMutex
	networks  map[uint16]env.NetworkControllers
}

// NewMultiNetworkControllers creates a MultiNetworkControllers with the given host
// network controllers as networkId 0.
func NewMultiNetworkControllers(hostCtrls env.NetworkControllers) *MultiNetworkControllers {
	return &MultiNetworkControllers{
		hostCtrls: hostCtrls,
		networks:  make(map[uint16]env.NetworkControllers),
	}
}

// GetChannel returns the channel for the given controller in the given network.
// NetworkId 0 is the host network.
func (self *MultiNetworkControllers) GetChannel(networkId uint16, ctrlId string) channel.Channel {
	ctrls := self.getControllers(networkId)
	if ctrls == nil {
		return nil
	}
	return ctrls.GetChannel(ctrlId)
}

// ForEach iterates over all controllers across all networks.
func (self *MultiNetworkControllers) ForEach(f func(networkId uint16, ctrlId string, ch channel.Channel)) {
	self.hostCtrls.ForEach(func(ctrlId string, ch channel.Channel) {
		f(0, ctrlId, ch)
	})

	self.mu.RLock()
	defer self.mu.RUnlock()
	for networkId, ctrls := range self.networks {
		nid := networkId // capture for closure
		ctrls.ForEach(func(ctrlId string, ch channel.Channel) {
			f(nid, ctrlId, ch)
		})
	}
}

// AddNetwork registers a client network's controllers.
func (self *MultiNetworkControllers) AddNetwork(networkId uint16, ctrls env.NetworkControllers) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.networks[networkId] = ctrls
}

// RemoveNetwork unregisters and closes a client network's controllers.
func (self *MultiNetworkControllers) RemoveNetwork(networkId uint16) env.NetworkControllers {
	self.mu.Lock()
	defer self.mu.Unlock()
	ctrls := self.networks[networkId]
	delete(self.networks, networkId)
	return ctrls
}

// DefaultRequestTimeout delegates to the host network's controllers.
func (self *MultiNetworkControllers) DefaultRequestTimeout() time.Duration {
	return self.hostCtrls.DefaultRequestTimeout()
}

func (self *MultiNetworkControllers) getControllers(networkId uint16) env.NetworkControllers {
	if networkId == 0 {
		return self.hostCtrls
	}
	self.mu.RLock()
	defer self.mu.RUnlock()
	return self.networks[networkId]
}
