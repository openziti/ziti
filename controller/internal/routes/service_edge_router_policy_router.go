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
	"github.com/openziti/edge-api/rest_management_api_server/operations/service_edge_router_policy"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/model"
	permissions "github.com/openziti/ziti/controller/permissions"
	"github.com/openziti/ziti/controller/response"
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
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "service-edge-router-policy", permissions.Delete)
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyDetailServiceEdgeRouterPolicyHandler = service_edge_router_policy.DetailServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.DetailServiceEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "service-edge-router-policy", permissions.Read)
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyListServiceEdgeRouterPoliciesHandler = service_edge_router_policy.ListServiceEdgeRouterPoliciesHandlerFunc(func(params service_edge_router_policy.ListServiceEdgeRouterPoliciesParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "service-edge-router-policy", permissions.Read)
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyUpdateServiceEdgeRouterPolicyHandler = service_edge_router_policy.UpdateServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.UpdateServiceEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "service-edge-router-policy", permissions.Update)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyCreateServiceEdgeRouterPolicyHandler = service_edge_router_policy.CreateServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.CreateServiceEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "service-edge-router-policy", permissions.Create)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyPatchServiceEdgeRouterPolicyHandler = service_edge_router_policy.PatchServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.PatchServiceEdgeRouterPolicyParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "service-edge-router-policy", permissions.Update)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	//Additional Lists
	ae.ManagementApi.ServiceEdgeRouterPolicyListServiceEdgeRouterPolicyEdgeRoutersHandler = service_edge_router_policy.ListServiceEdgeRouterPolicyEdgeRoutersHandlerFunc(func(params service_edge_router_policy.ListServiceEdgeRouterPolicyEdgeRoutersParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "router", permissions.Read)
		return ae.IsAllowed(r.ListEdgeRouters, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.ServiceEdgeRouterPolicyListServiceEdgeRouterPolicyServicesHandler = service_edge_router_policy.ListServiceEdgeRouterPolicyServicesHandlerFunc(func(params service_edge_router_policy.ListServiceEdgeRouterPolicyServicesParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "service", permissions.Read)
		return ae.IsAllowed(r.ListServices, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})
}

func (r *ServiceEdgeRouterPolicyRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.ServiceEdgeRouterPolicy](ae, rc, ae.Managers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToRestEntity)
}

func (r *ServiceEdgeRouterPolicyRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler[*model.ServiceEdgeRouterPolicy](ae, rc, ae.Managers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToRestEntity)
}

func (r *ServiceEdgeRouterPolicyRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params service_edge_router_policy.CreateServiceEdgeRouterPolicyParams) {
	Create(rc, rc, ServiceEdgeRouterPolicyLinkFactory, func() (string, error) {
		return MapCreate(ae.Managers.ServiceEdgeRouterPolicy.Create, MapCreateServiceEdgeRouterPolicyToModel(params.Policy), rc)
	})
}

func (r *ServiceEdgeRouterPolicyRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.ServiceEdgeRouterPolicy)
}

func (r *ServiceEdgeRouterPolicyRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params service_edge_router_policy.UpdateServiceEdgeRouterPolicyParams) {
	Update(rc, func(id string) error {
		return ae.Managers.ServiceEdgeRouterPolicy.Update(MapUpdateServiceEdgeRouterPolicyToModel(params.ID, params.Policy), nil, rc.NewChangeContext())
	})
}

func (r *ServiceEdgeRouterPolicyRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params service_edge_router_policy.PatchServiceEdgeRouterPolicyParams) {
	Patch(rc, func(id string, fields fields.UpdatedFields) error {
		return ae.Managers.ServiceEdgeRouterPolicy.Update(MapPatchServiceEdgeRouterPolicyToModel(params.ID, params.Policy), fields.FilterMaps("tags"), rc.NewChangeContext())
	})
}

func (r *ServiceEdgeRouterPolicyRouter) ListEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler[*model.ServiceEdgeRouterPolicy, *model.EdgeRouter](ae, rc, ae.Managers.ServiceEdgeRouterPolicy, ae.Managers.EdgeRouter, MapEdgeRouterToRestEntity)
}

func (r *ServiceEdgeRouterPolicyRouter) ListServices(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler[*model.ServiceEdgeRouterPolicy, *model.ServiceDetail](ae, rc, ae.Managers.ServiceEdgeRouterPolicy, ae.Managers.EdgeService.GetDetailLister(), GetServiceMapper(ae))
}
