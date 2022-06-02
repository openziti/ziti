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
	"github.com/openziti/edge/rest_management_api_server/operations/edge_router_policy"
	"github.com/openziti/fabric/controller/api"
)

func init() {
	r := NewEdgeRouterPolicyRouter()
	env.AddRouter(r)
}

type EdgeRouterPolicyRouter struct {
	BasePath string
}

func NewEdgeRouterPolicyRouter() *EdgeRouterPolicyRouter {
	return &EdgeRouterPolicyRouter{
		BasePath: "/" + EntityNameEdgeRouterPolicy,
	}
}

func (r *EdgeRouterPolicyRouter) Register(ae *env.AppEnv) {
	//CRUD
	ae.ManagementApi.EdgeRouterPolicyDeleteEdgeRouterPolicyHandler = edge_router_policy.DeleteEdgeRouterPolicyHandlerFunc(func(params edge_router_policy.DeleteEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterPolicyDetailEdgeRouterPolicyHandler = edge_router_policy.DetailEdgeRouterPolicyHandlerFunc(func(params edge_router_policy.DetailEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterPolicyListEdgeRouterPoliciesHandler = edge_router_policy.ListEdgeRouterPoliciesHandlerFunc(func(params edge_router_policy.ListEdgeRouterPoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterPolicyUpdateEdgeRouterPolicyHandler = edge_router_policy.UpdateEdgeRouterPolicyHandlerFunc(func(params edge_router_policy.UpdateEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterPolicyCreateEdgeRouterPolicyHandler = edge_router_policy.CreateEdgeRouterPolicyHandlerFunc(func(params edge_router_policy.CreateEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterPolicyPatchEdgeRouterPolicyHandler = edge_router_policy.PatchEdgeRouterPolicyHandlerFunc(func(params edge_router_policy.PatchEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	//Additional Lists
	ae.ManagementApi.EdgeRouterPolicyListEdgeRouterPolicyEdgeRoutersHandler = edge_router_policy.ListEdgeRouterPolicyEdgeRoutersHandlerFunc(func(params edge_router_policy.ListEdgeRouterPolicyEdgeRoutersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListEdgeRouters, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.EdgeRouterPolicyListEdgeRouterPolicyIdentitiesHandler = edge_router_policy.ListEdgeRouterPolicyIdentitiesHandlerFunc(func(params edge_router_policy.ListEdgeRouterPolicyIdentitiesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListIdentities, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *EdgeRouterPolicyRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Managers.EdgeRouterPolicy, MapEdgeRouterPolicyToRestEntity)
}

func (r *EdgeRouterPolicyRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Managers.EdgeRouterPolicy, MapEdgeRouterPolicyToRestEntity)
}

func (r *EdgeRouterPolicyRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params edge_router_policy.CreateEdgeRouterPolicyParams) {
	Create(rc, rc, EdgeRouterPolicyLinkFactory, func() (string, error) {
		return ae.Managers.EdgeRouterPolicy.Create(MapCreateEdgeRouterPolicyToModel(params.Policy))
	})
}

func (r *EdgeRouterPolicyRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.EdgeRouterPolicy)
}

func (r *EdgeRouterPolicyRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params edge_router_policy.UpdateEdgeRouterPolicyParams) {
	Update(rc, func(id string) error {
		return ae.Managers.EdgeRouterPolicy.Update(MapUpdateEdgeRouterPolicyToModel(params.ID, params.Policy))
	})
}

func (r *EdgeRouterPolicyRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params edge_router_policy.PatchEdgeRouterPolicyParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {
		return ae.Managers.EdgeRouterPolicy.Patch(MapPatchEdgeRouterPolicyToModel(params.ID, params.Policy), fields.FilterMaps("tags"))
	})
}

func (r *EdgeRouterPolicyRouter) ListEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Managers.EdgeRouterPolicy, ae.Managers.EdgeRouter, MapEdgeRouterToRestEntity)
}

func (r *EdgeRouterPolicyRouter) ListIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Managers.EdgeRouterPolicy, ae.Managers.Identity, MapIdentityToRestEntity)
}
