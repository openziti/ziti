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
	r := NewServicePolicyRouter()
	env.AddRouter(r)
}

type ServicePolicyRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewServicePolicyRouter() *ServicePolicyRouter {
	return &ServicePolicyRouter{
		BasePath: "/" + EntityNameServicePolicy,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *ServicePolicyRouter) Register(ae *env.AppEnv) {
	sr := registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())

	serviceUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameService)
	identityUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameIdentity)

	servicesListHandler := ae.WrapHandler(ir.ListServices, permissions.IsAdmin())
	identitiesListHandler := ae.WrapHandler(ir.ListIdentities, permissions.IsAdmin())

	//gets
	sr.HandleFunc(serviceUrl, servicesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(serviceUrl+"/", servicesListHandler).Methods(http.MethodGet)

	sr.HandleFunc(identityUrl, identitiesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(identityUrl+"/", identitiesListHandler).Methods(http.MethodGet)
}

func (ir *ServicePolicyRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.ServicePolicy, MapServicePolicyToApiEntity)
}

func (ir *ServicePolicyRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.ServicePolicy, MapServicePolicyToApiEntity, ir.IdType)
}

func (ir *ServicePolicyRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ServicePolicyApi{}
	Create(rc, rc.RequestResponder, ae.Schemes.ServicePolicy.Post, apiEntity, (&ServicePolicyApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.ServicePolicy.Create(apiEntity.ToModel(""))
	})
}

func (ir *ServicePolicyRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.ServicePolicy)
}

func (ir *ServicePolicyRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ServicePolicyApi{}
	Update(rc, ae.Schemes.ServicePolicy.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.ServicePolicy.Update(apiEntity.ToModel(id))
	})
}

func (ir *ServicePolicyRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ServicePolicyApi{}
	Patch(rc, ae.Schemes.ServicePolicy.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.ServicePolicy.Patch(apiEntity.ToModel(id), fields.FilterMaps("tags"))
	})
}

func (ir *ServicePolicyRouter) ListServices(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.ServicePolicy.CollectServices, MapServiceToApiEntity)
}

func (ir *ServicePolicyRouter) ListIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.ServicePolicy.CollectIdentities, MapIdentityToApiEntity)
}
