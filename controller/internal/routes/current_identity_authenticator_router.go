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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_server/operations/current_api_session"
	"github.com/openziti/foundation/storage/boltz"
)

func init() {
	r := NewCurrentIdentityAuthenticatorRouter()
	env.AddRouter(r)
}

type CurrentIdentityAuthenticatorRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewCurrentIdentityAuthenticatorRouter() *CurrentIdentityAuthenticatorRouter {
	return &CurrentIdentityAuthenticatorRouter{
		BasePath: "/" + EntityNameAuthenticator,
		IdType:   response.IdTypeUuid,
	}
}

func (r *CurrentIdentityAuthenticatorRouter) Register(ae *env.AppEnv) {
	ae.Api.CurrentAPISessionDetailCurrentIdentityAuthenticatorHandler = current_api_session.DetailCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.DetailCurrentIdentityAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentAPISessionListCurrentIdentityAuthenticatorsHandler = current_api_session.ListCurrentIdentityAuthenticatorsHandlerFunc(func(params current_api_session.ListCurrentIdentityAuthenticatorsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentAPISessionUpdateCurrentIdentityAuthenticatorHandler = current_api_session.UpdateCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.UpdateCurrentIdentityAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})

	ae.Api.CurrentAPISessionPatchCurrentIdentityAuthenticatorHandler = current_api_session.PatchCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.PatchCurrentIdentityAuthenticatorParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})
}

func (r *CurrentIdentityAuthenticatorRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	List(rc, func(rc *response.RequestContext, queryOptions *QueryOptions) (*QueryResult, error) {
		query, err := queryOptions.getFullQuery(ae.Handlers.Authenticator.GetStore())
		if err != nil {
			return nil, err
		}

		result, err := ae.Handlers.Authenticator.ListForIdentity(rc.Identity.Id, query)
		if err != nil {
			pfxlog.Logger().Errorf("error executing list query: %+v", err)
			return nil, err
		}

		apiAuthenticators, err := MapAuthenticatorsToRestEntities(ae, rc, result.Authenticators)
		if err != nil {
			return nil, err
		}
		return NewQueryResult(apiAuthenticators, result.GetMetaData()), nil
	})
}

func (r *CurrentIdentityAuthenticatorRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	Detail(rc, func(rc *response.RequestContext, id string) (entity interface{}, err error) {
		authenticator, err := ae.GetHandlers().Authenticator.ReadForIdentity(rc.Identity.Id, id)
		if err != nil {
			return nil, err
		}

		if authenticator == nil {
			return nil, boltz.NewNotFoundError(ae.GetHandlers().Authenticator.GetStore().GetSingularEntityType(), "id", id)
		}

		apiAuthenticator, err := MapAuthenticatorToRestModel(ae, authenticator)

		if err != nil {
			return nil, err
		}

		return apiAuthenticator, nil
	})
}

func (r *CurrentIdentityAuthenticatorRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params current_api_session.UpdateCurrentIdentityAuthenticatorParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.Authenticator.UpdateSelf(MapUpdateAuthenticatorWithCurrentToModel(params.ID, rc.Identity.Id, params.Body))
	})
}

func (r *CurrentIdentityAuthenticatorRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params current_api_session.PatchCurrentIdentityAuthenticatorParams) {
	Patch(rc, func(id string, fields JsonFields) error {
		return ae.Handlers.Authenticator.PatchSelf(MapPatchAuthenticatorWithCurrentToModel(params.ID, rc.Identity.Id, params.Body), fields.FilterMaps("tags"))
	})
}
