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

func (self *Dispatcher) entityCountEventGenerator(interval time.Duration, handler EntityCountEventHandler) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			self.generateEntityCountEvent(handler)
		case <-self.closeNotify:
			return
		}
	}
}

func (self *Dispatcher) generateEntityCountEvent(handler EntityCountEventHandler) {
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

	handler.AcceptEntityCountEvent(event)
}

func (self *Dispatcher) registerEntityCountEventHandler(val interface{}, config map[interface{}]interface{}) error {
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

	go self.entityCountEventGenerator(interval, handler)

	return nil
}
