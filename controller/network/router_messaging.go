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
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/model"
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

type terminatorInfo struct {
	xt.Terminator
	marker uint64
}

type terminatorValidations struct {
	terminators     map[string]terminatorInfo
	checkInProgress atomic.Bool
	lastSend        time.Time
}

type routerEvent interface {
	handle(c *RouterMessaging)
}

func NewRouterMessaging(env model.Env, routerCommPool goroutines.Pool) *RouterMessaging {
	result := &RouterMessaging{
		env:                     env,
		managers:                env.GetManagers(),
		eventsC:                 make(chan routerEvent, 16),
		routerUpdates:           map[string]*routerUpdates{},
		terminatorValidations:   map[string]*terminatorValidations{},
		routerCommPool:          routerCommPool,
		queuedTerminatorDeletes: map[string]struct{}{},
	}

	env.GetManagers().Terminator.GetStore().AddEntityEventListenerF(result.TerminatorCreated, boltz.EntityCreated)

	return result
}

type RouterMessaging struct {
	env                     model.Env
	managers                *model.Managers
	eventsC                 chan routerEvent
	routerUpdates           map[string]*routerUpdates
	terminatorValidations   map[string]*terminatorValidations
	queuedTerminatorDeletes map[string]struct{}
	routerCommPool          goroutines.Pool
	markerCounter           atomic.Uint64
	deleteInProgress        atomic.Bool
	deleteStarted           concurrenz.AtomicValue[time.Time]
}

func (self *RouterMessaging) getNextMarker() uint64 {
	result := self.markerCounter.Add(1)
	for result == 0 {
		result = self.markerCounter.Add(1)
	}
	return result
}

func (self *RouterMessaging) RouterConnected(r *model.Router) {
	self.routerChanged(r.Id, true)
}

