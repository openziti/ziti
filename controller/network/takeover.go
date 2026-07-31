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
	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var (
	// ErrCircuitMutationInProgress indicates the circuit's reroute guard was held by another
	// operation (smart reroute, fault reroute, or another takeover). The takeover is retryable.
	ErrCircuitMutationInProgress = errors.New("circuit mutation already in progress")
	// ErrCircuitNotReroutable indicates the circuit is not reroutable, so it cannot be taken over.
	ErrCircuitNotReroutable = errors.New("circuit is not reroutable")
	// ErrStaleRerouteToken indicates the token's iteration no longer matches the circuit's, so it
	// has already been superseded by a prior takeover.
	ErrStaleRerouteToken = errors.New("stale reroute token iteration")
	// ErrWrongRerouteSide indicates the token authorizes the wrong endpoint for this takeover.
	ErrWrongRerouteSide = errors.New("reroute token side mismatch")
)

// TakeoverCircuit reattaches a reroutable circuit's ingress to newIngress, splicing a new path that
// preserves the terminator side. The circuit may be either actively routing (the SDK is moving
// ahead of, or independent of, any fault the controller has noticed) or held in Limbo after an
// ingress loss; the SDK's valid token is the authorization, not the circuit's state, so takeover is
// not gated on Limbo. It is serialized with the other reroute writers via the circuit's Rerouting
// guard (returning ErrCircuitMutationInProgress on contention), re-validates the token under the
// guard, and installs the new path with commit-after-all-succeed plus rollback. On success it
// advances the iteration, returns the circuit to active state, and returns the new iteration so the
// caller can mint a fresh token. On any route-install failure it rolls back and leaves the circuit
// on its old path.
func (network *Network) TakeoverCircuit(circuit *model.Circuit, claims *edge.RerouteClaims, newIngress *model.Router) (uint64, error) {
	log := pfxlog.Logger().WithField("circuitId", circuit.Id)

	if !circuit.Rerouting.CompareAndSwap(false, true) {
		return 0, ErrCircuitMutationInProgress
	}
	defer circuit.Rerouting.Store(false)

	state := circuit.RerouteState()
	if state == nil {
		return 0, ErrCircuitNotReroutable
	}
	if claims.Side != edge.TokenSideIngress {
		return 0, ErrWrongRerouteSide
	}
	if claims.Iteration != state.Iteration() {
		return 0, ErrStaleRerouteToken
	}

	oldPath := circuit.Path
	newPath, err := network.BuildTakeoverPath(oldPath, newIngress)
	if err != nil {
		return 0, err
	}

	deadline := time.Now().Add(network.options.RouteTimeout)
	rms := network.CreateRouteMessages(newPath, SmartRerouteAttempt, circuit.Id, circuit.Terminator, deadline)

	var accepted []*model.Router
	for i := 0; i < len(newPath.Nodes); i++ {
		if _, err := sendRoute(newPath.Nodes[i], rms[i], network.options.RouteTimeout); err != nil {
			log.WithError(err).Errorf("takeover route install failed at [r/%s], rolling back", newPath.Nodes[i].Id)
			network.rollbackTakeoverRoutes(log, circuit.Id, accepted, newPath)
			return 0, err
		}
		accepted = append(accepted, newPath.Nodes[i])
	}

	// commit: every router on the new path accepted its route
	circuit.Path = newPath
	circuit.UpdatedAt = time.Now()
	newIteration := state.Iteration() + 1
	if limbo, ok := state.(*model.LimboRerouteState); ok {
		limbo.StopTimer()
	}
	circuit.SetRerouteState(model.NewActiveRerouteState(newIteration))

	// best-effort: unroute old-path routers that are not on the new path
	network.unrouteRemovedPathNodes(log, circuit.Id, oldPath, newPath)

	log.WithField("newIngress", newIngress.Id).WithField("iteration", newIteration).Info("circuit taken over")
	network.CircuitEvent(event.CircuitUpdated, circuit, nil)

	return newIteration, nil
}

// rollbackTakeoverRoutes unroutes the routers that accepted a not-yet-committed takeover install.
// The terminator router (newPath.Nodes[len-1]) is excluded: a circuit-scoped unroute there would
// tear down the terminator's xgress endpoint, which the Limbo hold exists to preserve. Circuit-
// scoped unroute is safe on shared transit routers because the old path is already non-functional
// (its ingress is dead), and the next takeover recomputes and re-installs whatever it needs.
func (network *Network) rollbackTakeoverRoutes(log *logrus.Entry, circuitId string, accepted []*model.Router, newPath *model.Path) {
	var terminatorId string
	if len(newPath.Nodes) > 0 {
		terminatorId = newPath.Nodes[len(newPath.Nodes)-1].Id
	}
	for _, r := range accepted {
		if r.Id == terminatorId {
			continue
		}
		if err := sendUnroute(r, circuitId, true); err != nil {
			log.WithError(err).Errorf("error rolling back takeover route on [r/%s]", r.Id)
		}
	}
}
