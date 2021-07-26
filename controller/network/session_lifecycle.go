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
	SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, path *Path)
	SessionDeleted(sessionId *identity.TokenId, clientId *identity.TokenId)
	PathUpdated(sessionId *identity.TokenId, path *Path)
}

func (network *Network) SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, path *Path) {
	event := &sessionCreatedEvent{
		sessionId: sessionId,
		clientId:  clientId,
		serviceId: serviceId,
		path:      path,
	}
	network.eventDispatcher.Dispatch(event)
}

func (network *Network) SessionDeleted(sessionId *identity.TokenId, clientId *identity.TokenId) {
	event := &sessionDeletedEvent{
		sessionId: sessionId,
		clientId:  clientId,
	}
	network.eventDispatcher.Dispatch(event)
}

func (network *Network) PathUpdated(sessionId *identity.TokenId, path *Path) {
	event := &sessionPathEvent{
		sessionId: sessionId,
		path:      path,
	}
	network.eventDispatcher.Dispatch(event)
}

type sessionCreatedEvent struct {
	sessionId *identity.TokenId
	clientId  *identity.TokenId
	serviceId string
	path      *Path
}

func (event *sessionCreatedEvent) Handle() {
	handlers := getSessionEventHandlers()
	for _, handler := range handlers {
		go handler.SessionCreated(event.sessionId, event.clientId, event.serviceId, event.path)
	}
}

type sessionDeletedEvent struct {
	sessionId *identity.TokenId
	clientId  *identity.TokenId
}

func (event *sessionDeletedEvent) Handle() {
	handlers := getSessionEventHandlers()
	for _, handler := range handlers {
		go handler.SessionDeleted(event.sessionId, event.clientId)
	}
}

type sessionPathEvent struct {
	sessionId *identity.TokenId
	path      *Path
}

func (event *sessionPathEvent) Handle() {
	handlers := getSessionEventHandlers()
	for _, handler := range handlers {
		go handler.PathUpdated(event.sessionId, event.path)
	}
}
