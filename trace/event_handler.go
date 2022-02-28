/*
	Copyright NetFoundry, Inc.

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

package trace

import (
	"github.com/openziti/channel/trace/pb"
	"github.com/openziti/fabric/event"
	"github.com/openziti/foundation/util/cowslice"
)

var EventHandlerRegistry = cowslice.NewCowSlice(make([]EventHandler, 0))

func getTraceEventHandlers() []EventHandler {
	return EventHandlerRegistry.Value().([]EventHandler)
}

// EventHandler is for types wishing to receive trace messages
type EventHandler interface {
	Accept(event *trace_pb.ChannelMessage)
}

type eventWrapper struct {
	wrapped *trace_pb.ChannelMessage
}

func (event *eventWrapper) Handle() {
	handlers := getTraceEventHandlers()
	for _, handler := range handlers {
		handler.Accept(event.wrapped)
	}
}

func NewDispatchWrapper(delegate func(event event.Event)) EventHandler {
	return &DispatchWrapper{
		delegate: delegate,
	}
}

type DispatchWrapper struct {
	delegate func(event event.Event)
}

func (dispatchWrapper *DispatchWrapper) Accept(event *trace_pb.ChannelMessage) {
	eventWrapper := &eventWrapper{wrapped: event}
	dispatchWrapper.delegate(eventWrapper)
}
