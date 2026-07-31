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

package network

import (
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/model"
)

// DefaultLimboGracePeriod is how long the controller holds a Limbo circuit before tearing it down
// if no takeover or underlay recovery restores it.
const DefaultLimboGracePeriod = 10 * time.Second

// EnterLimbo transitions a reroutable circuit into Limbo, holding it for a grace period instead of
// tearing it down, so the SDK can reattach it via a different router. Entry is serialized with the
// other reroute writers via the circuit's Rerouting guard: if another mutation is in progress it
// will resolve the circuit, so this is a no-op.
func (network *Network) EnterLimbo(circuit *model.Circuit) {
	log := pfxlog.Logger().WithField("circuitId", circuit.Id)

	if !circuit.Rerouting.CompareAndSwap(false, true) {
		log.Info("not entering limbo, a circuit mutation is already in progress")
		return
	}
	defer circuit.Rerouting.Store(false)

	state := circuit.RerouteState()
	if state == nil || state.InLimbo() {
		return
	}

	deadline := time.Now().Add(DefaultLimboGracePeriod)
	timer := time.AfterFunc(DefaultLimboGracePeriod, func() {
		network.limboExpired(circuit.Id)
	})
	circuit.SetRerouteState(model.NewLimboRerouteState(state.Iteration(), deadline, timer))

	log.Info("circuit entered limbo")
	network.CircuitEvent(event.CircuitUpdated, circuit, nil)
}

// limboExpired tears down a Limbo circuit whose grace period elapsed with no takeover or recovery.
// It is guarded so it never races a concurrent takeover: on guard contention it defers (a takeover
// in progress will resolve the circuit), and it re-checks the circuit is still in Limbo before
// removing it.
func (network *Network) limboExpired(circuitId string) {
	log := pfxlog.Logger().WithField("circuitId", circuitId)

	circuit, found := network.GetCircuit(circuitId)
	if !found {
		return
	}

	if !circuit.Rerouting.CompareAndSwap(false, true) {
		log.Info("limbo grace expired but a circuit mutation is in progress; deferring teardown")
		return
	}
	defer circuit.Rerouting.Store(false)

	limbo, ok := circuit.RerouteState().(*model.LimboRerouteState)
	if !ok {
		return // already exited limbo (takeover succeeded or underlay recovered)
	}
	limbo.StopTimer()

	log.Info("circuit limbo grace period expired, tearing down")
	if err := network.RemoveCircuit(circuitId, true); err != nil {
		log.WithError(err).Error("error removing circuit after limbo grace expiry")
	}
}
