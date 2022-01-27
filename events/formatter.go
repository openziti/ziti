package events

import (
	"encoding/json"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/iomonad"
	"io"
	"reflect"
	"time"
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
	jsonRep := map[string]interface{}{}
	jsonRep["namespace"] = event.Namespace
	jsonRep["metric"] = event.Metric
	jsonRep["source_id"] = event.SourceAppId
	jsonRep["source_event_id"] = event.SourceEventId
	jsonRep["version"] = event.Version
	if event.SourceEntityId != "" {
		jsonRep["source_entity_id"] = event.SourceEntityId
	}

	ts := event.Timestamp.AsTime()

	jsonRep["timestamp"] = ts.Format(time.RFC3339Nano)
	if len(event.Tags) > 0 {
		jsonRep["tags"] = event.Tags
	}

	metrics := map[string]interface{}{}

	for name, val := range event.Metrics {
		metrics[name] = val
	}

	jsonRep["metrics"] = metrics

	buf, err := json.Marshal(jsonRep)
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
