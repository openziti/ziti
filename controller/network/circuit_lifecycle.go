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

package network

import (
	"github.com/pkg/errors"
	"time"

	"github.com/openziti/foundation/util/cowslice"
)

var CircuitEventHandlerRegistry = cowslice.NewCowSlice(make([]CircuitEventHandler, 0))

func getCircuitEventHandlers() []CircuitEventHandler {
	return CircuitEventHandlerRegistry.Value().([]CircuitEventHandler)
}

type CircuitEventType int

const (
	CircuitCreated CircuitEventType = 1
	CircuitUpdated CircuitEventType = 2
	CircuitDeleted CircuitEventType = 3
	CircuitFailed  CircuitEventType = 4
)

func (self CircuitEventType) String() string {
	switch self {
	case CircuitCreated:
		return "created"
	case CircuitUpdated:
		return "updated"
	case CircuitDeleted:
		return "deleted"
	case CircuitFailed:
		return "failed"
	default:
		return "invalid"
	}
}

var CircuitTypes = []CircuitEventType{CircuitCreated, CircuitUpdated, CircuitDeleted, CircuitFailed}

type CircuitEvent struct {
	Type             CircuitEventType
	CircuitId        string
	ClientId         string
	ServiceId        string
	InstanceId       string
	CreationTimespan *time.Duration
	Path             *Path
	Cost             *uint32
	FailureCause     CircuitFailureCause
}

func (event *CircuitEvent) Handle() {
	handlers := getCircuitEventHandlers()
	for _, handler := range handlers {
		go handler.AcceptCircuitEvent(event)
	}
}

type CircuitEventHandler interface {
	AcceptCircuitEvent(event *CircuitEvent)
}

func (network *Network) CircuitEvent(eventType CircuitEventType, circuit *Circuit, creationTimespan *time.Duration, cost *uint32) {
	event := &CircuitEvent{
		Type:             eventType,
		CircuitId:        circuit.Id,
		ClientId:         circuit.ClientId,
		ServiceId:        circuit.Service.Id,
		InstanceId:       circuit.Terminator.GetInstanceId(),
		CreationTimespan: creationTimespan,
		Path:             circuit.Path,
		Cost:             cost,
	}
	network.eventDispatcher.Dispatch(event)
}

type CircuitFailureCause string

const (
	CircuitFailureInvalidService                   CircuitFailureCause = "INVALID_SERVICE"
	CircuitFailureIdGenerationError                CircuitFailureCause = "ID_GENERATION_ERR"
	CircuitFailureNoTerminators                    CircuitFailureCause = "NO_TERMINATORS"
	CircuitFailureNoOnlineTerminators              CircuitFailureCause = "NO_ONLINE_TERMINATORS"
	CircuitFailureNoPath                           CircuitFailureCause = "NO_PATH"
	CircuitFailurePathMissingLink                  CircuitFailureCause = "PATH_MISSING_LINK"
	CircuitFailureInvalidStrategy                  CircuitFailureCause = "INVALID_STRATEGY"
	CircuitFailureStrategyError                    CircuitFailureCause = "STRATEGY_ERR"
	CircuitFailureRouterErrGeneric                 CircuitFailureCause = "ROUTER_ERR_GENERIC"
	CircuitFailureRouterErrInvalidTerminator       CircuitFailureCause = "ROUTER_ERR_INVALID_TERMINATOR"
	CircuitFailureRouterErrMisconfiguredTerminator CircuitFailureCause = "ROUTER_ERR_MISCONFIGURED_TERMINATOR"
	CircuitFailureRouterErrDialTimedOut            CircuitFailureCause = "ROUTER_ERR_DIAL_TIMED_OUT"
	CircuitFailureRouterErrDialConnRefused         CircuitFailureCause = "ROUTER_ERR_CONN_REFUSED"
)

type CircuitError interface {
	error
	Cause() CircuitFailureCause
}

type circuitError struct {
	error
	CircuitFailureCause
}

func (self *circuitError) Cause() CircuitFailureCause {
	return self.CircuitFailureCause
}

func (self *circuitError) Unwrap() error {
	return self.error
}

func newCircuitErrorf(cause CircuitFailureCause, t string, args ...any) CircuitError {
	return &circuitError{
		error:               errors.Errorf(t, args...),
		CircuitFailureCause: cause,
	}
}

func newCircuitErrWrap(cause CircuitFailureCause, err error) CircuitError {
	return &circuitError{
		error:               err,
		CircuitFailureCause: cause,
	}
}

func (network *Network) CircuitFailedEvent(circuitId string, clientId string, serviceId string, instanceId string, startTime time.Time, path *Path, cost *uint32, cause CircuitFailureCause) {
	elapsed := time.Now().Sub(startTime)
	event := &CircuitEvent{
		Type:             CircuitFailed,
		CircuitId:        circuitId,
		ClientId:         clientId,
		ServiceId:        serviceId,
		InstanceId:       instanceId,
		CreationTimespan: &elapsed,
		Path:             path,
		Cost:             cost,
		FailureCause:     cause,
	}
	network.eventDispatcher.Dispatch(event)
}
