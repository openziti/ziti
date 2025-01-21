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

func (self *Dispatcher) AddSdkEventHandler(handler event.SdkEventHandler) {
	self.sdkEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveSdkEventHandler(handler event.SdkEventHandler) {
	self.sdkEventHandlers.DeleteIf(func(val event.SdkEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.SdkEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) AcceptSdkEvent(evt *event.SdkEvent) {
	evt.EventSrcId = self.ctrlId
	for _, handler := range self.sdkEventHandlers.Value() {
		go handler.AcceptSdkEvent(evt)
	}
}

func (self *Dispatcher) registerSdkEventHandler(val interface{}, _ map[string]interface{}) error {
	handler, ok := val.(event.SdkEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/ziti/controller/event/SdkEventHandler interface.", reflect.TypeOf(val))
	}

	self.AddSdkEventHandler(handler)
	return nil
}

func (self *Dispatcher) unregisterSdkEventHandler(val interface{}) {
	if handler, ok := val.(event.SdkEventHandler); ok {
		self.RemoveSdkEventHandler(handler)
	}
}
