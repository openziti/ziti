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
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/cowslice"
)

var SessionEventHandlerRegistry = cowslice.NewCowSlice(make([]SessionEventHandler, 0))

func getSessionEventHandlers() []SessionEventHandler {
	return SessionEventHandlerRegistry.Value().([]SessionEventHandler)
}

type SessionEventHandler interface {
	SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, circuit *Circuit)
	SessionDeleted(sessionId *identity.TokenId)
	CircuitUpdated(sessionId *identity.TokenId, circuit *Circuit)
}

func (network *Network) SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, circuit *Circuit) {
	event := &sessionCreatedEvent{
		sessionId: sessionId,
		clientId:  clientId,
		serviceId: serviceId,
		circuit:   circuit,
	}
	network.eventDispatcher.Dispatch(event)
}

func (network *Network) SessionDeleted(sessionId *identity.TokenId) {
	event := &sessionDeletedEvent{
		sessionId: sessionId,
	}
	network.eventDispatcher.Dispatch(event)
}

func (network *Network) CircuitUpdated(sessionId *identity.TokenId, circuit *Circuit) {
	event := &sessionCircuitEvent{
		sessionId: sessionId,
		circuit:   circuit,
	}
	network.eventDispatcher.Dispatch(event)
}

type sessionCreatedEvent struct {
	sessionId *identity.TokenId
	clientId  *identity.TokenId
	serviceId string
	circuit   *Circuit
}

func (event *sessionCreatedEvent) Handle() {
	handlers := getSessionEventHandlers()
	for _, handler := range handlers {
		go handler.SessionCreated(event.sessionId, event.clientId, event.serviceId, event.circuit)
	}
}

type sessionDeletedEvent struct {
	sessionId *identity.TokenId
}

func (event *sessionDeletedEvent) Handle() {
	handlers := getSessionEventHandlers()
	for _, handler := range handlers {
		go handler.SessionDeleted(event.sessionId)
	}
}

type sessionCircuitEvent struct {
	sessionId *identity.TokenId
	circuit   *Circuit
}

func (event *sessionCircuitEvent) Handle() {
	handlers := getSessionEventHandlers()
	for _, handler := range handlers {
		go handler.CircuitUpdated(event.sessionId, event.circuit)
	}
}
