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
	"container/heap"
	"encoding/json"
	"strings"
	"time"

	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/event"
)

// PeerDialer proactively manages channel connections to all cluster peers.
// It runs a single-threaded event loop that tracks per-peer dial state and
// dispatches dial attempts with exponential backoff on failure.
type PeerDialer struct {
	mesh        *impl
	env         Env
	config      *config.PeerDialerConfig
	closeNotify <-chan struct{}

	events     chan peerDialEvent
	states     map[string]*peerDialState
	retryQueue peerDialStateHeap
}

// NewPeerDialer creates a new PeerDialer. Call Run to start the event loop.
func NewPeerDialer(mesh *impl, env Env, cfg *config.PeerDialerConfig, closeNotify <-chan struct{}) *PeerDialer {
	return &PeerDialer{
		mesh:        mesh,
		env:         env,
		config:      cfg,
		closeNotify: closeNotify,
		events:      make(chan peerDialEvent, 64),
		states:      make(map[string]*peerDialState),
	}
}

// PeerConnected notifies the dialer that a peer connection has been established.
func (self *PeerDialer) PeerConnected(address string) {
	self.queueEvent(&peerConnectedEvent{address: address})
}

// PeerDisconnected notifies the dialer that a peer connection has been lost.
func (self *PeerDialer) PeerDisconnected(address string) {
	self.queueEvent(&peerDisconnectedEvent{address: address})
}

// MembershipChanged notifies the dialer that cluster membership has changed.
func (self *PeerDialer) MembershipChanged() {
	self.queueEvent(&membershipChangedEvent{})
}

func (self *PeerDialer) queueEvent(evt peerDialEvent) {
	select {
	case <-self.closeNotify:
	case self.events <- evt:
	}
}

// Run starts the PeerDialer event loop. It blocks until closeNotify is closed.
func (self *PeerDialer) Run() {
	log := pfxlog.Logger().WithField("component", "peerDialer")
	log.Info("starting peer dialer")

	// Subscribe to cluster membership change events
	self.env.GetEventDispatcher().AddClusterEventHandler(event.ClusterEventHandlerF(func(evt *event.ClusterEvent) {
		if evt.EventType == event.ClusterMembersChanged {
			self.MembershipChanged()
		}
	}))

	// Initial scan
	self.scan()

	fullScanTicker := time.NewTicker(30 * time.Second)
	defer fullScanTicker.Stop()

	queueCheckTicker := time.NewTicker(5 * time.Second)
	defer queueCheckTicker.Stop()

	for {
		select {
		case evt := <-self.events:
			evt.handle(self)
		case <-queueCheckTicker.C:
			self.evaluateRetryQueue()
		case <-fullScanTicker.C:
			self.scan()
		case <-self.closeNotify:
			log.Info("stopping peer dialer")
			return
		}
	}
}

// scan checks the current set of peer addresses and ensures each one has a dial state.
func (self *PeerDialer) scan() {
	log := pfxlog.Logger().WithField("component", "peerDialer")

	addresses := self.env.GetPeerAddresses()

	// Build set of current addresses for cleanup
	currentAddrs := make(map[string]struct{}, len(addresses))
	for _, addr := range addresses {
		currentAddrs[addr] = struct{}{}
	}

	// Remove states for addresses that are no longer peers
	for addr := range self.states {
		if _, ok := currentAddrs[addr]; !ok {
			log.WithField("address", addr).Info("peer removed from cluster, cleaning up dial state")
			delete(self.states, addr)
			// Close existing connection to removed member
			if peer := self.mesh.GetPeer(raft.ServerAddress(addr)); peer != nil {
				go func() {
					if err := peer.Channel.Close(); err != nil {
						pfxlog.Logger().WithError(err).
							WithField("address", addr).
							Error("error closing channel to removed peer")
					}
				}()
			}
		}
	}

	newCount := 0
	spreadInterval := 50 * time.Millisecond

	for _, addr := range addresses {
		// Already connected, skip
		if self.mesh.GetPeer(raft.ServerAddress(addr)) != nil {
			if state, exists := self.states[addr]; exists {
				if state.status != peerStatusConnected {
					state.dialSucceeded()
				}
			}
			continue
		}

		if state, exists := self.states[addr]; exists {
			if state.status == peerStatusConnected || state.status == peerStatusDialing {
				continue
			}
			// Already in NeedsDial, leave it on the heap
			continue
		}

		state := &peerDialState{
			address:  addr,
			nextDial: time.Now().Add(time.Duration(newCount) * spreadInterval),
		}
		self.states[addr] = state
		heap.Push(&self.retryQueue, state)
		newCount++
	}

	if newCount > 0 {
		log.WithField("newPeers", newCount).Info("scan found peers needing dial")
	}
}

