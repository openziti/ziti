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

package xgress_common

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/xgress"
	cmap "github.com/orcaman/concurrent-map/v2"
)

// DialCircuitRegistry tracks the initiating xgress of every dial circuit a router-local
// client (an edge router tunneler or an embedded SDK) has established, keyed by circuit id,
// so they can be closed when the router's identity loses dial access to a service.
// Safe for concurrent use.
type DialCircuitRegistry struct {
	circuits cmap.ConcurrentMap[string, *dialCircuit]
}

type dialCircuit struct {
	serviceId string
	x         *xgress.Xgress
}

// NewDialCircuitRegistry returns an empty registry. Register a dial circuit with PrepareTrack
// followed by Publish, and close tracked circuits with CloseForService, CloseAll, or RevalidateAll.
func NewDialCircuitRegistry() *DialCircuitRegistry {
	return &DialCircuitRegistry{
		circuits: cmap.New[*dialCircuit](),
	}
}

// PrepareTrack installs x's registry cleanup handler, so that x is removed from the registry when it
// closes. Call it before x is exposed to any close path (before HandleXgressBind hands x to the
// forwarder): the handler must be present before anything can call Xgress.Close, because
// Xgress.AddCloseHandler mutates the same unsynchronized handler slice that Close iterates. Pair
// every PrepareTrack with a Publish once x is bound.
func (self *DialCircuitRegistry) PrepareTrack(x *xgress.Xgress) {
	x.AddCloseHandler(xgress.CloseHandlerF(func(closed *xgress.Xgress) { self.remove(closed) }))
}

// Publish makes x visible to revocation under serviceId, keyed by circuit id. Call it after x is
// bound: a revocation may close x as soon as it is published, and closing an unbound xgress
// dereferences a nil data-plane adapter. Requires PrepareTrack to have installed x's cleanup
// handler. If x was already closed before publication (its handler ran while nothing was
// published), Publish reconciles by removing the just-set entry.
func (self *DialCircuitRegistry) Publish(serviceId string, x *xgress.Xgress) {
	self.circuits.Set(x.CircuitId(), &dialCircuit{serviceId: serviceId, x: x})
	if x.IsClosed() {
		self.remove(x)
	}
}

func (self *DialCircuitRegistry) remove(x *xgress.Xgress) {
	self.circuits.RemoveCb(x.CircuitId(), func(_ string, v *dialCircuit, exists bool) bool {
		return exists && v.x == x
	})
}

// CloseForService closes every tracked dial circuit for serviceId.
func (self *DialCircuitRegistry) CloseForService(serviceId string, reason string) {
	self.closeMatching(func(c *dialCircuit) (bool, string) {
		return c.serviceId == serviceId, reason
	})
}

// CloseAll closes every tracked dial circuit.
func (self *DialCircuitRegistry) CloseAll(reason string) {
	self.closeMatching(func(*dialCircuit) (bool, string) {
		return true, reason
	})
}

// RevalidateAll closes every tracked dial circuit for which check returns an error. check is
// called at most once per service. check must not call back into the registry.
func (self *DialCircuitRegistry) RevalidateAll(check func(serviceId string) error) {
	results := map[string]error{}
	self.closeMatching(func(c *dialCircuit) (bool, string) {
		err, checked := results[c.serviceId]
		if !checked {
			err = check(c.serviceId)
			results[c.serviceId] = err
		}
		if err != nil {
			return true, err.Error()
		}
		return false, ""
	})
}

// closeMatching closes every circuit for which decide returns true, using the returned reason.
// decide runs under the registry's iteration and must not call back into the registry.
func (self *DialCircuitRegistry) closeMatching(decide func(*dialCircuit) (bool, string)) {
	type closing struct {
		circuit *dialCircuit
		reason  string
	}

	// collect first: closing removes entries from the map being iterated
	var toClose []closing
	self.circuits.IterCb(func(_ string, c *dialCircuit) {
		if match, reason := decide(c); match {
			toClose = append(toClose, closing{circuit: c, reason: reason})
		}
	})

	for _, entry := range toClose {
		pfxlog.Logger().WithField("circuitId", entry.circuit.x.CircuitId()).
			WithField("serviceId", entry.circuit.serviceId).
			WithField("reason", entry.reason).
			Info("closing dial circuit")
		entry.circuit.x.Close()
	}
}
