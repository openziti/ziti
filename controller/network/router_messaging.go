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
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/xt"
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

type terminatorValidations struct {
	terminators     map[string]xt.Terminator
	checkInProgress atomic.Bool
	lastSend        time.Time
}

type routerEvent interface {
	handle(c *RouterMessaging)
}

func NewRouterMessaging(managers *Managers, routerCommPool goroutines.Pool) *RouterMessaging {
	result := &RouterMessaging{
		managers:              managers,
		eventsC:               make(chan routerEvent, 16),
		routerUpdates:         map[string]*routerUpdates{},
		terminatorValidations: map[string]*terminatorValidations{},
		routerCommPool:        routerCommPool,
	}

	managers.stores.Terminator.AddEntityEventListenerF(result.TerminatorCreated, boltz.EntityCreated)

	return result
}

type RouterMessaging struct {
	managers              *Managers
	eventsC               chan routerEvent
	routerUpdates         map[string]*routerUpdates
	terminatorValidations map[string]*terminatorValidations
	routerCommPool        goroutines.Pool
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

func (self *RouterMessaging) TerminatorCreated(terminator *db.Terminator) {
	self.queueEvent(&terminatorCreatedEvent{
		terminator: terminator,
	})
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

		if len(self.routerUpdates) > 0 {
			self.syncStates()
		}

		if len(self.terminatorValidations) > 0 {
			self.sendTerminatorValidationRequests()
		}
	}
}

func (self *RouterMessaging) getRouterStates(routerId string) *routerUpdates {
	result, found := self.routerUpdates[routerId]
	if !found {
		result = &routerUpdates{
			changedRouters: map[string]struct{}{},
		}
		self.routerUpdates[routerId] = result
	}
	return result
}

func (self *RouterMessaging) getTerminatorValidations(routerId string) *terminatorValidations {
	result, found := self.terminatorValidations[routerId]
	if !found {
		result = &terminatorValidations{
			terminators: map[string]xt.Terminator{},
		}
		self.terminatorValidations[routerId] = result
	}
	return result
}

