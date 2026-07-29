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
	"sync/atomic"
	"time"

	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/idgen"
	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/pkg/errors"
)

const (
	GroupDefault = "default"
)

type event interface {
	Handle(registry *linkRegistryImpl)
}

type removeLinkDest struct {
	id string
}

func (self *removeLinkDest) Handle(registry *linkRegistryImpl) {
	dest := registry.destinations[self.id]
	delete(registry.destinations, self.id)
	if dest != nil {
		for _, state := range dest.linkMap {
			state.updateStatus(StatusDestRemoved)
			if link, _ := registry.GetLink(state.linkId); link != nil {
				if err := link.Close(); err != nil {
					linkLog.Error("error closing link",
						"linkKey", state.linkKey,
						"linkId", link.Id(),
						"error", err)
				}
			}
		}
	}
}

type linkDestUpdate struct {
	id        string
	ctrlId    string
	version   string
	healthy   bool
	listeners []*ctrl_pb.Listener
}

func (self *linkDestUpdate) Handle(registry *linkRegistryImpl) {
	dest := registry.destinations[self.id]

	if dest == nil {
		dest = newLinkDest(self.id)
		registry.destinations[self.id] = dest
	}

	wasHealthy := dest.healthy
	dest.updateHealthFromCtrl(self.ctrlId, self.healthy)
	dest.healthy = dest.isHealthy()

	if !wasHealthy && dest.healthy {
		dest.unhealthyAt = time.Time{}
	} else if wasHealthy && !dest.healthy {
		dest.unhealthyAt = time.Now()
	}

	// Only apply listener changes when this update reports healthy — an
	// unhealthy update has no listeners and the orphan cleanup would
	// incorrectly remove states created by a healthy controller's update.
	if self.healthy {
		dest.version.Store(self.version)
		becameHealthy := !wasHealthy && dest.healthy
		self.ApplyListenerChanges(registry, dest, becameHealthy)
	}
}

func (self *linkDestUpdate) ApplyListenerChanges(registry *linkRegistryImpl, dest *linkDest, becameHealthy bool) {
	currentLinkKeys := map[string]struct{}{}

	for k := range dest.linkMap {
		currentLinkKeys[k] = struct{}{}
	}

	for _, listener := range self.listeners {
		for _, dialer := range registry.env.GetXlinkDialers() {
			if stringz.ContainsAny(listener.Groups, dialer.GetGroups()...) {
				linkKey := registry.GetLinkKey(dialer.GetBinding(), listener.Protocol, self.id, listener.GetLocalBinding())

				delete(currentLinkKeys, linkKey)

				log := linkLog.With("routerId", self.id,
					"address", listener.Address,
					"linkKey", linkKey)

				existingLinkState, ok := dest.linkMap[linkKey]
				if !ok {
					newLinkState := &linkState{
						linkKey:      linkKey,
						linkId:       idgen.MustNewUUIDString(),
						status:       StatusPending,
						dest:         dest,
						listener:     listener,
						dialer:       dialer,
						allowedDials: -1,
					}
					dest.linkMap[linkKey] = newLinkState
					log.Info("new potential link")
					registry.scheduleNewLink(newLinkState)
				} else {
					log.Info("link already known")
					if existingLinkState.listener.Address != listener.Address {
						log.Info("link address changed, updating",
							"oldAddr", existingLinkState.listener.Address,
							"newAddr", listener.Address)
						if existingLinkState.link != nil {
							if err := existingLinkState.link.Close(); err != nil {
								log.Error("error closing existing link", "error", err)
							}
						}
					}
					existingLinkState.listener = listener // even if the key is the same, the address could have changed

					// if link isn't established, try establishing now
					if becameHealthy && existingLinkState.status != StatusEstablished {
						existingLinkState.retryDelay = time.Duration(0)
						existingLinkState.nextDial = time.Now()
						registry.evaluateLinkState(existingLinkState)
					}
				}
			}
		}
	}

	// anything left is an orphaned link entry
	for linkKey := range currentLinkKeys {
		if v, ok := dest.linkMap[linkKey]; ok {
			// this will prevent the link from being recreated once closed
			delete(dest.linkMap, linkKey)
			if v.link != nil {
				linkLog.Info("closing link as link groups no longer align",
					"routerId", self.id, "linkKey", linkKey)
				_ = v.link.Close()
			}
		}
	}
}

