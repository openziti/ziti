package events

import (
	"fmt"
	"reflect"
	"time"

	"github.com/openziti/fabric/controller/network"
	"github.com/pkg/errors"
)

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
		accepted := map[network.CircuitEventType]struct{}{}
		for _, include := range includeList {
			found := false
			for _, t := range network.CircuitTypes {
				if include == t.String() {
					accepted[t] = struct{}{}
					found = true
					break
				}
			}
			if !found {
				return errors.Errorf("invalid include %v for fabric.circuits. valid values are %+v", include, network.CircuitTypes)
			}
		}
		AddCircuitEventHandler(&fabricCircuitTypeFilterEventAdapter{
			accepted: accepted,
			wrapped:  adapter,
		})
	}

	return nil
}

type fabricCircuitTypeFilterEventAdapter struct {
	accepted map[network.CircuitEventType]struct{}
	wrapped  network.CircuitEventHandler
}

func (self *fabricCircuitTypeFilterEventAdapter) AcceptCircuitEvent(event *network.CircuitEvent) {
	if _, found := self.accepted[event.Type]; found {
		self.wrapped.AcceptCircuitEvent(event)
	}
}

type CircuitEvent struct {
	Namespace              string         `json:"namespace"`
	EventType              string         `json:"event_type"`
	CircuitId              string         `json:"circuit_id"`
	Timestamp              time.Time      `json:"timestamp"`
	ClientId               string         `json:"client_id"`
	ServiceId              string         `json:"service_id"`
	InstanceId             string         `json:"instance_id"`
	CreationTimespan       *time.Duration `json:"creation_timespan"`
	Path                   string         `json:"path"`
	TerminatorLocalAddress string         `json:"terminator_local_address"`
	LinkCount              int            `json:"link_count"`
	Cost                   *uint32        `json:"path_cost"`
	FailureCause           *string        `json:"failure_cause"`
}

func (event *CircuitEvent) String() string {
	return fmt.Sprintf("%v.%v circuitId=%v clientId=%v serviceId=%v path=%v%s",
		event.Namespace, event.EventType, event.CircuitId, event.ClientId, event.ServiceId, event.Path, func() (out string) {
			if event.TerminatorLocalAddress != "" {
				out = fmt.Sprintf("%s (%s)", out, event.TerminatorLocalAddress)
			}
			if event.CreationTimespan != nil {
				out = fmt.Sprintf("%s creationTimespan=%s", out, *event.CreationTimespan)
			}
			return
		}())
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

func (adapter *circuitEventAdapter) AcceptCircuitEvent(netEvent *network.CircuitEvent) {
	eventType := "created"
	if netEvent.Type == network.CircuitUpdated {
		eventType = "pathUpdated"
	} else if netEvent.Type == network.CircuitDeleted {
		eventType = "deleted"
	} else if netEvent.Type == network.CircuitFailed {
		eventType = "failed"
	}

	var failureCause *string
	if cause := string(netEvent.FailureCause); cause != "" {
		failureCause = &cause
	}

	pathDesc := ""
	if netEvent.Path != nil {
		pathDesc = netEvent.Path.String()
	}

	event := &CircuitEvent{
		Namespace:              "fabric.circuits",
		EventType:              eventType,
		CircuitId:              netEvent.CircuitId,
		Timestamp:              time.Now(),
		ClientId:               netEvent.ClientId,
		ServiceId:              netEvent.ServiceId,
		InstanceId:             netEvent.InstanceId,
		CreationTimespan:       netEvent.CreationTimespan,
		Path:                   pathDesc,
		TerminatorLocalAddress: netEvent.Path.TerminatorLocalAddr,
		LinkCount:              len(netEvent.Path.Links),
		Cost:                   netEvent.Cost,
		FailureCause:           failureCause,
	}

	adapter.handler.AcceptCircuitEvent(event)
}
