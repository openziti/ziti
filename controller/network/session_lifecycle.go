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
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type SessionLifeCycleListener interface {
	SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, circuit *Circuit)
	SessionDeleted(sessionId *identity.TokenId)
	CircuitUpdated(sessionId *identity.TokenId, circuit *Circuit)
}

// SessionLifeCycleController allows for sending session changes to multiple Listeners
type SessionLifeCycleController interface {
	SessionLifeCycleListener

	// AddListener adds the given listener to the controller
	AddListener(listener SessionLifeCycleListener)

	// RemoveListener removes the given listener from the controller
	RemoveListener(listener SessionLifeCycleListener)

	// Shutdown stops the controller
	Shutdown()
}

// NewSessionLifeCycleController creates a new SessionLifeCycleController instance
func NewSessionLifeCyleController() SessionLifeCycleController {
	controller := &sessionLifeCycleControllerImpl{
		changeChan: make(chan sessionLifeCycleControllerChange, 1),
		eventChan:  make(chan sessionLifeCycleEvent, 25),
		listeners:  make(map[SessionLifeCycleListener]SessionLifeCycleListener),
	}

	go controller.run()
	return controller
}

type sessionLifeCycleControllerImpl struct {
	changeChan chan sessionLifeCycleControllerChange
	eventChan  chan sessionLifeCycleEvent
	listeners  map[SessionLifeCycleListener]SessionLifeCycleListener
}

func (controller *sessionLifeCycleControllerImpl) SessionCreated(sessionId *identity.TokenId, clientId *identity.TokenId, serviceId string, circuit *Circuit) {
	event := &sessionCreatedEvent{
		sessionId: sessionId,
		clientId:  clientId,
		serviceId: serviceId,
		circuit:   circuit,
	}
	controller.eventChan <- event
}

func (controller *sessionLifeCycleControllerImpl) SessionDeleted(sessionId *identity.TokenId) {
	event := &sessionDeletedEvent{
		sessionId: sessionId,
	}
	controller.eventChan <- event
}

func (controller *sessionLifeCycleControllerImpl) CircuitUpdated(sessionId *identity.TokenId, circuit *Circuit) {
	event := &sessionCircuitEvent{
		sessionId: sessionId,
		circuit:   circuit,
	}
	controller.eventChan <- event
}

func (controller *sessionLifeCycleControllerImpl) AddListener(listener SessionLifeCycleListener) {
	controller.changeChan <- sessionLifeCycleControllerChange{listener, sessionLifeCycleControllerAddListener}
}

func (controller *sessionLifeCycleControllerImpl) RemoveListener(listener SessionLifeCycleListener) {
	controller.changeChan <- sessionLifeCycleControllerChange{listener, sessionLifeCycleControllerRemoveListener}
}

func (controller *sessionLifeCycleControllerImpl) Shutdown() {
	controller.changeChan <- sessionLifeCycleControllerChange{nil, sessionLifeCycleControllerShutdown}
}

func (controller *sessionLifeCycleControllerImpl) run() {
	running := true
	for running {
		select {
		case change := <-controller.changeChan:
			if sessionLifeCycleControllerAddListener == change.changeType {
				controller.listeners[change.listener] = change.listener
			} else if sessionLifeCycleControllerRemoveListener == change.changeType {
				delete(controller.listeners, change.listener)
			} else if sessionLifeCycleControllerShutdown == change.changeType {
				running = false
			}
		case event := <-controller.eventChan:
			event.handle(controller)
		}
	}
}

type sessionLifecycleControllerChangeType uint8

const (
	sessionLifeCycleControllerAddListener = iota
	sessionLifeCycleControllerRemoveListener
	sessionLifeCycleControllerShutdown
)

type sessionLifeCycleControllerChange struct {
	listener   SessionLifeCycleListener
	changeType sessionLifecycleControllerChangeType
}

type sessionLifeCycleEvent interface {
	handle(controller *sessionLifeCycleControllerImpl)
}

type sessionCreatedEvent struct {
	sessionId *identity.TokenId
	clientId  *identity.TokenId
	serviceId string
	circuit   *Circuit
}

func (event *sessionCreatedEvent) handle(controller *sessionLifeCycleControllerImpl) {
	for _, handler := range controller.listeners {
		go handler.SessionCreated(event.sessionId, event.clientId, event.serviceId, event.circuit)
	}
}

type sessionDeletedEvent struct {
	sessionId *identity.TokenId
}

func (event *sessionDeletedEvent) handle(controller *sessionLifeCycleControllerImpl) {
	for _, handler := range controller.listeners {
		go handler.SessionDeleted(event.sessionId)
	}
}

type sessionCircuitEvent struct {
	sessionId *identity.TokenId
	circuit   *Circuit
}

func (event *sessionCircuitEvent) handle(controller *sessionLifeCycleControllerImpl) {
	for _, handler := range controller.listeners {
		go handler.CircuitUpdated(event.sessionId, event.circuit)
	}
}
