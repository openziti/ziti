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
	"github.com/openziti/fabric/controller/idgen"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/orcaman/concurrent-map/v2"
	"time"
)

type Circuit struct {
	Id         string
	ClientId   string
	Service    *Service
	Terminator xt.Terminator
	Path       *Path
	Rerouting  concurrenz.AtomicBoolean
	PeerData   xt.PeerData
	CreatedAt  time.Time
}

func (self *Circuit) cost() int64 {
	var cost int64
	for _, l := range self.Path.Links {
		cost += l.GetCost()
	}
	for _, r := range self.Path.Nodes {
		cost += int64(r.Cost)
	}
	return cost
}

func (self *Circuit) HasRouter(routerId string) bool {
	if self == nil || self.Path == nil {
		return false
	}
	for _, node := range self.Path.Nodes {
		if node.Id == routerId {
			return true
		}
	}
	return false
}

type circuitController struct {
	circuits    cmap.ConcurrentMap[*Circuit]
	idGenerator idgen.Generator
}

func newCircuitController() *circuitController {
	return &circuitController{
		circuits:    cmap.New[*Circuit](),
		idGenerator: idgen.NewGenerator(),
	}
}

func (self *circuitController) nextCircuitId() (string, error) {
	return self.idGenerator.NextAlphaNumericPrefixedId()
}

func (self *circuitController) add(circuit *Circuit) {
	self.circuits.Set(circuit.Id, circuit)
}

func (self *circuitController) get(id string) (*Circuit, bool) {
	if circuit, found := self.circuits.Get(id); found {
		return circuit, true
	}
	return nil, false
}

func (self *circuitController) all() []*Circuit {
	circuits := make([]*Circuit, 0)
	for tuple := range self.circuits.IterBuffered() {
		circuits = append(circuits, tuple.Val)
	}
	return circuits
}

func (self *circuitController) remove(circuit *Circuit) {
	self.circuits.Remove(circuit.Id)
}
