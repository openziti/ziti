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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/controller/xt"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"strings"
	"time"
)

func (self *Dispatcher) AddTerminatorEventHandler(handler event.TerminatorEventHandler) {
	self.terminatorEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveTerminatorEventHandler(handler event.TerminatorEventHandler) {
	self.terminatorEventHandlers.DeleteIf(func(val event.TerminatorEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.TerminatorEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) AcceptTerminatorEvent(event *event.TerminatorEvent) {
	go func() {
		for _, handler := range self.terminatorEventHandlers.Value() {
			handler.AcceptTerminatorEvent(event)
		}
	}()
}

func (self *Dispatcher) registerTerminatorEventHandler(eventType string, val interface{}, options map[string]interface{}) error {
	handler, ok := val.(event.TerminatorEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/ziti/controller/event/TerminatorEventHandler interface.", reflect.TypeOf(val))
	}

	if eventType != event.TerminatorEventNS {
		handler = &terminatorEventOldNsAdapter{
			namespace: eventType,
			wrapped:   handler,
		}
	}

	propagateAlways := false
	if val, found := options["propagateAlways"]; found {
		if b, ok := val.(bool); ok {
			propagateAlways = b
		} else if s, ok := val.(string); ok {
			propagateAlways = strings.EqualFold(s, "true")
		} else {
			return errors.New("invalid value for propagateAlways, must be boolean or string")
		}
	}

	if propagateAlways {
		self.AddTerminatorEventHandler(handler)
	} else {
		self.AddTerminatorEventHandler(&terminatorEventFilter{TerminatorEventHandler: handler})
	}

	return nil
}

func (self *Dispatcher) unregisterTerminatorEventHandler(val interface{}) {
	if handler, ok := val.(event.TerminatorEventHandler); ok {
		self.RemoveTerminatorEventHandler(handler)
	}
}

func (self *Dispatcher) initTerminatorEvents(n *network.Network) {
	terminatorEvtAdapter := &terminatorEventAdapter{
		Network:    n,
		Dispatcher: self,
	}

	n.GetStores().Terminator.AddEntityEventListenerF(terminatorEvtAdapter.terminatorCreated, boltz.EntityCreated)
	n.GetStores().Terminator.AddEntityEventListenerF(terminatorEvtAdapter.terminatorUpdated, boltz.EntityUpdated)
	n.GetStores().Terminator.AddEntityEventListenerF(terminatorEvtAdapter.terminatorDeleted, boltz.EntityDeleted)

	n.AddRouterPresenceHandler(terminatorEvtAdapter)
}

type terminatorEventOldNsAdapter struct {
	namespace string
	wrapped   event.TerminatorEventHandler
}

func (self *terminatorEventOldNsAdapter) AcceptTerminatorEvent(evt *event.TerminatorEvent) {
	nsEvent := *evt
	nsEvent.Namespace = self.namespace
	self.wrapped.AcceptTerminatorEvent(&nsEvent)
}

func (self *terminatorEventOldNsAdapter) IsWrapping(value event.TerminatorEventHandler) bool {
	if self.wrapped == value {
		return true
	}
	if w, ok := self.wrapped.(event.TerminatorEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}

type terminatorEventFilter struct {
	event.TerminatorEventHandler
}

func (self *terminatorEventFilter) IsWrapping(value event.TerminatorEventHandler) bool {
	if self.TerminatorEventHandler == value {
		return true
	}
	if w, ok := self.TerminatorEventHandler.(event.TerminatorEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}

func (self *terminatorEventFilter) AcceptTerminatorEvent(evt *event.TerminatorEvent) {
	if !evt.IsModelEvent() || evt.PropagateIndicator {
		self.TerminatorEventHandler.AcceptTerminatorEvent(evt)
	}
}

// terminatorEventAdapter converts router presence online/offline events and terminator entity change events to
// event.TerminatorEvent instances
type terminatorEventAdapter struct {
	Network    *network.Network
	Dispatcher *Dispatcher
}

func (self *terminatorEventAdapter) RouterConnected(r *model.Router) {
	self.routerChange(event.TerminatorRouterOnline, r)
}

func (self *terminatorEventAdapter) RouterDisconnected(r *model.Router) {
	self.routerChange(event.TerminatorRouterOffline, r)
}

func (self *terminatorEventAdapter) routerChange(eventType event.TerminatorEventType, r *model.Router) {
	var terminators []*db.Terminator
	err := self.Network.GetDb().View(func(tx *bbolt.Tx) error {
		cursor := self.Network.GetStores().Router.GetRelatedEntitiesCursor(tx, r.Id, db.EntityTypeTerminators, true)
		for cursor.IsValid() {
			id := cursor.Current()
			terminator, found, err := self.Network.GetStores().Terminator.FindById(tx, string(id))
			if err != nil {
				pfxlog.Logger().WithError(err).Errorf("failure while generating terminator events for %v with terminator %v on router %v", eventType, string(id), r.Id)
			} else if found {
				terminators = append(terminators, terminator)
			}
			cursor.Next()
		}
		return nil
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("failure while generating terminator events for %v for router %v", eventType, r.Id)
	}

	for _, terminator := range terminators {
		// This calls Db.View() down the line, so avoid nesting tx
		self.terminatorChanged(eventType, terminator)
	}
}

func (self *terminatorEventAdapter) terminatorCreated(terminator *db.Terminator) {
	self.terminatorChanged(event.TerminatorCreated, terminator)
}

func (self *terminatorEventAdapter) terminatorUpdated(terminator *db.Terminator) {
	self.terminatorChanged(event.TerminatorUpdated, terminator)
}

func (self *terminatorEventAdapter) terminatorDeleted(terminator *db.Terminator) {
	self.terminatorChanged(event.TerminatorDeleted, terminator)
}

func (self *terminatorEventAdapter) terminatorChanged(eventType event.TerminatorEventType, terminator *db.Terminator) {
	terminator = self.Network.Service.NotifyTerminatorChanged(terminator)
	self.createTerminatorEvent(eventType, terminator)
}

func (self *terminatorEventAdapter) createTerminatorEvent(eventType event.TerminatorEventType, terminator *db.Terminator) {
	service, _ := self.Network.Service.Read(terminator.Service)

	totalTerminators := -1
	usableDefaultTerminators := -1
	usableRequiredTerminators := -1

	if service != nil {
		totalTerminators = len(service.Terminators)
		usableDefaultTerminators = 0
		usableRequiredTerminators = 0
		for _, t := range service.Terminators {
			routerOnline := self.Network.ConnectedRouter(t.Router)
			if t.Precedence.IsDefault() && routerOnline {
				usableDefaultTerminators++
			} else if t.Precedence.IsRequired() && routerOnline {
				usableRequiredTerminators++
			}
		}
	}

	evt := &event.TerminatorEvent{
		Namespace:                 event.TerminatorEventNS,
		EventType:                 eventType,
		EventSrcId:                self.Dispatcher.ctrlId,
		Timestamp:                 time.Now(),
		ServiceId:                 terminator.Service,
		TerminatorId:              terminator.Id,
		RouterId:                  terminator.Router,
		HostId:                    terminator.HostId,
		InstanceId:                terminator.InstanceId,
		RouterOnline:              self.Network.ConnectedRouter(terminator.Router),
		Precedence:                terminator.Precedence,
		StaticCost:                terminator.Cost,
		DynamicCost:               xt.GlobalCosts().GetDynamicCost(terminator.Id),
		TotalTerminators:          totalTerminators,
		UsableDefaultTerminators:  usableDefaultTerminators,
		UsableRequiredTerminators: usableRequiredTerminators,
		PropagateIndicator:        self.Network.Dispatcher.IsLeaderOrLeaderless(),
	}

	self.Dispatcher.AcceptTerminatorEvent(evt)
}
