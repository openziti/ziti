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
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/event"
	"github.com/pkg/errors"
	"time"
)

func (network *Network) fillCircuitPath(e *event.CircuitEvent, path *Path) {
	if path == nil {
		return
	}
	for _, r := range path.Nodes {
		e.Path.Nodes = append(e.Path.Nodes, r.Id)
	}
	for _, l := range path.Links {
		e.Path.Links = append(e.Path.Links, l.Id)
	}
	e.Path.IngressId = path.IngressId
	e.Path.EgressId = path.EgressId
	e.Path.TerminatorLocalAddr = path.TerminatorLocalAddr
	e.Path.TerminatorRemoteAddr = path.TerminatorRemoteAddr
	e.LinkCount = len(path.Links)
}

func (network *Network) CircuitEvent(eventType event.CircuitEventType, circuit *Circuit, creationTimespan *time.Duration) {
	var cost *uint32
	if eventType == event.CircuitCreated {
		c := circuit.Terminator.GetRouteCost()
		cost = &c
	}

	circuitEvent := &event.CircuitEvent{
		Namespace:        event.CircuitEventsNs,
		Version:          event.CircuitEventsVersion,
		EventType:        eventType,
		CircuitId:        circuit.Id,
		Timestamp:        time.Now(),
		ClientId:         circuit.ClientId,
		ServiceId:        circuit.Service.Id,
		TerminatorId:     circuit.Terminator.GetId(),
		InstanceId:       circuit.Terminator.GetInstanceId(),
		CreationTimespan: creationTimespan,
		Cost:             cost,
	}
	network.fillCircuitPath(circuitEvent, circuit.Path)
	network.eventDispatcher.AcceptCircuitEvent(circuitEvent)
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

func (network *Network) CircuitFailedEvent(circuitId string, clientId string, serviceId string, instanceId string, startTime time.Time, path *Path, t xt.CostedTerminator, cause CircuitFailureCause) {
	var failureCause *string
	if strCause := string(cause); strCause != "" {
		failureCause = &strCause
	}

	var cost *uint32
	if t != nil {
		c := t.GetRouteCost()
		cost = &c
	}

	var terminatorId string
	if t != nil {
		terminatorId = t.GetId()
	}

	elapsed := time.Now().Sub(startTime)
	circuitEvent := &event.CircuitEvent{
		Namespace:        event.CircuitEventsNs,
		Version:          event.CircuitEventsVersion,
		EventType:        event.CircuitFailed,
		CircuitId:        circuitId,
		Timestamp:        time.Now(),
		ClientId:         clientId,
		ServiceId:        serviceId,
		TerminatorId:     terminatorId,
		InstanceId:       instanceId,
		CreationTimespan: &elapsed,
		Cost:             cost,
		FailureCause:     failureCause,
	}
	network.fillCircuitPath(circuitEvent, path)
	network.eventDispatcher.AcceptCircuitEvent(circuitEvent)
}
