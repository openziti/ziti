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
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/event"
	"github.com/openziti/foundation/v2/iomonad"
	"io"
	"reflect"
	"strings"
)

type LoggingHandlerFactory interface {
	NewLoggingHandler(format string, buffer int, out io.WriteCloser) (interface{}, error)
}

type LoggingEvent interface {
	WriteTo(output io.Writer) error
}

type BaseFormatter struct {
	events chan LoggingEvent
	output io.Writer
}

func (f *BaseFormatter) Run() {
	for evt := range f.events {
		if err := evt.WriteTo(f.output); err != nil {
			pfxlog.Logger().WithError(err).Errorf("failed to output event of type %v", reflect.TypeOf(evt))
		}
		_, _ = f.output.Write([]byte("\n"))
	}
}

func (f *BaseFormatter) AcceptLoggingEvent(event LoggingEvent) {
	f.events <- event
}

func marshalJson(v interface{}, output io.Writer) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

type JsonCircuitEvent event.CircuitEvent

func (event *JsonCircuitEvent) WriteTo(output io.Writer) error {
	return marshalJson(event, output)
}

type JsonLinkEvent event.LinkEvent

func (event *JsonLinkEvent) WriteTo(output io.Writer) error {
	return marshalJson(event, output)
}

type JsonMetricsEvent event.MetricsEvent

func (event *JsonMetricsEvent) WriteTo(output io.Writer) error {
	return marshalJson(event, output)
}

type JsonRouterEvent event.RouterEvent

func (event *JsonRouterEvent) WriteTo(output io.Writer) error {
	return marshalJson(event, output)
}

type JsonServiceEvent event.ServiceEvent

func (event *JsonServiceEvent) WriteTo(output io.Writer) error {
	return marshalJson(event, output)
}

type JsonTerminatorEvent event.TerminatorEvent

func (event *JsonTerminatorEvent) WriteTo(output io.Writer) error {
	return marshalJson(event, output)
}

type JsonUsageEvent event.UsageEvent

func (event *JsonUsageEvent) WriteTo(output io.Writer) error {
	return marshalJson(event, output)
}

func NewJsonFormatter(queueDepth int, output io.Writer) *JsonFormatter {
	return &JsonFormatter{
		BaseFormatter: BaseFormatter{
			events: make(chan LoggingEvent, queueDepth),
			output: output,
		},
	}
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

type PlainTextCircuitEvent event.CircuitEvent

func (self *PlainTextCircuitEvent) WriteTo(output io.Writer) error {
	_, err := output.Write([]byte((*event.CircuitEvent)(self).String()))
	return err
}

type PlainTextLinkEvent event.LinkEvent

func (self *PlainTextLinkEvent) WriteTo(output io.Writer) error {
	_, err := output.Write([]byte((*event.LinkEvent)(self).String()))
	return err
}

type PlainTextMetricsEvent event.MetricsEvent

func (event *PlainTextMetricsEvent) WriteTo(output io.Writer) error {
	w := iomonad.Wrap(output)
	for name, val := range event.Metrics {
		if intVal, ok := val.(int64); ok {
			w.Printf("%v: %9d\n", name, intVal)
		} else {
			w.Printf("%s: %v\n", name, val)
		}
	}

	return w.GetError()
}

type PlainTextUsageEvent event.UsageEvent

func (self *PlainTextUsageEvent) WriteTo(output io.Writer) error {
	_, err := output.Write([]byte((*event.UsageEvent)(self).String()))
	return err
}

type PlainTextServiceEvent event.ServiceEvent

func (self *PlainTextServiceEvent) WriteTo(output io.Writer) error {
	_, err := output.Write([]byte((*event.ServiceEvent)(self).String()))
	return err
}

type PlainTextTerminatorEvent event.TerminatorEvent

func (self *PlainTextTerminatorEvent) WriteTo(output io.Writer) error {
	_, err := output.Write([]byte((*event.TerminatorEvent)(self).String()))
	return err
}

type PlainTextRouterEvent event.RouterEvent

func (self *PlainTextRouterEvent) WriteTo(output io.Writer) error {
	_, err := output.Write([]byte((*event.RouterEvent)(self).String()))
	return err
}

func NewPlainTextFormatter(queueDepth int, output io.Writer) *PlainTextFormatter {
	return &PlainTextFormatter{
		BaseFormatter: BaseFormatter{
			events: make(chan LoggingEvent, queueDepth),
			output: output,
		},
	}
}

type PlainTextFormatter struct {
	BaseFormatter
}

func (formatter *PlainTextFormatter) AcceptCircuitEvent(evt *event.CircuitEvent) {
	formatter.AcceptLoggingEvent((*PlainTextCircuitEvent)(evt))
}

func (formatter *PlainTextFormatter) AcceptLinkEvent(evt *event.LinkEvent) {
	formatter.AcceptLoggingEvent((*PlainTextLinkEvent)(evt))
}

func (formatter *PlainTextFormatter) AcceptMetricsEvent(evt *event.MetricsEvent) {
	formatter.AcceptLoggingEvent((*PlainTextMetricsEvent)(evt))
}

func (formatter *PlainTextFormatter) AcceptRouterEvent(evt *event.RouterEvent) {
	formatter.AcceptLoggingEvent((*PlainTextRouterEvent)(evt))
}

func (formatter *PlainTextFormatter) AcceptServiceEvent(evt *event.ServiceEvent) {
	formatter.AcceptLoggingEvent((*PlainTextServiceEvent)(evt))
}

func (formatter *PlainTextFormatter) AcceptTerminatorEvent(evt *event.TerminatorEvent) {
	formatter.AcceptLoggingEvent((*PlainTextTerminatorEvent)(evt))
}

func (formatter *PlainTextFormatter) AcceptUsageEvent(evt *event.UsageEvent) {
	formatter.AcceptLoggingEvent((*PlainTextUsageEvent)(evt))
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
		return nil, errors.New(fmt.Sprintf("Unhandled metric type %s", event.MetricType))
	}

	return []byte(result), nil
}
