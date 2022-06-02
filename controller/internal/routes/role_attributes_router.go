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
	"github.com/openziti/edge/rest_management_api_server/operations/role_attributes"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
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
		return ae.IsAllowed(r.listEdgeRouterRoleAttributes, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.RoleAttributesListIdentityRoleAttributesHandler = role_attributes.ListIdentityRoleAttributesHandlerFunc(func(params role_attributes.ListIdentityRoleAttributesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listIdentityRoleAttributes, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.RoleAttributesListServiceRoleAttributesHandler = role_attributes.ListServiceRoleAttributesHandlerFunc(func(params role_attributes.ListServiceRoleAttributesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listServiceRoleAttributes, params.HTTPRequest, "", "", permissions.IsAdmin())
	})
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
