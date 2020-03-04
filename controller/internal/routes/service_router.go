/*
	Copyright 2020 NetFoundry, Inc.

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
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"net/http"
	"strings"

	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/response"
)

func init() {
	r := NewServiceRouter()
	env.AddRouter(r)
}

type ServiceRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewServiceRouter() *ServiceRouter {
	return &ServiceRouter{
		BasePath: "/" + EntityNameService,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *ServiceRouter) Register(ae *env.AppEnv) {
	sr := registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, &crudResolvers{
		Create:  permissions.IsAdmin(),
		Read:    permissions.IsAuthenticated(),
		Update:  permissions.IsAdmin(),
		Delete:  permissions.IsAdmin(),
		Default: permissions.IsAdmin(),
	})

	serviceEdgeRouterPolicyUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameServiceEdgeRouterPolicy)
	serviceEdgeRouterPolicyListHandler := ae.WrapHandler(ir.listServiceEdgeRouterPolicies, permissions.IsAdmin())
	sr.HandleFunc(serviceEdgeRouterPolicyUrl, serviceEdgeRouterPolicyListHandler).Methods(http.MethodGet)
	sr.HandleFunc(serviceEdgeRouterPolicyUrl+"/", serviceEdgeRouterPolicyListHandler).Methods(http.MethodGet)

	servicePolicyUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameServicePolicy)
	servicePoliciesListHandler := ae.WrapHandler(ir.listServicePolicies, permissions.IsAdmin())
	sr.HandleFunc(servicePolicyUrl, servicePoliciesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(servicePolicyUrl+"/", servicePoliciesListHandler).Methods(http.MethodGet)

	configsUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameConfig)
	configsListHandler := ae.WrapHandler(ir.listConfigs, permissions.IsAdmin())
	sr.HandleFunc(configsUrl, configsListHandler).Methods(http.MethodGet)
	sr.HandleFunc(configsUrl+"/", configsListHandler).Methods(http.MethodGet)

	terminatorsUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameTerminator)
	terminatorsListHandler := ae.WrapHandler(ir.listTerminators, permissions.IsAdmin())
	sr.HandleFunc(terminatorsUrl, terminatorsListHandler).Methods(http.MethodGet)
	sr.HandleFunc(terminatorsUrl+"/", terminatorsListHandler).Methods(http.MethodGet)
}

func (ir *ServiceRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	// ListWithHandler won't do search limiting by logged in user
	List(rc, func(rc *response.RequestContext, queryOptions *QueryOptions) (*QueryResult, error) {
		identity := rc.Identity
		if rc.Identity.IsAdmin {
			if asId := rc.Request.URL.Query().Get("asIdentity"); asId != "" {
				var err error
				identity, err = ae.Handlers.Identity.ReadOneByQuery(fmt.Sprintf(`id = "%v" or name = "%v"`, asId, asId))
				if err != nil {
					return nil, err
				}
				if identity == nil {
					return nil, boltz.NewNotFoundError("identity", "id or name", asId)
				}
			}
		}

		// allow overriding config types
		configTypes := rc.ApiSession.ConfigTypes
		if requestedConfigTypes := rc.Request.URL.Query().Get("configTypes"); requestedConfigTypes != "" {
			configTypes = mapConfigTypeNamesToIds(ae, strings.Split(requestedConfigTypes, ","), identity.Id)
		}

		query, err := queryOptions.getFullQuery(ae.Handlers.EdgeService.GetStore())
		if err != nil {
			return nil, err
		}

		result, err := ae.Handlers.EdgeService.PublicQueryForIdentity(identity, configTypes, query)
		if err != nil {
			pfxlog.Logger().Errorf("error executing list query: %+v", err)
			return nil, err
		}
		services, err := MapServicesToApiEntities(ae, rc, result.Services)
		if err != nil {
			return nil, err
		}
		return NewQueryResult(services, &result.QueryMetaData), nil
	})
}

func (ir *ServiceRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	// DetailWithHandler won't do search limiting by logged in user
	Detail(rc, ir.IdType, func(rc *response.RequestContext, id string) (interface{}, error) {
		service, err := ae.Handlers.EdgeService.ReadForIdentity(id, rc.ApiSession.IdentityId, rc.ApiSession.ConfigTypes)
		if err != nil {
			return nil, err
		}
		return MapServiceToApiEntity(ae, rc, service)
	})
}

func (ir *ServiceRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	serviceCreate := &ServiceApiCreate{}
	Create(rc, rc.RequestResponder, ae.Schemes.Service.Post, serviceCreate, (&ServiceApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.EdgeService.Create(serviceCreate.ToModel())
	})
}

func (ir *ServiceRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.EdgeService)
}

func (ir *ServiceRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	serviceUpdate := &ServiceApiUpdate{}
	Update(rc, ae.Schemes.Service.Put, ir.IdType, serviceUpdate, func(id string) error {
		return ae.Handlers.EdgeService.Update(serviceUpdate.ToModel(id))
	})
}

func (ir *ServiceRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	serviceUpdate := &ServiceApiUpdate{}
	Patch(rc, ae.Schemes.Service.Patch, ir.IdType, serviceUpdate, func(id string, fields JsonFields) error {
		return ae.Handlers.EdgeService.Patch(serviceUpdate.ToModel(id), fields.ConcatNestedNames().FilterMaps("tags"))
	})
}

func (ir *ServiceRouter) listServiceEdgeRouterPolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ir.listAssociations(ae, rc, ae.Handlers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToApiEntity)
}

func (ir *ServiceRouter) listServicePolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ir.listAssociations(ae, rc, ae.Handlers.ServicePolicy, MapServicePolicyToApiEntity)
}

func (ir *ServiceRouter) listConfigs(ae *env.AppEnv, rc *response.RequestContext) {
	ir.listAssociations(ae, rc, ae.Handlers.Config, MapConfigToApiEntity)
}

func (ir *ServiceRouter) listTerminators(ae *env.AppEnv, rc *response.RequestContext) {
	ir.listAssociations(ae, rc, ae.Handlers.Terminator, MapTerminatorToApiEntity)
}

func (ir *ServiceRouter) listAssociations(ae *env.AppEnv, rc *response.RequestContext, associationLoader models.EntityRetriever, mapper ModelToApiMapper) {
	ListAssociationWithHandler(ae, rc, ir.IdType, ae.Handlers.EdgeService, associationLoader, mapper)
}
