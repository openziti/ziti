package event

import (
	"fmt"
	"time"
)

const (
	UsageEventNS       = "usage"
	UsageEventsVersion = 2
)

// A UsageEventV2 is emitted for service usage interval metrics in the v2 format.
//
// Note: In version prior to 1.4.0, the namespace was `fabric.usage`
//
// Valid values for the usage event v2 type are:
//   - usage.ingress.rx - A read from an external connection to an initiating router
//   - usage.ingress.tx - A write to an external connection from an initiating router
//   - usage.egress.rx  - A read from an external connection to an egress router
//   - usage.egress.tx  - A write to an external connection from an egress router
//   - usage.fabric.rx  - A read from a fabric link to a router
//   - usage.fabric.tx  - A write to a fabric link from a router
//
// Example: Ingress Data Received Usage Event
//
//	{
//	 "namespace": "usage",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T12:35:42.448238469-05:00",
//	 "version": 2,
//	 "event_type": "usage.ingress.rx",
//	 "source_id": "5g2QrZxFcw",
//	 "circuit_id": "gZrStElHY",
//	 "usage": 47,
//	 "interval_start_utc": 1737145920,
//	 "interval_length": 60,
//	 "tags": {
//	   "clientId": "haxn9lB0uc",
//	   "hostId": "IahyE.5Scw",
//	   "serviceId": "3pjMOKY2icS8fkQ1lfHmrP"
//	 }
//	}
//
// Example: Fabric Data Sent Usage Event
//
//	{
//	 "namespace": "usage",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T12:35:42.448238469-05:00",
//	 "version": 2,
//	 "event_type": "usage.fabric.tx",
//	 "source_id": "5g2QrZxFcw",
//	 "circuit_id": "gZrStElHY",
//	 "usage": 47,
//	 "interval_start_utc": 1737145920,
//	 "interval_length": 60,
//	 "tags": null
//	}
type UsageEventV2 struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The usage events version, which will always be 2 for this format.
	Version uint32 `json:"version"`

	// The type of usage. For valid values see above.
	EventType string `json:"event_type"`

	// The id of the router reporting the usage
	SourceId string `json:"source_id"`

	// The circuit id whose usage is being reported.
	CircuitId string `json:"circuit_id"`

	// The number of bytes of usage in the interval.
	Usage uint64 `json:"usage"`

	// The start time of the interval. It is represented as Unix time, number of seconds
	// since the beginning of the current epoch.
	IntervalStartUTC int64 `json:"interval_start_utc"`

	// The interval length in seconds.
	IntervalLength uint64 `json:"interval_length"`

	// Metadata, which may include things like the client and hosting identities and the service id.
	Tags map[string]string `json:"tags"`
}

func (event *UsageEventV2) String() string {
	return fmt.Sprintf("%v source=%v session=%v usage=%v intervalStart=%v intervalLength=%v",
		event.EventType, event.SourceId, event.CircuitId, event.Usage, event.IntervalStartUTC, event.IntervalLength)
}

type UsageEventHandler interface {
	AcceptUsageEvent(event *UsageEventV2)
}

type UsageEventHandlerWrapper interface {
	UsageEventHandler
	IsWrapping(value UsageEventHandler) bool
}

// A UsageEventV3 is emitted for service usage interval metrics in the v3 format.
//
// Note: In version prior to 1.4.0, the namespace was `fabric.usage`
//
// Valid values for the usage types are:
//   - ingress.rx - A read from an external connection to an initiating router
//   - ingress.tx - A write to an external connection from an initiating router
//   - egress.rx  - A read from an external connection to an egress router
//   - egress.tx  - A write to an external connection from an egress router
//   - fabric.rx  - A read from a fabric link to a router
//   - fabric.tx  - A write to a fabric link from a router
//
// Example: Untagged Usage Data Event
//
//	{
//	 "namespace": "usage",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T12:35:42.448238469-05:00",
//	 "version": 3,
//	 "source_id": "5g2QrZxFcw",
//	 "circuit_id": "bcRu0EQFe",
//	 "usage": {
//	   "fabric.rx": 47,
//	   "fabric.tx": 47
//	 },
//	 "interval_start_utc": 1737146220,
//	 "interval_length": 60,
//	 "tags": null
//	}
//
// Example: Tagged Usage Data Event
//
//	{
//	 "namespace": "usage",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T12:35:42.448238469-05:00",
//	 "version": 3,
//	 "source_id": "5g2QrZxFcw",
//	 "circuit_id": "bcRu0EQFe",
//	 "usage": {
//	   "ingress.rx": 47,
//	   "ingress.tx": 47
//	 },
//	 "interval_start_utc": 1737146220,
//	 "interval_length": 60,
//	 "tags": {
//	   "clientId": "haxn9lB0uc",
//	   "hostId": "IahyE.5Scw",
//	   "serviceId": "3pjMOKY2icS8fkQ1lfHmrP"
//	 }
//	}
type UsageEventV3 struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The usage events version, which will always be 3 for this format.
	Version uint32 `json:"version"`

	// The id of the router reporting the usage
	SourceId string `json:"source_id"`

	// The circuit id whose usage is being reported.
	CircuitId string `json:"circuit_id"`

	// Map of usage type to amount number of bytes used in the given interval.
	// For valid values for usage type, see above.
	Usage map[string]uint64 `json:"usage"`

	// The start time of the interval. It is represented as Unix time, number of seconds
	// since the beginning of the current epoch.
	IntervalStartUTC int64 `json:"interval_start_utc"`

	// The interval length in seconds.
	IntervalLength uint64 `json:"interval_length"`

	// Metadata, which may include things like the client and hosting identities and the service id.
	Tags map[string]string `json:"tags"`
}

func (event *UsageEventV3) String() string {
	return fmt.Sprintf("source=%v session=%v usage=%v intervalStart=%v intervalLength=%v",
		event.SourceId, event.CircuitId, event.Usage, event.IntervalStartUTC, event.IntervalLength)
}

type UsageEventV3Handler interface {
	AcceptUsageEventV3(event *UsageEventV3)
}

type UsageEventV3HandlerWrapper interface {
	UsageEventV3Handler
	IsWrapping(value UsageEventV3Handler) bool
}
