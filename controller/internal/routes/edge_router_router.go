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
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_management_api_server/operations/edge_router"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/storage/ast"
)

func init() {
	r := NewEdgeRouterRouter()
	env.AddRouter(r)
}

type EdgeRouterRouter struct {
	BasePath string
}

func NewEdgeRouterRouter() *EdgeRouterRouter {
	return &EdgeRouterRouter{
		BasePath: "/" + EntityNameEdgeRouter,
	}
}

func (r *EdgeRouterRouter) Register(ae *env.AppEnv) {
	//CRUD
	ae.ManagementApi.EdgeRouterDeleteEdgeRouterHandler = edge_router.DeleteEdgeRouterHandlerFunc(func(params edge_router.DeleteEdgeRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterDetailEdgeRouterHandler = edge_router.DetailEdgeRouterHandlerFunc(func(params edge_router.DetailEdgeRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterListEdgeRoutersHandler = edge_router.ListEdgeRoutersHandlerFunc(func(params edge_router.ListEdgeRoutersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterUpdateEdgeRouterHandler = edge_router.UpdateEdgeRouterHandlerFunc(func(params edge_router.UpdateEdgeRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterCreateEdgeRouterHandler = edge_router.CreateEdgeRouterHandlerFunc(func(params edge_router.CreateEdgeRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterPatchEdgeRouterHandler = edge_router.PatchEdgeRouterHandlerFunc(func(params edge_router.PatchEdgeRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	//special actions
	ae.ManagementApi.EdgeRouterReEnrollEdgeRouterHandler = edge_router.ReEnrollEdgeRouterHandlerFunc(func(params edge_router.ReEnrollEdgeRouterParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ReEnroll, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	// additional lists
	ae.ManagementApi.EdgeRouterListEdgeRouterEdgeRouterPoliciesHandler = edge_router.ListEdgeRouterEdgeRouterPoliciesHandlerFunc(func(params edge_router.ListEdgeRouterEdgeRouterPoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listEdgeRouterPolicies, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterListEdgeRouterServiceEdgeRouterPoliciesHandler = edge_router.ListEdgeRouterServiceEdgeRouterPoliciesHandlerFunc(func(params edge_router.ListEdgeRouterServiceEdgeRouterPoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listServiceEdgeRouterPolicies, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterListEdgeRouterIdentitiesHandler = edge_router.ListEdgeRouterIdentitiesHandlerFunc(func(params edge_router.ListEdgeRouterIdentitiesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listIdentities, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterListEdgeRouterServicesHandler = edge_router.ListEdgeRouterServicesHandlerFunc(func(params edge_router.ListEdgeRouterServicesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listServices, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *EdgeRouterRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	roleFilters := rc.Request.URL.Query()["roleFilter"]
	roleSemantic := rc.Request.URL.Query().Get("roleSemantic")

	if len(roleFilters) > 0 {
		ListWithQueryF(ae, rc, ae.Managers.EdgeRouter, MapEdgeRouterToRestEntity, func(query ast.Query) (*models.EntityListResult, error) {
			cursorProvider, err := ae.GetStores().EdgeRouter.GetRoleAttributesCursorProvider(roleFilters, roleSemantic)
			if err != nil {
				return nil, err
			}
			return ae.Managers.EdgeRouter.BasePreparedListIndexed(cursorProvider, query)
		})
	} else {
		ListWithHandler(ae, rc, ae.Managers.EdgeRouter, MapEdgeRouterToRestEntity)
	}
}

func (r *EdgeRouterRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Managers.EdgeRouter, MapEdgeRouterToRestEntity)
}

func (r *EdgeRouterRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params edge_router.CreateEdgeRouterParams) {
	Create(rc, rc, EdgeRouterLinkFactory, func() (string, error) {
		return ae.Managers.EdgeRouter.Create(MapCreateEdgeRouterToModel(params.EdgeRouter))
	})
}

func (r *EdgeRouterRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.EdgeRouter)
}

func (r *EdgeRouterRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params edge_router.UpdateEdgeRouterParams) {
	Update(rc, func(id string) error {
		return ae.Managers.EdgeRouter.Update(MapUpdateEdgeRouterToModel(params.ID, params.EdgeRouter), true)
	})
}

func (r *EdgeRouterRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params edge_router.PatchEdgeRouterParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {
		return ae.Managers.EdgeRouter.Patch(MapPatchEdgeRouterToModel(params.ID, params.EdgeRouter), fields.FilterMaps("tags"))
	})
}

func (r *EdgeRouterRouter) listServiceEdgeRouterPolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Managers.EdgeRouter, ae.Managers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToRestEntity)
}

func (r *EdgeRouterRouter) listEdgeRouterPolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Managers.EdgeRouter, ae.Managers.EdgeRouterPolicy, MapEdgeRouterPolicyToRestEntity)
}

func (r *EdgeRouterRouter) listIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	filterTemplate := `not isEmpty(from edgeRouterPolicies where anyOf(routers) = "%v")`
	ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Managers.Identity, MapIdentityToRestEntity)
}

func (r *EdgeRouterRouter) listServices(ae *env.AppEnv, rc *response.RequestContext) {
	filterTemplate := `not isEmpty(from serviceEdgeRouterPolicies where anyOf(routers) = "%v")`
	ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Managers.EdgeService, MapServiceToRestEntity)
}

func (r *EdgeRouterRouter) ReEnroll(ae *env.AppEnv, rc *response.RequestContext) {
	id, _ := rc.GetEntityId()

	var router *model.EdgeRouter
	var err error

	if router, err = ae.GetManagers().EdgeRouter.Read(id); err != nil {
		rc.RespondWithError(err)
		return
	}

	if router == nil {
		rc.RespondWithNotFound()
		return
	}

	if err := ae.GetManagers().EdgeRouter.ReEnroll(router); err != nil {
		rc.RespondWithApiError(apierror.NewEdgeRouterFailedReEnrollment(err))
		return
	}

	rc.RespondWithEmptyOk()
}
