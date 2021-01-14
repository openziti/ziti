package persistence

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/cowslice"
)

type ServiceEventType byte

func (self ServiceEventType) String() string {
	if self == ServiceDialAccessGained {
		return "dial-gained"
	}
	if self == ServiceDialAccessLost {
		return "dial-lost"
	}
	if self == ServiceBindAccessGained {
		return "bind-gained"
	}
	if self == ServiceBindAccessLost {
		return "bind-lost"
	}
	if self == ServiceUpdated {
		return "service-updated"
	}

	return "unknown"
}

const (
	ServiceDialAccessGained ServiceEventType = 1
	ServiceDialAccessLost   ServiceEventType = 2
	ServiceBindAccessGained ServiceEventType = 3
	ServiceBindAccessLost   ServiceEventType = 4
	ServiceUpdated          ServiceEventType = 5
)

var ServiceEvents = &ServiceEventsRegistry{
	handlers: cowslice.NewCowSlice(make([]ServiceEventHandler, 0)),
}

func init() {
	ServiceEvents.AddServiceEventHandler(func(event *ServiceEvent) {
		pfxlog.Logger().Tracef("identity %v -> service %v %v", event.IdentityId, event.ServiceId, event.Type.String())
	})
}

type ServiceEvent struct {
	Type       ServiceEventType
	IdentityId string
	ServiceId  string
}

func (self *ServiceEvent) String() string {
	return fmt.Sprintf("service event [identity %v -> service %v %v]", self.IdentityId, self.ServiceId, self.Type.String())
}

type ServiceEventHandler func(event *ServiceEvent)

type ServiceEventsRegistry struct {
	handlers *cowslice.CowSlice
}

func (self *ServiceEventsRegistry) AddServiceEventHandler(listener ServiceEventHandler) {
	cowslice.Append(self.handlers, listener)
}

func (self *ServiceEventsRegistry) RemoveServiceEventHandler(listener ServiceEventHandler) {
	cowslice.Delete(self.handlers, listener)
}

func (self *ServiceEventsRegistry) dispatchEventsAsync(events []*ServiceEvent) {
	go self.dispatchEvents(events)
}

func (self *ServiceEventsRegistry) dispatchEvents(events []*ServiceEvent) {
	for _, event := range events {
		self.dispatchEvent(event)
	}
}

func (self *ServiceEventsRegistry) dispatchEvent(event *ServiceEvent) {
	handlers := self.handlers.Value().([]ServiceEventHandler)
	for _, handler := range handlers {
		handler(event)
	}
}
