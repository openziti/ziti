package events

import (
	"fmt"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/identity/identity"
	"github.com/pkg/errors"
	"reflect"
	"time"
)

const SessionEventTypeCreated = "created"
const SessionEventTypeDeleted = "deleted"
const SessionEventTypeCircuitUpdated = "circuitUpdated"

func registerSessionEventHandler(val interface{}, config map[interface{}]interface{}) error {
	handler, ok := val.(SessionEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/events/SessionEventHandler interface.", reflect.TypeOf(val))
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
			return errors.Errorf("invalid type %v for fabric.sessions include configuration", reflect.TypeOf(includeVar))
		}
	}

	adapter := &sessionEventAdapter{
		handler: handler,
	}

	if len(includeList) == 0 {
		AddSessionEventHandler(adapter)
	} else {
		for _, include := range includeList {
			if include == SessionEventTypeCreated {
				AddSessionEventHandler(&fabricSessionCreatedEventAdapter{
					wrapped: adapter,
				})
			} else if include == SessionEventTypeDeleted {
				AddSessionEventHandler(&fabricSessionDeletedEventAdapter{
					wrapped: adapter,
				})
			} else if include == SessionEventTypeCircuitUpdated {
				AddSessionEventHandler(&fabricSessionCircuitUpdatedEventAdapter{
					wrapped: adapter,
				})
			} else {
				return errors.Errorf("invalid include %v for fabric.sessions. valid values are ['created', 'deleted', 'circuitUpdated']", include)
			}
		}
	}

	return nil
}

type fabricSessionCreatedEventAdapter struct {
	wrapped network.SessionEventHandler
}

func (adapter *fabricSessionCreatedEventAdapter) SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, circuit *network.Circuit) {
	adapter.wrapped.SessionCreated(sessionId, clientId, serviceId, circuit)
}

func (adapter *fabricSessionCreatedEventAdapter) SessionDeleted(*identity.TokenId, *identity.TokenId) {
}

func (adapter *fabricSessionCreatedEventAdapter) CircuitUpdated(*identity.TokenId, *network.Circuit) {
}

type fabricSessionDeletedEventAdapter struct {
	wrapped network.SessionEventHandler
}

func (adapter *fabricSessionDeletedEventAdapter) SessionCreated(*identity.TokenId, *identity.TokenId, string, *network.Circuit) {
}

func (adapter *fabricSessionDeletedEventAdapter) SessionDeleted(sessionId *identity.TokenId, clientId *identity.TokenId) {
	adapter.wrapped.SessionDeleted(sessionId, clientId)
}

func (adapter *fabricSessionDeletedEventAdapter) CircuitUpdated(*identity.TokenId, *network.Circuit) {
}

type fabricSessionCircuitUpdatedEventAdapter struct {
	wrapped network.SessionEventHandler
}

func (adapter *fabricSessionCircuitUpdatedEventAdapter) SessionCreated(*identity.TokenId, *identity.TokenId, string, *network.Circuit) {
}

func (adapter *fabricSessionCircuitUpdatedEventAdapter) SessionDeleted(*identity.TokenId, *identity.TokenId) {
}

func (adapter *fabricSessionCircuitUpdatedEventAdapter) CircuitUpdated(sessionId *identity.TokenId, circuit *network.Circuit) {
	adapter.wrapped.CircuitUpdated(sessionId, circuit)
}

// Will work for all fabric session event types
type SessionEvent struct {
	Namespace string    `json:"namespace"`
	EventType string    `json:"event_type"`
	SessionId string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	ClientId  string    `json:"client_id"`
	ServiceId string    `json:"service_id"`
	Circuit   string    `json:"circuit"`
}

func (event *SessionEvent) String() string {
	return fmt.Sprintf("%v.%v sessionId=%v clientId=%v serviceId=%v circuit=%v",
		event.Namespace, event.EventType, event.SessionId, event.ClientId, event.ServiceId, event.Circuit)
}

type SessionEventHandler interface {
	AcceptSessionEvent(event *SessionEvent)
}

func RegisterSessionEventHandler(handler SessionEventHandler) func() {
	adapter := &sessionEventAdapter{
		handler: handler,
	}

	AddSessionEventHandler(adapter)

	return func() {
		RemoveSessionEventHandler(adapter)
	}
}

type sessionEventAdapter struct {
	handler SessionEventHandler
}

func (adapter *sessionEventAdapter) SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, circuit *network.Circuit) {
	event := &SessionEvent{
		Namespace: "fabric.sessions",
		EventType: "created",
		SessionId: sessionId.Token,
		Timestamp: time.Now(),
		ClientId:  clientId.Token,
		ServiceId: serviceId,
		Circuit:   circuit.String(),
	}

	adapter.handler.AcceptSessionEvent(event)
}

func (adapter *sessionEventAdapter) SessionDeleted(sessionId *identity.TokenId, clientId *identity.TokenId) {
	event := &SessionEvent{
		Namespace: "fabric.sessions",
		EventType: "deleted",
		SessionId: sessionId.Token,
		Timestamp: time.Now(),
		ClientId:  clientId.Token,
	}

	adapter.handler.AcceptSessionEvent(event)
}

func (adapter *sessionEventAdapter) CircuitUpdated(sessionId *identity.TokenId, circuit *network.Circuit) {
	event := &SessionEvent{
		Namespace: "fabric.sessions",
		EventType: "circuitUpdated",
		SessionId: sessionId.Token,
		Timestamp: time.Now(),
		Circuit:   circuit.String(),
	}

	adapter.handler.AcceptSessionEvent(event)
}
