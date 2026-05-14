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
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/stringz"
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
					pfxlog.Logger().
						WithField("linkKey", state.linkKey).
						WithField("linkId", link.Id()).
						WithError(err).
						Error("error closing link")
				}
			}
		}
	}
}

type linkDestUpdate struct {
	id        string
	version   string
	healthy   bool
	listeners []*ctrl_pb.Listener
}

func (self *linkDestUpdate) Handle(registry *linkRegistryImpl) {
	dest := registry.destinations[self.id]

	becameHealthy := false

	if dest == nil {
		dest = newLinkDest(self.id)
		registry.destinations[self.id] = dest
	} else {
		if !dest.healthy && self.healthy {
			becameHealthy = true
		}
	}
	dest.update(self)
	// Cache the latest listener set so the registry's local dialer
	// rescan path (localDialersChangedEvent) can re-evaluate matches
	// without an upstream listener event.
	dest.listeners = self.listeners

	if self.healthy {
		// Peer listener updates are authoritative about which listeners the peer
		// offers, so a vanished or moved listener closes its established link
		// here. This is long-standing peer-driven behavior, independent of the
		// local stale-link GC policy (gcMode).
		self.ApplyListenerChanges(registry, dest, becameHealthy, true)
	}
}

// localDialersChangedEvent triggers a re-evaluation of every known
// healthy linkDest against the current local dialer set. Used when the
// router's own dialer configuration changes (managed-config Apply, local
// YAML reload). Synthesizes a linkDestUpdate per destination using the
// destination's cached listener set; ApplyListenerChanges then iterates
// each listener × current local dialer and creates linkStates for any
// newly-possible matches.
type localDialersChangedEvent struct{}

func (localDialersChangedEvent) Handle(registry *linkRegistryImpl) {
	for _, dest := range registry.destinations {
		if !dest.healthy || len(dest.listeners) == 0 {
			continue
		}
		synthetic := &linkDestUpdate{
			id:        dest.id,
			version:   dest.version.Load(),
			healthy:   true,
			listeners: dest.listeners,
		}
		// A local dialer change only opens or refreshes dial opportunities. It
		// detaches pairings that no longer match so they can't redial through a
		// removed dialer, but must not close established links: closing a
		// locally-stale link is the stale-link GC's job (governed by gcMode), so
		// gcMode: preserve is honored.
		synthetic.ApplyListenerChanges(registry, dest, false, false)
	}
}

