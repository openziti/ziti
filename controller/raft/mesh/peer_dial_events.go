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

package mesh

import (
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/common/inspect"
)

type peerDialEvent interface {
	handle(d *PeerDialer)
}

// peerConnectedEvent is sent when a peer connection is established, either by dialing or accepting.
type peerConnectedEvent struct {
	address string
}

func (self *peerConnectedEvent) handle(d *PeerDialer) {
	state, exists := d.states[self.address]
	if !exists {
		return
	}

	state.dialSucceeded()
	// Schedule a re-check after FastFailureWindow to detect fast failures
	state.nextDial = time.Now().Add(d.config.FastFailureWindow)
	d.scheduleRetry(state)

	pfxlog.Logger().WithField("address", self.address).
		Info("peer dial state updated to connected")
}

// peerDisconnectedEvent is sent when a peer connection is lost.
type peerDisconnectedEvent struct {
	address string
}

func (self *peerDisconnectedEvent) handle(d *PeerDialer) {
	log := pfxlog.Logger().WithField("address", self.address)

	state, exists := d.states[self.address]
	if !exists {
		// Only redial if the address is still a known peer
		if !d.isCurrentPeer(self.address) {
			return
		}
		state = &peerDialState{
			address:   self.address,
			heapIndex: -1,
		}
		d.states[self.address] = state
	}

	state.connectionLost(d.config)
	d.scheduleRetry(state)

	log.WithField("nextDial", time.Until(state.nextDial).Round(time.Millisecond)).
		Info("peer disconnected, queued for redial")
}

// membershipChangedEvent is sent when the cluster membership changes.
type membershipChangedEvent struct{}

func (self *membershipChangedEvent) handle(d *PeerDialer) {
	d.scan()
}

// peerDialResultEvent is sent from a dial worker goroutine with the outcome.
type peerDialResultEvent struct {
	address     string
	err         error
	peerVersion *versions.VersionInfo // non-nil on success
}

func (self *peerDialResultEvent) handle(d *PeerDialer) {
	state, exists := d.states[self.address]
	if !exists {
		return
	}

	log := pfxlog.Logger().WithField("address", self.address)

	if self.err != nil {
		state.dialFailed(d.config)
		log.WithError(self.err).
			WithField("retryDelay", state.retryDelay).
			WithField("nextDial", time.Until(state.nextDial).Round(time.Millisecond)).
			Warn("peer dial attempt failed, will retry with backoff")
		d.scheduleRetry(state)
		return
	}

	state.lastPeerVersion = self.peerVersion
	state.dialSucceeded()
	state.nextDial = time.Now().Add(d.config.FastFailureWindow)
	d.scheduleRetry(state)

	log.Info("successfully connected to peer")
}

// inspectPeerDialStatesEvent captures the current state of all peer dials for inspection.
type inspectPeerDialStatesEvent struct {
	result chan *inspect.PeerDialerInspectResult
}

func (self *inspectPeerDialStatesEvent) handle(d *PeerDialer) {
	result := &inspect.PeerDialerInspectResult{
		Config: inspect.PeerDialerConfigDetail{
			MinRetryInterval:   d.config.MinRetryInterval.String(),
			MaxRetryInterval:   d.config.MaxRetryInterval.String(),
			RetryBackoffFactor: d.config.RetryBackoffFactor,
			FastFailureWindow:  d.config.FastFailureWindow.String(),
		},
	}
	for _, state := range d.states {
		result.Peers = append(result.Peers, &inspect.PeerDialerPeerDial{
			Address:      state.address,
			Status:       state.status.String(),
			DialAttempts: state.dialAttempts,
			RetryDelay:   state.retryDelay.String(),
			NextDial:     time.Until(state.nextDial).Round(time.Millisecond).String(),
		})
	}
	self.result <- result
}
