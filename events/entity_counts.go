package events

import (
	"fmt"
	"github.com/openziti/edge/controller/persistence"
	"github.com/pkg/errors"
	"reflect"
	"time"
)

const EntityCountEventNS = "edge.entityCounts"

type EntityCountEvent struct {
	Namespace string           `json:"namespace"`
	Timestamp time.Time        `json:"timestamp"`
	Counts    map[string]int64 `json:"counts"`
	Error     string           `json:"error"`
}

func (event *EntityCountEvent) String() string {
	return fmt.Sprintf("%v timestamp=%v counts=%+v err=%v",
		event.Namespace, event.Timestamp, event.Counts, event.Error)
}

var entityCountEventGenerator func(interval time.Duration, handler EntityCountEventHandler)

func generateEntityCountEvent(dbProvider persistence.DbProvider, stores *persistence.Stores, handler EntityCountEventHandler) {
	event := &EntityCountEvent{
		Namespace: EntityCountEventNS,
		Timestamp: time.Now(),
	}

	data, err := stores.GetEntityCounts(dbProvider)
	if err != nil {
		event.Error = err.Error()
	} else {
		event.Counts = data
	}

	handler.AcceptEntityCountEvent(event)
}

type EntityCountEventHandler interface {
	AcceptEntityCountEvent(event *EntityCountEvent)
}

func registerEntityCountEventHandler(val interface{}, config map[interface{}]interface{}) error {
	handler, ok := val.(EntityCountEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/edge/events/EntityCountEventHandler interface.", reflect.TypeOf(val))
	}

	interval := time.Minute * 5

	if val, ok := config["interval"]; ok {
		if strVal, ok := val.(string); ok {
			var err error
			interval, err = time.ParseDuration(strVal)
			if err != nil {
				return errors.Wrapf(err, "invalid duration value for edge.entityCounts interval: '%v'", strVal)
			}
		} else {
			return errors.Errorf("invalid type %v for edge.entityCounts interval configuration", reflect.TypeOf(val))
		}
	}

	go entityCountEventGenerator(interval, handler)

	return nil
}
