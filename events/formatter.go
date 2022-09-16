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
	"github.com/openziti/fabric/events"
	"github.com/pkg/errors"
	"io"
	"strings"
)

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

func (event *JsonSessionEvent) WriteTo(output io.Writer) error {
	return marshalJson(event, output)
}

type JsonApiSessionEvent ApiSessionEvent

func (event *JsonApiSessionEvent) WriteTo(output io.Writer) error {
	return marshalJson(event, output)
}

type JsonEntityCountEvent EntityCountEvent

func (event *JsonEntityCountEvent) WriteTo(output io.Writer) error {
	return marshalJson(event, output)
}

func marshalJson(v interface{}, output io.Writer) error {
	buf, err := json.Marshal(v)
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

func (event *PlainTextApiSessionEvent) WriteTo(output io.Writer) error {
	_, err := output.Write([]byte((*ApiSessionEvent)(event).String() + "\n"))
	return err
}

type PlainTextSessionEvent SessionEvent

func (event *PlainTextSessionEvent) WriteTo(output io.Writer) error {
	_, err := output.Write([]byte((*SessionEvent)(event).String() + "\n"))
	return err
}

type PlainTextEntityCountEvent EntityCountEvent

func (event *PlainTextEntityCountEvent) WriteTo(output io.Writer) error {
	_, err := output.Write([]byte((*EntityCountEvent)(event).String() + "\n"))
	return err
}
