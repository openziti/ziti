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
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/event"
	"io"
	"strings"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/controller/network"
	"github.com/pkg/errors"
)

type delegatingRegistrar struct {
	RegistrationHandler   event.RegistrationHandler
	UnregistrationHandler event.UnregistrationHandler
}

func (self *delegatingRegistrar) Register(eventType string, handler interface{}, config map[string]interface{}) error {
	return self.RegistrationHandler(eventType, handler, config)
}

func (self *delegatingRegistrar) Unregister(handler interface{}) {
	self.UnregistrationHandler(handler)
}

func NewDispatcher(closeNotify <-chan struct{}) *Dispatcher {
	result := &Dispatcher{
		closeNotify: closeNotify,
		entityChangeEventsDispatcher: entityChangeEventDispatcher{
			closeNotify:    closeNotify,
			notifyCh:       make(chan struct{}, 1),
			globalMetadata: map[string]any{},
		},
	}
	result.entityChangeEventsDispatcher.dispatcher = result

	result.RegisterEventTypeFunctions("edge.apiSessions", result.registerApiSessionEventHandler, result.unregisterApiSessionEventHandler)
	result.RegisterEventTypeFunctions("fabric.circuits", result.registerCircuitEventHandler, result.unregisterCircuitEventHandler)
	result.RegisterEventTypeFunctions("edge.entityCounts", result.registerEntityCountEventHandler, result.unregisterEntityCountEventHandler)
	result.RegisterEventTypeFunctions("fabric.links", result.registerLinkEventHandler, result.unregisterLinkEventHandler)
	result.RegisterEventTypeFunctions("fabric.routers", result.registerRouterEventHandler, result.unregisterRouterEventHandler)
	result.RegisterEventTypeFunctions("services", result.registerServiceEventHandler, result.unregisterServiceEventHandler) // V2 Handler
	result.RegisterEventTypeFunctions("edge.sessions", result.registerSessionEventHandler, result.unregisterSessionEventHandler)
	result.RegisterEventTypeFunctions("fabric.terminators", result.registerTerminatorEventHandler, result.unregisterTerminatorEventHandler)
	result.RegisterEventTypeFunctions("fabric.usage", result.registerUsageEventHandler, result.unregisterUsageEventHandler)
	result.RegisterEventTypeFunctions("edge.authentications", result.registerAuthenticationEventHandler, result.unregisterAuthenticationEventHandler)

	result.RegisterEventTypeFunctions(event.ApiSessionEventNS, result.registerApiSessionEventHandler, result.unregisterApiSessionEventHandler)
	result.RegisterEventTypeFunctions(event.AuthenticationEventNS, result.registerAuthenticationEventHandler, result.unregisterAuthenticationEventHandler)
	result.RegisterEventTypeFunctions(event.CircuitEventNS, result.registerCircuitEventHandler, result.unregisterCircuitEventHandler)
	result.RegisterEventTypeFunctions(event.ClusterEventNS, result.registerClusterEventHandler, result.unregisterClusterEventHandler)
	result.RegisterEventTypeFunctions(event.ConnectEventNS, result.registerConnectEventHandler, result.unregisterConnectEventHandler)
	result.RegisterEventTypeFunctions(event.EntityChangeEventNS, result.registerEntityChangeEventHandler, result.unregisterEntityChangeEventHandler)
	result.RegisterEventTypeFunctions(event.EntityCountEventNS, result.registerEntityCountEventHandler, result.unregisterEntityCountEventHandler)
	result.RegisterEventTypeFunctions(event.LinkEventNS, result.registerLinkEventHandler, result.unregisterLinkEventHandler)
	result.RegisterEventTypeFunctions(event.MetricsEventNS, result.registerMetricsEventHandler, result.unregisterMetricsEventHandler)
	result.RegisterEventTypeFunctions(event.RouterEventNS, result.registerRouterEventHandler, result.unregisterRouterEventHandler)
	result.RegisterEventTypeFunctions(event.ServiceEventNS, result.registerServiceEventHandler, result.unregisterServiceEventHandler)
	result.RegisterEventTypeFunctions(event.SessionEventNS, result.registerSessionEventHandler, result.unregisterSessionEventHandler)
	result.RegisterEventTypeFunctions(event.SdkEventNS, result.registerSdkEventHandler, result.unregisterSdkEventHandler)
	result.RegisterEventTypeFunctions(event.TerminatorEventNS, result.registerTerminatorEventHandler, result.unregisterTerminatorEventHandler)
	result.RegisterEventTypeFunctions(event.UsageEventNS, result.registerUsageEventHandler, result.unregisterUsageEventHandler)

	result.RegisterFormatterFactory("json", event.FormatterFactoryF(func(sink event.FormattedEventSink) io.Closer {
		return NewJsonFormatter(16, sink)
	}))

	result.RegisterEventHandlerFactory("file", FileEventLoggerFactory{})
	result.RegisterEventHandlerFactory("stdout", StdOutLoggerFactory{})
	result.RegisterEventHandlerFactory("amqp", AMQPEventLoggerFactory{})

	return result
}

