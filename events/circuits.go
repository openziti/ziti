package events

import (
	"fmt"
	"github.com/openziti/fabric/controller/network"
	"github.com/pkg/errors"
	"reflect"
	"time"
)

const CircuitEventTypeCreated = "created"
const CircuitEventTypeDeleted = "deleted"
const CircuitEventTypePathUpdated = "pathUpdated"

func registerCircuitEventHandler(val interface{}, config map[interface{}]interface{}) error {
	handler, ok := val.(CircuitEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/events/CircuitEventHandler interface.", reflect.TypeOf(val))
	}

	var includeList []string
	if includeVar, ok := config["include"]; ok {
		if includeStr, ok := includeVar.(string); ok {
			includeList = append(includeList, includeStr)
		} else if includeIntfList, ok := includeVar.([]interface{}); ok {
			for _, val := range includeIntfList {
				includeList = append(includeList, fmt.Sprintf("%v", val))
			}
		} else {
			return errors.Errorf("invalid type %v for fabric.circuits include configuration", reflect.TypeOf(includeVar))
		}
	}

	adapter := &circuitEventAdapter{
		handler: handler,
	}

	if len(includeList) == 0 {
		AddCircuitEventHandler(adapter)
	} else {
		for _, include := range includeList {
			if include == CircuitEventTypeCreated {
				AddCircuitEventHandler(&fabricCircuitCreatedEventAdapter{
					wrapped: adapter,
				})
			} else if include == CircuitEventTypeDeleted {
				AddCircuitEventHandler(&fabricCircuitDeletedEventAdapter{
					wrapped: adapter,
				})
			} else if include == CircuitEventTypePathUpdated {
				AddCircuitEventHandler(&fabricCircuitPathUpdatedEventAdapter{
					wrapped: adapter,
				})
			} else {
				return errors.Errorf("invalid include %v for fabric.circuits. valid values are ['created', 'deleted', 'circuitUpdated']", include)
			}
		}
	}

	return nil
}

type fabricCircuitCreatedEventAdapter struct {
	wrapped network.CircuitEventHandler
}

func (adapter *fabricCircuitCreatedEventAdapter) CircuitCreated(circuitId string, clientId string, serviceId string, circuit *network.Path) {
	adapter.wrapped.CircuitCreated(circuitId, clientId, serviceId, circuit)
}

func (adapter *fabricCircuitCreatedEventAdapter) CircuitDeleted(string, string) {
}

func (adapter *fabricCircuitCreatedEventAdapter) PathUpdated(string, *network.Path) {
}

type fabricCircuitDeletedEventAdapter struct {
	wrapped network.CircuitEventHandler
}

func (adapter *fabricCircuitDeletedEventAdapter) CircuitCreated(string, string, string, *network.Path) {
}

func (adapter *fabricCircuitDeletedEventAdapter) CircuitDeleted(circuitId string, clientId string) {
	adapter.wrapped.CircuitDeleted(circuitId, clientId)
}

func (adapter *fabricCircuitDeletedEventAdapter) PathUpdated(string, *network.Path) {
}

type fabricCircuitPathUpdatedEventAdapter struct {
	wrapped network.CircuitEventHandler
}

func (adapter *fabricCircuitPathUpdatedEventAdapter) CircuitCreated(string, string, string, *network.Path) {
}

func (adapter *fabricCircuitPathUpdatedEventAdapter) CircuitDeleted(string, string) {
}

func (adapter *fabricCircuitPathUpdatedEventAdapter) PathUpdated(circuitId string, path *network.Path) {
	adapter.wrapped.PathUpdated(circuitId, path)
}

// Will work for all fabric circuit event types
type CircuitEvent struct {
	Namespace string    `json:"namespace"`
	EventType string    `json:"event_type"`
	CircuitId string    `json:"circuit_id"`
	Timestamp time.Time `json:"timestamp"`
	ClientId  string    `json:"client_id"`
	ServiceId string    `json:"service_id"`
	Path      string    `json:"circuit"`
}

func (event *CircuitEvent) String() string {
	return fmt.Sprintf("%v.%v circuitId=%v clientId=%v serviceId=%v path=%v",
		event.Namespace, event.EventType, event.CircuitId, event.ClientId, event.ServiceId, event.Path)
}

type CircuitEventHandler interface {
	AcceptCircuitEvent(event *CircuitEvent)
}

func RegisterCircuitEventHandler(handler CircuitEventHandler) func() {
	adapter := &circuitEventAdapter{
		handler: handler,
	}

	AddCircuitEventHandler(adapter)

	return func() {
		RemoveCircuitEventHandler(adapter)
	}
}

type circuitEventAdapter struct {
	handler CircuitEventHandler
}

func (adapter *circuitEventAdapter) CircuitCreated(circuitId string, clientId string, serviceId string, path *network.Path) {
	event := &CircuitEvent{
		Namespace: "fabric.circuits",
		EventType: "created",
		CircuitId: circuitId,
		Timestamp: time.Now(),
		ClientId:  clientId,
		ServiceId: serviceId,
		Path:      path.String(),
	}

	adapter.handler.AcceptCircuitEvent(event)
}

func (adapter *circuitEventAdapter) CircuitDeleted(circuitId string, clientId string) {
	event := &CircuitEvent{
		Namespace: "fabric.circuits",
		EventType: "deleted",
		CircuitId: circuitId,
		Timestamp: time.Now(),
		ClientId:  clientId,
	}

	adapter.handler.AcceptCircuitEvent(event)
}

func (adapter *circuitEventAdapter) PathUpdated(circuitId string, path *network.Path) {
	event := &CircuitEvent{
		Namespace: "fabric.circuits",
		EventType: "pathUpdated",
		CircuitId: circuitId,
		Timestamp: time.Now(),
		Path:      path.String(),
	}

	adapter.handler.AcceptCircuitEvent(event)
}
