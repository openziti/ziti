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

package handler_ctrl

import (
	"container/heap"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/controller/model"
)

type dialEvent interface {
	handle(d *CtrlDialer)
}

type routerDisconnectedEvent struct {
	routerId string
	router   *model.Router
}

func (self *routerDisconnectedEvent) handle(d *CtrlDialer) {
	log := pfxlog.Logger().WithField("routerId", self.routerId)

	state, exists := d.states[self.routerId]
	if !exists {
		addresses := d.getRouterIdEndpointsNeedingDial(self.routerId, self.router)
		if len(addresses) == 0 {
			return
		}
		state = &routerDialState{
			routerId:  self.routerId,
			addresses: addresses,
		}
		d.states[self.routerId] = state
	}

	state.connectionLost(d.config)
	heap.Push(&d.retryQueue, state)

	log.WithField("nextDial", time.Until(state.nextDial).Round(time.Millisecond)).
		Debug("router disconnected, queued for redial")
}

type routerUpdatedEvent struct {
	routerId string
}

func (self *routerUpdatedEvent) handle(d *CtrlDialer) {
	state, exists := d.states[self.routerId]
	if exists && state.status == statusConnected {
		return
	}

	addresses := d.getRouterIdEndpointsNeedingDial(self.routerId, nil)
	if len(addresses) == 0 {
		if exists {
			delete(d.states, self.routerId)
		}
		return
	}

	if !exists {
		state = &routerDialState{
			routerId:  self.routerId,
			addresses: addresses,
			nextDial:  time.Now(),
		}
		d.states[self.routerId] = state
		heap.Push(&d.retryQueue, state)
	} else {
		state.addresses = addresses
		if state.addrIndex >= len(state.addresses) {
			state.addrIndex = state.addrIndex % len(state.addresses)
		}
	}
}

type routerConnectedEvent struct {
	routerId string
}

func (self *routerConnectedEvent) handle(d *CtrlDialer) {
	if state, exists := d.states[self.routerId]; exists {
		state.status = statusConnected
		state.connectedAt = time.Now()
		state.retryDelay = 0
		state.dialAttempts = 0
	}
	// don't delete from states map — let scan/evaluate skip connected routers naturally
}

type routerDeletedEvent struct {
	routerId string
}

func (self *routerDeletedEvent) handle(d *CtrlDialer) {
	delete(d.states, self.routerId)
}

type dialResultEvent struct {
	routerId string
	err      error
}

func (self *dialResultEvent) handle(d *CtrlDialer) {
	state, exists := d.states[self.routerId]
	if !exists {
		return
	}

	log := pfxlog.Logger().WithField("routerId", self.routerId)

	if self.err != nil {
		state.dialFailed(d.config)
		log.WithError(self.err).
			WithField("retryDelay", state.retryDelay).
			WithField("nextDial", time.Until(state.nextDial).Round(time.Millisecond)).
			Warn("dial attempt failed, will retry with backoff")
		heap.Push(&d.retryQueue, state)
		return
	}

	state.dialSucceeded()
	// schedule a re-check after FastFailureWindow to detect fast failures
	state.nextDial = time.Now().Add(d.config.FastFailureWindow)
	heap.Push(&d.retryQueue, state)

	log.Info("successfully connected to router ctrl channel")
}

type validateDialStateSnapshot struct {
	routerId string
	status   routerDialStatus
}

type validateDialStatesEvent struct {
	result chan map[string]*validateDialStateSnapshot
}

func (self *validateDialStatesEvent) handle(d *CtrlDialer) {
	states := make(map[string]*validateDialStateSnapshot, len(d.states))
	for k, v := range d.states {
		states[k] = &validateDialStateSnapshot{
			routerId: v.routerId,
			status:   v.status,
		}
	}
	self.result <- states
}

type inspectDialStatesEvent struct {
	result chan *inspect.CtrlDialerInspectResult
}

func (self *inspectDialStatesEvent) handle(d *CtrlDialer) {
	result := &inspect.CtrlDialerInspectResult{
		Enabled: true,
		Config: inspect.CtrlDialerConfigDetail{
			Groups:             d.config.Groups,
			DialDelay:          d.config.DialDelay.String(),
			MinRetryInterval:   d.config.MinRetryInterval.String(),
			MaxRetryInterval:   d.config.MaxRetryInterval.String(),
			RetryBackoffFactor: d.config.RetryBackoffFactor,
			FastFailureWindow:  d.config.FastFailureWindow.String(),
			QueueSize:          d.config.QueueSize,
			MaxWorkers:         d.config.MaxWorkers,
		},
	}

	for _, state := range d.states {
		result.Routers = append(result.Routers, &inspect.CtrlDialerRouterDial{
			RouterId:       state.routerId,
			Addresses:      state.addresses,
			CurrentAddress: state.currentAddress(),
			Status:         state.status.String(),
			DialAttempts:   state.dialAttempts,
			RetryDelay:     state.retryDelay.String(),
			NextDial:       time.Until(state.nextDial).Round(time.Millisecond).String(),
		})
	}

	self.result <- result
}
