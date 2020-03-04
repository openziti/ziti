/*
	Copyright 2019 NetFoundry, Inc.

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
	r := NewServiceEdgeRouterPolicyRouter()
	env.AddRouter(r)
}

type ServiceEdgeRouterPolicyRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewServiceEdgeRouterPolicyRouter() *ServiceEdgeRouterPolicyRouter {
	return &ServiceEdgeRouterPolicyRouter{
		BasePath: "/" + EntityNameServiceEdgeRouterPolicy,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *ServiceEdgeRouterPolicyRouter) Register(ae *env.AppEnv) {
	sr := registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())

	edgeRouterUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameEdgeRouter)
	identityUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameService)

	edgeRoutersListHandler := ae.WrapHandler(ir.ListEdgeRouters, permissions.IsAdmin())
	servicesListHandler := ae.WrapHandler(ir.ListServices, permissions.IsAdmin())

	//gets
	sr.HandleFunc(edgeRouterUrl, edgeRoutersListHandler).Methods(http.MethodGet)
	sr.HandleFunc(edgeRouterUrl+"/", edgeRoutersListHandler).Methods(http.MethodGet)

	sr.HandleFunc(identityUrl, servicesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(identityUrl+"/", servicesListHandler).Methods(http.MethodGet)
}

func (ir *ServiceEdgeRouterPolicyRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToApiEntity)
}

func (ir *ServiceEdgeRouterPolicyRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToApiEntity, ir.IdType)
}

func (ir *ServiceEdgeRouterPolicyRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ServiceEdgeRouterPolicyApi{}
	Create(rc, rc.RequestResponder, ae.Schemes.ServiceEdgeRouterPolicy.Post, apiEntity, (&ServiceEdgeRouterPolicyApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.ServiceEdgeRouterPolicy.Create(apiEntity.ToModel(""))
	})
}

func (ir *ServiceEdgeRouterPolicyRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.ServiceEdgeRouterPolicy)
}

func (ir *ServiceEdgeRouterPolicyRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ServiceEdgeRouterPolicyApi{}
	Update(rc, ae.Schemes.ServiceEdgeRouterPolicy.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.ServiceEdgeRouterPolicy.Update(apiEntity.ToModel(id))
	})
}

func (ir *ServiceEdgeRouterPolicyRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ServiceEdgeRouterPolicyApi{}
	Patch(rc, ae.Schemes.ServiceEdgeRouterPolicy.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.ServiceEdgeRouterPolicy.Patch(apiEntity.ToModel(id), fields.FilterMaps("tags"))
	})
}

func (ir *ServiceEdgeRouterPolicyRouter) ListEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ir.IdType, ae.Handlers.ServiceEdgeRouterPolicy, ae.Handlers.EdgeRouter, MapEdgeRouterToApiEntity)
}

func (ir *ServiceEdgeRouterPolicyRouter) ListServices(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ir.IdType, ae.Handlers.ServiceEdgeRouterPolicy, ae.Handlers.EdgeService, MapServiceToApiEntity)
}
