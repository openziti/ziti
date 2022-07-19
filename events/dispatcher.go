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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/event"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/pkg/errors"
	"strings"
)

func NewDispatcher(closeNotify <-chan struct{}) *Dispatcher {
	result := &Dispatcher{
		closeNotify: closeNotify,
		eventC:      make(chan event.Event, 25),
	}

	result.RegisterEventType(event.CircuitEventsNs, result.registerCircuitEventHandler)
	result.RegisterEventType(event.LinkEventsNs, result.registerLinkEventHandler)
	result.RegisterEventType(event.MetricsEventsNs, result.registerMetricsEventHandler)
	result.RegisterEventType(event.RouterEventsNs, result.registerRouterEventHandler)
	result.RegisterEventType(event.ServiceEventsNs, result.registerServiceEventHandler)
	result.RegisterEventType(event.TerminatorEventsNs, result.registerTerminatorEventHandler)
	result.RegisterEventType(event.UsageEventsNs, result.registerUsageEventHandler)

	result.RegisterEventHandlerFactory("file", FileEventLoggerFactory{})
	result.RegisterEventHandlerFactory("stdout", StdOutLoggerFactory{})

	go result.eventLoop()

	return result
}

type Dispatcher struct {
	circuitEventHandlers    concurrenz.CopyOnWriteSlice[event.CircuitEventHandler]
	linkEventHandlers       concurrenz.CopyOnWriteSlice[event.LinkEventHandler]
	metricsEventHandlers    concurrenz.CopyOnWriteSlice[event.MetricsEventHandler]
	metricsMsgEventHandlers concurrenz.CopyOnWriteSlice[event.MetricsMessageHandler]
	routerEventHandlers     concurrenz.CopyOnWriteSlice[event.RouterEventHandler]
	serviceEventHandlers    concurrenz.CopyOnWriteSlice[event.ServiceEventHandler]
	terminatorEventHandlers concurrenz.CopyOnWriteSlice[event.TerminatorEventHandler]
	usageEventHandlers      concurrenz.CopyOnWriteSlice[event.UsageEventHandler]

	metricsMappers concurrenz.CopyOnWriteSlice[event.MetricsMapper]

	registrationHandlers  concurrenz.CopyOnWriteMap[string, event.RegistrationHandler]
	eventHandlerFactories concurrenz.CopyOnWriteMap[string, event.HandlerFactory]

	closeNotify <-chan struct{}
	eventC      chan event.Event
}

func (self *Dispatcher) InitializeNetworkEvents(n *network.Network) {
	self.initMetricsEvents(n)
	self.initRouterEvents(n)
	self.initServiceEvents(n)
	self.initTerminatorEvents(n)
	self.initUsageEvents()

	self.AddMetricsMapper(ctrlChannelMetricsMapper{}.mapMetrics)
	self.AddMetricsMapper((&linkMetricsMapper{network: n}).mapMetrics)
}

func (self *Dispatcher) AddMetricsMapper(mapper event.MetricsMapper) {
	self.metricsMappers.Append(mapper)
}

func (self *Dispatcher) eventLoop() {
	pfxlog.Logger().Info("event dispatcher: started")
	defer pfxlog.Logger().Info("event dispatcher: stopped")

	for {
		select {
		case evt := <-self.eventC:
			evt.Handle()
		case <-self.closeNotify:
			return
		}
	}
}

func (self *Dispatcher) Dispatch(event event.Event) {
	select {
	case self.eventC <- event:
	case <-self.closeNotify:
	}
}

func (self *Dispatcher) RegisterEventType(eventType string, registrationHandler event.RegistrationHandler) {
	self.registrationHandlers.Put(eventType, registrationHandler)
}

func (self *Dispatcher) RegisterEventHandlerFactory(eventHandlerType string, factory event.HandlerFactory) {
	self.eventHandlerFactories.Put(eventHandlerType, factory)
}

// WireEventHandlers takes the given handler configs and creates handlers and subscriptions for each of them.
/**
Example configuration:
events:
  jsonLogger:
    subscriptions:
      - type: metrics
        sourceFilter: .*
        metricFilter: .*xgress.*tx*.m1_rate
      - type: fabric.circuits
        include:
          - created
      - type: edge.sessions
        include:
          - created
    handler:
      type: file
      format: json
      path: /tmp/ziti-events.log

*/
func (self *Dispatcher) WireEventHandlers(eventHandlerConfigs []*EventHandlerConfig) error {
	logger := pfxlog.Logger()
	for _, eventHandlerConfig := range eventHandlerConfigs {
		handler, err := self.createHandler(eventHandlerConfig.Id, eventHandlerConfig.Config)
		if err != nil {
			logger.Errorf("Unable to create event handler: %v", err)
			return err
		}
		if err = self.processSubscriptions(handler, eventHandlerConfig); err != nil {
			logger.Errorf("Unable to process subscription for event handler: %v", err)
			return err
		}
	}
	return nil
}

func (self *Dispatcher) createHandler(id interface{}, config map[interface{}]interface{}) (interface{}, error) {
	handlerVal, ok := config["handler"]
	if !ok {
		return nil, errors.Errorf("no event handler defined for %v", id)
	}

	handlerMap, ok := handlerVal.(map[interface{}]interface{})
	if !ok {
		return nil, errors.Errorf("event configuration for %v is not a map", id)
	}

	handlerTypeVal, ok := handlerMap["type"]
	if !ok {
		return nil, errors.Errorf("no handler type for %v provided", id)
	}

	handlerType := fmt.Sprintf("%v", handlerTypeVal)
	pfxlog.Logger().Infof("Create handler of type: %s", handlerType)

	handlerFactory := self.eventHandlerFactories.Get(handlerType)
	if handlerFactory == nil {
		return nil, errors.Errorf("invalid handler type %v for handler %v provided", handlerType, id)
	}

	return handlerFactory.NewEventHandler(handlerMap)
}

func (self *Dispatcher) processSubscriptions(handler interface{}, eventHandlerConfig *EventHandlerConfig) error {
	logger := pfxlog.Logger()

	subs, ok := eventHandlerConfig.Config["subscriptions"]

	if !ok {
		return errors.Errorf("event handler %v doesn't define any subscriptions", eventHandlerConfig.Id)
	}

	subscriptionList, ok := subs.([]interface{})
	if !ok {
		return errors.Errorf("event handler %v subscriptions is not a list", eventHandlerConfig.Id)
	}

	eventTypes := self.registrationHandlers.AsMap()

	for idx, sub := range subscriptionList {
		subMap, ok := sub.(map[interface{}]interface{})
		if !ok {
			return errors.Errorf("The subscription at index %v for event handler %v is not a map", idx, eventHandlerConfig.Id)
		}
		eventTypeVal, ok := subMap["type"]

		if !ok {
			return errors.Errorf("The subscription at index %v for event handler %v has no type", idx, eventHandlerConfig.Id)
		}

		logger.Infof("Processing subscriptions for event type: %s", eventTypeVal)
		eventType := fmt.Sprintf("%v", eventTypeVal)

		if regHandler, ok := eventTypes[eventType]; ok {
			if err := regHandler(handler, subMap); err != nil {
				return err
			}
			logger.Infof("Registration of event handler %s succeeded", eventTypeVal)
		} else {
			var validTypes []string
			for k := range eventTypes {
				validTypes = append(validTypes, k)
			}
			logger.Warnf("invalid event type %v. valid types are %v", eventType, strings.Join(validTypes, ","))
		}
	}
	return nil
}

type EventHandlerConfig struct {
	Id     interface{}
	Config map[interface{}]interface{}
}
