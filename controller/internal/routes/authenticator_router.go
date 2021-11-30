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
	"github.com/openziti/edge/rest_management_api_server/operations/authenticator"
	"github.com/openziti/fabric/controller/api"
)

func init() {
	r := NewAuthenticatorRouter()
	env.AddRouter(r)
}

type AuthenticatorRouter struct {
	BasePath string
}

func NewAuthenticatorRouter() *AuthenticatorRouter {
	return &AuthenticatorRouter{
		BasePath: "/" + EntityNameAuthenticator,
	}
}

func (r *AuthenticatorRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.AuthenticatorDeleteAuthenticatorHandler = authenticator.DeleteAuthenticatorHandlerFunc(func(params authenticator.DeleteAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorDetailAuthenticatorHandler = authenticator.DetailAuthenticatorHandlerFunc(func(params authenticator.DetailAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorListAuthenticatorsHandler = authenticator.ListAuthenticatorsHandlerFunc(func(params authenticator.ListAuthenticatorsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorUpdateAuthenticatorHandler = authenticator.UpdateAuthenticatorHandlerFunc(func(params authenticator.UpdateAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorCreateAuthenticatorHandler = authenticator.CreateAuthenticatorHandlerFunc(func(params authenticator.CreateAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.AuthenticatorPatchAuthenticatorHandler = authenticator.PatchAuthenticatorHandlerFunc(func(params authenticator.PatchAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *AuthenticatorRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Authenticator, MapAuthenticatorToRestEntity)
}

func (r *AuthenticatorRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Authenticator, MapAuthenticatorToRestEntity)
}

func (r *AuthenticatorRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params authenticator.CreateAuthenticatorParams) {
	Create(rc, rc, AuthenticatorLinkFactory, func() (string, error) {
		authenticator, err := MapCreateToAuthenticatorModel(params.Authenticator)

		if err != nil {
			return "", err
		}

		return ae.Handlers.Authenticator.Create(authenticator)
	})
}

func (r *AuthenticatorRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.Authenticator)
}

func (r *AuthenticatorRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params authenticator.UpdateAuthenticatorParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.Authenticator.Update(MapUpdateAuthenticatorToModel(params.ID, params.Authenticator))
	})
}

func (r *AuthenticatorRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params authenticator.PatchAuthenticatorParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {
		model := MapPatchAuthenticatorToModel(params.ID, params.Authenticator)

		if fields.IsUpdated("password") {
			fields.AddField("salt")
		}

		return ae.Handlers.Authenticator.Patch(model, fields.FilterMaps("tags"))
	})
}
