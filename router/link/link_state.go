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
		// A brand new destination is treated as just affirmed, so it gets the full grace period before it
		// can be considered abandoned rather than being eligible for removal the moment it appears.
		lastAffirmedAt: time.Now(),
	}
}

type linkDest struct {
	id          string
	version     concurrenz.AtomicValue[string]
	healthy     bool
	unhealthyAt time.Time
	linkMap     map[string]*linkState
	ctrlHealth  map[string]bool // per-controller health reports
	// lastAffirmedAt is the last time a controller the router was connected to said this destination was up.
	// It drives removal of an abandoned destination, which healthy cannot: healthy is a sticky union over
	// every report ever received, so a controller that said "up" and then went away holds it true forever.
	// See affirmedBy for why removal asks a different question than the dial backoff does.
	lastAffirmedAt time.Time

	// listeners caches the most recent listener set advertised by this
	// destination router. Stored so we can re-evaluate matches when the
	// LOCAL dialer set changes (in addition to the existing path that
	// re-evaluates on peer listener changes via UpdateLinkDest).
	listeners []*ctrl_pb.Listener
}

// updateHealthFromCtrl records a health report from a specific controller.
func (self *linkDest) updateHealthFromCtrl(ctrlId string, healthy bool) {
	if self.ctrlHealth == nil {
		self.ctrlHealth = map[string]bool{}
	}
	self.ctrlHealth[ctrlId] = healthy
}

// isHealthy returns true if any controller reports the destination as healthy.
func (self *linkDest) isHealthy() bool {
	for _, healthy := range self.ctrlHealth {
		if healthy {
			return true
		}
	}
	return false
}

// affirmedBy reports whether any of the given controllers says this destination is up. It exists so that
// deciding a destination has been abandoned can ignore reports from controllers the router is no longer
// connected to, which isHealthy deliberately does not.
//
// The two want different answers. The dial backoff reads isHealthy, and treating a controller outage as
// "nobody says this is up" would switch every destination to the unhealthy backoff, whose retry interval
// starts at a minute and multiplies by ten toward an hour; a controller restart would then stall re-dialing
// links that are perfectly fine. Removal has no such problem, because the caller only acts once a destination
// has gone unaffirmed for far longer than any outage lasts.
func (self *linkDest) affirmedBy(ctrlIds map[string]struct{}) bool {
	for ctrlId, healthy := range self.ctrlHealth {
		if healthy {
			if _, ok := ctrlIds[ctrlId]; ok {
				return true
			}
		}
	}
	return false
}

// abandoned reports whether the router should stop tracking this destination: either there is nothing left to
// dial, or nothing established to it and no controller has affirmed it in long enough that the notification
// saying it was deleted must have been missed.
//
// An established link keeps a destination regardless of affirmation. Affirmation lapses on a controller
// outage, and a link that is up is better evidence that the destination is there than a controller's silence
// is that it is not.
func (self *linkDest) abandoned(hasEstablishedLinks bool, now time.Time) bool {
	if len(self.linkMap) == 0 {
		return true
	}
	return !hasEstablishedLinks && now.Sub(self.lastAffirmedAt) > destAbandonedAfter
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
		oldState := self.status
		self.status = status
		linkLog.Info("status updated",
			"key", self.linkKey,
			"oldState", oldState,
			"newState", status,
			"linkId", self.linkId,
			"iteration", self.dialAttempts.Load())
		if self.status != StatusEstablished {
			self.link = nil
		}
	}
}

func (self *linkState) GetLinkKey() string {
	return self.linkKey
}

func (self *linkState) GetLinkId() string {
	return self.linkId
}

func (self *linkState) GetRouterId() string {
	return self.dest.id
}

func (self *linkState) GetAddress() string {
	return self.listener.Address
}

func (self *linkState) GetLinkProtocol() string {
	return self.listener.Protocol
}

func (self *linkState) GetRouterVersion() string {
	return self.dest.version.Load()
}

func (self *linkState) GetIteration() uint32 {
	return uint32(self.dialAttempts.Load())
}

func (self *linkState) addPendingLinkFault(linkId string, iteration uint32) {
	for idx, fault := range self.linkFaults {
		if fault.linkId == linkId {
			if fault.iteration < iteration {
				linkLog.Info("updating link fault", "linkId", linkId, "iteration", iteration)
				// note 'fault' is not a pointer, so it's a copy and if we update it, the entry in the slice won't change
				self.linkFaults[idx].iteration = iteration
			} else {
				linkLog.Info("link fault covered by existing link fault", "linkId", linkId, "iteration", iteration)
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
