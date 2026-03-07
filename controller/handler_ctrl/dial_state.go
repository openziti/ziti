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
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/openziti/ziti/v2/controller/config"
)

type routerDialStatus int

const (
	statusNeedsDial routerDialStatus = iota
	statusDialing
	statusConnected
)

func (s routerDialStatus) String() string {
	switch s {
	case statusNeedsDial:
		return "NeedsDial"
	case statusDialing:
		return "Dialing"
	case statusConnected:
		return "Connected"
	default:
		return "Unknown"
	}
}

type routerDialState struct {
	routerId     string
	addresses    []string
	addrIndex    int
	status       routerDialStatus
	retryDelay   time.Duration
	nextDial     time.Time
	dialAttempts uint32
	connectedAt  time.Time
	dialActive   atomic.Bool
}

func (self *routerDialState) currentAddress() string {
	return self.addresses[self.addrIndex%len(self.addresses)]
}

func (self *routerDialState) nextAddress() {
	self.addrIndex = (self.addrIndex + 1) % len(self.addresses)
}

func (self *routerDialState) dialFailed(cfg *config.CtrlDialerConfig) {
	self.status = statusNeedsDial
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
	self.nextAddress()
}

func (self *routerDialState) dialSucceeded() {
	self.status = statusConnected
	self.connectedAt = time.Now()
	// retryDelay is intentionally NOT reset here. If the connection dies within
	// FastFailureWindow, connectionLost() will continue the backoff progression.
	// retryDelay gets reset in connectionLost() for normal (long-lived) disconnects,
	// and in evaluateDialState() when the connection survives past FastFailureWindow.
}

func (self *routerDialState) connectionLost(cfg *config.CtrlDialerConfig) {
	if self.status == statusConnected && time.Since(self.connectedAt) < cfg.FastFailureWindow {
		// fast failure: connection died quickly, treat as a dial failure with backoff
		self.dialFailed(cfg)
		return
	}

	// normal disconnection: reset backoff and address index, retry immediately
	self.status = statusNeedsDial
	self.retryDelay = 0
	self.addrIndex = 0
	self.nextDial = time.Now()
}
