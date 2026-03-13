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

// peerDialSpreadInterval staggers initial dials across newly-discovered peers
// so they don't all fire at t=0 on controller startup. This avoids TLS
// rate-limiter bursts and reduces how often the cross-dial tie-break has to
// run. 50ms gives ~300ms total spread on a 7-node cluster — modest
// mitigation, not worth exposing as a tunable.
const peerDialSpreadInterval = 50 * time.Millisecond

// peerDialEventsBufferSize is deliberately generous: producers are membership
// changes, per-peer connect/disconnect callbacks, and one result per
// in-flight dial. For realistic HA cluster sizes (<= ~11 peers) queue depth
// stays in single digits, so 64 leaves plenty of headroom without being
// worth exposing as a tunable.
const peerDialEventsBufferSize = 64

// peerDialInspectTimeout caps how long the diagnostic inspect endpoint waits
// for the event loop to respond before returning an empty result. Keeps
// `ziti fabric inspect` snappy when the loop is wedged.
const peerDialInspectTimeout = time.Second

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
		events: make(chan peerDialEvent, peerDialEventsBufferSize),
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

	fullScanTicker := time.NewTicker(self.config.ScanInterval)
	defer fullScanTicker.Stop()

	queueCheckTicker := time.NewTicker(self.config.QueueCheckInterval)
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
	for addr, state := range self.states {
		if _, ok := currentAddrs[addr]; !ok {
			log.WithField("address", addr).Info("peer removed from cluster, cleaning up dial state")
			self.removeFromRetry(state)
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

		state := newPeerDialState(addr)
		state.nextDial = time.Now().Add(time.Duration(newCount) * peerDialSpreadInterval)
		self.states[addr] = state
		self.scheduleRetry(state)
		newCount++
	}

	if newCount > 0 {
		log.WithField("newPeers", newCount).Info("scan found peers needing dial")
	}
}

// scheduleRetry inserts the state onto the retry heap if it is not already
// present, or calls heap.Fix to re-order it if it is. This prevents the same
// state from being pushed multiple times from different event handlers.
func (self *PeerDialer) scheduleRetry(state *peerDialState) {
	if state.heapIndex >= 0 {
		heap.Fix(&self.retryQueue, state.heapIndex)
		return
	}
	heap.Push(&self.retryQueue, state)
}

// removeFromRetry removes the state from the retry heap if it is present.
func (self *PeerDialer) removeFromRetry(state *peerDialState) {
	if state.heapIndex >= 0 {
		heap.Remove(&self.retryQueue, state.heapIndex)
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
		self.scheduleRetry(state)
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
		self.scheduleRetry(state)
		return
	}

	if !state.dialActive.CompareAndSwap(false, true) {
		return // already being dialed
	}

	state.status = peerStatusDialing

	// Resolve all fields that depend on state here, on the event loop, before
	// handing off to the dial goroutine. Passing pre-resolved values avoids
	// reading state fields concurrently with event-loop writes.
	stripSigningCert := shouldStripSigningCert(state)

	go self.doDial(state, state.address, stripSigningCert)
}

// shouldStripSigningCert returns true if the dial should omit the new signing
// cert header because a previous attempt failed with a peer that could not
// accept the larger hello (pre-v2.0.0 or hello never completed).
func shouldStripSigningCert(state *peerDialState) bool {
	if state.dialAttempts == 0 {
		return false
	}
	if state.lastPeerVersion == nil {
		return true
	}
	hasMin, _ := state.lastPeerVersion.HasMinimumVersion("v2.0.0")
	return !hasMin
}

func (self *PeerDialer) doDial(state *peerDialState, address string, stripSigningCert bool) {
	defer state.dialActive.Store(false)

	log := pfxlog.Logger().WithField("component", "peerDialer").
		WithField("address", address)

	log.Info("dialing peer")
	peer, dialErr := self.mesh.DialPeer(address, self.config.DialTimeout, stripSigningCert)

	var peerVersion *versions.VersionInfo
	if peer != nil {
		peerVersion = peer.Version
	}
	self.queueEvent(&peerDialResultEvent{address: address, err: dialErr, peerVersion: peerVersion})
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
	case <-time.After(peerDialInspectTimeout):
		return true, nil, nil
	case <-self.closeNotify:
		return false, nil, nil
	}
}

func (self *PeerDialer) GetPeer(address string) *Peer {
	return self.mesh.GetPeer(raft.ServerAddress(address))
}
