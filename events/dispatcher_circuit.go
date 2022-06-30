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
	"github.com/openziti/fabric/event"
	"github.com/pkg/errors"
	"reflect"
)

func (self *Dispatcher) AddCircuitEventHandler(handler event.CircuitEventHandler) {
	self.circuitEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveCircuitEventHandler(handler event.CircuitEventHandler) {
	self.circuitEventHandlers.Delete(handler)
}

func (self *Dispatcher) AcceptCircuitEvent(event *event.CircuitEvent) {
	go func() {
		for _, handler := range self.circuitEventHandlers.Value() {
			handler.AcceptCircuitEvent(event)
		}
	}()
}

func (self *Dispatcher) registerCircuitEventHandler(val interface{}, config map[interface{}]interface{}) error {
	handler, ok := val.(event.CircuitEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/event/CircuitEventHandler interface.", reflect.TypeOf(val))
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
			return errors.Errorf("invalid type %v for fabric.circuits include configuration", reflect.TypeOf(includeVar))
		}
	}

	if len(includeList) == 0 {
		self.AddCircuitEventHandler(handler)
		return nil
	}

	accepted := map[event.CircuitEventType]struct{}{}
	for _, include := range includeList {
		found := false
		for _, t := range event.CircuitEventTypes {
			if include == string(t) {
				accepted[t] = struct{}{}
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("invalid include %v for fabric.circuits. valid values are %+v", include, event.CircuitEventTypes)
		}
	}
	result := &filteredCircuitEventHandler{
		accepted: accepted,
		wrapped:  handler,
	}
	self.AddCircuitEventHandler(result)
	return nil
}

type filteredCircuitEventHandler struct {
	accepted map[event.CircuitEventType]struct{}
	wrapped  event.CircuitEventHandler
}

func (self *filteredCircuitEventHandler) AcceptCircuitEvent(event *event.CircuitEvent) {
	if _, found := self.accepted[event.EventType]; found {
		self.wrapped.AcceptCircuitEvent(event)
	}
}