// ApplyListenerChanges reconciles dest's link states against self.listeners
// crossed with the current local dialers: it creates a linkState for each newly
// matching listener/dialer pair, refreshes existing ones, and detaches (removes
// from the dial map) pairings that no longer match so they can never redial.
//
// closeOrphans decides whether detaching (or an address change) also closes the
// pairing's established Xlink:
//   - Peer listener updates pass true. The peer's advertised listener set is
//     authoritative, so a listener that vanished or moved closes its link
//     immediately. This is long-standing behavior and is not gated by gcMode.
//   - The local dialer rescan passes false. A change to this router's own
//     dialers only detaches pairings and never closes an established link;
//     whether a locally-stale link is closed is left to the stale-link GC, so
//     gcMode: preserve keeps the link. With closeOrphans false this function
//     never closes an established Xlink.
func (self *linkDestUpdate) ApplyListenerChanges(registry *linkRegistryImpl, dest *linkDest, becameHealthy bool, closeOrphans bool) {
	currentLinkKeys := map[string]struct{}{}

	for k := range dest.linkMap {
		currentLinkKeys[k] = struct{}{}
	}

	for _, listener := range self.listeners {
		for _, dialer := range registry.env.GetXlinkDialers() {
			if stringz.ContainsAny(listener.Groups, dialer.GetGroups()...) {
				linkKey := registry.GetLinkKey(dialer.GetBinding(), listener.Protocol, self.id, listener.GetLocalBinding())

				delete(currentLinkKeys, linkKey)

				log := pfxlog.Logger().WithField("routerId", self.id).
					WithField("address", listener.Address).
					WithField("linkKey", linkKey)

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
					registry.evaluateLinkState(newLinkState)
				} else {
					log.Info("link already known")
					if existingLinkState.listener.Address != listener.Address {
						log.WithField("oldAddr", existingLinkState.listener.Address).
							WithField("newAddr", listener.Address).
							Info("link address changed, updating")
						if closeOrphans && existingLinkState.link != nil {
							if err := existingLinkState.link.Close(); err != nil {
								log.WithError(err).Error("error closing existing link")
							}
						}
					}
					existingLinkState.listener = listener // even if the key is the same, the address could have changed
					existingLinkState.dialer = dialer     // dialer details (backoff, options, max connections) can change while the key stays the same

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

	// Any key left unmatched is an orphaned pairing. Always detach it from the
	// dial map so it can't redial through a now-removed dialer/listener. Closing
	// its established link is caller-dependent: peer-driven updates close it,
	// while a local rescan leaves it for the stale-link GC (see closeOrphans).
	for linkKey := range currentLinkKeys {
		if v, ok := dest.linkMap[linkKey]; ok {
			delete(dest.linkMap, linkKey)
			if closeOrphans && v.link != nil {
				log := pfxlog.Logger().WithField("routerId", self.id).
					WithField("linkKey", linkKey)
				log.Info("closing link as link groups no longer align")
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
	log := pfxlog.Logger().WithField("linkKey", link.Key()).WithField("linkId", link.Id())
	dest, found := registry.destinations[link.DestinationId()]
	if !found {
		if link.IsDialed() { // if link was created by listener, rather than dialer we may not have an entry for it
			log.WithField("linkDest", link.DestinationId()).Warnf("unable to mark link as %s, link destination not present in registry", self.status)
		}
		return
	}

	state, found := dest.linkMap[link.Key()]
	if !found {
		if link.IsDialed() { // if link was created by listener, rather than dialer we may not have an entry for it
			if self.status == StatusLinkFailed {
				// The dial state was detached (e.g. a local dialer rescan) while its
				// established link was preserved; now that the link has closed, report
				// the fault directly so the controller drops it. No state remains to
				// carry the fault through the normal notification loop.
				log.WithField("linkDest", link.DestinationId()).Info("reporting fault for closed detached link")
				registry.sendLinkFaultDirect(link)
			} else {
				log.WithField("linkDest", link.DestinationId()).Warnf("unable to mark link as %s, link state not present in registry", self.status)
			}
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
		state.retryDelay = time.Duration(0)
		state.nextDial = time.Now()
		registry.evaluateLinkState(state)
		state.addPendingLinkFault(link.Id(), link.Iteration())
		state.link = nil
	}
}

type addLinkFaultForReplacedLink struct {
	link xlink.Xlink
}

func (self *addLinkFaultForReplacedLink) Handle(registry *linkRegistryImpl) {
	link := self.link
	log := pfxlog.Logger().WithField("linkKey", link.Key()).WithField("linkId", link.Id())
	dest, found := registry.destinations[link.DestinationId()]
	if !found {
		if link.IsDialed() { // if link was created by listener, rather than dialer we may not have an entry for it
			log.WithField("linkDest", link.DestinationId()).Info("link destination not present in registry")
		}
		return
	}

	state, found := dest.linkMap[link.Key()]
	if !found {
		if link.IsDialed() { // if link was created by listener, rather than dialer we may not have an entry for it
			log.WithField("linkDest", link.DestinationId()).Info("link state not present in registry")
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

// getDestinationListenersEvent snapshots the registry's per-destination
// listener cache so off-loop callers (e.g., the stale-link handler) can
// read it without racing with event-loop writes.
type getDestinationListenersEvent struct {
	result atomic.Pointer[map[string][]*ctrl_pb.Listener]
	done   chan struct{}
}

func (self *getDestinationListenersEvent) Handle(registry *linkRegistryImpl) {
	snapshot := make(map[string][]*ctrl_pb.Listener, len(registry.destinations))
	for id, dest := range registry.destinations {
		if len(dest.listeners) == 0 {
			continue
		}
		listeners := make([]*ctrl_pb.Listener, len(dest.listeners))
		copy(listeners, dest.listeners)
		snapshot[id] = listeners
	}
	self.result.Store(&snapshot)
	close(self.done)
}

func (self *getDestinationListenersEvent) GetResults(timeout time.Duration) map[string][]*ctrl_pb.Listener {
	select {
	case <-self.done:
		return *self.result.Load()
	case <-time.After(timeout):
		return nil
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
