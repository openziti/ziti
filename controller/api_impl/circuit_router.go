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

package api_impl

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/change"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/rest_model"
	"github.com/openziti/fabric/rest_server/operations"
	"github.com/openziti/fabric/rest_server/operations/circuit"
	"github.com/openziti/storage/boltz"
	"sort"
)

func init() {
	r := NewCircuitRouter()
	AddRouter(r)
}

type CircuitRouter struct {
	BasePath string
}

func NewCircuitRouter() *CircuitRouter {
	return &CircuitRouter{
		BasePath: "/" + EntityNameCircuit,
	}
}

func (r *CircuitRouter) Register(fabricApi *operations.ZitiFabricAPI, wrapper RequestWrapper) {
	fabricApi.CircuitDetailCircuitHandler = circuit.DetailCircuitHandlerFunc(func(params circuit.DetailCircuitParams) middleware.Responder {
		return wrapper.WrapRequest(r.Detail, params.HTTPRequest, params.ID, "")
	})

	fabricApi.CircuitListCircuitsHandler = circuit.ListCircuitsHandlerFunc(func(params circuit.ListCircuitsParams) middleware.Responder {
		return wrapper.WrapRequest(r.ListCircuits, params.HTTPRequest, "", "")
	})

	fabricApi.CircuitDeleteCircuitHandler = circuit.DeleteCircuitHandlerFunc(func(params circuit.DeleteCircuitParams) middleware.Responder {
		return wrapper.WrapRequest(func(n *network.Network, rc api.RequestContext) { r.Delete(n, rc, params) }, params.HTTPRequest, params.ID, "")
	})
}

func (r *CircuitRouter) ListCircuits(n *network.Network, rc api.RequestContext) {
	ListWithEnvelopeFactory(rc, defaultToListEnvelope, func(rc api.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {
		circuits := n.GetAllCircuits()
		sort.Slice(circuits, func(i, j int) bool {
			return circuits[i].Id < circuits[j].Id
		})
		apiCircuits := make([]*rest_model.CircuitDetail, 0, len(circuits))
		for _, modelCircuit := range circuits {
			apiCircuit, err := MapCircuitToRestModel(n, rc, modelCircuit)
			if err != nil {
				return nil, err
			}
			apiCircuits = append(apiCircuits, apiCircuit)
		}
		result := &QueryResult{
			Result:           apiCircuits,
			Count:            int64(len(circuits)),
			Limit:            -1,
			Offset:           0,
			FilterableFields: nil,
		}
		return result, nil
	})
}

func (r *CircuitRouter) Detail(n *network.Network, rc api.RequestContext) {
	Detail(rc, func(rc api.RequestContext, id string) (interface{}, error) {
		l, found := n.GetCircuit(id)
		if !found {
			return nil, boltz.NewNotFoundError("circuit", "id", id)
		}
		apiCircuit, err := MapCircuitToRestModel(n, rc, l)
		if err != nil {
			return nil, err
		}
		return apiCircuit, nil
	})
}

func (r *CircuitRouter) Delete(network *network.Network, rc api.RequestContext, p circuit.DeleteCircuitParams) {
	DeleteWithHandler(rc, DeleteHandlerF(func(id string, _ *change.Context) error {
		return network.RemoveCircuit(id, p.Options.Immediate)
	}))
}
