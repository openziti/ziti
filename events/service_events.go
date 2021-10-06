package events

import (
	"fmt"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/openziti/foundation/util/cowslice"
	"github.com/pkg/errors"
	"reflect"
	"strings"
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
			for combinedId, count := range bucket.Values {
				ids := strings.Split(combinedId, ":")
				serviceId := ids[0]
				terminatorId := ""
				if len(ids) > 1 {
					terminatorId = ids[1]
				}
				event := &ServiceEvent{
					Namespace:        "service.events",
					Version:          2,
					EventType:        name,
					ServiceId:        serviceId,
					TerminatorId:     terminatorId,
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
	Version          uint32 `json:"version"`
	EventType        string `json:"event_type"`
	ServiceId        string `json:"service_id"`
	TerminatorId     string `json:"terminator_id"`
	Count            uint64 `json:"count"`
	IntervalStartUTC int64  `json:"interval_start_utc"`
	IntervalLength   uint64 `json:"interval_length"`
}

func (event *ServiceEvent) String() string {
	return fmt.Sprintf("%v service=%v terminator=%v count=%v intervalStart=%v intervalLength=%v",
		event.EventType, event.ServiceId, event.TerminatorId, event.Count, event.IntervalStartUTC, event.IntervalLength)
}

type ServiceEventHandler interface {
	AcceptServiceEvent(event *ServiceEvent)
}
