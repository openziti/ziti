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

package routes

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/permissions"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/rest_server/operations/circuit"
)

func init() {
	r := NewCircuitRouter()
	env.AddRouter(r)
}

type CircuitRouter struct {
	BasePath string
}

func NewCircuitRouter() *CircuitRouter {
	return &CircuitRouter{
		BasePath: "/" + EntityNameCircuit,
	}
}

func (r *CircuitRouter) Register(ae *env.AppEnv) {
	ae.FabricApi.CircuitDetailCircuitHandler = circuit.DetailCircuitHandlerFunc(func(params circuit.DetailCircuitParams) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, permissions.Ops, permissions.Read)
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.DefaultOpsAccess())
	})

	ae.FabricApi.CircuitListCircuitsHandler = circuit.ListCircuitsHandlerFunc(func(params circuit.ListCircuitsParams) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, permissions.Ops, permissions.Read)
		return ae.IsAllowed(r.ListCircuits, params.HTTPRequest, "", "", permissions.DefaultOpsAccess())
	})

	ae.FabricApi.CircuitDeleteCircuitHandler = circuit.DeleteCircuitHandlerFunc(func(params circuit.DeleteCircuitParams) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, permissions.Ops, permissions.Delete)
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.DefaultOpsAccess())
	})
}

func (r *CircuitRouter) ListCircuits(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.Circuit](ae, rc, ae.Managers.Circuit, MapCircuitToRestModel)
}

func (r *CircuitRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler[*model.Circuit](ae, rc, ae.Managers.Circuit, MapCircuitToRestModel)
}

func (r *CircuitRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	Delete(rc, func(rc *response.RequestContext, id string) error {
		return ae.GetHostController().GetNetwork().RemoveCircuit(id, true)
	})
}
