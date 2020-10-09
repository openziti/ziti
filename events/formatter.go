package events

import (
	"encoding/json"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/iomonad"
	"io"
	"reflect"
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

type JsonFabricSessionEvent SessionEvent

func (event *JsonFabricSessionEvent) WriteTo(output io.WriteCloser) error {
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
	jsonRep["source_id"] = event.SourceId
	jsonRep["timestamp"] = event.Timestamp
	jsonRep["tags"] = event.Tags

	metrics := map[string]interface{}{}

	for name, val := range event.IntMetrics {
		metrics[name] = val
	}

	for name, val := range event.FloatMetrics {
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

func (formatter *JsonFormatter) AcceptSessionEvent(event *SessionEvent) {
	formatter.AcceptLoggingEvent((*JsonFabricSessionEvent)(event))
}

func (formatter *JsonFormatter) AcceptMetricsEvent(event *MetricsEvent) {
	formatter.AcceptLoggingEvent((*JsonMetricsEvent)(event))
}

func (formatter *JsonFormatter) AcceptUsageEvent(event *UsageEvent) {
	formatter.AcceptLoggingEvent((*JsonUsageEvent)(event))
}

type PlainTextFabricSessionEvent SessionEvent

func (event *PlainTextFabricSessionEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*SessionEvent)(event).String()))
	return err
}

type PlainTextMetricsEvent MetricsEvent

func (event *PlainTextMetricsEvent) WriteTo(output io.WriteCloser) error {
	w := iomonad.Wrap(output)
	for name, val := range event.IntMetrics {
		w.Printf("%v: %9d\n", name, val)
	}

	for name, val := range event.FloatMetrics {
		w.Printf("%s: %v\n", name, val)
	}

	return w.GetError()
}

type PlainTextUsageEvent UsageEvent

func (event *PlainTextUsageEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*UsageEvent)(event).String()))
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

func (formatter *PlainTextFormatter) AcceptSessionEvent(event *SessionEvent) {
	formatter.AcceptLoggingEvent((*PlainTextFabricSessionEvent)(event))
}

func (formatter *PlainTextFormatter) AcceptMetricsEvent(event *MetricsEvent) {
	formatter.AcceptLoggingEvent((*PlainTextMetricsEvent)(event))
}

func (formatter *PlainTextFormatter) AcceptUsageEvent(event *UsageEvent) {
	formatter.AcceptLoggingEvent((*PlainTextUsageEvent)(event))
}
