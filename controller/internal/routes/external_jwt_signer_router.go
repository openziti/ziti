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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_management_api_server/operations/external_j_w_t_signer"
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
	ae.ManagementApi.ExternaljwtSignerDeleteExternalJwtSignerHandler = external_j_w_t_signer.DeleteExternalJwtSignerHandlerFunc(func(params external_j_w_t_signer.DeleteExternalJwtSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternaljwtSignerDetailExternalJwtSignerHandler = external_j_w_t_signer.DetailExternalJwtSignerHandlerFunc(func(params external_j_w_t_signer.DetailExternalJwtSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternaljwtSignerListExternalJwtSignersHandler = external_j_w_t_signer.ListExternalJwtSignersHandlerFunc(func(params external_j_w_t_signer.ListExternalJwtSignersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternaljwtSignerUpdateExternalJwtSignerHandler = external_j_w_t_signer.UpdateExternalJwtSignerHandlerFunc(func(params external_j_w_t_signer.UpdateExternalJwtSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternaljwtSignerCreateExternalJwtSignerHandler = external_j_w_t_signer.CreateExternalJwtSignerHandlerFunc(func(params external_j_w_t_signer.CreateExternalJwtSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ExternaljwtSignerPatchExternalJwtSignerHandler = external_j_w_t_signer.PatchExternalJwtSignerHandlerFunc(func(params external_j_w_t_signer.PatchExternalJwtSignerParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *ExternalJwtSignerRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.ExternalJwtSigner, MapExternalJwtSignerToRestEntity)
}

func (r *ExternalJwtSignerRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.ExternalJwtSigner, MapExternalJwtSignerToRestEntity)
}

func (r *ExternalJwtSignerRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params external_j_w_t_signer.CreateExternalJwtSignerParams) {
	Create(rc, rc, ExternalJwtSignerLinkFactory, func() (string, error) {
		return ae.Handlers.ExternalJwtSigner.Create(MapCreateExternalJwtSignerToModel(params.ExternalJwtSigner))
	})
}

func (r *ExternalJwtSignerRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.ExternalJwtSigner)
}

func (r *ExternalJwtSignerRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params external_j_w_t_signer.UpdateExternalJwtSignerParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.ExternalJwtSigner.Update(MapUpdateExternalJwtSignerToModel(params.ID, params.ExternalJwtSigner))
	})
}

func (r *ExternalJwtSignerRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params external_j_w_t_signer.PatchExternalJwtSignerParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {

		if fields.IsUpdated(persistence.FieldExternalJwtSignerCertPem) {
			fields.AddField(persistence.FieldExternalJwtSignerCommonName)
			fields.AddField(persistence.FieldExternalJwtSignerNotBefore)
			fields.AddField(persistence.FieldExternalJwtSignerNotAfter)
			fields.AddField(persistence.FieldExternalJwtSignerFingerprint)
		}
		
		return ae.Handlers.ExternalJwtSigner.Patch(MapPatchExternalJwtSignerToModel(params.ID, params.ExternalJwtSigner), fields.FilterMaps("tags", "data"))
	})
}
