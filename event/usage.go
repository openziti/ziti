package event

import "fmt"

const (
	UsageEventsNs      = "fabric.usage"
	UsageEventsVersion = 2
)

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
