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
	"github.com/openziti/edge/rest_management_api_server/operations/auth_policy"
	"github.com/openziti/fabric/controller/api"
)

func init() {
	r := NewAuthPolicyRouter()
	env.AddRouter(r)
}

type AuthPolicyRouter struct {
	BasePath string
}

func NewAuthPolicyRouter() *AuthPolicyRouter {
	return &AuthPolicyRouter{
		BasePath: "/" + EntityNameAuthPolicy,
	}
}

func (r *AuthPolicyRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.AuthPolicyDeleteAuthPolicyHandler = auth_policy.DeleteAuthPolicyHandlerFunc(func(params auth_policy.DeleteAuthPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthPolicyDetailAuthPolicyHandler = auth_policy.DetailAuthPolicyHandlerFunc(func(params auth_policy.DetailAuthPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthPolicyListAuthPoliciesHandler = auth_policy.ListAuthPoliciesHandlerFunc(func(params auth_policy.ListAuthPoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthPolicyUpdateAuthPolicyHandler = auth_policy.UpdateAuthPolicyHandlerFunc(func(params auth_policy.UpdateAuthPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthPolicyCreateAuthPolicyHandler = auth_policy.CreateAuthPolicyHandlerFunc(func(params auth_policy.CreateAuthPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthPolicyPatchAuthPolicyHandler = auth_policy.PatchAuthPolicyHandlerFunc(func(params auth_policy.PatchAuthPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *AuthPolicyRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.AuthPolicy, MapAuthPolicyToRestEntity)
}

func (r *AuthPolicyRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.AuthPolicy, MapAuthPolicyToRestEntity)
}

func (r *AuthPolicyRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params auth_policy.CreateAuthPolicyParams) {
	Create(rc, rc, AuthPolicyLinkFactory, func() (string, error) {
		return ae.Handlers.AuthPolicy.Create(MapCreateAuthPolicyToModel(params.AuthPolicy))
	})
}

func (r *AuthPolicyRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.AuthPolicy)
}

func (r *AuthPolicyRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params auth_policy.UpdateAuthPolicyParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.AuthPolicy.Update(MapUpdateAuthPolicyToModel(params.ID, params.AuthPolicy))
	})
}

func (r *AuthPolicyRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params auth_policy.PatchAuthPolicyParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {
		return ae.Handlers.AuthPolicy.Patch(MapPatchAuthPolicyToModel(params.ID, params.AuthPolicy), fields.FilterMaps("tags"))
	})
}
