/*
	Copyright 2019 Netfoundry, Inc.

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
	"net/http"
)

func init() {
	r := NewEdgeRouterPolicyRouter()
	env.AddRouter(r)
}

type EdgeRouterPolicyRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewEdgeRouterPolicyRouter() *EdgeRouterPolicyRouter {
	return &EdgeRouterPolicyRouter{
		BasePath: "/" + EntityNameEdgeRouterPolicy,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *EdgeRouterPolicyRouter) Register(ae *env.AppEnv) {
	sr := registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())

	edgeRouterUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameEdgeRouter)
	identityUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameIdentity)

	edgeRoutersListHandler := ae.WrapHandler(ir.ListEdgeRouters, permissions.IsAdmin())
	identitiesListHandler := ae.WrapHandler(ir.ListIdentities, permissions.IsAdmin())

	//gets
	sr.HandleFunc(edgeRouterUrl, edgeRoutersListHandler).Methods(http.MethodGet)
	sr.HandleFunc(edgeRouterUrl+"/", edgeRoutersListHandler).Methods(http.MethodGet)

	sr.HandleFunc(identityUrl, identitiesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(identityUrl+"/", identitiesListHandler).Methods(http.MethodGet)
}

func (ir *EdgeRouterPolicyRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.EdgeRouterPolicy, MapEdgeRouterPolicyToApiEntity)
}

func (ir *EdgeRouterPolicyRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.EdgeRouterPolicy, MapEdgeRouterPolicyToApiEntity, ir.IdType)
}

func (ir *EdgeRouterPolicyRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &EdgeRouterPolicyApi{}
	Create(rc, rc.RequestResponder, ae.Schemes.EdgeRouterPolicy.Post, apiEntity, (&EdgeRouterPolicyApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.EdgeRouterPolicy.Create(apiEntity.ToModel(""))
	})
}

func (ir *EdgeRouterPolicyRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.EdgeRouterPolicy)
}

func (ir *EdgeRouterPolicyRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &EdgeRouterPolicyApi{}
	Update(rc, ae.Schemes.EdgeRouterPolicy.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.EdgeRouterPolicy.Update(apiEntity.ToModel(id))
	})
}

func (ir *EdgeRouterPolicyRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &EdgeRouterPolicyApi{}
	Patch(rc, ae.Schemes.EdgeRouterPolicy.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.EdgeRouterPolicy.Patch(apiEntity.ToModel(id), fields.FilterMaps("tags"))
	})
}

func (ir *EdgeRouterPolicyRouter) ListEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.EdgeRouterPolicy.CollectEdgeRouters, MapEdgeRouterToApiEntity)
}

func (ir *EdgeRouterPolicyRouter) ListIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.EdgeRouterPolicy.CollectIdentities, MapIdentityToApiEntity)
}
