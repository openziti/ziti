/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/orcaman/concurrent-map/v2"
)

type Circuit struct {
	Id         string
	ClientId   string
	Service    *Service
	Terminator xt.Terminator
	Path       *Path
	Rerouting  concurrenz.AtomicBoolean
	PeerData   xt.PeerData
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

func (self *Circuit) HasRouter(r *Router) bool {
	if self == nil || self.Path == nil {
		return false
	}
	for _, node := range self.Path.Nodes {
		if node.Id == r.Id {
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

func (c *circuitController) nextCircuitId() (string, error) {
	return c.idGenerator.NextAlphaNumericPrefixedId()
}

func (c *circuitController) add(sn *Circuit) {
	c.circuits.Set(sn.Id, sn)
}

func (c *circuitController) get(id string) (*Circuit, bool) {
	if circuit, found := c.circuits.Get(id); found {
		return circuit, true
	}
	return nil, false
}

func (c *circuitController) all() []*Circuit {
	circuits := make([]*Circuit, 0)
	for tuple := range c.circuits.IterBuffered() {
		circuits = append(circuits, tuple.Val)
	}
	return circuits
}

func (c *circuitController) remove(sn *Circuit) {
	c.circuits.Remove(sn.Id)
}
