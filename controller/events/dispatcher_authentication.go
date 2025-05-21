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
	"fmt"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/event"
	"github.com/pkg/errors"
	"reflect"
)

func (self *Dispatcher) AddAuthenticationEventHandler(handler event.AuthenticationEventHandler) {
	self.authenticationEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveAuthenticationEventHandler(handler event.AuthenticationEventHandler) {
	self.authenticationEventHandlers.DeleteIf(func(val event.AuthenticationEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.AuthenticationEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) AcceptAuthenticationEvent(evt *event.AuthenticationEvent) {
	for _, handler := range self.authenticationEventHandlers.Value() {
		go handler.AcceptAuthenticationEvent(evt)
	}
}

func (self *Dispatcher) registerAuthenticationEventHandler(eventType string, val interface{}, config map[string]interface{}) error {
	handler, ok := val.(event.AuthenticationEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/ziti/controller/event/AuthenticationEventHandler interface.", reflect.TypeOf(val))
	}

	var includeList []string
	if includeVar, ok := config["include"]; ok {
		if includeStr, ok := includeVar.(string); ok {
			includeList = append(includeList, includeStr)
		} else if includeIntfList, ok := includeVar.([]interface{}); ok {
			for _, val := range includeIntfList {
				includeList = append(includeList, fmt.Sprintf("%v", val))
			}
		} else {
			return errors.Errorf("invalid type %v for %v include configuration", reflect.TypeOf(includeVar), event.AuthenticationEventNS)
		}
	}

	if len(includeList) == 0 || (len(includeList) == 2 && stringz.ContainsAll(includeList, event.AuthenticationEventTypeSuccess, event.AuthenticationEventTypeFail)) {
		self.AddAuthenticationEventHandler(handler)
	} else {
		for _, include := range includeList {
			if include != event.AuthenticationEventTypeSuccess && include != event.AuthenticationEventTypeFail {
				return errors.Errorf("invalid include %v for %v. valid values are ['success', 'fail']", include, event.AuthenticationEventNS)
			}
		}

		self.AddAuthenticationEventHandler(&authenticationEventAdapter{
			wrapped:     handler,
			includeList: includeList,
		})
	}

	return nil
}

func (self *Dispatcher) unregisterAuthenticationEventHandler(val interface{}) {
	if handler, ok := val.(event.AuthenticationEventHandler); ok {
		self.RemoveAuthenticationEventHandler(handler)
	}
}

type authenticationEventAdapter struct {
	wrapped     event.AuthenticationEventHandler
	includeList []string
}

func (adapter *authenticationEventAdapter) AcceptAuthenticationEvent(evt *event.AuthenticationEvent) {
	if stringz.Contains(adapter.includeList, evt.EventType) {
		adapter.wrapped.AcceptAuthenticationEvent(evt)
	}
}

func (self *authenticationEventAdapter) IsWrapping(value event.AuthenticationEventHandler) bool {
	if self.wrapped == value {
		return true
	}
	if w, ok := self.wrapped.(event.AuthenticationEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}
