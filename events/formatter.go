package events

import (
	"encoding/json"
	"github.com/openziti/fabric/events"
	"github.com/pkg/errors"
	"io"
	"strings"
)

func init() {
	events.RegisterEventHandlerType("file", edgeFileEventLoggerFactory{})
	events.RegisterEventHandlerType("stdout", edgeStdOutLoggerFactory{})
}

type edgeFormatterFactory struct{}

func (f edgeFormatterFactory) NewLoggingHandler(format string, buffer int, out io.WriteCloser) (interface{}, error) {
	if strings.EqualFold(format, "json") {
		result := &EdgeJsonFormatter{
			JsonFormatter: *events.NewJsonFormatter(buffer, out),
		}
		go result.Run()
		return result, nil
	}

	if strings.EqualFold(format, "plain") {
		result := &EdgePlainTextFormatter{
			PlainTextFormatter: *events.NewPlainTextFormatter(buffer, out),
		}
		go result.Run()
		return result, nil
	}

	return nil, errors.Errorf("invalid 'format' for event log output file: %v", format)
}

type edgeStdOutLoggerFactory struct{}

func (edgeStdOutLoggerFactory) NewEventHandler(config map[interface{}]interface{}) (interface{}, error) {
	return events.NewFileEventLogger(edgeFormatterFactory{}, true, config)
}

type edgeFileEventLoggerFactory struct{}

func (edgeFileEventLoggerFactory) NewEventHandler(config map[interface{}]interface{}) (interface{}, error) {
	return events.NewFileEventLogger(edgeFormatterFactory{}, false, config)
}

type EdgeJsonFormatter struct {
	events.JsonFormatter
}

func (formatter *EdgeJsonFormatter) AcceptEdgeSessionEvent(event *EdgeSessionEvent) {
	formatter.AcceptLoggingEvent((*JsonEdgeSessionEvent)(event))
}

type JsonEdgeSessionEvent EdgeSessionEvent

func (event *JsonEdgeSessionEvent) WriteTo(output io.WriteCloser) error {
	buf, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

type EdgePlainTextFormatter struct {
	events.PlainTextFormatter
}

func (formatter *EdgePlainTextFormatter) AcceptEdgeSessionEvent(event *EdgeSessionEvent) {
	formatter.AcceptLoggingEvent((*PlainTextEdgeSessionEvent)(event))
}

type PlainTextEdgeSessionEvent EdgeSessionEvent

func (event *PlainTextEdgeSessionEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*EdgeSessionEvent)(event).String() + "\n"))
	return err
}
