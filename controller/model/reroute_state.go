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

package model

import "time"

// RerouteState is a circuit's SDK-reroute state, modeled as an immutable snapshot so it can be read
// race-free via a single atomic load without holding the circuit's Rerouting guard. Transitions
// build a new state and publish it atomically. A nil RerouteState means the circuit is not
// reroutable. The concrete implementations are the two states a reroutable circuit moves between:
// ActiveRerouteState (normal operation) and LimboRerouteState (held for grace).
type RerouteState interface {
	// Iteration is the per-circuit monotonic counter that reroute tokens are bound to; a
	// successful takeover advances it, invalidating prior tokens.
	Iteration() uint64
	// InLimbo reports whether the circuit is currently held in Limbo awaiting SDK reattach or
	// underlay recovery.
	InLimbo() bool
}

// ActiveRerouteState is the normal-operation reroute state: the circuit is reroutable and serving
// traffic. It carries only the current token iteration.
type ActiveRerouteState struct {
	iteration uint64
}

// NewActiveRerouteState returns the active (non-Limbo) reroute state at the given iteration.
func NewActiveRerouteState(iteration uint64) *ActiveRerouteState {
	return &ActiveRerouteState{iteration: iteration}
}

func (self *ActiveRerouteState) Iteration() uint64 { return self.iteration }
func (self *ActiveRerouteState) InLimbo() bool      { return false }

// LimboRerouteState is the Limbo hold state: the controller is holding the circuit for a grace
// period awaiting SDK reattach or underlay recovery, instead of tearing it down. It carries the
// grace deadline and the timer that fires teardown at expiry.
type LimboRerouteState struct {
	iteration uint64
	deadline  time.Time
	timer     *time.Timer
}

// NewLimboRerouteState returns a Limbo reroute state at the given iteration, with the supplied
// grace deadline and the timer that will fire teardown at expiry.
func NewLimboRerouteState(iteration uint64, deadline time.Time, timer *time.Timer) *LimboRerouteState {
	return &LimboRerouteState{iteration: iteration, deadline: deadline, timer: timer}
}

func (self *LimboRerouteState) Iteration() uint64  { return self.iteration }
func (self *LimboRerouteState) InLimbo() bool       { return true }
func (self *LimboRerouteState) Deadline() time.Time { return self.deadline }

// StopTimer stops the grace-expiry timer when the circuit transitions out of Limbo.
func (self *LimboRerouteState) StopTimer() {
	if self.timer != nil {
		self.timer.Stop()
	}
}
