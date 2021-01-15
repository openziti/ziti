package events

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/cowslice"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
)

func registerTerminatorEventHandler(val interface{}, config map[interface{}]interface{}) error {
	handler, ok := val.(TerminatorEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/events/TerminatorEventHandler interface.", reflect.TypeOf(val))
	}

	AddTerminatorEventHandler(handler)

	return nil
}

type TerminatorEvent struct {
	Namespace                 string    `json:"namespace"`
	EventType                 string    `json:"event_type"`
	Timestamp                 time.Time `json:"timestamp"`
	ServiceId                 string    `json:"service_id"`
	TerminatorId              string    `json:"terminator_id"`
	RouterId                  string    `json:"router_id"`
	RouterOnline              bool      `json:"router_online"`
	Precedence                string    `json:"precedence"`
	StaticCost                uint16    `json:"static_cost"`
	DynamicCost               uint16    `json:"dynamic_cost"`
	TotalTerminators          int       `json:"total_terminators"`
	UsableDefaultTerminators  int       `json:"usable_default_terminators"`
	UsableRequiredTerminators int       `json:"usable_required_terminators"`
}

var terminatorEventHandlerRegistry = cowslice.NewCowSlice(make([]TerminatorEventHandler, 0))

func getTerminatorEventHandlers() []TerminatorEventHandler {
	return terminatorEventHandlerRegistry.Value().([]TerminatorEventHandler)
}

func (event *TerminatorEvent) String() string {
	return fmt.Sprintf("%v.%v time=%v serviceId=%v terminatorId=%v routerId=%v routerOnline=%v precedence=%v "+
		"staticCost=%v dynamicCost=%v totalTerminators=%v usableDefaultTerminator=%v usableRequiredTerminators=%v",
		event.Namespace, event.EventType, event.Timestamp, event.ServiceId, event.TerminatorId, event.RouterId, event.RouterOnline,
		event.Precedence, event.StaticCost, event.DynamicCost, event.TotalTerminators, event.UsableDefaultTerminators,
		event.UsableRequiredTerminators)
}

type TerminatorEventHandler interface {
	AcceptTerminatorEvent(event *TerminatorEvent)
}

func InitTerminatorEventRouter(n *network.Network) {
	r := &terminatorEventRouter{
		network: n,
	}

	n.GetStores().Terminator.AddListener(boltz.EventCreate, r.terminatorCreated)
	n.GetStores().Terminator.AddListener(boltz.EventUpdate, r.terminatorUpdated)
	n.GetStores().Terminator.AddListener(boltz.EventDelete, r.terminatorDeleted)
	n.AddRouterPresenceHandler(r)
}

type terminatorEventRouter struct {
	network *network.Network
}

func (self *terminatorEventRouter) RouterConnected(r *network.Router) {
	self.routerChange("router-online", r)
}

func (self *terminatorEventRouter) RouterDisconnected(r *network.Router) {
	self.routerChange("router-offline", r)
}

func (self *terminatorEventRouter) routerChange(eventType string, r *network.Router) {
	err := self.network.GetDb().View(func(tx *bbolt.Tx) error {
		cursor := self.network.GetStores().Router.GetRelatedEntitiesCursor(tx, r.Id, db.EntityTypeTerminators, true)
		for cursor.IsValid() {
			id := cursor.Current()
			terminator, err := self.network.GetStores().Terminator.LoadOneById(tx, string(id))
			if err != nil {
				pfxlog.Logger().WithError(err).Errorf("failure while generating terminator events for %v with terminator %v on router %v", eventType, string(id), r.Id)
			} else {
				self.terminatorChanged(eventType, terminator)
			}
			cursor.Next()
		}
		return nil
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("failure while generating terminator events for %v for router %v", eventType, r.Id)
	}
}

func (self *terminatorEventRouter) terminatorCreated(args ...interface{}) {
	self.terminatorChanged("created", args...)
}

func (self *terminatorEventRouter) terminatorUpdated(args ...interface{}) {
	self.terminatorChanged("updated", args...)
}

func (self *terminatorEventRouter) terminatorDeleted(args ...interface{}) {
	self.terminatorChanged("deleted", args...)
}

func (self *terminatorEventRouter) terminatorChanged(eventType string, args ...interface{}) {
	var terminator *db.Terminator
	if len(args) == 1 {
		terminator, _ = args[0].(*db.Terminator)
	}

	if terminator == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	terminator = self.network.Services.NotifyTerminatorChanged(terminator)
	self.createTerminatorEvent(eventType, terminator)
}

func (self *terminatorEventRouter) createTerminatorEvent(eventType string, terminator *db.Terminator) {
	service, _ := self.network.Services.Read(terminator.Service)

	totalTerminators := -1
	usableDefaultTerminators := -1
	usableRequiredTerminators := -1

	if service != nil {
		totalTerminators = len(service.Terminators)
		usableDefaultTerminators = 0
		usableRequiredTerminators = 0
		for _, t := range service.Terminators {
			routerOnline := self.network.ConnectedRouter(t.Router)
			if t.Precedence.IsDefault() && routerOnline {
				usableDefaultTerminators++
			} else if t.Precedence.IsRequired() && routerOnline {
				usableRequiredTerminators++
			}
		}
	}

	event := &TerminatorEvent{
		Namespace:                 "fabric.terminators",
		EventType:                 eventType,
		Timestamp:                 time.Now(),
		ServiceId:                 terminator.Service,
		TerminatorId:              terminator.Id,
		RouterId:                  terminator.Router,
		RouterOnline:              self.network.ConnectedRouter(terminator.Router),
		Precedence:                terminator.Precedence,
		StaticCost:                terminator.Cost,
		DynamicCost:               xt.GlobalCosts().GetDynamicCost(terminator.Id),
		TotalTerminators:          totalTerminators,
		UsableDefaultTerminators:  usableDefaultTerminators,
		UsableRequiredTerminators: usableRequiredTerminators,
	}

	for _, handler := range getTerminatorEventHandlers() {
		go handler.AcceptTerminatorEvent(event)
	}
}
