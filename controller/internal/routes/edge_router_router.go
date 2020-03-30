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
	"fmt"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/storage/ast"
)

func init() {
	r := NewEdgeRouterRouter()
	env.AddRouter(r)
}

type EdgeRouterRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewEdgeRouterRouter() *EdgeRouterRouter {
	return &EdgeRouterRouter{
		BasePath: "/" + EntityNameEdgeRouter,
		IdType:   response.IdTypeString,
	}
}

func (ir *EdgeRouterRouter) Register(ae *env.AppEnv) {
	sr := registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())

	serviceEdgeRouterPoliciesUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameServiceEdgeRouterPolicy)
	ae.HandleGet(sr, serviceEdgeRouterPoliciesUrl, ir.listServiceEdgeRouterPolicies, permissions.IsAdmin())

	edgeRouterPolicyUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameEdgeRouterPolicy)
	ae.HandleGet(sr, edgeRouterPolicyUrl, ir.listEdgeRouterPolicies, permissions.IsAdmin())

	identitiesUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameIdentity)
	ae.HandleGet(sr, identitiesUrl, ir.listIdentities, permissions.IsAdmin())

	servicesUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameService)
	ae.HandleGet(sr, servicesUrl, ir.listServices, permissions.IsAdmin())
}

func (ir *EdgeRouterRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	roleFilters := rc.Request.URL.Query()["roleFilter"]
	roleSemantic := rc.Request.URL.Query().Get("roleSemantic")

	if len(roleFilters) > 0 {
		ListWithQueryF(ae, rc, ae.Handlers.EdgeRouter, MapEdgeRouterToApiEntity, func(query ast.Query) (*models.EntityListResult, error) {
			cursorProvider, err := ae.GetStores().EdgeRouter.GetRoleAttributesCursorProvider(roleFilters, roleSemantic)
			if err != nil {
				return nil, err
			}
			return ae.Handlers.EdgeRouter.BasePreparedListIndexed(cursorProvider, query)
		})
	} else {
		ListWithHandler(ae, rc, ae.Handlers.EdgeRouter, MapEdgeRouterToApiEntity)
	}
}

func (ir *EdgeRouterRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.EdgeRouter, MapEdgeRouterToApiEntity, ir.IdType)
}

func (ir *EdgeRouterRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &EdgeRouterApi{}
	linkBuilder := (&EdgeRouterApiList{}).BuildSelfLink
	Create(rc, rc.RequestResponder, ae.Schemes.EdgeRouter.Post, apiEntity, linkBuilder, func() (string, error) {
		return ae.Handlers.EdgeRouter.Create(apiEntity.ToModel(""))
	})
}

func (ir *EdgeRouterRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.EdgeRouter)
}

func (ir *EdgeRouterRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &EdgeRouterApi{}
	Update(rc, ae.Schemes.EdgeRouter.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.EdgeRouter.Update(apiEntity.ToModel(id), true)
	})
}

func (ir *EdgeRouterRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &EdgeRouterApi{}
	Patch(rc, ae.Schemes.EdgeRouter.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.EdgeRouter.Patch(apiEntity.ToModel(id), fields)
	})
}

func (ir *EdgeRouterRouter) listServiceEdgeRouterPolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ir.IdType, ae.Handlers.EdgeRouter, ae.Handlers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToApiEntity)
}

func (ir *EdgeRouterRouter) listEdgeRouterPolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ir.IdType, ae.Handlers.EdgeRouter, ae.Handlers.EdgeRouterPolicy, MapEdgeRouterPolicyToApiEntity)
}

func (ir *EdgeRouterRouter) listIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	filterTemplate := `not isEmpty(from edgeRouterPolicies where anyOf(edgeRouters) = "%v")`
	ListAssociationsWithFilter(ae, rc, ir.IdType, filterTemplate, ae.Handlers.Identity, MapIdentityToApiEntity)
}

func (ir *EdgeRouterRouter) listServices(ae *env.AppEnv, rc *response.RequestContext) {
	filterTemplate := `not isEmpty(from serviceEdgeRouterPolicies where anyOf(edgeRouters) = "%v")`
	ListAssociationsWithFilter(ae, rc, ir.IdType, filterTemplate, ae.Handlers.EdgeService, MapServiceToApiEntity)
}
