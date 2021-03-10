package events

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/metrics"
	"github.com/pkg/errors"
	"strings"
	"sync"
)

type registrationHandler func(handler interface{}, config map[interface{}]interface{}) error

var registryLock = &sync.Mutex{}
var eventTypes = map[string]registrationHandler{}
var eventHandlerTypeFactories = map[string]EventHandlerFactory{}
var eventHandlerConfigs []*eventHandlerConfig

type EventHandlerFactory interface {
	NewEventHandler(config map[interface{}]interface{}) (interface{}, error)
}

type eventHandlerConfig struct {
	id     interface{}
	config map[interface{}]interface{}
}

func (eventHandlerConfig *eventHandlerConfig) createHandler() (interface{}, error) {
	handlerVal, ok := eventHandlerConfig.config["handler"]
	if !ok {
		return nil, errors.Errorf("no event handler defined for %v", eventHandlerConfig.id)
	}

	handlerMap, ok := handlerVal.(map[interface{}]interface{})
	if !ok {
		return nil, errors.Errorf("event configuration for %v is not a map", eventHandlerConfig.id)
	}

	handlerTypeVal, ok := handlerMap["type"]
	if !ok {
		return nil, errors.Errorf("no handler type for %v provided", eventHandlerConfig.id)
	}

	handlerType := fmt.Sprintf("%v", handlerTypeVal)
	pfxlog.Logger().Infof("Create handler of type: %s", handlerType)

	handlerFactory, ok := eventHandlerTypeFactories[handlerType]
	if !ok {
		return nil, errors.Errorf("invalid handler type %v for handler %v provided", handlerType, eventHandlerConfig.id)
	}

	return handlerFactory.NewEventHandler(handlerMap)
}

func (eventHandlerConfig *eventHandlerConfig) processSubscriptions(handler interface{}) error {
	logger := pfxlog.Logger()

	subs, ok := eventHandlerConfig.config["subscriptions"]

	if !ok {
		return errors.Errorf("event handler %v doesn't define any subscriptions", eventHandlerConfig.id)
	}

	subscriptionList, ok := subs.([]interface{})
	if !ok {
		return errors.Errorf("event handler %v subscriptions is not a list", eventHandlerConfig.id)
	}

	for idx, sub := range subscriptionList {
		subMap, ok := sub.(map[interface{}]interface{})
		if !ok {
			return errors.Errorf("The subscription at index %v for event handler %v is not a map", idx, eventHandlerConfig.id)
		}
		eventTypeVal, ok := subMap["type"]

		if !ok {
			return errors.Errorf("The subscription at index %v for event handler %v has no type", idx, eventHandlerConfig.id)
		}

		logger.Infof("Processing subscriptions for event type: %s", eventTypeVal)
		eventType := fmt.Sprintf("%v", eventTypeVal)

		if registrationHandler, ok := eventTypes[eventType]; ok {
			if err := registrationHandler(handler, subMap); err != nil {
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

func RegisterEventType(eventType string, registrationHandler registrationHandler) {
	registryLock.Lock()
	defer registryLock.Unlock()
	eventTypes[eventType] = registrationHandler
}

func RegisterEventHandlerType(eventHandlerType string, factory EventHandlerFactory) {
	registryLock.Lock()
	defer registryLock.Unlock()
	eventHandlerTypeFactories[eventHandlerType] = factory
}

func RegisterEventHandler(id interface{}, config map[interface{}]interface{}) {
	registryLock.Lock()
	defer registryLock.Unlock()
	eventHandlerConfigs = append(eventHandlerConfigs, &eventHandlerConfig{
		id:     id,
		config: config,
	})
}

/**
Example configuration:
events:
  jsonLogger:
    subscriptions:
      - type: metrics
        sourceFilter: .*
        metricFilter: .*xgress.*tx*.m1_rate
      - type: fabric.sessions
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
func WireEventHandlers(serviceEventInitializer func(handler metrics.Handler)) error {
	registryLock.Lock()
	defer registryLock.Unlock()

	serviceEventInitializer(serviceEventAdapter{})

	logger := pfxlog.Logger()
	for _, eventHandlerConfig := range eventHandlerConfigs {
		handler, err := eventHandlerConfig.createHandler()
		if err != nil {
			logger.Errorf("Unable to create event handler: %v", err)
			return err
		}
		if err := eventHandlerConfig.processSubscriptions(handler); err != nil {
			logger.Errorf("Unable to process subscription for event handler: %v", err)
			return err
		}
	}

	eventTypes = nil
	eventHandlerTypeFactories = nil
	eventHandlerConfigs = nil

	return nil
}
