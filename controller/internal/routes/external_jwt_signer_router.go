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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_management_api_server/operations/external_jwt_signer"
	"github.com/openziti/fabric/controller/api"
)

func init() {
	r := NewExternalJwtSignerRouter()
	env.AddRouter(r)
}

type ExternalJwtSignerRouter struct {
	BasePath string
}

func NewExternalJwtSignerRouter() *ExternalJwtSignerRouter {
	return &ExternalJwtSignerRouter{
		BasePath: "/" + EntityNameExternalJwtSigner,
	}
}

func (r *ExternalJwtSignerRouter) Register(ae *env.AppEnv) {
	// client

	ae.ManagementApi.ExternalJWTSignerListExternalJWTSignersHandler = external_jwt_signer.ListExternalJWTSignersHandlerFunc(func(params external_jwt_signer.ListExternalJWTSignersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListClient, params.HTTPRequest, "", "", permissions.Always())
	})

	// management
	ae.ManagementApi.ExternalJWTSignerDeleteExternalJWTSignerHandler = external_jwt_signer.DeleteExternalJWTSignerHandlerFunc(func(params external_jwt_signer.DeleteExternalJWTSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternalJWTSignerDetailExternalJWTSignerHandler = external_jwt_signer.DetailExternalJWTSignerHandlerFunc(func(params external_jwt_signer.DetailExternalJWTSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternalJWTSignerListExternalJWTSignersHandler = external_jwt_signer.ListExternalJWTSignersHandlerFunc(func(params external_jwt_signer.ListExternalJWTSignersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternalJWTSignerUpdateExternalJWTSignerHandler = external_jwt_signer.UpdateExternalJWTSignerHandlerFunc(func(params external_jwt_signer.UpdateExternalJWTSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternalJWTSignerCreateExternalJWTSignerHandler = external_jwt_signer.CreateExternalJWTSignerHandlerFunc(func(params external_jwt_signer.CreateExternalJWTSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternalJWTSignerPatchExternalJWTSignerHandler = external_jwt_signer.PatchExternalJWTSignerHandlerFunc(func(params external_jwt_signer.PatchExternalJWTSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *ExternalJwtSignerRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.ExternalJwtSigner, MapExternalJwtSignerToRestEntity)
}

func (r *ExternalJwtSignerRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.ExternalJwtSigner, MapExternalJwtSignerToRestEntity)
}

func (r *ExternalJwtSignerRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params external_jwt_signer.CreateExternalJWTSignerParams) {
	Create(rc, rc, ExternalJwtSignerLinkFactory, func() (string, error) {
		return ae.Handlers.ExternalJwtSigner.Create(MapCreateExternalJwtSignerToModel(params.ExternalJWTSigner))
	})
}

func (r *ExternalJwtSignerRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.ExternalJwtSigner)
}

func (r *ExternalJwtSignerRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params external_jwt_signer.UpdateExternalJWTSignerParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.ExternalJwtSigner.Update(MapUpdateExternalJwtSignerToModel(params.ID, params.ExternalJWTSigner))
	})
}

func (r *ExternalJwtSignerRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params external_jwt_signer.PatchExternalJWTSignerParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {

		if fields.IsUpdated(persistence.FieldExternalJwtSignerCertPem) {
			fields.AddField(persistence.FieldExternalJwtSignerCommonName)
			fields.AddField(persistence.FieldExternalJwtSignerNotBefore)
			fields.AddField(persistence.FieldExternalJwtSignerNotAfter)
			fields.AddField(persistence.FieldExternalJwtSignerFingerprint)
		}

		return ae.Handlers.ExternalJwtSigner.Patch(MapPatchExternalJwtSignerToModel(params.ID, params.ExternalJWTSigner), fields.FilterMaps("tags", "data"))
	})
}

func (r *ExternalJwtSignerRouter) ListClient(ae *env.AppEnv, rc *response.RequestContext) {
	List(rc, func(rc *response.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {

		query, err := queryOptions.getFullQuery(ae.Handlers.EdgeService.GetStore())
		if err != nil {
			return nil, err
		}

		result, err := ae.Handlers.ExternalJwtSigner.PublicQuery(query)
		if err != nil {
			pfxlog.Logger().Errorf("error executing list query: %+v", err)
			return nil, err
		}
		apiEntities, err := MapClientExtJwtSignersToRestEntity(ae, rc, result.ExtJwtSigners)
		if err != nil {
			return nil, err
		}
		qmd := &result.QueryMetaData

		return NewQueryResult(apiEntities, qmd), nil
	})
}
