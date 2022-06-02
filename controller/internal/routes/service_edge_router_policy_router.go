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
	"github.com/openziti/edge/rest_management_api_server/operations/service_edge_router_policy"
	"github.com/openziti/fabric/controller/api"
)

func init() {
	r := NewServiceEdgeRouterPolicyRouter()
	env.AddRouter(r)
}

type ServiceEdgeRouterPolicyRouter struct {
	BasePath string
}

func NewServiceEdgeRouterPolicyRouter() *ServiceEdgeRouterPolicyRouter {
	return &ServiceEdgeRouterPolicyRouter{
		BasePath: "/" + EntityNameServiceEdgeRouterPolicy,
	}
}

func (r *ServiceEdgeRouterPolicyRouter) Register(ae *env.AppEnv) {
	// CRUD
	ae.ManagementApi.ServiceEdgeRouterPolicyDeleteServiceEdgeRouterPolicyHandler = service_edge_router_policy.DeleteServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.DeleteServiceEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyDetailServiceEdgeRouterPolicyHandler = service_edge_router_policy.DetailServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.DetailServiceEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyListServiceEdgeRouterPoliciesHandler = service_edge_router_policy.ListServiceEdgeRouterPoliciesHandlerFunc(func(params service_edge_router_policy.ListServiceEdgeRouterPoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyUpdateServiceEdgeRouterPolicyHandler = service_edge_router_policy.UpdateServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.UpdateServiceEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyCreateServiceEdgeRouterPolicyHandler = service_edge_router_policy.CreateServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.CreateServiceEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyPatchServiceEdgeRouterPolicyHandler = service_edge_router_policy.PatchServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.PatchServiceEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	//Additional Lists
	ae.ManagementApi.ServiceEdgeRouterPolicyListServiceEdgeRouterPolicyEdgeRoutersHandler = service_edge_router_policy.ListServiceEdgeRouterPolicyEdgeRoutersHandlerFunc(func(params service_edge_router_policy.ListServiceEdgeRouterPolicyEdgeRoutersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListEdgeRouters, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyListServiceEdgeRouterPolicyServicesHandler = service_edge_router_policy.ListServiceEdgeRouterPolicyServicesHandlerFunc(func(params service_edge_router_policy.ListServiceEdgeRouterPolicyServicesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListServices, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *ServiceEdgeRouterPolicyRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Managers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToRestEntity)
}

func (r *ServiceEdgeRouterPolicyRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Managers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToRestEntity)
}

func (r *ServiceEdgeRouterPolicyRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params service_edge_router_policy.CreateServiceEdgeRouterPolicyParams) {
	Create(rc, rc, ServiceEdgeRouterPolicyLinkFactory, func() (string, error) {
		return ae.Managers.ServiceEdgeRouterPolicy.Create(MapCreateServiceEdgeRouterPolicyToModel(params.Policy))
	})
}

func (r *ServiceEdgeRouterPolicyRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.ServiceEdgeRouterPolicy)
}

func (r *ServiceEdgeRouterPolicyRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params service_edge_router_policy.UpdateServiceEdgeRouterPolicyParams) {
	Update(rc, func(id string) error {
		return ae.Managers.ServiceEdgeRouterPolicy.Update(MapUpdateServiceEdgeRouterPolicyToModel(params.ID, params.Policy))
	})
}

func (r *ServiceEdgeRouterPolicyRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params service_edge_router_policy.PatchServiceEdgeRouterPolicyParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {
		return ae.Managers.ServiceEdgeRouterPolicy.Patch(MapPatchServiceEdgeRouterPolicyToModel(params.ID, params.Policy), fields.FilterMaps("tags"))
	})
}

func (r *ServiceEdgeRouterPolicyRouter) ListEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Managers.ServiceEdgeRouterPolicy, ae.Managers.EdgeRouter, MapEdgeRouterToRestEntity)
}

func (r *ServiceEdgeRouterPolicyRouter) ListServices(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Managers.ServiceEdgeRouterPolicy, ae.Managers.EdgeService, MapServiceToRestEntity)
}
