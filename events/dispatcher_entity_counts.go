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
	"github.com/pkg/errors"
	"reflect"
	"time"
)

func (self *Dispatcher) AddEntityCountEventHandler(handler EntityCountEventHandler, interval time.Duration) {
	self.entityCountEventHandlers.Append(&entityCountState{
		handler:  handler,
		interval: interval,
		nextRun:  time.Now(),
	})
}

func (self *Dispatcher) RemoveEntityCountEventHandler(handler EntityCountEventHandler) {
	for _, state := range self.entityCountEventHandlers.Value() {
		if state.handler == handler {
			self.entityCountEventHandlers.Delete(state)
		}
	}
}

func (self *Dispatcher) initEntityEvents() {
	go self.generateEntityEvents()
}

func (self *Dispatcher) generateEntityEvents() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case t := <-ticker.C:
			var event *EntityCountEvent
			for _, state := range self.entityCountEventHandlers.Value() {
				if t.After(state.nextRun) {
					if event == nil {
						event = self.generateEntityCountEvent()
					}
					state.handler.AcceptEntityCountEvent(event)
					state.nextRun = state.nextRun.Add(state.interval)
				}
			}
		case <-self.closeNotify:
			return
		}
	}
}

func (self *Dispatcher) generateEntityCountEvent() *EntityCountEvent {
	event := &EntityCountEvent{
		Namespace: EntityCountEventNS,
		Timestamp: time.Now(),
	}

	data, err := self.stores.GetEntityCounts(self.dbProvider)
	if err != nil {
		event.Error = err.Error()
	} else {
		event.Counts = data
	}

	return event
}

func (self *Dispatcher) registerEntityCountEventHandler(val interface{}, config map[string]interface{}) error {
	handler, ok := val.(EntityCountEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/edge/events/EntityCountEventHandler interface.", reflect.TypeOf(val))
	}

	interval := time.Minute * 5

	if val, ok := config["interval"]; ok {
		if strVal, ok := val.(string); ok {
			var err error
			interval, err = time.ParseDuration(strVal)
			if err != nil {
				return errors.Wrapf(err, "invalid duration value for edge.entityCounts interval: '%v'", strVal)
			}
		} else {
			return errors.Errorf("invalid type %v for edge.entityCounts interval configuration", reflect.TypeOf(val))
		}
	}

	self.AddEntityCountEventHandler(handler, interval)

	return nil
}

func (self *Dispatcher) unregisterEntityCountEventHandler(val interface{}) {
	if handler, ok := val.(EntityCountEventHandler); ok {
		self.RemoveEntityCountEventHandler(handler)
	}
}

type entityCountState struct {
	handler  EntityCountEventHandler
	interval time.Duration
	nextRun  time.Time
}
