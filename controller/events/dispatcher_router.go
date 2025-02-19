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
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
	"github.com/pkg/errors"
	"reflect"
	"time"
)

func (self *Dispatcher) AddRouterEventHandler(handler event.RouterEventHandler) {
	self.routerEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveRouterEventHandler(handler event.RouterEventHandler) {
	self.routerEventHandlers.DeleteIf(func(val event.RouterEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.RouterEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
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

func (self *Dispatcher) registerRouterEventHandler(eventType string, val interface{}, _ map[string]interface{}) error {
	handler, ok := val.(event.RouterEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/ziti/controller/event/RouterEventHandler interface.", reflect.TypeOf(val))
	}

	if eventType != event.RouterEventNS {
		handler = &routerEventOldNsAdapter{
			namespace: eventType,
			wrapped:   handler,
		}
	}

	self.AddRouterEventHandler(handler)

	return nil
}

func (self *Dispatcher) unregisterRouterEventHandler(val interface{}) {
	if handler, ok := val.(event.RouterEventHandler); ok {
		self.RemoveRouterEventHandler(handler)
	}
}

type routerEventOldNsAdapter struct {
	namespace string
	wrapped   event.RouterEventHandler
}

func (self *routerEventOldNsAdapter) AcceptRouterEvent(evt *event.RouterEvent) {
	nsEvent := *evt
	nsEvent.Namespace = self.namespace
	self.wrapped.AcceptRouterEvent(&nsEvent)
}

func (self *routerEventOldNsAdapter) IsWrapping(value event.RouterEventHandler) bool {
	if self.wrapped == value {
		return true
	}
	if w, ok := self.wrapped.(event.RouterEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}

// routerEventAdapter converts network router presence events to event.RouterEvent
type routerEventAdapter struct {
	*Dispatcher
}

func (self *routerEventAdapter) RouterConnected(r *model.Router) {
	self.routerChange(event.RouterOnline, r, true)
}

func (self *routerEventAdapter) RouterDisconnected(r *model.Router) {
	self.routerChange(event.RouterOffline, r, false)
}

func (self *routerEventAdapter) routerChange(eventType event.RouterEventType, r *model.Router, online bool) {
	evt := &event.RouterEvent{
		Namespace:    event.RouterEventNS,
		EventSrcId:   self.ctrlId,
		EventType:    eventType,
		Timestamp:    time.Now(),
		RouterId:     r.Id,
		RouterOnline: online,
	}

	self.Dispatcher.AcceptRouterEvent(evt)

	if eventType == event.RouterOnline {
		srcAddr := ""
		dstAddr := ""
		if ctrl := r.Control; ctrl != nil {
			srcAddr = r.Control.Underlay().GetRemoteAddr().String()
			dstAddr = r.Control.Underlay().GetLocalAddr().String()
		}

		connectEvent := &event.ConnectEvent{
			Namespace: event.ConnectEventNS,
			SrcType:   event.ConnectSourceRouter,
			DstType:   event.ConnectDestinationController,
			SrcId:     r.Id,
			SrcAddr:   srcAddr,
			DstId:     self.Dispatcher.ctrlId,
			DstAddr:   dstAddr,
			Timestamp: time.Now(),
		}
		self.Dispatcher.AcceptConnectEvent(connectEvent)
	}
}
