package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/iomonad"
	"io"
	"reflect"
	"strings"
)

type LoggingHandlerFactory interface {
	NewLoggingHandler(format string, buffer int, out io.WriteCloser) (interface{}, error)
}

type LoggingEvent interface {
	WriteTo(output io.WriteCloser) error
}

type BaseFormatter struct {
	events chan LoggingEvent
	output io.WriteCloser
}

func (f *BaseFormatter) Run() {
	for event := range f.events {
		if err := event.WriteTo(f.output); err != nil {
			pfxlog.Logger().WithError(err).Errorf("failed to output event of type %v", reflect.TypeOf(event))
		}
		_, _ = f.output.Write([]byte("\n"))
	}
}

func (f *BaseFormatter) AcceptLoggingEvent(event LoggingEvent) {
	f.events <- event
}

type JsonFabricCircuitEvent CircuitEvent

func (event *JsonFabricCircuitEvent) WriteTo(output io.WriteCloser) error {
	buf, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

type JsonMetricsEvent MetricsEvent

func (event *JsonMetricsEvent) WriteTo(output io.WriteCloser) error {
	buf, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

type JsonUsageEvent UsageEvent

func (event *JsonUsageEvent) WriteTo(output io.WriteCloser) error {
	buf, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

type JsonServiceEvent ServiceEvent

func (event *JsonServiceEvent) WriteTo(output io.WriteCloser) error {
	buf, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

type JsonTerminatorEvent TerminatorEvent

func (event *JsonTerminatorEvent) WriteTo(output io.WriteCloser) error {
	buf, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

type JsonRouterEvent RouterEvent

func (event *JsonRouterEvent) WriteTo(output io.WriteCloser) error {
	buf, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

func NewJsonFormatter(queueDepth int, output io.WriteCloser) *JsonFormatter {
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

func (formatter *JsonFormatter) AcceptCircuitEvent(event *CircuitEvent) {
	formatter.AcceptLoggingEvent((*JsonFabricCircuitEvent)(event))
}

func (formatter *JsonFormatter) AcceptMetricsEvent(event *MetricsEvent) {
	formatter.AcceptLoggingEvent((*JsonMetricsEvent)(event))
}

func (formatter *JsonFormatter) AcceptUsageEvent(event *UsageEvent) {
	formatter.AcceptLoggingEvent((*JsonUsageEvent)(event))
}

func (formatter *JsonFormatter) AcceptServiceEvent(event *ServiceEvent) {
	formatter.AcceptLoggingEvent((*JsonServiceEvent)(event))
}

func (formatter *JsonFormatter) AcceptTerminatorEvent(event *TerminatorEvent) {
	formatter.AcceptLoggingEvent((*JsonTerminatorEvent)(event))
}

func (formatter *JsonFormatter) AcceptRouterEvent(event *RouterEvent) {
	formatter.AcceptLoggingEvent((*JsonRouterEvent)(event))
}

type PlainTextFabricCircuitEvent CircuitEvent

func (event *PlainTextFabricCircuitEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*CircuitEvent)(event).String()))
	return err
}

type PlainTextMetricsEvent MetricsEvent

func (event *PlainTextMetricsEvent) WriteTo(output io.WriteCloser) error {
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

type PlainTextUsageEvent UsageEvent

func (event *PlainTextUsageEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*UsageEvent)(event).String()))
	return err
}

type PlainTextServiceEvent ServiceEvent

func (event *PlainTextServiceEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*ServiceEvent)(event).String()))
	return err
}

type PlainTextTerminatorEvent TerminatorEvent

func (event *PlainTextTerminatorEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*TerminatorEvent)(event).String()))
	return err
}

type PlainTextRouterEvent RouterEvent

func (event *PlainTextRouterEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*RouterEvent)(event).String()))
	return err
}

