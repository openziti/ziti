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
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/event"
	"github.com/pkg/errors"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
)

type LoggingHandlerFactory interface {
	NewLoggingHandler(format string, buffer int, out io.WriteCloser) (interface{}, error)
}

func NewWriterEventSink(out io.Writer) event.FormattedEventSink {
	return WriterEventSink{out: out}
}

type WriterEventSink struct {
	out io.Writer
}

func (self WriterEventSink) AcceptFormattedEvent(_ string, formattedEvent []byte) {
	if _, err := self.out.Write(formattedEvent); err != nil {
		pfxlog.Logger().WithError(err).Error("failed to output event")
	}
}

type FormatterEvent interface {
	GetEventType() string
	Format() ([]byte, error)
}

type BaseFormatter struct {
	closed      atomic.Bool
	closeNotify chan struct{}
	events      chan FormatterEvent
	sink        event.FormattedEventSink
}

func (f *BaseFormatter) Run() {
	for {
		select {
		case evt := <-f.events:
			if formattedEvent, err := evt.Format(); err != nil {
				pfxlog.Logger().WithError(err).Errorf("failed to output event of type %v", reflect.TypeOf(evt))
			} else {
				f.sink.AcceptFormattedEvent(evt.GetEventType(), formattedEvent)
			}
		case <-f.closeNotify:
			return
		}
	}
}

func (f *BaseFormatter) Close() error {
	if f.closed.CompareAndSwap(false, true) {
		close(f.closeNotify)
	}
	return nil
}

func (f *BaseFormatter) AcceptLoggingEvent(event FormatterEvent) {
	select {
	case f.events <- event:
	case <-f.closeNotify:
	}
}

