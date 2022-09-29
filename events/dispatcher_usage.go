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

func (self *Dispatcher) AddUsageEventV3Handler(handler event.UsageEventV3Handler) {
	self.usageEventV3Handlers.Append(handler)
}

func (self *Dispatcher) RemoveUsageEventV3Handler(handler event.UsageEventV3Handler) {
	self.usageEventV3Handlers.Delete(handler)
}

func (self *Dispatcher) AcceptUsageEvent(event *event.UsageEvent) {
	go func() {
		for _, handler := range self.usageEventHandlers.Value() {
			handler.AcceptUsageEvent(event)
		}
	}()
}

func (self *Dispatcher) AcceptUsageEventV3(event *event.UsageEventV3) {
	go func() {
		for _, handler := range self.usageEventV3Handlers.Value() {
			handler.AcceptUsageEventV3(event)
		}
	}()
}

func (self *Dispatcher) registerUsageEventHandler(val interface{}, config map[interface{}]interface{}) error {
	version := 2

	if configVal, found := config["version"]; found {
		if intVal, ok := configVal.(int); ok {
			version = intVal
		}
	}

	if version == 2 {
		handler, ok := val.(event.UsageEventHandler)
		if !ok {
			return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/event/UsageEventHandler interface.", reflect.TypeOf(val))
		}
		self.AddUsageEventHandler(handler)
	} else if version == 3 {
		handler, ok := val.(event.UsageEventV3Handler)
		if !ok {
			return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/event/UsageEventV3Handler interface.", reflect.TypeOf(val))
		}
		self.AddUsageEventV3Handler(handler)
	} else {
		return errors.Errorf("unsupported usage version: %v", version)
	}
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
	if len(self.dispatcher.usageEventHandlers.Value()) > 0 {
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
		for _, interval := range message.UsageCounters {
			for circuitId, bucket := range interval.Buckets {
				for usageType, usage := range bucket.Values {
					evt := &event.UsageEvent{
						Namespace:        event.UsageEventsNs,
						Version:          2,
						EventType:        "usage." + usageType,
						SourceId:         message.SourceId,
						CircuitId:        circuitId,
						Usage:            usage,
						IntervalStartUTC: interval.IntervalStartUTC,
						IntervalLength:   interval.IntervalLength,
						Tags:             bucket.Tags,
					}
					self.dispatcher.AcceptUsageEvent(evt)
				}
			}
		}
	}

	if len(self.dispatcher.usageEventV3Handlers.Value()) > 0 {
		for name, interval := range message.IntervalCounters {
			for _, bucket := range interval.Buckets {
				for circuitId, usage := range bucket.Values {
					evt := &event.UsageEventV3{
						Namespace: event.UsageEventsNs,
						Version:   3,
						SourceId:  message.SourceId,
						CircuitId: circuitId,
						Usage: map[string]uint64{
							name: usage,
						},
						IntervalStartUTC: bucket.IntervalStartUTC,
						IntervalLength:   interval.IntervalLength,
					}
					self.dispatcher.AcceptUsageEventV3(evt)
				}
			}
		}

		for _, interval := range message.UsageCounters {
			for circuitId, bucket := range interval.Buckets {
				evt := &event.UsageEventV3{
					Namespace:        event.UsageEventsNs,
					Version:          3,
					SourceId:         message.SourceId,
					CircuitId:        circuitId,
					Usage:            bucket.Values,
					IntervalStartUTC: interval.IntervalStartUTC,
					IntervalLength:   interval.IntervalLength,
					Tags:             bucket.Tags,
				}
				self.dispatcher.AcceptUsageEventV3(evt)
			}
		}
	}
}
