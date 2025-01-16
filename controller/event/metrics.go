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

package event

import (
	"github.com/openziti/metrics/metrics_pb"
	"time"
)

const (
	MetricsEventNS       = "metrics"
	MetricsEventsVersion = 3
)

// A MetricsEvent represents a point in time snapshot of a metric from a controller or router.
//
// Valid values for metric type are:
//   - intValue
//   - floatValue
//   - meter
//   - histogram
//   - timer
//
// Example: The service policy enforcer deletes meter
//
//	{
//	 "namespace": "metrics",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T02:45:21.890823877Z",
//	 "metric_type": "meter",
//	 "source_id": "ctrl_client",
//	 "version": 3,
//	 "metric": "service.policy.enforcer.run.deletes",
//	 "metrics": {
//	   "count": 0,
//	   "m15_rate": 0,
//	   "m1_rate": 0,
//	   "m5_rate": 0,
//	   "mean_rate": 0
//	 },
//	 "source_event_id": "c41fbf8d-cd14-4b8b-ae7b-0f0e93e2021d"
//	}
//
// Example: The api session create timer
//
//	{
//	 "namespace": "metrics",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T02:45:21.890823877Z",
//	 "metric_type": "timer",
//	 "source_id": "ctrl_client",
//	 "version": 3,
//	 "metric": "api-session.create",
//	 "metrics": {
//	   "count": 1,
//	   "m15_rate": 0.0006217645754885097,
//	   "m1_rate": 0.000002754186169011774,
//	   "m5_rate": 0.0005841004612303224,
//	   "max": 7598246,
//	   "mean": 7598246,
//	   "mean_rate": 0.0018542395091967903,
//	   "min": 7598246,
//	   "p50": 7598246,
//	   "p75": 7598246,
//	   "p95": 7598246,
//	   "p99": 7598246,
//	   "p999": 7598246,
//	   "p9999": 7598246,
//	   "std_dev": 0,
//	   "variance": 0
//	 },
//	 "source_event_id": "c41fbf8d-cd14-4b8b-ae7b-0f0e93e2021d"
//	}
type MetricsEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The type of metrics event. See above for valid values.
	MetricType string `json:"metric_type" mapstructure:"metric_type"`

	// The id of the router or controller which emitted the metric.
	SourceAppId string `json:"source_id" mapstructure:"source_id"`

	// If this metric is associated with an entity, such as link, this will
	// contain the entity id.
	SourceEntityId string `json:"source_entity_id,omitempty"  mapstructure:"source_entity_id,omitempty"`

	// The version of the metrics format. The current version is 3.
	Version uint32 `json:"version"`

	// The name of the metric.
	Metric string `json:"metric"`

	// The values that copmrise the metrics.
	Metrics map[string]any `json:"metrics"`

	// Some metrics include additional metadata.
	// For example link metrics may include source and destination.
	Tags map[string]string `json:"tags,omitempty"`

	// Events will often be collected together on a schedule. This is a correlation id
	// so that events can be tied together with other events emitted at the same time.
	SourceEventId string `json:"source_event_id" mapstructure:"source_event_id"`
}

type MetricsEventHandler interface {
	AcceptMetricsEvent(event *MetricsEvent)
}

type MetricsEventHandlerF func(event *MetricsEvent)

func (self MetricsEventHandlerF) AcceptMetricsEvent(event *MetricsEvent) {
	self(event)
}

type MetricsEventHandlerWrapper interface {
	MetricsEventHandler
	IsWrapping(value MetricsEventHandler) bool
}

type MetricsMessageHandler interface {
	// AcceptMetricsMsg is called when new metrics become available
	AcceptMetricsMsg(message *metrics_pb.MetricsMessage)
}

type MetricsMessageHandlerWrapper interface {
	MetricsMessageHandler
	IsWrapping(value MetricsEventHandler) bool
}

type MetricsMessageHandlerF func(msg *metrics_pb.MetricsMessage)

func (self MetricsMessageHandlerF) AcceptMetricsMsg(msg *metrics_pb.MetricsMessage) {
	self(msg)
}

type MetricsMapper func(msg *metrics_pb.MetricsMessage, event *MetricsEvent)
