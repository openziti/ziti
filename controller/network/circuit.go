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
	"github.com/openziti/identity"
	"github.com/openziti/storage/objectz"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/openziti/ziti/controller/idgen"
	"github.com/openziti/ziti/controller/xt"
	"github.com/orcaman/concurrent-map/v2"
	"sync/atomic"
	"time"
)

type Circuit struct {
	Id         string
	ClientId   string
	ServiceId  string
	Terminator xt.CostedTerminator
	Path       *Path
	Tags       map[string]string
	Rerouting  atomic.Bool
	PeerData   xt.PeerData
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (self *Circuit) GetId() string {
	return self.Id
}

func (self *Circuit) SetId(string) {
	// id cannot be updated
}

func (self *Circuit) GetCreatedAt() time.Time {
	return self.CreatedAt
}

func (self *Circuit) GetUpdatedAt() time.Time {
	return self.UpdatedAt
}

func (self *Circuit) GetTags() map[string]interface{} {
	result := map[string]interface{}{}
	for k, v := range self.Tags {
		result[k] = v
	}
	return result
}

func (self *Circuit) IsSystemEntity() bool {
	return false
}

func (self *Circuit) cost(minRouterCost uint16) int64 {
	return self.Path.cost(minRouterCost)
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

func (self *Circuit) IsEndpointRouter(routerId string) bool {
	if self == nil || self.Path == nil || len(self.Path.Nodes) == 0 {
		return false
	}
	return self.Path.Nodes[0].Id == routerId || self.Path.Nodes[len(self.Path.Nodes)-1].Id == routerId
}

type circuitController struct {
	circuits    cmap.ConcurrentMap[string, *Circuit]
	idGenerator idgen.Generator
	store       *objectz.ObjectStore[*Circuit]
}

func newCircuitController() *circuitController {
	result := &circuitController{
		circuits:    cmap.New[*Circuit](),
		idGenerator: idgen.NewGenerator(),
	}
	result.store = objectz.NewObjectStore[*Circuit](func() objectz.ObjectIterator[*Circuit] {
		return IterateCMap(result.circuits)
	})
	result.store.AddStringSymbol("id", func(entity *Circuit) *string {
		return &entity.Id
	})
	result.store.AddStringSymbol("clientId", func(entity *Circuit) *string {
		return &entity.ClientId
	})
	result.store.AddStringSymbol("service", func(entity *Circuit) *string {
		return &entity.ServiceId
	})
	result.store.AddStringSymbol("terminator", func(entity *Circuit) *string {
		val := entity.Terminator.GetId()
		return &val
	})
	result.store.AddDatetimeSymbol("createdAt", func(entity *Circuit) *time.Time {
		return &entity.CreatedAt
	})
	result.store.AddDatetimeSymbol("updatedAt", func(entity *Circuit) *time.Time {
		return &entity.CreatedAt
	})
	return result
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

type CreateCircuitParams interface {
	GetServiceId() string
	GetSourceRouter() *Router
	GetClientId() *identity.TokenId
	GetCircuitTags(terminator xt.CostedTerminator) map[string]string
	GetLogContext() logcontext.Context
	GetDeadline() time.Time
}
