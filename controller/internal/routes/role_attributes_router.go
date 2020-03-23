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
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-fabric/controller/models"
)

const (
	PathEdgeRouterRoleAttributes = "/edge-router-role-attributes"
	PathIdentityRoleAttributes   = "/identity-role-attributes"
	PathServiceRoleAttributes    = "/service-role-attributes"
)

func init() {
	r := NewRoleAttributesRouter()
	env.AddRouter(r)
}

type RoleAttributesRouter struct{}

func NewRoleAttributesRouter() *RoleAttributesRouter {
	return &RoleAttributesRouter{}
}

func (ir *RoleAttributesRouter) Register(ae *env.AppEnv) {
	ae.HandleGet(PathEdgeRouterRoleAttributes, ir.listEdgeRouterRoleAttributes, permissions.IsAdmin())
	ae.HandleGet(PathIdentityRoleAttributes, ir.listIdentityRoleAttributes, permissions.IsAdmin())
	ae.HandleGet(PathServiceRoleAttributes, ir.listServiceRoleAttributes, permissions.IsAdmin())
}

func (ir *RoleAttributesRouter) listEdgeRouterRoleAttributes(ae *env.AppEnv, rc *response.RequestContext) {
	ir.listRoleAttributes(rc, ae.Handlers.EdgeRouter)
}

func (ir *RoleAttributesRouter) listIdentityRoleAttributes(ae *env.AppEnv, rc *response.RequestContext) {
	ir.listRoleAttributes(rc, ae.Handlers.Identity)
}

func (ir *RoleAttributesRouter) listServiceRoleAttributes(ae *env.AppEnv, rc *response.RequestContext) {
	ir.listRoleAttributes(rc, ae.Handlers.EdgeService)
}

func (ir *RoleAttributesRouter) listRoleAttributes(rc *response.RequestContext, queryable roleAttributeQueryable) {
	List(rc, func(rc *response.RequestContext, queryOptions *QueryOptions) (*QueryResult, error) {
		results, qmd, err := queryable.QueryRoleAttributes(queryOptions.Predicate)
		if err != nil {
			return nil, err
		}
		return NewQueryResult(results, qmd), nil
	})
}

type roleAttributeQueryable interface {
	QueryRoleAttributes(queryString string) ([]string, *models.QueryMetaData, error)
}