var _ event.Dispatcher = (*Dispatcher)(nil)

type Dispatcher struct {
	ctrlId                    string
	circuitEventHandlers      concurrenz.CopyOnWriteSlice[event.CircuitEventHandler]
	entityChangeEventHandlers concurrenz.CopyOnWriteSlice[event.EntityChangeEventHandler]
	linkEventHandlers         concurrenz.CopyOnWriteSlice[event.LinkEventHandler]
	metricsEventHandlers      concurrenz.CopyOnWriteSlice[event.MetricsEventHandler]
	metricsMsgEventHandlers   concurrenz.CopyOnWriteSlice[event.MetricsMessageHandler]
	routerEventHandlers       concurrenz.CopyOnWriteSlice[event.RouterEventHandler]
	serviceEventHandlers      concurrenz.CopyOnWriteSlice[event.ServiceEventHandler]
	terminatorEventHandlers   concurrenz.CopyOnWriteSlice[event.TerminatorEventHandler]
	usageEventHandlers        concurrenz.CopyOnWriteSlice[event.UsageEventHandler]
	usageEventV3Handlers      concurrenz.CopyOnWriteSlice[event.UsageEventV3Handler]
	clusterEventHandlers      concurrenz.CopyOnWriteSlice[event.ClusterEventHandler]
	connectEventHandlers      concurrenz.CopyOnWriteSlice[event.ConnectEventHandler]
	sdkEventHandlers          concurrenz.CopyOnWriteSlice[event.SdkEventHandler]

	authenticationEventHandlers concurrenz.CopyOnWriteSlice[event.AuthenticationEventHandler]
	apiSessionEventHandlers     concurrenz.CopyOnWriteSlice[event.ApiSessionEventHandler]
	entityCountEventHandlers    concurrenz.CopyOnWriteSlice[*entityCountState]
	sessionEventHandlers        concurrenz.CopyOnWriteSlice[event.SessionEventHandler]

	metricsMappers concurrenz.CopyOnWriteSlice[event.MetricsMapper]

	registrationHandlers  concurrenz.CopyOnWriteMap[string, event.TypeRegistrar]
	eventHandlerFactories concurrenz.CopyOnWriteMap[string, event.HandlerFactory]
	formatterFactories    concurrenz.CopyOnWriteMap[string, event.FormatterFactory]

	network *network.Network
	stores  *db.Stores

	entityChangeEventsDispatcher entityChangeEventDispatcher
	entityTypes                  []string
	closeNotify                  <-chan struct{}
}

func (self *Dispatcher) InitializeNetworkEvents(n *network.Network) {
	self.network = n
	self.ctrlId = n.GetAppId()
	self.initMetricsEvents(n)
	self.initRouterEvents(n)
	self.initServiceEvents(n)
	self.initTerminatorEvents(n)
	self.initUsageEvents()
	self.initEntityChangeEvents(n)

	self.AddMetricsMapper(ctrlChannelMetricsMapper{}.mapMetrics)
	self.AddMetricsMapper((&linkMetricsMapper{network: n}).mapMetrics)
}