func (self *PeerDialer) evaluateRetryQueue() {
	now := time.Now()
	for self.retryQueue.Len() > 0 {
		state := self.retryQueue[0]
		if state.nextDial.After(now) {
			break
		}
		heap.Pop(&self.retryQueue)
		self.evaluateDialState(state)
	}
}

func (self *PeerDialer) evaluateDialState(state *peerDialState) {
	// If connected, check if it's still connected (fast failure detection)
	if state.status == peerStatusConnected {
		if self.mesh.GetPeer(raft.ServerAddress(state.address)) != nil {
			// Still connected and survived past FastFailureWindow - reset backoff and stop tracking
			state.retryDelay = 0
			state.dialAttempts = 0
			// Keep the state around in case the connection is later lost, but
			// don't put it back on the retry queue
			return
		}
		// Was marked connected but now disconnected
		state.connectionLost(self.config)
		heap.Push(&self.retryQueue, state)
		return
	}

	// Check if the address is still a peer
	if !self.isCurrentPeer(state.address) {
		delete(self.states, state.address)
		return
	}

	// Already connected (e.g. inbound connection arrived)
	if self.mesh.GetPeer(raft.ServerAddress(state.address)) != nil {
		state.dialSucceeded()
		state.nextDial = time.Now().Add(self.config.FastFailureWindow)
		heap.Push(&self.retryQueue, state)
		return
	}

	if !state.dialActive.CompareAndSwap(false, true) {
		return // already being dialed
	}

	state.status = peerStatusDialing

	go self.doDial(state)
}

func (self *PeerDialer) doDial(state *peerDialState) {
	defer state.dialActive.Store(false)

	log := pfxlog.Logger().WithField("component", "peerDialer").
		WithField("address", state.address)

	// If a previous dial attempt never completed the hello (no version learned),
	// or the peer is known to be pre-v2.0.0, strip the new signing cert header
	// so older controllers that enforce smaller hello sizes can accept the connection.
	stripSigningCert := false
	if state.dialAttempts > 0 {
		if state.lastPeerVersion == nil {
			stripSigningCert = true
		} else if hasMin, _ := state.lastPeerVersion.HasMinimumVersion("v2.0.0"); !hasMin {
			stripSigningCert = true
		}
	}

	log.Info("dialing peer")
	peer, dialErr := self.mesh.DialPeer(state.address, 10*time.Second, stripSigningCert)

	var peerVersion *versions.VersionInfo
	if peer != nil {
		peerVersion = peer.Version
	}
	self.queueEvent(&peerDialResultEvent{address: state.address, err: dialErr, peerVersion: peerVersion})
}

// isCurrentPeer returns true if the address is currently a cluster peer.
func (self *PeerDialer) isCurrentPeer(address string) bool {
	for _, addr := range self.env.GetPeerAddresses() {
		if addr == address {
			return true
		}
	}
	return false
}

// Inspect returns the current peer dialer state as JSON if name matches the peer dialer key.
func (self *PeerDialer) Inspect(name string) (bool, *string, error) {
	if !strings.EqualFold(name, inspect.PeerDialerKey) {
		return false, nil, nil
	}

	evt := &inspectPeerDialStatesEvent{
		result: make(chan *inspect.PeerDialerInspectResult, 1),
	}

	select {
	case self.events <- evt:
	case <-self.closeNotify:
		return false, nil, nil
	}

	select {
	case result := <-evt.result:
		js, err := json.Marshal(result)
		if err != nil {
			return true, nil, err
		}
		val := string(js)
		return true, &val, nil
	case <-time.After(time.Second):
		return true, nil, nil
	case <-self.closeNotify:
		return false, nil, nil
	}
}

func (self *PeerDialer) GetPeer(address string) *Peer {
	return self.mesh.GetPeer(raft.ServerAddress(address))
}