type updateLinkStatusForLink struct {
	link   xlink.Xlink
	status linkStatus
}

func (self *updateLinkStatusForLink) Handle(registry *linkRegistryImpl) {
	link := self.link
	log := linkLog.With("linkKey", link.Key(), "linkId", link.Id())
	dest, found := registry.destinations[link.DestinationId()]
	if !found {
		if link.IsDialed() { // if link was created by listener, rather than dialer we may not have an entry for it
			log.Warn("unable to mark link, link destination not present in registry",
				"linkDest", link.DestinationId(), "status", self.status)
		}
		return
	}

	state, found := dest.linkMap[link.Key()]
	if !found {
		if link.IsDialed() { // if link was created by listener, rather than dialer we may not have an entry for it
			log.Warn("unable to mark link, link state not present in registry",
				"linkDest", link.DestinationId(), "status", self.status)
		}
		return
	}

	if state.status == StatusDestRemoved {
		return
	}

	state.updateStatus(self.status)
	if state.status == StatusEstablished {
		state.connectedCount++
		state.retryDelay = time.Duration(0)
		state.ctrlsNotified = false
		state.link = self.link
		registry.triggerNotify()
	}

	if state.status == StatusLinkFailed {
		// Use the healthy min retry interval instead of zero to prevent a
		// tight establish→fail→retry loop when links keep failing shortly
		// after establishment (e.g., multi-underlay connection failures
		// under load).
		minRetry := state.dialer.GetHealthyBackoffConfig().GetMinRetryInterval()
		state.retryDelay = minRetry
		state.nextDial = time.Now().Add(minRetry)
		heap.Push(registry.linkStateQueue, state)

		if notifier := registry.env.GetLinkGossipNotifier(); notifier != nil &&
			registry.ctrls.AllControllersHaveCapability(capabilities.ControllerLinkGossip) {
			if err := notifier.NotifyLinkFault(link.Id(), link.Iteration()); err != nil {
				// Send failed — fall back to pending fault so it gets retried
				// on the next notification cycle.
				state.addPendingLinkFault(link.Id(), link.Iteration())
			}
		} else {
			state.addPendingLinkFault(link.Id(), link.Iteration())
		}

		state.link = nil
	}
}

type addLinkFaultForReplacedLink struct {
	link xlink.Xlink
}

func (self *addLinkFaultForReplacedLink) Handle(registry *linkRegistryImpl) {
	link := self.link
	log := linkLog.With("linkKey", link.Key(), "linkId", link.Id())
	dest, found := registry.destinations[link.DestinationId()]
	if !found {
		if link.IsDialed() { // if link was created by listener, rather than dialer we may not have an entry for it
			log.Info("link destination not present in registry", "linkDest", link.DestinationId())
		}
		return
	}

	state, found := dest.linkMap[link.Key()]
	if !found {
		if link.IsDialed() { // if link was created by listener, rather than dialer we may not have an entry for it
			log.Info("link state not present in registry", "linkDest", link.DestinationId())
		}
		return
	}

	state.addPendingLinkFault(link.Id(), link.Iteration())
}

type updateLinkStatusToDialFailed struct {
	linkState   *linkState
	applyFailed bool
}

func (self *updateLinkStatusToDialFailed) Handle(registry *linkRegistryImpl) {
	if self.linkState.status == StatusDialing {
		self.linkState.updateStatus(StatusDialFailed)
		self.linkState.dialFailed(registry, self.applyFailed)
	}
}

type inspectLinkStatesEvent struct {
	result atomic.Pointer[[]*inspect.LinkDest]
	done   chan struct{}
}

