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

import (
	"testing"
	"time"
)

func TestCircuitRerouteStateTransitions(t *testing.T) {
	circuit := &Circuit{Id: "circuit-1"}

	// A fresh circuit is not reroutable.
	if circuit.IsReroutable() {
		t.Fatal("a circuit with no reroute state must not be reroutable")
	}
	if circuit.RerouteState() != nil {
		t.Fatal("expected nil reroute state on a fresh circuit")
	}

	// Becoming reroutable: active state at iteration 0.
	circuit.SetRerouteState(NewActiveRerouteState(0))
	if !circuit.IsReroutable() {
		t.Fatal("expected reroutable after setting active state")
	}
	if circuit.IsInLimbo() {
		t.Fatal("active state must not report in-limbo")
	}
	if got := circuit.RerouteState().Iteration(); got != 0 {
		t.Fatalf("expected iteration 0, got %d", got)
	}

	// Active -> Limbo. This swaps a *LimboRerouteState in where an *ActiveRerouteState was; it must
	// not panic (a naive atomic.Value would panic on the inconsistent concrete type).
	timer := time.NewTimer(time.Hour)
	circuit.SetRerouteState(NewLimboRerouteState(0, time.Now().Add(time.Second), timer))
	if !circuit.IsInLimbo() {
		t.Fatal("expected in-limbo after entering limbo")
	}
	limbo, ok := circuit.RerouteState().(*LimboRerouteState)
	if !ok {
		t.Fatal("expected reroute state to be a *LimboRerouteState")
	}
	limbo.StopTimer()

	// Limbo -> active at the next iteration (a takeover), swapping concrete types back again.
	circuit.SetRerouteState(NewActiveRerouteState(1))
	if circuit.IsInLimbo() {
		t.Fatal("expected not in-limbo after returning to active")
	}
	if got := circuit.RerouteState().Iteration(); got != 1 {
		t.Fatalf("expected iteration 1 after takeover, got %d", got)
	}

	// Clearing marks the circuit non-reroutable again.
	circuit.SetRerouteState(nil)
	if circuit.IsReroutable() {
		t.Fatal("expected not reroutable after clearing reroute state")
	}
}
