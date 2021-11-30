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
	"github.com/openziti/edge/rest_management_api_server/operations/service_policy"
	"github.com/openziti/fabric/controller/api"
)

func init() {
	r := NewServicePolicyRouter()
	env.AddRouter(r)
}

type ServicePolicyRouter struct {
	BasePath string
}

func NewServicePolicyRouter() *ServicePolicyRouter {
	return &ServicePolicyRouter{
		BasePath: "/" + EntityNameServicePolicy,
	}
}

func (r *ServicePolicyRouter) Register(ae *env.AppEnv) {
	//CRUD
	ae.ManagementApi.ServicePolicyDeleteServicePolicyHandler = service_policy.DeleteServicePolicyHandlerFunc(func(params service_policy.DeleteServicePolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServicePolicyDetailServicePolicyHandler = service_policy.DetailServicePolicyHandlerFunc(func(params service_policy.DetailServicePolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServicePolicyListServicePoliciesHandler = service_policy.ListServicePoliciesHandlerFunc(func(params service_policy.ListServicePoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServicePolicyUpdateServicePolicyHandler = service_policy.UpdateServicePolicyHandlerFunc(func(params service_policy.UpdateServicePolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServicePolicyCreateServicePolicyHandler = service_policy.CreateServicePolicyHandlerFunc(func(params service_policy.CreateServicePolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServicePolicyPatchServicePolicyHandler = service_policy.PatchServicePolicyHandlerFunc(func(params service_policy.PatchServicePolicyParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	//Additional Lists
	ae.ManagementApi.ServicePolicyListServicePolicyServicesHandler = service_policy.ListServicePolicyServicesHandlerFunc(func(params service_policy.ListServicePolicyServicesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListServices, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServicePolicyListServicePolicyIdentitiesHandler = service_policy.ListServicePolicyIdentitiesHandlerFunc(func(params service_policy.ListServicePolicyIdentitiesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListIdentities, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServicePolicyListServicePolicyPostureChecksHandler = service_policy.ListServicePolicyPostureChecksHandlerFunc(func(params service_policy.ListServicePolicyPostureChecksParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListPostureChecks, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *ServicePolicyRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.ServicePolicy, MapServicePolicyToRestEntity)
}

func (r *ServicePolicyRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.ServicePolicy, MapServicePolicyToRestEntity)
}

func (r *ServicePolicyRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params service_policy.CreateServicePolicyParams) {
	Create(rc, rc, ServicePolicyLinkFactory, func() (string, error) {
		return ae.Handlers.ServicePolicy.Create(MapCreateServicePolicyToModel(params.Policy))
	})
}

func (r *ServicePolicyRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.ServicePolicy)
}

func (r *ServicePolicyRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params service_policy.UpdateServicePolicyParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.ServicePolicy.Update(MapUpdateServicePolicyToModel(params.ID, params.Policy))
	})
}

func (r *ServicePolicyRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params service_policy.PatchServicePolicyParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {
		return ae.Handlers.ServicePolicy.Patch(MapPatchServicePolicyToModel(params.ID, params.Policy), fields.FilterMaps("tags"))
	})
}

func (r *ServicePolicyRouter) ListServices(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Handlers.ServicePolicy, ae.Handlers.EdgeService, MapServiceToRestEntity)
}

func (r *ServicePolicyRouter) ListIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Handlers.ServicePolicy, ae.Handlers.Identity, MapIdentityToRestEntity)
}

func (r *ServicePolicyRouter) ListPostureChecks(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Handlers.ServicePolicy, ae.Handlers.PostureCheck, MapPostureCheckToRestEntity)
}
