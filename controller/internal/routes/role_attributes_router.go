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
	"github.com/openziti/edge-api/rest_management_api_server/operations/role_attributes"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/permissions"
	"github.com/openziti/ziti/v2/controller/response"
)

func init() {
	r := NewRoleAttributesRouter()
	env.AddRouter(r)
}

type RoleAttributesRouter struct{}

func NewRoleAttributesRouter() *RoleAttributesRouter {
	return &RoleAttributesRouter{}
}

func (r *RoleAttributesRouter) Register(ae *env.AppEnv) {
	ae.ManagementApi.RoleAttributesListEdgeRouterRoleAttributesHandler = role_attributes.ListEdgeRouterRoleAttributesHandlerFunc(func(params role_attributes.ListEdgeRouterRoleAttributesParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "router", permissions.Read)
		return ae.IsAllowed(r.listEdgeRouterRoleAttributes, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.RoleAttributesListIdentityRoleAttributesHandler = role_attributes.ListIdentityRoleAttributesHandlerFunc(func(params role_attributes.ListIdentityRoleAttributesParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "identity", permissions.Read)
		return ae.IsAllowed(r.listIdentityRoleAttributes, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.RoleAttributesListServiceRoleAttributesHandler = role_attributes.ListServiceRoleAttributesHandlerFunc(func(params role_attributes.ListServiceRoleAttributesParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "service", permissions.Read)
		return ae.IsAllowed(r.listServiceRoleAttributes, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.RoleAttributesListPostureCheckRoleAttributesHandler = role_attributes.ListPostureCheckRoleAttributesHandlerFunc(func(params role_attributes.ListPostureCheckRoleAttributesParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "posture-check", permissions.Read)
		return ae.IsAllowed(r.listPostureCheckAttributes, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.RoleAttributesListIdentityRoleAttributeUsageHandler = role_attributes.ListIdentityRoleAttributeUsageHandlerFunc(func(params role_attributes.ListIdentityRoleAttributeUsageParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "identity", permissions.Read)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.listRoleAttributeUsage(ae, rc, model.RoleAttributeKindIdentity, boolOrFalse(params.WithIds))
		}, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.RoleAttributesListEdgeRouterRoleAttributeUsageHandler = role_attributes.ListEdgeRouterRoleAttributeUsageHandlerFunc(func(params role_attributes.ListEdgeRouterRoleAttributeUsageParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "router", permissions.Read)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.listRoleAttributeUsage(ae, rc, model.RoleAttributeKindEdgeRouter, boolOrFalse(params.WithIds))
		}, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.RoleAttributesListServiceRoleAttributeUsageHandler = role_attributes.ListServiceRoleAttributeUsageHandlerFunc(func(params role_attributes.ListServiceRoleAttributeUsageParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "service", permissions.Read)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.listRoleAttributeUsage(ae, rc, model.RoleAttributeKindService, boolOrFalse(params.WithIds))
		}, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.ManagementApi.RoleAttributesListPostureCheckRoleAttributeUsageHandler = role_attributes.ListPostureCheckRoleAttributeUsageHandlerFunc(func(params role_attributes.ListPostureCheckRoleAttributeUsageParams, _ interface{}) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, "posture-check", permissions.Read)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.listRoleAttributeUsage(ae, rc, model.RoleAttributeKindPostureCheck, boolOrFalse(params.WithIds))
		}, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})
}

func boolOrFalse(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func (r *RoleAttributesRouter) listEdgeRouterRoleAttributes(ae *env.AppEnv, rc *response.RequestContext) {
	r.listRoleAttributes(rc, ae.Managers.EdgeRouter)
}

func (r *RoleAttributesRouter) listIdentityRoleAttributes(ae *env.AppEnv, rc *response.RequestContext) {
	r.listRoleAttributes(rc, ae.Managers.Identity)
}

func (r *RoleAttributesRouter) listServiceRoleAttributes(ae *env.AppEnv, rc *response.RequestContext) {
	r.listRoleAttributes(rc, ae.Managers.EdgeService)
}

func (r *RoleAttributesRouter) listPostureCheckAttributes(ae *env.AppEnv, rc *response.RequestContext) {
	r.listRoleAttributes(rc, ae.Managers.PostureCheck)
}

func (r *RoleAttributesRouter) listRoleAttributes(rc *response.RequestContext, queryable roleAttributeQueryable) {
	List(rc, func(rc *response.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {
		results, qmd, err := queryable.QueryRoleAttributes(queryOptions.Predicate)
		if err != nil {
			return nil, err
		}

		var list rest_model.RoleAttributesList

		for _, result := range results {
			list = append(list, result)
		}

		return NewQueryResult(list, qmd), nil
	})
}

type roleAttributeQueryable interface {
	QueryRoleAttributes(queryString string) ([]string, *models.QueryMetaData, error)
}

func (r *RoleAttributesRouter) listRoleAttributeUsage(ae *env.AppEnv, rc *response.RequestContext, kind model.RoleAttributeKind, includeIds bool) {
	List(rc, func(rc *response.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {
		results, qmd, err := model.QueryRoleAttributeUsage(ae, kind, queryOptions.Predicate, includeIds)
		if err != nil {
			return nil, err
		}

		list := make(rest_model.RoleAttributeUsageList, 0, len(results))
		for _, result := range results {
			attr := result.RoleAttribute
			usage := make(map[string]rest_model.RoleAttributeSourceUsage, len(result.Usage))
			for source, src := range result.Usage {
				entry := rest_model.RoleAttributeSourceUsage{Count: &src.Count}
				if includeIds {
					// When the caller asked for ids, always emit a (possibly
					// empty) array so a null in the response unambiguously
					// means "ids were not requested".
					if src.Ids == nil {
						entry.Ids = []string{}
					} else {
						entry.Ids = src.Ids
					}
				}
				usage[string(source)] = entry
			}
			list = append(list, &rest_model.RoleAttributeUsageDetail{
				RoleAttribute: &attr,
				Usage:         usage,
			})
		}

		return NewQueryResult(list, qmd), nil
	})
}
