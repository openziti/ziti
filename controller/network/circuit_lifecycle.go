/*
	Copyright NetFoundry, Inc.

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
)

func (self CircuitEventType) String() string {
	switch self {
	case CircuitCreated:
		return "created"
	case CircuitUpdated:
		return "updated"
	case CircuitDeleted:
		return "deleted"
	default:
		return "invalid"
	}
}

var CircuitTypes = []CircuitEventType{CircuitCreated, CircuitUpdated, CircuitDeleted}

type CircuitEvent struct {
	Type             CircuitEventType
	CircuitId        string
	ClientId         string
	ServiceId        string
	CreationTimespan *time.Duration
	Path             *Path
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

func (network *Network) CircuitEvent(eventType CircuitEventType, circuit *Circuit, creationTimespan *time.Duration) {
	event := &CircuitEvent{
		Type:             eventType,
		CircuitId:        circuit.Id,
		ClientId:         circuit.ClientId,
		ServiceId:        circuit.Service.Id,
		CreationTimespan: creationTimespan,
		Path:             circuit.Path,
	}
	network.eventDispatcher.Dispatch(event)
}
