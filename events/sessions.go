package events

import (
	"fmt"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/identity/identity"
	"github.com/pkg/errors"
	"reflect"
)


const	SessionEventTypeCreated = "created"
const	SessionEventTypeDeleted = "deleted"
const	SessionEventTypeCircuitUpdated = "circuitUpdated"


func registerSessionEventHandler(val interface{}, config map[interface{}]interface{}) error {

	handler, ok := val.(network.SessionEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/controller/network/SessionEventHandler interface.", reflect.TypeOf(val))
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
			return errors.Errorf("invalid type %v for fabric.sessions include configuration %v", reflect.TypeOf(includeVar))
		}
	}

	if len(includeList) == 0 {
		AddSessionEventHandler(handler)
	} else {
		for _, include := range includeList {
			if include == SessionEventTypeCreated {
				AddSessionEventHandler(&fabricSessionCreatedEventAdapter{
					wrapped: handler,
				})
			} else if include == SessionEventTypeDeleted {
				AddSessionEventHandler(&fabricSessionDeletedEventAdapter{
					wrapped: handler,
				})
			} else if include == SessionEventTypeCircuitUpdated {
				AddSessionEventHandler(&fabricSessionCircuitUpdatedEventAdapter{
					wrapped: handler,
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

func (adapter *fabricSessionCreatedEventAdapter) SessionDeleted(sessionId *identity.TokenId, clientId *identity.TokenId) {
}

func (adapter *fabricSessionCreatedEventAdapter) CircuitUpdated(sessionId *identity.TokenId, circuit *network.Circuit) {
}

type fabricSessionDeletedEventAdapter struct {
	wrapped network.SessionEventHandler
}

func (adapter *fabricSessionDeletedEventAdapter) SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, circuit *network.Circuit) {
}

func (adapter *fabricSessionDeletedEventAdapter) SessionDeleted(sessionId *identity.TokenId, clientId *identity.TokenId) {
	adapter.wrapped.SessionDeleted(sessionId, clientId)
}

func (adapter *fabricSessionDeletedEventAdapter) CircuitUpdated(sessionId *identity.TokenId, circuit *network.Circuit) {
}

type fabricSessionCircuitUpdatedEventAdapter struct {
	wrapped network.SessionEventHandler
}

func (adapter *fabricSessionCircuitUpdatedEventAdapter) SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, circuit *network.Circuit) {
}

func (adapter *fabricSessionCircuitUpdatedEventAdapter) SessionDeleted(sessionId *identity.TokenId, clientId *identity.TokenId) {
}

func (adapter *fabricSessionCircuitUpdatedEventAdapter) CircuitUpdated(sessionId *identity.TokenId, circuit *network.Circuit) {
	adapter.wrapped.CircuitUpdated(sessionId, circuit)
}
