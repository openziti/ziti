/*
	(c) Copyright NetFoundry Inc.

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

package link

import (
	"container/heap"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/xlink"
)

const (
	StatusPending     linkStatus = "pending"
	StatusDialing     linkStatus = "dialing"
	StatusQueueFailed linkStatus = "queueFailed"
	StatusDialFailed  linkStatus = "dialFailed"
	StatusLinkFailed  linkStatus = "linkFailed"
	StatusDestRemoved linkStatus = "destRemoved"
	StatusEstablished linkStatus = "established"
)

type linkStatus string

func (self linkStatus) String() string {
	return string(self)
}

func newLinkDest(destId string) *linkDest {
	return &linkDest{
		id:          destId,
		healthy:     true,
		unhealthyAt: time.Time{},
		linkMap:     map[string]*linkState{},
	}
}

// removedDest marks a destination this router was told is gone, so a peer state computed before the removal
// cannot recreate it. Router ids are never reused, so the marker is settled fact rather than a guess.
type removedDest struct {
	storeIndex uint64
	at         time.Time
}

type linkDest struct {
	id          string
	version     concurrenz.AtomicValue[string]
	healthy     bool
	unhealthyAt time.Time
	linkMap     map[string]*linkState

	// listeners caches the most recent listener set advertised by this
	// destination router. Stored so we can re-evaluate matches when the
	// LOCAL dialer set changes (in addition to the existing path that
	// re-evaluates on peer listener changes via UpdateLinkDest).
	listeners []*ctrl_pb.Listener
}

func (self *linkDest) update(update *linkDestUpdate) {
	if self.healthy && !update.healthy {
		self.unhealthyAt = time.Now()
	}

	self.healthy = update.healthy

	if update.healthy {
		self.version.Store(update.version)
	}
}

type linkFault struct {
	linkId    string
	iteration uint32
}

type linkState struct {
	linkKey        string
	linkId         string
	status         linkStatus
	dialAttempts   atomic.Uint64
	connectedCount uint64
	retryDelay     time.Duration
	nextDial       time.Time
	dest           *linkDest
	listener       *ctrl_pb.Listener
	dialer         xlink.Dialer
	allowedDials   int64
	ctrlsNotified  bool
	linkFaults     []linkFault
	dialActive     atomic.Bool
	link           xlink.Xlink
}

func (self *linkState) updateStatus(status linkStatus) {
	if self.status != status {
		log := pfxlog.Logger().
			WithField("key", self.linkKey).
			WithField("oldState", self.status).
			WithField("newState", status).
			WithField("linkId", self.linkId).
			WithField("iteration", self.dialAttempts.Load())
		self.status = status
		log.Info("status updated")
		if self.status != StatusEstablished {
			self.link = nil
		}
	}
}

// newDialAttempt copies what a dial needs from the state. Call it on the registry's event loop, which owns the
// state.
func (self *linkState) newDialAttempt(iteration uint32) *dialAttempt {
	return &dialAttempt{
		dialer:          self.dialer,
		linkId:          self.linkId,
		routerId:        self.dest.id,
		address:         self.listener.Address,
		linkProtocol:    self.listener.Protocol,
		listenerBinding: self.listener.LocalBinding,
		routerVersion:   self.dest.version.Load(),
		iteration:       iteration,
	}
}

// dialAttempt is one dial of a linkState, fixed when the dial is queued, and is the xlink.Dial its dialer is
// handed. The dial runs on the dialer pool while the event loop goes on updating the state, which can move
// the listener's address or replace the dialer, so the dial reads only this copy.
type dialAttempt struct {
	dialer          xlink.Dialer
	linkId          string
	routerId        string
	address         string
	linkProtocol    string
	listenerBinding string
	routerVersion   string
	iteration       uint32
}

func (self *dialAttempt) GetLinkId() string          { return self.linkId }
func (self *dialAttempt) GetRouterId() string        { return self.routerId }
func (self *dialAttempt) GetAddress() string         { return self.address }
func (self *dialAttempt) GetLinkProtocol() string    { return self.linkProtocol }
func (self *dialAttempt) GetListenerBinding() string { return self.listenerBinding }
func (self *dialAttempt) GetRouterVersion() string   { return self.routerVersion }
func (self *dialAttempt) GetIteration() uint32       { return self.iteration }

func (self *linkState) addPendingLinkFault(linkId string, iteration uint32) {
	log := pfxlog.Logger().WithField("linkId", linkId).WithField("iteration", iteration)
	for idx, fault := range self.linkFaults {
		if fault.linkId == linkId {
			if fault.iteration < iteration {
				log.Info("updating link fault")
				// note 'fault' is not a pointer, so it's a copy and if we update it, the entry in the slice won't change
				self.linkFaults[idx].iteration = iteration
			} else {
				log.Info("link fault covered by existing link fault")
			}
			return
		}
	}

	self.linkFaults = append(self.linkFaults, linkFault{
		linkId:    linkId,
		iteration: iteration,
	})
}

func (self *linkState) clearFaultsForLinkId(linkId string) {
	faults := self.linkFaults
	self.linkFaults = nil

	for _, fault := range faults {
		if fault.linkId != linkId {
			self.linkFaults = append(self.linkFaults, fault)
		}
	}
}

func (self *linkState) clearFault(toClear linkFault) {
	faults := self.linkFaults
	self.linkFaults = nil

	for _, fault := range faults {
		if fault.linkId != toClear.linkId || fault.iteration > toClear.iteration {
			self.linkFaults = append(self.linkFaults, fault)
		}
	}
}

func (self *linkState) dialFailed(registry *linkRegistryImpl, applyFailed bool) {
	if self.allowedDials > 0 {
		self.allowedDials--
	}

	if self.allowedDials == 0 {
		delete(self.dest.linkMap, self.linkKey)
		return
	}

	backoffConfig := self.dialer.GetHealthyBackoffConfig()
	if !self.dest.healthy {
		backoffConfig = self.dialer.GetUnhealthyBackoffConfig()
	}

	factor := backoffConfig.GetRetryBackoffFactor() + (rand.Float64() - 0.5)
	if factor < 1 {
		factor = 1
	}

	self.retryDelay = time.Duration(float64(self.retryDelay) * factor)
	if self.retryDelay < backoffConfig.GetMinRetryInterval() {
		self.retryDelay = backoffConfig.GetMinRetryInterval()
	}

	if self.retryDelay > backoffConfig.GetMaxRetryInterval() {
		self.retryDelay = backoffConfig.GetMaxRetryInterval()
	}

	self.nextDial = time.Now().Add(self.retryDelay)

	if applyFailed {
		// if apply failed, likely due to a duplication, apply a random delay to the redial to try and avoid
		// conflicting dials
		self.nextDial = self.nextDial.Add(time.Duration(rand.Int31n(4)+1) * time.Second)
	}

	heap.Push(registry.linkStateQueue, self)
}