func (self *RouterMessaging) RouterDisconnected(r *model.Router) {
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
	case <-self.env.GetCloseNotifyChannel():
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
		case <-self.env.GetCloseNotifyChannel():
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

		if !self.env.GetManagers().Dispatcher.IsLeaderless() {
			if len(self.terminatorValidations) > 0 {
				self.sendTerminatorValidationRequests()
			}
			self.processQueuedDeletes()
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
			terminators: map[string]terminatorInfo{},
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
		notifyRouter := self.managers.Router.GetConnected(notifyRouterId)
		if notifyRouter == nil {
			// if the router disconnected, we're going to sync everything anyway, so clear anything pending here
			delete(self.routerUpdates, k)
			continue
		}

		if v.sendInProgress.Load() {
			continue
		}

		if !notifyRouter.SupportsRouterLinkMgmt() {
			// if the router doesn't support router based link mgmt, don't send these messages
			delete(self.routerUpdates, k)
			continue
		}

		for routerId := range updates.changedRouters {
			router := self.managers.Router.GetConnected(routerId)
			if router != nil {
				changes.Changes = append(changes.Changes, &ctrl_pb.PeerStateChange{
					Id:        routerId,
					Version:   router.VersionInfo.Version,
					State:     ctrl_pb.PeerState_Healthy,
					Listeners: router.Listeners,
				})
			} else {
				exists, err := self.managers.Router.Exists(routerId)
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
	notifyRouter := self.managers.Router.GetConnected(routerId)
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

	supportsVerifyV2, err := notifyRouter.VersionInfo.HasMinimumVersion("0.31.1")
	if err != nil {
		pfxlog.Logger().WithError(err).Error(`version check of "0.31.1" failed`)
		supportsVerifyV2 = true
	}

	var terminators []*ctrl_pb.Terminator

	localCtrlId := self.env.GetId()

	var toRemove []string
	for _, terminator := range updates.terminators {
		if localCtrlId != terminator.GetSourceCtrl() && !self.managers.Dispatcher.IsLeader() {
			toRemove = append(toRemove, terminator.GetId())
		} else if time.Since(terminator.GetCreatedAt()) > 5*time.Second {
			pfxlog.Logger().WithField("terminatorId", terminator.GetId()).Info("queuing validate of terminator")
			terminators = append(terminators, &ctrl_pb.Terminator{
				Id:      terminator.GetId(),
				Binding: terminator.GetBinding(),
				Address: terminator.GetAddress(),
				Marker:  terminator.marker,
			})
		}
	}

	for _, terminatorId := range toRemove {
		delete(updates.terminators, terminatorId)
	}

	if len(terminators) == 0 || !updates.checkInProgress.CompareAndSwap(false, true) {
		return
	}

	var req interface {
		protobufs.TypedMessage
		GetTerminators() []*ctrl_pb.Terminator
	}

	if !supportsVerifyV2 {
		req = &ctrl_pb.ValidateTerminatorsRequest{Terminators: terminators}
	} else {
		req = &ctrl_pb.ValidateTerminatorsV2Request{
			Terminators: terminators,
			FixInvalid:  true,
		}
	}

	queueErr := self.routerCommPool.QueueOrError(func() {
		ch := notifyRouter.Control
		if ch == nil || self.managers.Dispatcher.IsLeaderless() {
			updates.checkInProgress.Store(false)
			return
		}

		for _, terminator := range req.GetTerminators() {
			pfxlog.Logger().WithField("terminatorId", terminator.GetId()).Debug("queuing validate of terminator")
		}

		if err = protobufs.MarshalTyped(req).WithTimeout(time.Second * 1).SendAndWaitForWire(ch); err != nil {
			pfxlog.Logger().WithError(err).WithField("routerId", notifyRouter.Id).Error("failed to send validate terminators request to router")
		} else if !supportsVerifyV2 {
			// V1 doesn't send responses, it will just send deletes if the terminator is invalid.
			// we're going to mark these ok. If they're not, we should get a delete message. Older
			// routers can still fail to delete, if the delete gets lost for some reason.
			self.generateMockTerminatorValidationResponse(notifyRouter, updates, false)
		}
	})

	if queueErr != nil {
		updates.checkInProgress.Store(false)
	} else {
		updates.lastSend = time.Now()
	}
}

func (self *RouterMessaging) generateMockTerminatorValidationResponse(r *model.Router, validations *terminatorValidations, onlyNonLocal bool) {
	handler := &terminatorValidationRespReceived{
		router: r,
		resp: &ctrl_pb.ValidateTerminatorsV2Response{
			States: map[string]*ctrl_pb.RouterTerminatorState{},
		},
	}

	localCtrlId := self.env.GetId()

	for id, t := range validations.terminators {
		if !onlyNonLocal || t.GetSourceCtrl() != localCtrlId {
			handler.resp.States[id] = &ctrl_pb.RouterTerminatorState{
				Valid:  true,
				Marker: t.marker,
			}
		}
	}

	self.queueEvent(handler)
}

func (self *RouterMessaging) processQueuedDeletes() {
	if len(self.queuedTerminatorDeletes) == 0 {
		return
	}

	if self.deleteInProgress.Load() {
		if time.Since(self.deleteStarted.Load()) < 30*time.Second {
			return
		}
	}

	self.deleteInProgress.Store(false)

	log := pfxlog.Logger()

	var toDelete []string
	count := 0
	for terminatorId := range self.queuedTerminatorDeletes {
		if present, _ := self.env.GetManagers().Terminator.IsEntityPresent(terminatorId); present {
			toDelete = append(toDelete, terminatorId)
			count++
			if count >= 100 {
				break
			}
		} else {
			delete(self.queuedTerminatorDeletes, terminatorId)
		}
	}

	if len(toDelete) == 0 {
		return
	}

	self.deleteInProgress.Store(true)
	self.deleteStarted.Store(time.Now())

	err := self.routerCommPool.QueueOrError(func() {
		changeCtx := change.New().SetChangeAuthorName(self.env.GetId()).
			SetChangeAuthorType(change.AuthorTypeController).
			SetSourceType(change.SourceTypeControlChannel)
		if err := self.env.GetManagers().Terminator.DeleteBatch(toDelete, changeCtx); err != nil {
			for _, terminatorId := range toDelete {
				log.WithField("terminatorId", terminatorId).
					WithError(err).
					Info("batch delete failed")
			}
		} else {
			self.queueEvent(&terminatorBatchDeleteCompleted{
				deletedTerminatorIds: toDelete,
			})
		}
		self.deleteInProgress.Store(false)
	})

	if err != nil {
		log.WithError(err).Error("unable to queue terminator deletes")
		self.deleteInProgress.Store(false)
	}
}

func (self *RouterMessaging) NewValidationResponseHandler(n *Network, r *model.Router) channel.ReceiveHandlerF {
	return func(m *channel.Message, ch channel.Channel) {
		log := pfxlog.Logger().WithField("routerId", r.Id)
		resp := &ctrl_pb.ValidateTerminatorsV2Response{}
		if err := protobufs.TypedResponse(resp).Unmarshall(m, nil); err != nil {
			log.WithError(err).Error("unable to unmarshall validate terminators v2 response")
			return
		}

		handler := &terminatorValidationRespReceived{
			router: r,
			resp:   resp,
		}

		self.queueEvent(handler)
	}
}

func (self *RouterMessaging) ValidateRouterTerminators(terminators []*model.Terminator) {
	self.queueEvent(&validateTerminators{
		terminators: terminators,
	})
}

func (self *RouterMessaging) Inspect() (*inspect.RouterMessagingState, error) {
	evt := &routerMessagingInspectEvent{
		resultC: make(chan *inspect.RouterMessagingState, 1),
	}

	timeout := time.After(time.Second)

	select {
	case self.eventsC <- evt:
	case <-timeout:
		return nil, errors.New("timed out waiting to queue inspect event to router messaging")
	}

	select {
	case result := <-evt.resultC:
		return result, nil
	case <-timeout:
		return nil, errors.New("timed out waiting for inspect result from router messaging")
	}
}

type routerChangedEvent struct {
	routerId  string
	connected bool
}

func (self *routerChangedEvent) handle(c *RouterMessaging) {
	pfxlog.Logger().WithField("routerId", self.routerId).
		WithField("connected", self.connected).
		Info("calculating router updates for router")

	routers := c.managers.Router.AllConnected()

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
	routerStates.terminators[self.terminator.Id] = terminatorInfo{
		Terminator: self.terminator,
		marker:     c.getNextMarker(),
	}
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
	terminators []*model.Terminator
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
		validations.terminators[terminator.Id] = terminatorInfo{
			Terminator: terminator,
			marker:     c.getNextMarker(),
		}
	}
}

type terminatorValidationRespReceived struct {
	router *model.Router
	resp   *ctrl_pb.ValidateTerminatorsV2Response
}

func (self *terminatorValidationRespReceived) handle(c *RouterMessaging) {
	states := c.getTerminatorValidations(self.router.Id)
	defer states.checkInProgress.Store(false)

	for terminatorId, state := range self.resp.States {
		if terminator, ok := states.terminators[terminatorId]; ok {
			if state.Marker == 0 || state.Marker == terminator.marker {
				if !state.Valid {
					pfxlog.Logger().WithField("terminatorId", terminatorId).
						WithField("reason", state.Reason.String()).
						Info("terminator not valid, queuing terminator delete")
					c.queuedTerminatorDeletes[terminatorId] = struct{}{}
				}
				delete(states.terminators, terminatorId)
			}
		}
	}

	if len(states.terminators) == 0 {
		delete(c.terminatorValidations, self.router.Id)
	}
}

type terminatorBatchDeleteCompleted struct {
	deletedTerminatorIds []string
}

func (self *terminatorBatchDeleteCompleted) handle(c *RouterMessaging) {
	for _, terminatorId := range self.deletedTerminatorIds {
		delete(c.queuedTerminatorDeletes, terminatorId)
	}
}

type routerMessagingInspectEvent struct {
	resultC chan *inspect.RouterMessagingState
}

func (self *routerMessagingInspectEvent) handle(c *RouterMessaging) {
	result := &inspect.RouterMessagingState{}

	getRouterName := func(routerId string) string {
		if router, _ := c.managers.Router.Read(routerId); router != nil {
			return router.Name
		}
		return "<unknown>"
	}

	for routerId, updates := range c.routerUpdates {
		u := &inspect.RouterUpdates{
			Router: inspect.RouterInfo{
				Id:   routerId,
				Name: getRouterName(routerId),
			},
			Version: updates.version,
		}
		for updatedRouterId := range updates.changedRouters {
			u.ChangedRouters = append(u.ChangedRouters, inspect.RouterInfo{
				Id:   updatedRouterId,
				Name: getRouterName(updatedRouterId),
			})
		}
		result.RouterUpdates = append(result.RouterUpdates, u)
	}

	for routerId, pendingValidations := range c.terminatorValidations {
		v := &inspect.TerminatorValidations{
			Router: inspect.RouterInfo{
				Id:   routerId,
				Name: getRouterName(routerId),
			},
			CheckInProgress: pendingValidations.checkInProgress.Load(),
			LastSend:        pendingValidations.lastSend.Format("2006-01-02 15:04:05"),
		}

		for terminatorId := range pendingValidations.terminators {
			v.Terminators = append(v.Terminators, terminatorId)
		}

		result.TerminatorValidations = append(result.TerminatorValidations, v)
	}

	select {
	case self.resultC <- result:
	default:
	}
}
