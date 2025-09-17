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
	"reflect"

	"github.com/openziti/ziti/controller/event"
	"github.com/pkg/errors"
)

func (self *Dispatcher) AddAlertEventHandler(handler event.AlertEventHandler) {
	self.alertEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveAlertEventHandler(handler event.AlertEventHandler) {
	self.alertEventHandlers.DeleteIf(func(val event.AlertEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.AlertEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) AcceptAlertEvent(evt *event.AlertEvent) {
	evt.EventSrcId = self.ctrlId
	for _, handler := range self.alertEventHandlers.Value() {
		go handler.AcceptAlertEvent(evt)
	}
}

func (self *Dispatcher) registerAlertEventHandler(_ string, val interface{}, _ map[string]interface{}) error {
	handler, ok := val.(event.AlertEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/ziti/controller/event/AlertEventHandler interface.", reflect.TypeOf(val))
	}

	self.AddAlertEventHandler(handler)
	return nil
}

func (self *Dispatcher) unregisterAlertEventHandler(val interface{}) {
	if handler, ok := val.(event.AlertEventHandler); ok {
		self.RemoveAlertEventHandler(handler)
	}
}
