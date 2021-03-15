package events

import (
	"fmt"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/openziti/foundation/util/cowslice"
	"github.com/pkg/errors"
	"reflect"
)

func registerServiceEventHandler(val interface{}, _ map[interface{}]interface{}) error {
	handler, ok := val.(ServiceEventHandler)
	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/events/ServiceEventHandler interface.", reflect.TypeOf(val))
	}

	AddServiceEventHandler(handler)
	return nil
}

var serviceEventHandlerRegistry = cowslice.NewCowSlice(make([]ServiceEventHandler, 0))

func getServiceEventHandlers() []ServiceEventHandler {
	return serviceEventHandlerRegistry.Value().([]ServiceEventHandler)
}

type serviceEventAdapter struct{}

func (self serviceEventAdapter) AcceptMetrics(message *metrics_pb.MetricsMessage) {
	for name, interval := range message.IntervalCounters {
		for _, bucket := range interval.Buckets {
			for serviceId, count := range bucket.Values {
				event := &ServiceEvent{
					Namespace:        "service.events",
					EventType:        name,
					ServiceId:        serviceId,
					Count:            count,
					IntervalStartUTC: bucket.IntervalStartUTC,
					IntervalLength:   interval.IntervalLength,
				}
				self.dispatchEvent(event)
			}
		}
	}
}

func (self serviceEventAdapter) dispatchEvent(event *ServiceEvent) {
	for _, handler := range getServiceEventHandlers() {
		handler.AcceptServiceEvent(event)
	}
}

type ServiceEvent struct {
	Namespace        string `json:"namespace"`
	EventType        string `json:"event_type"`
	ServiceId        string `json:"service_id"`
	Count            uint64 `json:"count"`
	IntervalStartUTC int64  `json:"interval_start_utc"`
	IntervalLength   uint64 `json:"interval_length"`
}

func (event *ServiceEvent) String() string {
	return fmt.Sprintf("%v service=%v count=%v intervalStart=%v intervalLength=%v",
		event.EventType, event.ServiceId, event.Count, event.IntervalStartUTC, event.IntervalLength)
}

type ServiceEventHandler interface {
	AcceptServiceEvent(event *ServiceEvent)
}
