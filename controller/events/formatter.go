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
	"github.com/openziti/fabric/events"
	"github.com/pkg/errors"
	"io"
	"strings"
)

type edgeFormatterFactory struct{}

func (f edgeFormatterFactory) NewLoggingHandler(format string, buffer int, out io.WriteCloser) (interface{}, error) {
	if strings.EqualFold(format, "json") {
		result := &EdgeJsonFormatter{
			JsonFormatter: *events.NewJsonFormatter(buffer, events.NewWriterEventSink(out)),
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

func (event *JsonSessionEvent) GetEventType() string {
	return "session"
}

func (event *JsonSessionEvent) Format() ([]byte, error) {
	return events.MarshalJson(event)
}

type JsonApiSessionEvent ApiSessionEvent

func (event *JsonApiSessionEvent) GetEventType() string {
	return "apiSession"
}

func (event *JsonApiSessionEvent) Format() ([]byte, error) {
	return events.MarshalJson(event)
}

type JsonEntityCountEvent EntityCountEvent

func (event *JsonEntityCountEvent) GetEventType() string {
	return "entityCount"
}

func (event *JsonEntityCountEvent) Format() ([]byte, error) {
	return events.MarshalJson(event)
}
