package events

import (
	"fmt"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/util/cowslice"
	"github.com/pkg/errors"
	"reflect"
	"time"
)

func registerRouterEventHandler(val interface{}, config map[interface{}]interface{}) error {
	handler, ok := val.(RouterEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/events/RouterEventHandler interface.", reflect.TypeOf(val))
	}

	AddRouterEventHandler(handler)

	return nil
}

type RouterEvent struct {
	Namespace    string    `json:"namespace"`
	EventType    string    `json:"event_type"`
	Timestamp    time.Time `json:"timestamp"`
	RouterId     string    `json:"router_id"`
	RouterOnline bool      `json:"router_online"`
}

var routerEventHandlerRegistry = cowslice.NewCowSlice(make([]RouterEventHandler, 0))

func getRouterEventHandlers() []RouterEventHandler {
	return routerEventHandlerRegistry.Value().([]RouterEventHandler)
}

func (event *RouterEvent) String() string {
	return fmt.Sprintf("%v.%v time=%v routerId=%v routerOnline=%v",
		event.Namespace, event.EventType, event.Timestamp, event.RouterId, event.RouterOnline)
}

type RouterEventHandler interface {
	AcceptRouterEvent(event *RouterEvent)
}

func InitRouterEventRouter(n *network.Network) {
	r := &routerEventRouter{}
	n.AddRouterPresenceHandler(r)
}

type routerEventRouter struct{}

func (self *routerEventRouter) RouterConnected(r *network.Router) {
	self.routerChange("router-online", r, true)
}

func (self *routerEventRouter) RouterDisconnected(r *network.Router) {
	self.routerChange("router-offline", r, false)
}

func (self *routerEventRouter) routerChange(eventType string, r *network.Router, online bool) {
	event := &RouterEvent{
		Namespace:    "fabric.routers",
		EventType:    eventType,
		Timestamp:    time.Now(),
		RouterId:     r.Id,
		RouterOnline: online,
	}

	for _, handler := range getRouterEventHandlers() {
		go handler.AcceptRouterEvent(event)
	}
}
