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
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/controller/config"
)

type peerDialStatus int

const (
	peerStatusNeedsDial peerDialStatus = iota
	peerStatusDialing
	peerStatusConnected
)

func (s peerDialStatus) String() string {
	switch s {
	case peerStatusNeedsDial:
		return "NeedsDial"
	case peerStatusDialing:
		return "Dialing"
	case peerStatusConnected:
		return "Connected"
	default:
		return "Unknown"
	}
}

// peerDialState tracks the dial state for a single peer address.
type peerDialState struct {
	address         string
	status          peerDialStatus
	retryDelay      time.Duration
	nextDial        time.Time
	dialAttempts    uint32
	connectedAt     time.Time
	dialActive      atomic.Bool
	lastPeerVersion *versions.VersionInfo // nil if hello didn't complete on last attempt
	heapIndex       int                   // position on the retry heap; -1 if not on the heap
}

// newPeerDialState returns a zero-valued peerDialState for the given address,
// with heapIndex initialized to -1 (not yet on the retry heap).
func newPeerDialState(address string) *peerDialState {
	return &peerDialState{
		address:   address,
		heapIndex: -1,
	}
}

func (self *peerDialState) dialFailed(cfg *config.PeerDialerConfig) {
	self.status = peerStatusNeedsDial
	self.dialAttempts++

	factor := cfg.RetryBackoffFactor + (rand.Float64() - 0.5)
	if factor < 1 {
		factor = 1
	}

	self.retryDelay = time.Duration(float64(self.retryDelay) * factor)
	if self.retryDelay < cfg.MinRetryInterval {
		self.retryDelay = cfg.MinRetryInterval
	}
	if self.retryDelay > cfg.MaxRetryInterval {
		self.retryDelay = cfg.MaxRetryInterval
	}

	self.nextDial = time.Now().Add(self.retryDelay)
}

func (self *peerDialState) dialSucceeded() {
	self.status = peerStatusConnected
	self.connectedAt = time.Now()
}

func (self *peerDialState) connectionLost(cfg *config.PeerDialerConfig) {
	if self.status == peerStatusConnected && time.Since(self.connectedAt) < cfg.FastFailureWindow {
		self.dialFailed(cfg)
		return
	}

	self.status = peerStatusNeedsDial
	self.retryDelay = 0
	self.nextDial = time.Now()
}
