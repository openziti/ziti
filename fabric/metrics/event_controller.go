/*
	Copyright 2019 Netfoundry, Inc.

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

package metrics

import (
	"github.com/netfoundry/ziti-fabric/fabric/pb/ctrl_pb"
	"github.com/michaelquigley/pfxlog"
)

// HandlerType is used define known handler types
type HandlerType string

const (
	// HandlerTypeInfluxDB represents the HandlerTypeInfluxDB reporter
	HandlerTypeInfluxDB HandlerType = "influxdb"
)

// Handler represents a sink for metric events
type Handler interface {
	// AcceptMetrics is called when new metrics become available
	AcceptMetrics(message *ctrl_pb.MetricsMessage)
}

// EventController allows for sending metrics to multiple Handlers
type EventController interface {
	Handler

	// AddHandler adds the given arbitrary handler to the event controller
	AddHandler(handler Handler)

	// RemoveHandler removes the given handler from the event controller
	RemoveHandler(handler Handler)

	// Shutdown stops the event controller
	Shutdown()
}

// NewEventController creates a new EventController instance
func NewEventController(cfg *Config) EventController {
	eventController := &eventControllerImpl{
		changeChan: make(chan eventControllerChange, 1),
		metricChan: make(chan *ctrl_pb.MetricsMessage, 25),
	}

	if cfg != nil && cfg.handlers != nil {
		eventController.handlers = cfg.handlers
	} else {
		eventController.handlers = make(map[Handler]Handler)
	}

	go eventController.run()
	return eventController
}

type eventControllerImpl struct {
	changeChan chan eventControllerChange
	metricChan chan *ctrl_pb.MetricsMessage
	handlers   map[Handler]Handler
}

func (eventController *eventControllerImpl) AcceptMetrics(message *ctrl_pb.MetricsMessage) {
	eventController.metricChan <- message
}

func (eventController *eventControllerImpl) AddHandler(handler Handler) {
	eventController.changeChan <- eventControllerChange{handler, addHandler}
}

func (eventController *eventControllerImpl) RemoveHandler(handler Handler) {
	eventController.changeChan <- eventControllerChange{handler, removeHandler}
}

func (eventController *eventControllerImpl) Shutdown() {
	eventController.changeChan <- eventControllerChange{nil, shutdownEventController}
}

func (eventController *eventControllerImpl) run() {
	log := pfxlog.Logger()
	log.Info("started")
	defer log.Errorf("exited")

	running := true
	for running {
		select {
		case change := <-eventController.changeChan:
			if addHandler == change.changeType {
				eventController.handlers[change.handler] = change.handler
			} else if removeHandler == change.changeType {
				delete(eventController.handlers, change.handler)
			} else if shutdownEventController == change.changeType {
				running = false
			}
		case msg := <-eventController.metricChan:
			for _, handler := range eventController.handlers {
				go handler.AcceptMetrics(msg)
			}
		}
	}
}

type eventControllerChangeType uint8

const (
	addHandler = iota
	removeHandler
	shutdownEventController
)

type eventControllerChange struct {
	handler    Handler
	changeType eventControllerChangeType
}
