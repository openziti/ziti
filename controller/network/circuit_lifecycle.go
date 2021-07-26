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
	"github.com/openziti/foundation/util/cowslice"
)

var CircuitEventHandlerRegistry = cowslice.NewCowSlice(make([]CircuitEventHandler, 0))

func getCircuitEventHandlers() []CircuitEventHandler {
	return CircuitEventHandlerRegistry.Value().([]CircuitEventHandler)
}

type CircuitEventHandler interface {
	CircuitCreated(circuitId string, clientId string, serviceId string, path *Path)
	CircuitDeleted(circuitId string, clientId string)
	PathUpdated(circuitId string, path *Path)
}

func (network *Network) CircuitCreated(circuitId string, clientId string, serviceId string, path *Path) {
	event := &circuitCreatedEvent{
		circuitId: circuitId,
		clientId:  clientId,
		serviceId: serviceId,
		path:      path,
	}
	network.eventDispatcher.Dispatch(event)
}

func (network *Network) CircuitDeleted(circuitId string, clientId string) {
	event := &circuitDeletedEvent{
		circuitId: circuitId,
		clientId:  clientId,
	}
	network.eventDispatcher.Dispatch(event)
}

func (network *Network) PathUpdated(circuitId string, path *Path) {
	event := &circuitPathEvent{
		circuitId: circuitId,
		path:      path,
	}
	network.eventDispatcher.Dispatch(event)
}

type circuitCreatedEvent struct {
	circuitId string
	clientId  string
	serviceId string
	path      *Path
}

func (event *circuitCreatedEvent) Handle() {
	handlers := getCircuitEventHandlers()
	for _, handler := range handlers {
		go handler.CircuitCreated(event.circuitId, event.clientId, event.serviceId, event.path)
	}
}

type circuitDeletedEvent struct {
	circuitId string
	clientId  string
}

func (event *circuitDeletedEvent) Handle() {
	handlers := getCircuitEventHandlers()
	for _, handler := range handlers {
		go handler.CircuitDeleted(event.circuitId, event.clientId)
	}
}

type circuitPathEvent struct {
	circuitId string
	path      *Path
}

func (event *circuitPathEvent) Handle() {
	handlers := getCircuitEventHandlers()
	for _, handler := range handlers {
		go handler.PathUpdated(event.circuitId, event.path)
	}
}
