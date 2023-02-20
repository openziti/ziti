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

package events

import (
	"github.com/openziti/fabric/event"
	"github.com/pkg/errors"
	"reflect"
)

func (self *Dispatcher) AddClusterEventHandler(handler event.ClusterEventHandler) {
	self.clusterEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveClusterEventHandler(handler event.ClusterEventHandler) {
	self.clusterEventHandlers.Delete(handler)
}

func (self *Dispatcher) AcceptClusterEvent(event *event.ClusterEvent) {
	go func() {
		for _, handler := range self.clusterEventHandlers.Value() {
			handler.AcceptClusterEvent(event)
		}
	}()
}

func (self *Dispatcher) registerClusterEventHandler(val interface{}, _ map[string]interface{}) error {
	handler, ok := val.(event.ClusterEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/event/ClusterEventHandler interface.", reflect.TypeOf(val))
	}

	self.clusterEventHandlers.Append(handler)

	return nil
}

func (self *Dispatcher) unregisterClusterEventHandler(val interface{}) {
	if handler, ok := val.(event.ClusterEventHandler); ok {
		self.RemoveClusterEventHandler(handler)
	}
}
