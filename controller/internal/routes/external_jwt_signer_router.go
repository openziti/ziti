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
	"github.com/michaelquigley/pfxlog"
	extJwtClient "github.com/openziti/edge-api/rest_client_api_server/operations/external_jwt_signer"
	extJwtManagement "github.com/openziti/edge-api/rest_management_api_server/operations/external_jwt_signer"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
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
	ae.ClientApi.ExternalJWTSignerListExternalJWTSignersHandler = extJwtClient.ListExternalJWTSignersHandlerFunc(func(params extJwtClient.ListExternalJWTSignersParams) middleware.Responder {
		return ae.IsAllowed(r.ListForClient, params.HTTPRequest, "", "", permissions.Always())
	})

	// management
	ae.ManagementApi.ExternalJWTSignerDeleteExternalJWTSignerHandler = extJwtManagement.DeleteExternalJWTSignerHandlerFunc(func(params extJwtManagement.DeleteExternalJWTSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.DeleteForManagement, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternalJWTSignerDetailExternalJWTSignerHandler = extJwtManagement.DetailExternalJWTSignerHandlerFunc(func(params extJwtManagement.DetailExternalJWTSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.DetailForManagement, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternalJWTSignerListExternalJWTSignersHandler = extJwtManagement.ListExternalJWTSignersHandlerFunc(func(params extJwtManagement.ListExternalJWTSignersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListForManagement, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternalJWTSignerUpdateExternalJWTSignerHandler = extJwtManagement.UpdateExternalJWTSignerHandlerFunc(func(params extJwtManagement.UpdateExternalJWTSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.UpdateForManagement(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternalJWTSignerCreateExternalJWTSignerHandler = extJwtManagement.CreateExternalJWTSignerHandlerFunc(func(params extJwtManagement.CreateExternalJWTSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.CreateForManagement(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternalJWTSignerPatchExternalJWTSignerHandler = extJwtManagement.PatchExternalJWTSignerHandlerFunc(func(params extJwtManagement.PatchExternalJWTSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.PatchForManagement(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *ExternalJwtSignerRouter) ListForManagement(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.ExternalJwtSigner](ae, rc, ae.Managers.ExternalJwtSigner, MapExternalJwtSignerToRestEntityForManagement)
}

func (r *ExternalJwtSignerRouter) DetailForManagement(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler[*model.ExternalJwtSigner](ae, rc, ae.Managers.ExternalJwtSigner, MapExternalJwtSignerToRestEntityForManagement)
}

func (r *ExternalJwtSignerRouter) CreateForManagement(ae *env.AppEnv, rc *response.RequestContext, params extJwtManagement.CreateExternalJWTSignerParams) {
	Create(rc, rc, ExternalJwtSignerLinkFactory, func() (string, error) {
		return MapCreate(ae.Managers.ExternalJwtSigner.Create, MapCreateExternalJwtSignerToModelForManagement(params.ExternalJWTSigner), rc)
	})
}

func (r *ExternalJwtSignerRouter) DeleteForManagement(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.ExternalJwtSigner)
}

func (r *ExternalJwtSignerRouter) UpdateForManagement(ae *env.AppEnv, rc *response.RequestContext, params extJwtManagement.UpdateExternalJWTSignerParams) {
	Update(rc, func(id string) error {
		return ae.Managers.ExternalJwtSigner.Update(MapUpdateExternalJwtSignerToModelForManagement(params.ID, params.ExternalJWTSigner), nil, rc.NewChangeContext())
	})
}

func (r *ExternalJwtSignerRouter) PatchForManagement(ae *env.AppEnv, rc *response.RequestContext, params extJwtManagement.PatchExternalJWTSignerParams) {
	Patch(rc, func(id string, patchFields fields.UpdatedFields) error {

		if patchFields.IsUpdated(db.FieldExternalJwtSignerCertPem) {
			patchFields.AddField(db.FieldExternalJwtSignerCommonName)
			patchFields.AddField(db.FieldExternalJwtSignerNotBefore)
			patchFields.AddField(db.FieldExternalJwtSignerNotAfter)
			patchFields.AddField(db.FieldExternalJwtSignerFingerprint)
		}

		externalJwtSigner := MapPatchExternalJwtSignerToModelForManagement(params.ID, params.ExternalJWTSigner)
		return ae.Managers.ExternalJwtSigner.Update(externalJwtSigner, patchFields.FilterMaps("tags", "data"), rc.NewChangeContext())
	})
}

func (r *ExternalJwtSignerRouter) ListForClient(ae *env.AppEnv, rc *response.RequestContext) {
	List(rc, func(rc *response.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {
		query, err := queryOptions.getFullQuery(ae.Managers.EdgeService.GetStore())
		if err != nil {
			return nil, err
		}

		result, err := ae.Managers.ExternalJwtSigner.PublicQuery(query)
		if err != nil {
			pfxlog.Logger().Errorf("error executing list query: %+v", err)
			return nil, err
		}
		apiEntities, err := MapExtJwtSignersToRestEntityForClient(ae, rc, result.ExtJwtSigners)
		if err != nil {
			return nil, err
		}
		qmd := &result.QueryMetaData

		return NewQueryResult(apiEntities, qmd), nil
	})
}