func NewPlainTextFormatter(queueDepth int, output io.WriteCloser) *PlainTextFormatter {
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

func (formatter *PlainTextFormatter) AcceptSessionEvent(event *CircuitEvent) {
	formatter.AcceptLoggingEvent((*PlainTextFabricCircuitEvent)(event))
}

func (formatter *PlainTextFormatter) AcceptMetricsEvent(event *MetricsEvent) {
	formatter.AcceptLoggingEvent((*PlainTextMetricsEvent)(event))
}

func (formatter *PlainTextFormatter) AcceptUsageEvent(event *UsageEvent) {
	formatter.AcceptLoggingEvent((*PlainTextUsageEvent)(event))
}

func (formatter *PlainTextFormatter) AcceptServiceEvent(event *ServiceEvent) {
	formatter.AcceptLoggingEvent((*PlainTextServiceEvent)(event))
}

func (formatter *PlainTextFormatter) AcceptTerminatorEvent(event *TerminatorEvent) {
	formatter.AcceptLoggingEvent((*PlainTextTerminatorEvent)(event))
}

func (formatter *PlainTextFormatter) AcceptRouterEvent(event *RouterEvent) {
	formatter.AcceptLoggingEvent((*PlainTextRouterEvent)(event))
}

var histogramBuckets = map[string]string{"p50": "0.50", "p75": "0.75", "p95": "0.95", "p99": "0.99", "p999": "0.999", "p9999": "0.9999"}

type PrometheusMetricsEvent MetricsEvent

func (event *PrometheusMetricsEvent) WriteTo(output io.WriteCloser) error {
	buf, err := event.Marshal()
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

func (event *PrometheusMetricsEvent) makeSafe(key string) string {
	key = strings.Replace(key, " ", "_", -1)
	key = strings.Replace(key, ".", "_", -1)
	key = strings.Replace(key, "-", "_", -1)
	key = strings.Replace(key, "=", "_", -1)
	key = strings.Replace(key, "/", "_", -1)
	return key
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
	tags = append(tags, event.newTag("sourceId", event.SourceAppId))

	return &tags
}

func (event *PrometheusMetricsEvent) getTagsAsString(tags *[]string) string {
	return "{" + strings.Join(*tags, ",") + "}"
}

func (event *PrometheusMetricsEvent) toGauge(metricKey string) string {
	t := "# HELP %[1]s %[1]s\n" +
		"# TYPE %[1]s gauge\n" +
		"%[1]s%[3]s %[2]v %[4]d\n"

	return fmt.Sprintf(t, event.makeSafe(event.Metric), event.Metrics[metricKey], event.getTagsAsString(event.getTags()), event.Timestamp.AsTime().UnixMilli())
}

func (event *PrometheusMetricsEvent) toHistogram() string {
	key := event.makeSafe(event.Metric + "_" + event.MetricType)
	tags := event.getTags()

	t := fmt.Sprintf("# HELP %[1]s %[1]s\n"+
		"# TYPE %[1]s histogram\n"+
		"%[1]s_count%[2]s %[3]v %[4]d\n",
		key, event.getTagsAsString(tags), event.Metrics["count"], event.Timestamp.AsTime().UnixMilli())

	for bucketName, bucketPromKey := range histogramBuckets {
		bucketTags := append(*tags, event.newTag("le", bucketPromKey))
		t += fmt.Sprintf("%s_bucket%s %v\n", key, event.getTagsAsString(&bucketTags), event.Metrics[bucketName])
	}

	return t
}

func (event *PrometheusMetricsEvent) Marshal() ([]byte, error) {
	var result string

	switch event.MetricType {
	case "intValue":
		result = event.toGauge("value")
	case "floatValue":
		result = event.toGauge("value")
	case "meter":
		result = event.toGauge("m1_rate")
	case "histogram":
		result = event.toHistogram()
	case "timer":
		result = event.toGauge("m1_rate")
		result += event.toHistogram()
	default:
		return nil, errors.New(fmt.Sprintf("Unhandled metric type %s", event.MetricType))
	}

	return []byte(result), nil
}