func (self *inspectLinkStatesEvent) Handle(registry *linkRegistryImpl) {
	var result []*inspect.LinkDest
	for _, dest := range registry.destinations {
		inspectDest := &inspect.LinkDest{
			Id:      dest.id,
			Version: dest.version.Load(),
			Healthy: dest.healthy,
		}
		unhealthySince := dest.unhealthyAt
		if !dest.healthy {
			inspectDest.UnhealthySince = &unhealthySince
		}

		for _, state := range dest.linkMap {
			establishedLinkId := ""
			if link := state.link; link != nil {
				establishedLinkId = link.Id()
			}
			inspectLinkState := &inspect.LinkState{
				Id:                state.linkId,
				Key:               state.linkKey,
				Status:            state.status.String(),
				DialAttempts:      state.dialAttempts.Load(),
				ConnectedCount:    state.connectedCount,
				RetryDelay:        state.retryDelay.String(),
				NextDial:          state.nextDial.Format(time.RFC3339),
				TargetAddress:     state.listener.Address,
				TargetGroups:      state.listener.Groups,
				TargetBinding:     state.listener.LocalBinding,
				DialerGroups:      state.dialer.GetGroups(),
				DialerBinding:     state.dialer.GetBinding(),
				CtrlsNotified:     state.ctrlsNotified,
				EstablishedLinkId: establishedLinkId,
			}
			if inspectLinkState.TargetBinding == "" {
				inspectLinkState.TargetBinding = "default"
			}
			if inspectLinkState.DialerBinding == "" {
				inspectLinkState.DialerBinding = "default"
			}
			inspectDest.LinkStates = append(inspectDest.LinkStates, inspectLinkState)
		}

		result = append(result, inspectDest)
	}
	self.result.Store(&result)
	close(self.done)
}

func (self *inspectLinkStatesEvent) GetResults(timeout time.Duration) ([]*inspect.LinkDest, error) {
	select {
	case <-self.done:
		return *self.result.Load(), nil
	case <-time.After(timeout):
		return nil, errors.New("timed out waiting for result")
	}
}

type markNewLinksNotified struct {
	links []stateAndLink
}

func (self *markNewLinksNotified) Handle(*linkRegistryImpl) {
	for _, pair := range self.links {
		if pair.state.status == StatusEstablished && pair.link == pair.state.link {
			pair.state.ctrlsNotified = true
			linkLog.Info("marked link notified to controllers",
				"linkKey", pair.state.linkKey,
				"linkId", pair.link.Id(),
				"iteration", pair.link.Iteration())
		} else {
			linkLog.Info("skipped marking link notified, state changed since collection",
				"linkKey", pair.state.linkKey,
				"linkId", pair.link.Id(),
				"status", pair.state.status,
				"linkMatch", pair.link == pair.state.link)
		}
	}
}

type markFaultedLinksNotified struct {
	successfullySent []stateAndFaults
}

func (self *markFaultedLinksNotified) Handle(*linkRegistryImpl) {
	for _, pair := range self.successfullySent {
		state := pair.state
		for _, fault := range pair.faults {
			state.clearFault(fault)
		}
	}
}

type linkConnStateChanged struct {
	link xlink.Xlink
}

func (self *linkConnStateChanged) Handle(registry *linkRegistryImpl) {
	link := self.link
	log := linkLog.With("linkKey", link.Key(), "linkId", link.Id())
	dest, found := registry.destinations[link.DestinationId()]
	if !found {
		log.Debug("ignoring conn state change, link destination not present in registry", "linkDest", link.DestinationId())
		return
	}

	state, found := dest.linkMap[link.Key()]
	if !found {
		log.Debug("ignoring conn state change, link state not present in registry", "linkDest", link.DestinationId())
		return
	}

	if state.status != StatusEstablished || state.link != link {
		return
	}

	state.ctrlsNotified = false
	registry.triggerNotify()
}

type scanForLinkIdEvent struct {
	linkId  string
	resultC chan bool
}

func (self *scanForLinkIdEvent) Handle(r *linkRegistryImpl) {
	for _, dest := range r.destinations {
		for _, state := range dest.linkMap {
			if state.linkId == self.linkId {
				self.resultC <- true
				return
			}
		}
	}
	self.resultC <- false
}
