package events

import (
	"fmt"
	"github.com/openziti/fabric/metrics"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/pkg/errors"
	"reflect"
)

func registerUsageEventHandler(val interface{}, _ map[interface{}]interface{}) error {
	handler, ok := val.(UsageEventHandler)
	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/events/UsageEventHandler interface.", reflect.TypeOf(val))
	}

	metrics.AddMetricsEventHandler(&usageAdapter{handler: handler})
	return nil
}

func RegisterUsageEventHandler(handler UsageEventHandler) func() {
	result := &usageAdapter{
		handler: handler,
	}

	metrics.AddMetricsEventHandler(result)
	return func() {
		metrics.RemoveMetricsEventHandler(result)
	}
}

type usageAdapter struct {
	handler UsageEventHandler
}

func (adapter *usageAdapter) AcceptMetrics(message *metrics_pb.MetricsMessage) {
	for name, interval := range message.IntervalCounters {
		for _, bucket := range interval.Buckets {
			for circuitId, usage := range bucket.Values {
				event := &UsageEvent{
					Namespace:        "fabric.usage",
					Version:          2,
					EventType:        name,
					SourceId:         message.SourceId,
					CircuitId:        circuitId,
					Usage:            usage,
					IntervalStartUTC: bucket.IntervalStartUTC,
					IntervalLength:   interval.IntervalLength,
				}
				adapter.handler.AcceptUsageEvent(event)
			}
		}
	}
}

type UsageEvent struct {
	Namespace        string `json:"namespace"`
	Version          uint32 `json:"version"`
	EventType        string `json:"event_type"`
	SourceId         string `json:"source_id"`
	CircuitId        string `json:"circuit_id"`
	Usage            uint64 `json:"usage"`
	IntervalStartUTC int64  `json:"interval_start_utc"`
	IntervalLength   uint64 `json:"interval_length"`
}

func (event *UsageEvent) String() string {
	return fmt.Sprintf("%v source=%v session=%v usage=%v intervalStart=%v intervalLength=%v",
		event.EventType, event.SourceId, event.CircuitId, event.Usage, event.IntervalStartUTC, event.IntervalLength)
}

type UsageEventHandler interface {
	AcceptUsageEvent(event *UsageEvent)
}
