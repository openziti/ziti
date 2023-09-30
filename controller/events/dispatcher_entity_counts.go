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
	"strings"
	"time"
)

func (self *Dispatcher) AddEntityCountEventHandler(handler event.EntityCountEventHandler, interval time.Duration, onlyLeaderEvents bool) {
	self.entityCountEventHandlers.Append(&entityCountState{
		handler:          handler,
		onlyLeaderEvents: onlyLeaderEvents,
		interval:         interval,
		nextRun:          time.Now(),
	})
}

func (self *Dispatcher) RemoveEntityCountEventHandler(handler event.EntityCountEventHandler) {
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
			var event *event.EntityCountEvent
			leader := self.network.Dispatcher.IsLeaderOrLeaderless()
			for _, state := range self.entityCountEventHandlers.Value() {
				if !state.onlyLeaderEvents || leader {
					if t.After(state.nextRun) {
						if event == nil {
							event = self.generateEntityCountEvent()
						}
						state.handler.AcceptEntityCountEvent(event)
						state.nextRun = state.nextRun.Add(state.interval)
					}
				}
			}
		case <-self.closeNotify:
			return
		}
	}
}

func (self *Dispatcher) generateEntityCountEvent() *event.EntityCountEvent {
	event := &event.EntityCountEvent{
		Namespace: event.EntityCountEventNS,
		Timestamp: time.Now(),
	}

	data, err := self.stores.GetEntityCounts(self.network.GetDb())
	if err != nil {
		event.Error = err.Error()
	} else {
		event.Counts = data
	}

	return event
}

func (self *Dispatcher) registerEntityCountEventHandler(val interface{}, config map[string]interface{}) error {
	handler, ok := val.(event.EntityCountEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/ziti/controller/events/EntityCountEventHandler interface.", reflect.TypeOf(val))
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

	propagateAlways := false
	if val, found := config["propagateAlways"]; found {
		if b, ok := val.(bool); ok {
			propagateAlways = b
		} else if s, ok := val.(string); ok {
			propagateAlways = strings.EqualFold(s, "true")
		} else {
			return errors.New("invalid value for propagateAlways, must be boolean or string")
		}
	}

	self.AddEntityCountEventHandler(handler, interval, !propagateAlways)

	return nil
}

func (self *Dispatcher) unregisterEntityCountEventHandler(val interface{}) {
	if handler, ok := val.(event.EntityCountEventHandler); ok {
		self.RemoveEntityCountEventHandler(handler)
	}
}

type entityCountState struct {
	handler          event.EntityCountEventHandler
	onlyLeaderEvents bool
	interval         time.Duration
	nextRun          time.Time
}
