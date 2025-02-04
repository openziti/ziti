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
	"github.com/openziti/metrics/metrics_pb"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/network"
	"github.com/pkg/errors"
	"reflect"
	"strings"
	"time"
)

func (self *Dispatcher) AddServiceEventHandler(handler event.ServiceEventHandler) {
	self.serviceEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveServiceEventHandler(handler event.ServiceEventHandler) {
	self.serviceEventHandlers.DeleteIf(func(val event.ServiceEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.ServiceEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) AcceptServiceEvent(event *event.ServiceEvent) {
	go func() {
		for _, handler := range self.serviceEventHandlers.Value() {
			handler.AcceptServiceEvent(event)
		}
	}()
}

func (self *Dispatcher) registerServiceEventHandler(eventType string, val interface{}, _ map[string]interface{}) error {
	handler, ok := val.(event.ServiceEventHandler)
	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/ziti/controller/event/ServiceEventHandler interface.", reflect.TypeOf(val))
	}

	if eventType == "services" {
		handler = &serviceEventOldNsAdapter{
			namespace: "service.events",
			wrapped:   handler,
		}
	}
	self.AddServiceEventHandler(handler)

	return nil
}

func (self *Dispatcher) unregisterServiceEventHandler(val interface{}) {
	if handler, ok := val.(event.ServiceEventHandler); ok {
		self.RemoveServiceEventHandler(handler)
	}
}

func (self *Dispatcher) initServiceEvents(n *network.Network) {
	n.InitServiceCounterDispatch(&serviceEventAdapter{
		Dispatcher: self,
	})
}

type serviceEventOldNsAdapter struct {
	namespace string
	wrapped   event.ServiceEventHandler
}

func (self *serviceEventOldNsAdapter) AcceptServiceEvent(evt *event.ServiceEvent) {
	nsEvent := *evt
	nsEvent.Namespace = self.namespace
	self.wrapped.AcceptServiceEvent(&nsEvent)
}

func (self *serviceEventOldNsAdapter) IsWrapping(value event.ServiceEventHandler) bool {
	if self.wrapped == value {
		return true
	}
	if w, ok := self.wrapped.(event.ServiceEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}

// serviceEventAdapter converts service interval counters into service events
type serviceEventAdapter struct {
	*Dispatcher
}

func (self *serviceEventAdapter) AcceptMetrics(message *metrics_pb.MetricsMessage) {
	for name, interval := range message.IntervalCounters {
		for _, bucket := range interval.Buckets {
			for combinedId, count := range bucket.Values {
				ids := strings.Split(combinedId, ":")
				serviceId := ids[0]
				terminatorId := ""
				if len(ids) > 1 {
					terminatorId = ids[1]
				}
				evt := &event.ServiceEvent{
					Namespace:        event.ServiceEventNS,
					EventSrcId:       self.ctrlId,
					Timestamp:        time.Now(),
					Version:          2,
					EventType:        name,
					ServiceId:        serviceId,
					TerminatorId:     terminatorId,
					Count:            count,
					IntervalStartUTC: bucket.IntervalStartUTC,
					IntervalLength:   interval.IntervalLength,
				}
				self.Dispatcher.AcceptServiceEvent(evt)
			}
		}
	}
}
