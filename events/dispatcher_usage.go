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
	"github.com/openziti/metrics/metrics_pb"
	"github.com/pkg/errors"
	"reflect"
)

func (self *Dispatcher) AddUsageEventHandler(handler event.UsageEventHandler) {
	self.usageEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveUsageEventHandler(handler event.UsageEventHandler) {
	self.usageEventHandlers.Delete(handler)
}

func (self *Dispatcher) AcceptUsageEvent(event *event.UsageEvent) {
	go func() {
		for _, handler := range self.usageEventHandlers.Value() {
			handler.AcceptUsageEvent(event)
		}
	}()
}

func (self *Dispatcher) registerUsageEventHandler(val interface{}, _ map[interface{}]interface{}) error {
	handler, ok := val.(event.UsageEventHandler)
	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/event/UsageEventHandler interface.", reflect.TypeOf(val))
	}

	self.AddUsageEventHandler(handler)
	return nil
}

func (self *Dispatcher) initUsageEvents() {
	self.AddMetricsMessageHandler(&usageEventAdapter{
		dispatcher: self,
	})
}

type usageEventAdapter struct {
	dispatcher *Dispatcher
}

func (self *usageEventAdapter) AcceptMetricsMsg(message *metrics_pb.MetricsMessage) {
	for name, interval := range message.IntervalCounters {
		for _, bucket := range interval.Buckets {
			for circuitId, usage := range bucket.Values {
				evt := &event.UsageEvent{
					Namespace:        event.UsageEventsNs,
					Version:          event.UsageEventsVersion,
					EventType:        name,
					SourceId:         message.SourceId,
					CircuitId:        circuitId,
					Usage:            usage,
					IntervalStartUTC: bucket.IntervalStartUTC,
					IntervalLength:   interval.IntervalLength,
				}
				self.dispatcher.AcceptUsageEvent(evt)
			}
		}
	}
}
