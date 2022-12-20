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

package event

import (
	"io"
	"regexp"
)

// A TypeRegistrar handles registering and unregistering handlers for a given event type
type TypeRegistrar interface {
	// Register takes a handler, which may implement multiple event handler
	// interfaces, and configure it using the configuration map provided
	Register(handler interface{}, config map[string]interface{}) error

	// Unregister will remove give handler, if implements the interface for
	// this event type and is registered to receive events of this type
	Unregister(handler interface{})
}

// A RegistrationHandler can take a handler, which may implement multiple event handler
// interfaces, and configure it using the configuration map provided
type RegistrationHandler func(handler interface{}, config map[string]interface{}) error

// A UnregistrationHandler will remove give handler, if implements the interface for
// this event type and is registered to receive events of this type
type UnregistrationHandler func(handler interface{})

// A HandlerFactory knows how to create a given event handler type using the provided
// configuration map
type HandlerFactory interface {
	NewEventHandler(config map[interface{}]interface{}) (interface{}, error)
}

// A FormattedEventSink accepts formatted events, i.e. events that have been turned into string or binary representations
type FormattedEventSink interface {
	AcceptFormattedEvent(eventType string, formattedEvent []byte)
}

// A FormatterFactory returns a formatter which will send events to the given FormattedEventSink
type FormatterFactory interface {
	NewFormatter(sink FormattedEventSink) io.Closer
}

// FormatterFactoryF is a function version of FormatterFactory
type FormatterFactoryF func(sink FormattedEventSink) io.Closer

func (self FormatterFactoryF) NewFormatter(sink FormattedEventSink) io.Closer {
	return self(sink)
}

// The Dispatcher interface manages handlers for a number of events as well as dispatching events
// to those handlers
type Dispatcher interface {
	RegisterEventType(eventType string, registrar TypeRegistrar)
	RegisterEventTypeFunctions(eventType string, registrationHandler RegistrationHandler, unregistrationHandler UnregistrationHandler)

	RegisterEventHandlerFactory(eventHandlerType string, factory HandlerFactory)
	RegisterFormatterFactory(formatterType string, factory FormatterFactory)

	GetFormatterFactory(formatterType string) FormatterFactory

	ProcessSubscriptions(handler interface{}, subscriptions []*Subscription) error
	RemoveAllSubscriptions(handler interface{})

	Dispatch(event Event)

	AddCircuitEventHandler(handler CircuitEventHandler)
	RemoveCircuitEventHandler(handler CircuitEventHandler)

	AddLinkEventHandler(handler LinkEventHandler)
	RemoveLinkEventHandler(handler LinkEventHandler)

	AddMetricsMapper(mapper MetricsMapper)

	AddMetricsEventHandler(handler MetricsEventHandler)
	RemoveMetricsEventHandler(handler MetricsEventHandler)

	AddMetricsMessageHandler(handler MetricsMessageHandler)
	RemoveMetricsMessageHandler(handler MetricsMessageHandler)
	NewFilteredMetricsAdapter(sourceFilter *regexp.Regexp, metricFilter *regexp.Regexp, handler MetricsEventHandler) MetricsMessageHandler

	AddRouterEventHandler(handler RouterEventHandler)
	RemoveRouterEventHandler(handler RouterEventHandler)

	AddServiceEventHandler(handler ServiceEventHandler)
	RemoveServiceEventHandler(handler ServiceEventHandler)

	AddTerminatorEventHandler(handler TerminatorEventHandler)
	RemoveTerminatorEventHandler(handler TerminatorEventHandler)

	AddUsageEventHandler(handler UsageEventHandler)
	RemoveUsageEventHandler(handler UsageEventHandler)

	CircuitEventHandler
	LinkEventHandler
	MetricsEventHandler
	MetricsMessageHandler
	RouterEventHandler
	ServiceEventHandler
	TerminatorEventHandler
	UsageEventHandler
}

type Event interface {
	Handle()
}

// A Subscription has information to configure an event handler. It contains the EventType to
// subscribe to and any optional configuration for the subscription. This might include things
// like event versions or event filtering
type Subscription struct {
	Type    string                 `json:"type"`
	Options map[string]interface{} `json:"options"`
}
