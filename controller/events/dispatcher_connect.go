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
	"github.com/openziti/ziti/controller/event"
	"github.com/pkg/errors"
	"reflect"
)

func (self *Dispatcher) AddConnectEventHandler(handler event.ConnectEventHandler) {
	self.connectEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveConnectEventHandler(handler event.ConnectEventHandler) {
	self.connectEventHandlers.DeleteIf(func(val event.ConnectEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.ConnectEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) AcceptConnectEvent(evt *event.ConnectEvent) {
	evt.EventSrcId = self.ctrlId
	for _, handler := range self.connectEventHandlers.Value() {
		go handler.AcceptConnectEvent(evt)
	}
}

func (self *Dispatcher) registerConnectEventHandler(val interface{}, _ map[string]interface{}) error {
	handler, ok := val.(event.ConnectEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/ziti/controller/event/ConnectEventHandler interface.", reflect.TypeOf(val))
	}

	self.AddConnectEventHandler(handler)
	return nil
}

func (self *Dispatcher) unregisterConnectEventHandler(val interface{}) {
	if handler, ok := val.(event.ConnectEventHandler); ok {
		self.RemoveConnectEventHandler(handler)
	}
}
