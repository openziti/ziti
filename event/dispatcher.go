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

import "regexp"

// A RegistrationHandler can take a handler, which may implement multiple event handler
// interfaces, and configure it using the configuration map provided
type RegistrationHandler func(handler interface{}, config map[interface{}]interface{}) error

// A HandlerFactory knows how to create a given event handler type using the provided
// configuration map
type HandlerFactory interface {
	NewEventHandler(config map[interface{}]interface{}) (interface{}, error)
}

// The Dispatcher interface manages handlers for a number of events as well as dispatching events
// to those handlers
type Dispatcher interface {
	RegisterEventType(eventType string, registrationHandler RegistrationHandler)
	RegisterEventHandlerFactory(eventHandlerType string, factory HandlerFactory)

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
