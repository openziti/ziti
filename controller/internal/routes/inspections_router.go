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
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/rest_model"
	"github.com/openziti/ziti/controller/rest_server/operations/inspect"
)

func init() {
	r := NewInspectRouter()
	env.AddRouter(r)
}

type InspectRouter struct {
	BasePath string
}

func NewInspectRouter() *InspectRouter {
	return &InspectRouter{
		BasePath: "/" + EntityNameInspect,
	}
}

func (r *InspectRouter) Register(ae *env.AppEnv) {
	ae.FabricApi.InspectInspectHandler = inspect.InspectHandlerFunc(func(params inspect.InspectParams) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Inspect(ae, rc, params.Request) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})
}

func (r *InspectRouter) Inspect(ae *env.AppEnv, rc *response.RequestContext, request *rest_model.InspectRequest) {
	result := ae.GetHostController().GetNetwork().Inspections.Inspect(stringz.OrEmpty(request.AppRegex), request.RequestedValues)
	resp := MapInspectResultToRestModel(ae.GetHostController().GetNetwork(), result)
	rc.Respond(resp, http.StatusOK)
}
