/*
	Copyright 2019 NetFoundry, Inc.

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

package trace

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/trace/pb"
)

// EventHandler is for types wishing to receive trace messages
type EventHandler interface {
	Accept(event *trace_pb.ChannelMessage)
}

// EventController allows for sending session changes to multiple Listeners
type EventController interface {
	EventHandler
	channel2.ReceiveHandler

	// AddHandler adds the given handler to the router
	AddHandler(handler EventHandler)

	// RemoveHandler removes the given handler from the router
	RemoveHandler(handler EventHandler)

	// Shutdown stops the router
	Shutdown()
}

// NewEventController creates a new EventController instance
func NewEventController() EventController {
	router := &eventControllerImpl{
		changeChan: make(chan routerEvent, 1),
		eventChan:  make(chan *trace_pb.ChannelMessage, 25),
		handlers:   make(map[EventHandler]EventHandler),
	}

	go router.run()
	return router
}

type eventControllerImpl struct {
	changeChan chan routerEvent
	eventChan  chan *trace_pb.ChannelMessage
	handlers   map[EventHandler]EventHandler
}

func (controller *eventControllerImpl) Accept(event *trace_pb.ChannelMessage) {
	controller.eventChan <- event
}

func (controller *eventControllerImpl) AddHandler(handler EventHandler) {
	controller.changeChan <- routerEvent{handler, addHandlerChange}
}

func (controller *eventControllerImpl) RemoveHandler(handler EventHandler) {
	controller.changeChan <- routerEvent{handler, removeHandlerChange}
}

func (controller *eventControllerImpl) Shutdown() {
	controller.changeChan <- routerEvent{nil, shutdownEventControllerChange}
}

func (controller *eventControllerImpl) run() {
	running := true
	for running {
		select {
		case change := <-controller.changeChan:
			if addHandlerChange == change.changeType {
				controller.handlers[change.handler] = change.handler
			} else if removeHandlerChange == change.changeType {
				delete(controller.handlers, change.handler)
			} else if shutdownEventControllerChange == change.changeType {
				running = false
			}
		case event := <-controller.eventChan:
			for _, handler := range controller.handlers {
				go handler.Accept(event)
			}
		}
	}
}

type eventControllerChangeType uint8

const (
	addHandlerChange = iota
	removeHandlerChange
	shutdownEventControllerChange
)

type routerEvent struct {
	handler    EventHandler
	changeType eventControllerChangeType
}

func (*eventControllerImpl) ContentType() int32 {
	return int32(ctrl_pb.ContentType_TraceEventType)
}

func (controller *eventControllerImpl) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	event := &trace_pb.ChannelMessage{}
	if err := proto.Unmarshal(msg.Body, event); err == nil {
		controller.Accept(event)
	} else {
		pfxlog.Logger().Errorf("unexpected error decoding trace message (%s)", err)
	}
}
