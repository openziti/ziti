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

package routes

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_management_api_server/operations/identity"
)

func init() {
	r := NewIdentityTypeRouter()
	env.AddRouter(r)
}

type IdentityTypeRouter struct {
	BasePath string
}

func NewIdentityTypeRouter() *IdentityTypeRouter {
	return &IdentityTypeRouter{
		BasePath: "/" + EntityNameIdentityType,
	}
}

func (r *IdentityTypeRouter) Register(ae *env.AppEnv) {

	ae.ManagementApi.IdentityDetailIdentityTypeHandler = identity.DetailIdentityTypeHandlerFunc(func(params identity.DetailIdentityTypeParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.IdentityListIdentityTypesHandler = identity.ListIdentityTypesHandlerFunc(func(params identity.ListIdentityTypesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

}

func (r *IdentityTypeRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Managers.IdentityType, MapIdentityTypeToRestEntity)
}

func (r *IdentityTypeRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Managers.IdentityType, MapIdentityTypeToRestEntity)
}