func (self *RouterMessaging) syncStates() {
	for k, v := range self.routerUpdates {
		notifyRouterId := k
		updates := v
		changes := &ctrl_pb.PeerStateChanges{}
		notifyRouter := self.managers.Routers.getConnected(notifyRouterId)
		if notifyRouter == nil {
			// if the router disconnected, we're going to sync everything anyway, so clear anything pending here
			delete(self.routerUpdates, k)
			continue
		}

		if v.sendInProgress.Load() {

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
			} else {
				exists, err := self.managers.Routers.Exists(routerId)
				if exists && err == nil {
					changes.Changes = append(changes.Changes, &ctrl_pb.PeerStateChange{
						Id:    routerId,
						State: ctrl_pb.PeerState_Unhealthy,
					})
				} else if err != nil {
					pfxlog.Logger().WithError(err).
						WithField("notifyRouterId", notifyRouter).
						WithField("routerId", routerId).
						Error("failed to check if router exists")
				}

				if !exists && err == nil {
					changes.Changes = append(changes.Changes, &ctrl_pb.PeerStateChange{
						Id:    routerId,
						State: ctrl_pb.PeerState_Removed,
					})
				}
			}
		}

		if !updates.sendInProgress.CompareAndSwap(false, true) {
			continue
		}

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

			self.queueEvent(&routerPeerChangesSendDone{
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

func (self *RouterMessaging) sendTerminatorValidationRequests() {
	for routerId, updates := range self.terminatorValidations {
		self.sendTerminatorValidationRequest(routerId, updates)
	}
}

func (self *RouterMessaging) sendTerminatorValidationRequest(routerId string, updates *terminatorValidations) {
	notifyRouter := self.managers.Routers.getConnected(routerId)
	if notifyRouter == nil {
		// if the router disconnected, we're going to sync everything anyway, so clear anything pending here
		delete(self.terminatorValidations, routerId)
		return
	}

	if updates.checkInProgress.Load() {
		if time.Since(updates.lastSend) > 3*time.Minute {
			updates.checkInProgress.Store(false)
		} else {
			return
		}
	}

	var terminators []*ctrl_pb.Terminator

	for _, terminator := range updates.terminators {
		if time.Since(terminator.GetCreatedAt()) > 5*time.Second {
			terminators = append(terminators, &ctrl_pb.Terminator{
				Id:      terminator.GetId(),
				Binding: terminator.GetBinding(),
				Address: terminator.GetAddress(),
			})
		}
	}

	if len(terminators) == 0 || !updates.checkInProgress.CompareAndSwap(false, true) {
		return
	}

	req := &ctrl_pb.ValidateTerminatorsV2Request{
		Terminators: terminators,
		FixInvalid:  false,
	}

	queueErr := self.routerCommPool.QueueOrError(func() {
		ch := notifyRouter.Control
		if ch == nil {
			return
		}

		if self.managers.Dispatcher.IsLeaderOrLeaderless() {
			if err := protobufs.MarshalTyped(req).WithTimeout(time.Second * 1).SendAndWaitForWire(ch); err != nil {
				pfxlog.Logger().WithError(err).WithField("routerId", notifyRouter.Id).Error("failed to send validate terminators request to router")
			}
		}
	})

	if queueErr != nil {
		updates.checkInProgress.Store(false)
	} else {
		updates.lastSend = time.Now()
	}
}

func (self *RouterMessaging) NewValidationResponseHandler(n *Network, r *Router) channel.ReceiveHandlerF {
	return func(m *channel.Message, ch channel.Channel) {
		log := pfxlog.Logger().WithField("routerId", r.Id)
		resp := &ctrl_pb.ValidateTerminatorsV2Response{}
		if err := protobufs.TypedResponse(resp).Unmarshall(m, nil); err != nil {
			log.WithError(err).Error("unable to unmarshall validate terminators v2 response")
			return
		}

		changeCtx := change.NewControlChannelChange(r.Id, r.Name, "fabric.validate.terminator", ch)

		handler := &terminatorValidationRespReceived{
			router:    r,
			changeCtx: changeCtx,
			resp:      resp,
		}
		handler.DeleteInvalid(n)
		self.queueEvent(handler)
	}
}

func (self *RouterMessaging) ValidateRouterTerminators(router *Router, terminators []*Terminator) {
	self.queueEvent(&validateTerminators{
		router:      router,
		terminators: terminators,
	})
}

type routerChangedEvent struct {
	routerId  string
	connected bool
}

func (self *routerChangedEvent) handle(c *RouterMessaging) {
	pfxlog.Logger().WithField("routerId", self.routerId).
		WithField("connected", self.connected).
		Info("calculating router updates for router")

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

type terminatorCreatedEvent struct {
	terminator *db.Terminator
}

func (self *terminatorCreatedEvent) handle(c *RouterMessaging) {
	routerStates := c.getTerminatorValidations(self.terminator.Router)
	routerStates.terminators[self.terminator.Id] = self.terminator
}

type routerPeerChangesSendDone struct {
	routerId string
	version  uint32
	success  bool
	states   *routerUpdates
}

func (self *routerPeerChangesSendDone) handle(c *RouterMessaging) {
	defer self.states.sendInProgress.Store(false)

	if states, ok := c.routerUpdates[self.routerId]; ok {
		if self.success && self.version == states.version {
			delete(c.routerUpdates, self.routerId)
		}
	}
}

type validateTerminators struct {
	router      *Router
	terminators []*Terminator
}

func (self *validateTerminators) handle(c *RouterMessaging) {
	var currentRouterId string
	var validations *terminatorValidations

	routers := map[string]*terminatorValidations{}

	for _, terminator := range self.terminators {
		if terminator.Router != currentRouterId || validations == nil {
			validations = c.getTerminatorValidations(terminator.Router)
			currentRouterId = terminator.Router
			routers[currentRouterId] = validations
		}
		validations.terminators[terminator.Id] = terminator
	}

	for routerId, updates := range routers {
		c.sendTerminatorValidationRequest(routerId, updates)
	}
}

type terminatorValidationRespReceived struct {
	router    *Router
	changeCtx *change.Context
	resp      *ctrl_pb.ValidateTerminatorsV2Response
	success   bool
}

func (self *terminatorValidationRespReceived) DeleteInvalid(n *Network) {
	log := pfxlog.Logger().WithField("routerId", self.router.Id)

	var toDelete []string
	for terminatorId, state := range self.resp.States {
		if !state.Valid {
			toDelete = append(toDelete, terminatorId)
			log.WithField("terminatorId", terminatorId).
				WithField("reason", state.Reason.String()).
				Info("queuing terminator for delete")
		}
	}

	if len(toDelete) > 0 {
		if err := n.Managers.Terminators.DeleteBatch(toDelete, self.changeCtx); err != nil {
			for _, terminatorId := range toDelete {
				log.WithField("terminatorId", terminatorId).
					WithError(err).
					Info("batch delete failed")
			}
		} else {
			self.success = true
		}
	}
}

func (self *terminatorValidationRespReceived) handle(c *RouterMessaging) {
	states := c.getTerminatorValidations(self.router.Id)
	defer states.checkInProgress.Store(false)

	for terminatorId, state := range self.resp.States {
		if state.Valid || self.success {
			delete(states.terminators, terminatorId)
		}
	}

	if len(states.terminators) == 0 {
		delete(c.terminatorValidations, self.router.Id)
	}
}
