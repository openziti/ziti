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
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"net/http"
)

func init() {
	r := NewIdentityRouter()
	env.AddRouter(r)
}

type IdentityRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewIdentityRouter() *IdentityRouter {
	return &IdentityRouter{
		BasePath: "/" + EntityNameIdentity,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *IdentityRouter) Register(ae *env.AppEnv) {
	sr := registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())

	currentIdentityRouter := ae.RootRouter.PathPrefix("/current-identity").Subrouter()
	currentIdentityRouter.HandleFunc("", ae.WrapHandler(detailCurrentUser, permissions.IsAuthenticated())).Methods(http.MethodGet)
	currentIdentityRouter.HandleFunc("/", ae.WrapHandler(detailCurrentUser, permissions.IsAuthenticated())).Methods(http.MethodGet)

	// edge router policies list
	edgeRouterPolicyUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameEdgeRouterPolicy)
	edgeRouterPoliciesListHandler := ae.WrapHandler(ir.ListEdgeRouterPolicies, permissions.IsAdmin())

	sr.HandleFunc(edgeRouterPolicyUrl, edgeRouterPoliciesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(edgeRouterPolicyUrl+"/", edgeRouterPoliciesListHandler).Methods(http.MethodGet)

	// service policies list
	servicePolicyUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameServicePolicy)
	servicePoliciesListHandler := ae.WrapHandler(ir.ListServicePolicies, permissions.IsAdmin())

	sr.HandleFunc(servicePolicyUrl, servicePoliciesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(servicePolicyUrl+"/", servicePoliciesListHandler).Methods(http.MethodGet)

	// service configs crud
	serviceConfigUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameIdentityServiceConfig)
	listServiceConfigHandler := ae.WrapHandler(ir.ListServiceConfigs, permissions.IsAdmin())
	serviceConfigAssignHandler := ae.WrapHandler(ir.AssignServiceConfigs, permissions.IsAdmin())
	serviceConfigRemoveHandler := ae.WrapHandler(ir.RemoveServiceConfigs, permissions.IsAdmin())

	sr.HandleFunc(serviceConfigUrl, listServiceConfigHandler).Methods(http.MethodGet)
	sr.HandleFunc(serviceConfigUrl+"/", listServiceConfigHandler).Methods(http.MethodGet)

	sr.HandleFunc(serviceConfigUrl, serviceConfigAssignHandler).Methods(http.MethodPost)
	sr.HandleFunc(servicePolicyUrl+"/", serviceConfigAssignHandler).Methods(http.MethodPost)

	sr.HandleFunc(serviceConfigUrl, serviceConfigRemoveHandler).Methods(http.MethodDelete)
	sr.HandleFunc(servicePolicyUrl+"/", serviceConfigRemoveHandler).Methods(http.MethodDelete)
}

func detailCurrentUser(ae *env.AppEnv, rc *response.RequestContext) {
	result, err := MapIdentityToApiEntity(ae, rc, rc.Identity)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}
	rc.RequestResponder.RespondWithOk(result, nil)
}

func (ir *IdentityRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Identity, MapIdentityToApiEntity)
}

func (ir *IdentityRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Identity, MapIdentityToApiEntity, ir.IdType)
}

func (ir *IdentityRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := NewIdentityApiCreate()
	Create(rc, rc.RequestResponder, ae.Schemes.Identity.Post, apiEntity, (&IdentityApiList{}).BuildSelfLink, func() (string, error) {
		identity, enrollments := apiEntity.ToModel()
		identityId, _, err := ae.Handlers.Identity.CreateWithEnrollments(identity, enrollments)
		return identityId, err
	})
}

func (ir *IdentityRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.Identity)
}

func (ir *IdentityRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &IdentityApiUpdate{}
	Update(rc, ae.Schemes.Identity.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.Identity.Update(apiEntity.ToModel(id))
	})
}

func (ir *IdentityRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &IdentityApiUpdate{}
	Patch(rc, ae.Schemes.Identity.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.Identity.Patch(apiEntity.ToModel(id), fields.FilterMaps("tags"))
	})
}

func (ir *IdentityRouter) ListEdgeRouterPolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.Identity.CollectEdgeRouterPolicies, MapEdgeRouterPolicyToApiEntity)
}

func (ir *IdentityRouter) ListServicePolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.Identity.CollectServicePolicies, MapServicePolicyToApiEntity)
}

func (ir *IdentityRouter) ListServiceConfigs(ae *env.AppEnv, rc *response.RequestContext) {
	listWithId(ae, rc, ir.IdType, func(id string) ([]interface{}, error) {
		configs, err := ae.Handlers.Identity.GetServiceConfigs(id)
		if err != nil {
			return nil, err
		}
		result := make([]interface{}, 0)
		for _, config := range configs {
			result = append(result, IdentityServiceConfig{Service: config.Service, Config: config.Config})
		}
		return result, nil
	})
}

func (ir *IdentityRouter) AssignServiceConfigs(ae *env.AppEnv, rc *response.RequestContext) {
	var serviceConfigList []IdentityServiceConfig
	Update(rc, ae.Schemes.Identity.ServiceConfigs, ir.IdType, &serviceConfigList, func(id string) error {
		var modelServiceConfigs []model.ServiceConfig
		for _, entity := range serviceConfigList {
			modelServiceConfigs = append(modelServiceConfigs, entity.toModel())
		}
		return ae.Handlers.Identity.AssignServiceConfigs(id, modelServiceConfigs)
	})
}

func (ir *IdentityRouter) RemoveServiceConfigs(ae *env.AppEnv, rc *response.RequestContext) {
	var serviceConfigList []IdentityServiceConfig
	UpdateAllowEmptyBody(rc, ae.Schemes.Identity.ServiceConfigs, ir.IdType, &serviceConfigList, true, func(id string) error {
		var modelServiceConfigs []model.ServiceConfig
		for _, entity := range serviceConfigList {
			modelServiceConfigs = append(modelServiceConfigs, entity.toModel())
		}
		return ae.Handlers.Identity.RemoveServiceConfigs(id, modelServiceConfigs)
	})
}