func MarshalJson(v interface{}) ([]byte, error) {
	buf, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

type JsonCircuitEvent event.CircuitEvent

func (event *JsonCircuitEvent) GetEventType() string {
	return "circuit"
}

func (event *JsonCircuitEvent) Format() ([]byte, error) {
	return MarshalJson(event)
}

type JsonLinkEvent event.LinkEvent

func (event *JsonLinkEvent) GetEventType() string {
	return "link"
}

func (event *JsonLinkEvent) Format() ([]byte, error) {
	return MarshalJson(event)
}

type JsonMetricsEvent event.MetricsEvent

func (event *JsonMetricsEvent) GetEventType() string {
	return "metrics"
}

func (event *JsonMetricsEvent) Format() ([]byte, error) {
	return MarshalJson(event)
}

type JsonRouterEvent event.RouterEvent

func (event *JsonRouterEvent) GetEventType() string {
	return "router"
}

func (event *JsonRouterEvent) Format() ([]byte, error) {
	return MarshalJson(event)
}

type JsonServiceEvent event.ServiceEvent

func (event *JsonServiceEvent) GetEventType() string {
	return "service"
}

func (event *JsonServiceEvent) Format() ([]byte, error) {
	return MarshalJson(event)
}

type JsonTerminatorEvent event.TerminatorEvent

func (event *JsonTerminatorEvent) GetEventType() string {
	return "terminator"
}

func (event *JsonTerminatorEvent) Format() ([]byte, error) {
	return MarshalJson(event)
}

type JsonUsageEvent event.UsageEvent

func (event *JsonUsageEvent) GetEventType() string {
	return "usage"
}

func (event *JsonUsageEvent) Format() ([]byte, error) {
	return MarshalJson(event)
}

func (event *JsonUsageEventV3) GetEventType() string {
	return "usage.v3"
}

type JsonUsageEventV3 event.UsageEventV3

func (event *JsonUsageEventV3) Format() ([]byte, error) {
	return MarshalJson(event)
}

type JsonClusterEvent event.ClusterEvent

func (event *JsonClusterEvent) GetEventType() string {
	return "cluster"
}

func (event *JsonClusterEvent) Format() ([]byte, error) {
	return MarshalJson(event)
}

type JsonEntityChangeEvent event.EntityChangeEvent

func (event *JsonEntityChangeEvent) GetEventType() string {
	return "entity.change"
}

func (event *JsonEntityChangeEvent) Format() ([]byte, error) {
	return MarshalJson(event)
}

func NewJsonFormatter(queueDepth int, sink event.FormattedEventSink) *JsonFormatter {
	result := &JsonFormatter{
		BaseFormatter: BaseFormatter{
			events:      make(chan FormatterEvent, queueDepth),
			closeNotify: make(chan struct{}),
			sink:        sink,
		},
	}
	go result.Run()
	return result
}

type JsonFormatter struct {
	BaseFormatter
}

func (formatter *JsonFormatter) AcceptCircuitEvent(evt *event.CircuitEvent) {
	formatter.AcceptLoggingEvent((*JsonCircuitEvent)(evt))
}

func (formatter *JsonFormatter) AcceptLinkEvent(evt *event.LinkEvent) {
	formatter.AcceptLoggingEvent((*JsonLinkEvent)(evt))
}

func (formatter *JsonFormatter) AcceptMetricsEvent(evt *event.MetricsEvent) {
	formatter.AcceptLoggingEvent((*JsonMetricsEvent)(evt))
}

func (formatter *JsonFormatter) AcceptServiceEvent(evt *event.ServiceEvent) {
	formatter.AcceptLoggingEvent((*JsonServiceEvent)(evt))
}

func (formatter *JsonFormatter) AcceptTerminatorEvent(evt *event.TerminatorEvent) {
	formatter.AcceptLoggingEvent((*JsonTerminatorEvent)(evt))
}

func (formatter *JsonFormatter) AcceptRouterEvent(evt *event.RouterEvent) {
	formatter.AcceptLoggingEvent((*JsonRouterEvent)(evt))
}

func (formatter *JsonFormatter) AcceptUsageEvent(evt *event.UsageEvent) {
	formatter.AcceptLoggingEvent((*JsonUsageEvent)(evt))
}

func (formatter *JsonFormatter) AcceptUsageEventV3(evt *event.UsageEventV3) {
	formatter.AcceptLoggingEvent((*JsonUsageEventV3)(evt))
}

func (formatter *JsonFormatter) AcceptClusterEvent(evt *event.ClusterEvent) {
	formatter.AcceptLoggingEvent((*JsonClusterEvent)(evt))
}

func (formatter *JsonFormatter) AcceptEntityChangeEvent(evt *event.EntityChangeEvent) {
	formatter.AcceptLoggingEvent((*JsonEntityChangeEvent)(evt))
}

var histogramBuckets = map[string]string{"p50": "0.50", "p75": "0.75", "p95": "0.95", "p99": "0.99", "p999": "0.999", "p9999": "0.9999"}

type PrometheusMetricsEvent event.MetricsEvent

func (event *PrometheusMetricsEvent) WriteTo(output io.WriteCloser, includeTimestamps bool) error {
	buf, err := event.Marshal(includeTimestamps)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

func (event *PrometheusMetricsEvent) getMetricName() string {
	key := strings.Replace(event.Metric, " ", "_", -1)
	key = strings.Replace(key, ".", "_", -1)
	key = strings.Replace(key, "-", "_", -1)
	key = strings.Replace(key, "=", "_", -1)
	key = strings.Replace(key, "/", "_", -1)
	key = strings.Replace(key, ":", "", -1)

	// Prometheus complains about metrics ending in _count, so "fix" that.
	if strings.HasSuffix(key, "_count") {
		key = strings.TrimSuffix(key, "_count") + "_c"
	}

	return "ziti_" + key
}

func (event *PrometheusMetricsEvent) newTag(name, value string) string {
	return fmt.Sprintf("%s=\"%s\"", name, value)
}

func (event *PrometheusMetricsEvent) getTags() *[]string {
	tags := make([]string, 0)
	if event.Tags != nil {
		for name, val := range event.Tags {
			tags = append(tags, event.newTag(name, val))

		}
	}
	tags = append(tags, event.newTag("source_id", event.SourceAppId))
	return &tags
}

func (event *PrometheusMetricsEvent) getTagsAsString(tags *[]string) string {
	return "{" + strings.Join(*tags, ",") + "}"
}

func (event *PrometheusMetricsEvent) getTimestampString(includeTimestamps bool) string {
	var ts string

	if includeTimestamps {
		ts += fmt.Sprintf(" %d", event.Timestamp.UnixMilli())
	}

	ts += "\n"
	return ts
}

func (event *PrometheusMetricsEvent) toGauge(metricKey string, includeTimestamps bool) string {
	format := "# HELP %[1]s %[1]s\n" +
		"# TYPE %[1]s gauge\n" +
		"%[1]s%[3]s %[2]v"

	metric := fmt.Sprintf(format, event.getMetricName(), event.Metrics[metricKey], event.getTagsAsString(event.getTags()))

	metric += event.getTimestampString(includeTimestamps)

	return metric
}

func (event *PrometheusMetricsEvent) toHistogram(includeTimestamps bool) string {
	key := event.getMetricName()
	tags := event.getTags()

	format := "# HELP %[1]s %[1]s\n" +
		"# TYPE %[1]s histogram\n" +
		"%[1]s_count%[2]s %[3]v"

	metric := fmt.Sprintf(format, key, event.getTagsAsString(tags), event.Metrics["count"])

	metric += event.getTimestampString(includeTimestamps)

	for bucketName, bucketPromKey := range histogramBuckets {
		bucketTags := append(*tags, event.newTag("le", bucketPromKey))
		metric += fmt.Sprintf("%s_bucket%s %v", key, event.getTagsAsString(&bucketTags), event.Metrics[bucketName])
		metric += event.getTimestampString(includeTimestamps)
	}

	return metric
}

func (event *PrometheusMetricsEvent) Marshal(includeTimestamps bool) ([]byte, error) {
	// Prometheus can be a little picky about metric/label naming and formats.
	// Run the output of any changes through  https://o11y.tools/metricslint/ to make sure they are OK

	var result string

	switch event.MetricType {
	case "intValue":
		result = event.toGauge("value", includeTimestamps)
	case "floatValue":
		result = event.toGauge("value", includeTimestamps)
	case "meter":
		result = event.toGauge("m1_rate", includeTimestamps)
	case "histogram":
		result = event.toHistogram(includeTimestamps)
	case "timer":
		result += event.toHistogram(includeTimestamps)
	default:
		return nil, errors.Errorf("Unhandled metric type %s", event.MetricType)
	}

	return []byte(result), nil
}