func (self *Dispatcher) InitializeEdgeEvents(stores *db.Stores) {
	self.stores = stores
	self.initApiSessionEvents(self.stores)
	self.initSessionEvents(self.stores)
	self.initEntityEvents()

	fabricStores := map[boltz.Store]struct{}{}

	for _, store := range self.network.GetStores().GetStoreList() {
		fabricStores[store] = struct{}{}
	}

	for _, store := range self.stores.GetStores() {
		if _, found := fabricStores[store]; !found {
			self.AddEntityChangeSource(store)
		}
	}
}

func (self *Dispatcher) AddMetricsMapper(mapper event.MetricsMapper) {
	self.metricsMappers.Append(mapper)
}

func (self *Dispatcher) RegisterEventType(eventType string, typeRegistrar event.TypeRegistrar) {
	self.registrationHandlers.Put(eventType, typeRegistrar)
}

func (self *Dispatcher) RegisterEventTypeFunctions(eventType string,
	registrationHandler event.RegistrationHandler,
	unregistrationHandler event.UnregistrationHandler) {
	self.RegisterEventType(eventType, &delegatingRegistrar{
		RegistrationHandler:   registrationHandler,
		UnregistrationHandler: unregistrationHandler,
	})
}

func (self *Dispatcher) RegisterEventHandlerFactory(eventHandlerType string, factory event.HandlerFactory) {
	self.eventHandlerFactories.Put(eventHandlerType, factory)
}

func (self *Dispatcher) GetFormatterFactory(formatType string) event.FormatterFactory {
	return self.formatterFactories.Get(formatType)
}

func (self *Dispatcher) RegisterFormatterFactory(formatType string, factory event.FormatterFactory) {
	self.formatterFactories.Put(formatType, factory)
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
	self.entityChangeEventsDispatcher.flushCommittedTxEvents(true)
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
	subs, ok := eventHandlerConfig.Config["subscriptions"]

	if !ok {
		return errors.Errorf("event handler %v doesn't define any subscriptions", eventHandlerConfig.Id)
	}

	subscriptionList, ok := subs.([]interface{})
	if !ok {
		return errors.Errorf("event handler %v subscriptions is not a list", eventHandlerConfig.Id)
	}

	var subscriptions []*event.Subscription

	for idx, sub := range subscriptionList {
		subMap, ok := sub.(map[interface{}]interface{})
		if !ok {
			return errors.Errorf("The subscription at index %v for event handler %v is not a map", idx, eventHandlerConfig.Id)
		}

		var eventType string
		var options map[string]interface{}

		for k, v := range subMap {
			if k == "type" {
				eventType = fmt.Sprintf("%v", v)
			} else {
				if options == nil {
					options = map[string]interface{}{}
				}
				options[fmt.Sprintf("%v", k)] = v
			}
		}

		if eventType == "" {
			return errors.Errorf("The subscription at index %v for event handler %v has no type", idx, eventHandlerConfig.Id)
		}

		subscriptions = append(subscriptions, &event.Subscription{
			Type:    eventType,
			Options: options,
		})
	}
	return self.ProcessSubscriptions(handler, subscriptions)
}

func (self *Dispatcher) ProcessSubscriptions(handler interface{}, subscriptions []*event.Subscription) error {
	logger := pfxlog.Logger()
	eventTypes := self.registrationHandlers.AsMap()

	for _, sub := range subscriptions {
		logger.WithField("type", sub.Type).Info("Processing subscriptions for event type")

		if registrar, ok := eventTypes[sub.Type]; ok {
			if err := registrar.Register(sub.Type, handler, sub.Options); err != nil {
				return err
			}
			logger.WithField("type", sub.Type).Info("Registration of event handler succeeded")
		} else {
			var validTypes []string
			for k := range eventTypes {
				validTypes = append(validTypes, k)
			}
			logger.WithField("type", sub.Type).Warnf("invalid event type. valid types are %v", strings.Join(validTypes, ","))
		}
	}
	return nil
}

func (self *Dispatcher) RemoveAllSubscriptions(handler interface{}) {
	for _, registrar := range self.registrationHandlers.AsMap() {
		registrar.Unregister(handler)
	}
}

type EventHandlerConfig struct {
	Id     interface{}
	Config map[interface{}]interface{}
}
