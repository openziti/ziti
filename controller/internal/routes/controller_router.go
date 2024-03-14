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
	controllersClient "github.com/openziti/edge-api/rest_client_api_server/operations/controllers"
	controllersMan "github.com/openziti/edge-api/rest_management_api_server/operations/controllers"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
)

func init() {
	r := NewControllerRouter()
	env.AddRouter(r)
}

type ControllerRouter struct {
	BasePath string
}

func NewControllerRouter() *ControllerRouter {
	return &ControllerRouter{
		BasePath: "/" + EntityNameController,
	}
}

func (r *ControllerRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.ControllersListControllersHandler = controllersMan.ListControllersHandlerFunc(func(params controllersMan.ListControllersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ClientApi.ControllersListControllersHandler = controllersClient.ListControllersHandlerFunc(func(params controllersClient.ListControllersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})
}

func (r *ControllerRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.Controller](ae, rc, ae.Managers.Controller, MapControllerToRestEntity)
}
