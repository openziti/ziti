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
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/orcaman/concurrent-map"
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

func (self *Circuit) latency() int64 {
	var latency int64
	for _, l := range self.Path.Links {
		latency += l.GetSrcLatency()
		latency += l.GetDstLatency()
	}
	return latency
}

type circuitController struct {
	circuits          cmap.ConcurrentMap // map[string]*Circuit
	circuitsByService cmap.ConcurrentMap // map[string]*Circuits
}

func newCircuitController() *circuitController {
	return &circuitController{
		circuits:          cmap.New(),
		circuitsByService: cmap.New(),
	}
}

func (c *circuitController) add(sn *Circuit) {
	c.circuits.Set(sn.Id, sn)

	if !c.circuitsByService.Has(sn.Service.Id) {
		c.circuitsByService.Set(sn.Service.Id, cmap.New())
	}
	t, _ := c.circuitsByService.Get(sn.Service.Id)
	circuitsForService := t.(cmap.ConcurrentMap)
	circuitsForService.Set(sn.Id, sn)
}

func (c *circuitController) get(id string) (*Circuit, bool) {
	if t, found := c.circuits.Get(id); found {
		return t.(*Circuit), true
	}
	return nil, false
}

func (c *circuitController) all() []*Circuit {
	circuits := make([]*Circuit, 0)
	for i := range c.circuits.IterBuffered() {
		circuits = append(circuits, i.Val.(*Circuit))
	}
	return circuits
}

func (c *circuitController) remove(sn *Circuit) {
	c.circuits.Remove(sn.Id)

	if t, found := c.circuitsByService.Get(sn.Service.Id); found {
		circuitsForService := t.(cmap.ConcurrentMap)
		circuitsForService.Remove(sn.Id)
		if circuitsForService.Count() < 1 {
			c.circuitsByService.Remove(sn.Service.Id)
		}
	}
}
