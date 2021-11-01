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

func (formatter *EdgeJsonFormatter) AcceptApiSessionEvent(event *ApiSessionEvent) {
	formatter.AcceptLoggingEvent((*JsonApiSessionEvent)(event))
}

func (formatter *EdgeJsonFormatter) AcceptSessionEvent(event *SessionEvent) {
	formatter.AcceptLoggingEvent((*JsonSessionEvent)(event))
}

func (formatter *EdgeJsonFormatter) AcceptEntityCountEvent(event *EntityCountEvent) {
	formatter.AcceptLoggingEvent((*JsonEntityCountEvent)(event))
}

type JsonSessionEvent SessionEvent

func (event *JsonSessionEvent) WriteTo(output io.WriteCloser) error {
	buf, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

type JsonApiSessionEvent ApiSessionEvent

func (event *JsonApiSessionEvent) WriteTo(output io.WriteCloser) error {
	buf, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = output.Write(buf)
	return err
}

type JsonEntityCountEvent EntityCountEvent

func (event *JsonEntityCountEvent) WriteTo(output io.WriteCloser) error {
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

func (formatter *EdgePlainTextFormatter) AcceptApiSessionEvent(event *ApiSessionEvent) {
	formatter.AcceptLoggingEvent((*PlainTextApiSessionEvent)(event))
}

func (formatter *EdgePlainTextFormatter) AcceptSessionEvent(event *SessionEvent) {
	formatter.AcceptLoggingEvent((*PlainTextSessionEvent)(event))
}

func (formatter *EdgePlainTextFormatter) AcceptEntityCountEvent(event *EntityCountEvent) {
	formatter.AcceptLoggingEvent((*PlainTextEntityCountEvent)(event))
}

type PlainTextApiSessionEvent ApiSessionEvent

func (event *PlainTextApiSessionEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*ApiSessionEvent)(event).String() + "\n"))
	return err
}

type PlainTextSessionEvent SessionEvent

func (event *PlainTextSessionEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*SessionEvent)(event).String() + "\n"))
	return err
}

type PlainTextEntityCountEvent EntityCountEvent

func (event *PlainTextEntityCountEvent) WriteTo(output io.WriteCloser) error {
	_, err := output.Write([]byte((*EntityCountEvent)(event).String() + "\n"))
	return err
}
