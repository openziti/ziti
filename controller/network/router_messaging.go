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

package network

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/v2/goroutines"
	log "github.com/sirupsen/logrus"
	"sync/atomic"
	"time"
)

type routerUpdates struct {
	version        uint32
	changedRouters map[string]struct{}
	sendInProgress atomic.Bool
}

func (self *routerUpdates) stateUpdated(routerId string) {
	self.version++
	self.changedRouters[routerId] = struct{}{}
}

type routerEvent interface {
	handle(c *RouterMessaging)
}

func NewRouterMessaging(managers *Managers, routerCommPool goroutines.Pool) *RouterMessaging {
	return &RouterMessaging{
		managers:       managers,
		eventsC:        make(chan routerEvent, 16),
		routers:        map[string]*routerUpdates{},
		routerCommPool: routerCommPool,
	}
}

type RouterMessaging struct {
	managers       *Managers
	eventsC        chan routerEvent
	routers        map[string]*routerUpdates
	routerCommPool goroutines.Pool
}

func (self *RouterMessaging) RouterConnected(r *Router) {
	self.routerChanged(r.Id, true)
}

func (self *RouterMessaging) RouterDisconnected(r *Router) {
	self.routerChanged(r.Id, false)
}

func (self *RouterMessaging) RouterDeleted(routerId string) {
	self.routerChanged(routerId, false)
}

func (self *RouterMessaging) routerChanged(routerId string, connected bool) {
	self.queueEvent(&routerChangedEvent{
		routerId:  routerId,
		connected: connected,
	})
}

func (self *RouterMessaging) queueEvent(evt routerEvent) {
	select {
	case self.eventsC <- evt:
	case <-self.managers.network.GetCloseNotify():
	}
}

func (self *RouterMessaging) run() {
	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case evt := <-self.eventsC:
			evt.handle(self)
		case <-ticker.C:
		case <-self.managers.network.GetCloseNotify():
			return
		}

		allEventsInQueueProcessed := false
		for !allEventsInQueueProcessed {
			select {
			case evt := <-self.eventsC:
				evt.handle(self)
			default:
				allEventsInQueueProcessed = true
			}
		}

		if len(self.routers) > 0 {
			self.syncStates()
		}
	}
}

func (self *RouterMessaging) getRouterStates(routerId string) *routerUpdates {
	result, found := self.routers[routerId]
	if !found {
		result = &routerUpdates{
			changedRouters: map[string]struct{}{},
		}
		self.routers[routerId] = result
	}
	return result
}

func (self *RouterMessaging) syncStates() {
	for k, v := range self.routers {
		notifyRouterId := k
		updates := v
		changes := &ctrl_pb.PeerStateChanges{}
		notifyRouter := self.managers.Routers.getConnected(notifyRouterId)
		if notifyRouter == nil {
			continue
		}

		for routerId := range updates.changedRouters {
			router := self.managers.Routers.getConnected(routerId)
			if router != nil {
				changes.Changes = append(changes.Changes, &ctrl_pb.PeerStateChange{
					Id:        routerId,
					Version:   router.VersionInfo.Version,
					State:     ctrl_pb.PeerState_Healthy,
					Listeners: router.Listeners,
				})
			} else if router, _ = self.managers.Routers.Read(routerId); router != nil {
				changes.Changes = append(changes.Changes, &ctrl_pb.PeerStateChange{
					Id:    routerId,
					State: ctrl_pb.PeerState_Unhealthy,
				})
			} else {
				changes.Changes = append(changes.Changes, &ctrl_pb.PeerStateChange{
					Id:    routerId,
					State: ctrl_pb.PeerState_Removed,
				})
			}
		}

		updates.sendInProgress.Store(true)
		currentStatesVersion := updates.version
		queueErr := self.routerCommPool.QueueOrError(func() {
			ch := notifyRouter.Control
			if ch == nil {
				return
			}

			success := true
			if err := protobufs.MarshalTyped(changes).WithTimeout(time.Second * 1).SendAndWaitForWire(ch); err != nil {
				pfxlog.Logger().WithError(err).WithField("routerId", notifyRouter.Id).Error("failed to send peer state changes to router")
				success = false
			}

			self.queueEvent(&routerSendDone{
				routerId: notifyRouter.Id,
				version:  currentStatesVersion,
				success:  success,
				states:   updates,
			})
		})

		if queueErr != nil {
			updates.sendInProgress.Store(false)
		}
	}
}

type routerChangedEvent struct {
	routerId  string
	connected bool
}

func (self *routerChangedEvent) handle(c *RouterMessaging) {
	log.Infof("calculating router updates for router %v, connected=%v", self.routerId, self.connected)
	routers := c.managers.Routers.allConnected()

	var sourceRouterState *routerUpdates
	for _, router := range routers {
		if router.Id == self.routerId {
			continue
		}
		c.getRouterStates(router.Id).stateUpdated(self.routerId)

		if self.connected {
			if sourceRouterState == nil {
				sourceRouterState = c.getRouterStates(self.routerId)
			}
			sourceRouterState.stateUpdated(router.Id)
		}
	}
}

type routerSendDone struct {
	routerId string
	version  uint32
	success  bool
	states   *routerUpdates
}

func (self *routerSendDone) handle(c *RouterMessaging) {
	if states, ok := c.routers[self.routerId]; ok {
		if self.success && self.version == states.version {
			delete(c.routers, self.routerId)
		}
	}
	self.states.sendInProgress.Store(false)
}
