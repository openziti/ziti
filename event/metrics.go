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
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	MetricsEventsNs      = "metrics"
	MetricsEventsVersion = 2
)

type MetricsEvent struct {
	MetricType     string                 `json:"metric_type" mapstructure:"metric_type"`
	Namespace      string                 `json:"namespace"`
	SourceAppId    string                 `json:"source_id" mapstructure:"source_id"`
	SourceEntityId string                 `json:"source_entity_id,omitempty"  mapstructure:"source_entity_id,omitempty"`
	Version        uint32                 `json:"version"`
	Timestamp      *timestamppb.Timestamp `json:"timestamp"`
	Metric         string                 `json:"metric"`
	Metrics        map[string]interface{} `json:"metrics"`
	Tags           map[string]string      `json:"tags,omitempty"`
	SourceEventId  string                 `json:"source_event_id" mapstructure:"source_event_id"`
}

type MetricsEventHandler interface {
	AcceptMetricsEvent(event *MetricsEvent)
}

type MetricsEventHandlerF func(event *MetricsEvent)

func (self MetricsEventHandlerF) AcceptMetricsEvent(event *MetricsEvent) {
	self(event)
}

type MetricsMessageHandler interface {
	// AcceptMetricsMsg is called when new metrics become available
	AcceptMetricsMsg(message *metrics_pb.MetricsMessage)
}

type MetricsMessageHandlerF func(msg *metrics_pb.MetricsMessage)

func (self MetricsMessageHandlerF) AcceptMetricsMsg(msg *metrics_pb.MetricsMessage) {
	self(msg)
}

type MetricsMapper func(msg *metrics_pb.MetricsMessage, event *MetricsEvent)
