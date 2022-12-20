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
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/event"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
)

func (self *Dispatcher) AddTerminatorEventHandler(handler event.TerminatorEventHandler) {
	self.terminatorEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveTerminatorEventHandler(handler event.TerminatorEventHandler) {
	self.terminatorEventHandlers.Delete(handler)
}

func (self *Dispatcher) AcceptTerminatorEvent(event *event.TerminatorEvent) {
	go func() {
		for _, handler := range self.terminatorEventHandlers.Value() {
			handler.AcceptTerminatorEvent(event)
		}
	}()
}

func (self *Dispatcher) registerTerminatorEventHandler(val interface{}, _ map[string]interface{}) error {
	handler, ok := val.(event.TerminatorEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/event/TerminatorEventHandler interface.", reflect.TypeOf(val))
	}

	self.AddTerminatorEventHandler(handler)

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

	n.GetStores().Terminator.AddListener(boltz.EventCreate, terminatorEvtAdapter.terminatorCreated)
	n.GetStores().Terminator.AddListener(boltz.EventUpdate, terminatorEvtAdapter.terminatorUpdated)
	n.GetStores().Terminator.AddListener(boltz.EventDelete, terminatorEvtAdapter.terminatorDeleted)

	n.AddRouterPresenceHandler(terminatorEvtAdapter)
}

// terminatorEventAdapter converts router presence online/offline events and terminator entity change events to
// event.TerminatorEvent instances
type terminatorEventAdapter struct {
	Network    *network.Network
	Dispatcher *Dispatcher
}

func (self *terminatorEventAdapter) RouterConnected(r *network.Router) {
	self.routerChange(event.TerminatorRouterOnline, r)
}

func (self *terminatorEventAdapter) RouterDisconnected(r *network.Router) {
	self.routerChange(event.TerminatorRouterOffline, r)
}

func (self *terminatorEventAdapter) routerChange(eventType event.TerminatorEventType, r *network.Router) {
	var terminators []*db.Terminator
	err := self.Network.GetDb().View(func(tx *bbolt.Tx) error {
		cursor := self.Network.GetStores().Router.GetRelatedEntitiesCursor(tx, r.Id, db.EntityTypeTerminators, true)
		for cursor.IsValid() {
			id := cursor.Current()
			terminator, err := self.Network.GetStores().Terminator.LoadOneById(tx, string(id))
			if err != nil {
				pfxlog.Logger().WithError(err).Errorf("failure while generating terminator events for %v with terminator %v on router %v", eventType, string(id), r.Id)
			} else {
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

func (self *terminatorEventAdapter) terminatorCreated(args ...interface{}) {
	self.terminatorChanged(event.TerminatorCreated, args...)
}

func (self *terminatorEventAdapter) terminatorUpdated(args ...interface{}) {
	self.terminatorChanged(event.TerminatorUpdated, args...)
}

func (self *terminatorEventAdapter) terminatorDeleted(args ...interface{}) {
	self.terminatorChanged(event.TerminatorDeleted, args...)
}

func (self *terminatorEventAdapter) terminatorChanged(eventType event.TerminatorEventType, args ...interface{}) {
	var terminator *db.Terminator
	if len(args) == 1 {
		terminator, _ = args[0].(*db.Terminator)
	}

	if terminator == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	terminator = self.Network.Services.NotifyTerminatorChanged(terminator)
	self.createTerminatorEvent(eventType, terminator)
}

func (self *terminatorEventAdapter) createTerminatorEvent(eventType event.TerminatorEventType, terminator *db.Terminator) {
	service, _ := self.Network.Services.Read(terminator.Service)

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
		Namespace:                 event.TerminatorEventsNs,
		EventType:                 eventType,
		Timestamp:                 time.Now(),
		ServiceId:                 terminator.Service,
		TerminatorId:              terminator.Id,
		RouterId:                  terminator.Router,
		HostId:                    terminator.HostId,
		RouterOnline:              self.Network.ConnectedRouter(terminator.Router),
		Precedence:                terminator.Precedence,
		StaticCost:                terminator.Cost,
		DynamicCost:               xt.GlobalCosts().GetDynamicCost(terminator.Id),
		TotalTerminators:          totalTerminators,
		UsableDefaultTerminators:  usableDefaultTerminators,
		UsableRequiredTerminators: usableRequiredTerminators,
	}

	self.Dispatcher.AcceptTerminatorEvent(evt)
}
