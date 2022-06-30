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

package events

import (
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/event"
	"github.com/pkg/errors"
	"reflect"
	"time"
)

func (self *Dispatcher) AddRouterEventHandler(handler event.RouterEventHandler) {
	self.routerEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveRouterEventHandler(handler event.RouterEventHandler) {
	self.routerEventHandlers.Delete(handler)
}

func (self *Dispatcher) AcceptRouterEvent(event *event.RouterEvent) {
	go func() {
		for _, handler := range self.routerEventHandlers.Value() {
			handler.AcceptRouterEvent(event)
		}
	}()
}

func (self *Dispatcher) initRouterEvents(n *network.Network) {
	routerEvtAdapter := &routerEventAdapter{
		Dispatcher: self,
	}
	n.AddRouterPresenceHandler(routerEvtAdapter)
}

func (self *Dispatcher) registerRouterEventHandler(val interface{}, _ map[interface{}]interface{}) error {
	handler, ok := val.(event.RouterEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/event/RouterEventHandler interface.", reflect.TypeOf(val))
	}

	self.AddRouterEventHandler(handler)

	return nil
}

// routerEventAdapter converts network router presence events to event.RouterEvent
type routerEventAdapter struct {
	*Dispatcher
}

func (self *routerEventAdapter) RouterConnected(r *network.Router) {
	self.routerChange(event.RouterOnline, r, true)
}

func (self *routerEventAdapter) RouterDisconnected(r *network.Router) {
	self.routerChange(event.RouterOffline, r, false)
}

func (self *routerEventAdapter) routerChange(eventType event.RouterEventType, r *network.Router, online bool) {
	evt := &event.RouterEvent{
		Namespace:    event.RouterEventsNs,
		EventType:    eventType,
		Timestamp:    time.Now(),
		RouterId:     r.Id,
		RouterOnline: online,
	}

	self.Dispatcher.AcceptRouterEvent(evt)
}
