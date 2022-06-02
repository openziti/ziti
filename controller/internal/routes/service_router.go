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
	"github.com/go-openapi/runtime/middleware"
	"github.com/michaelquigley/pfxlog"
	clientService "github.com/openziti/edge/rest_client_api_server/operations/service"
	managementService "github.com/openziti/edge/rest_management_api_server/operations/service"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/storage/boltz"
	"strings"
	"time"

	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
)

func init() {
	r := NewServiceRouter()
	env.AddRouter(r)
}

type ServiceRouter struct {
	BasePath  string
	listTimer metrics.Timer
}

func NewServiceRouter() *ServiceRouter {
	return &ServiceRouter{
		BasePath: "/" + EntityNameService,
	}
}

func (r *ServiceRouter) Register(ae *env.AppEnv) {
	r.listTimer = ae.GetHostController().GetNetwork().GetMetricsRegistry().Timer("services.list")

	//Client
	ae.ClientApi.ServiceListServicesHandler = clientService.ListServicesHandlerFunc(func(params clientService.ListServicesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListClientServices, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})

	ae.ClientApi.ServiceDetailServiceHandler = clientService.DetailServiceHandlerFunc(func(params clientService.DetailServiceParams, _ interface{}) middleware.Responder {
		//r.Detail limits by identity
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})

	ae.ClientApi.ServiceListServiceTerminatorsHandler = clientService.ListServiceTerminatorsHandlerFunc(func(params clientService.ListServiceTerminatorsParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.listClientTerminators, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})

	//Management
	ae.ManagementApi.ServiceDeleteServiceHandler = managementService.DeleteServiceHandlerFunc(func(params managementService.DeleteServiceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceDetailServiceHandler = managementService.DetailServiceHandlerFunc(func(params managementService.DetailServiceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceListServicesHandler = managementService.ListServicesHandlerFunc(func(params managementService.ListServicesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.ListManagementServices, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceUpdateServiceHandler = managementService.UpdateServiceHandlerFunc(func(params managementService.UpdateServiceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceCreateServiceHandler = managementService.CreateServiceHandlerFunc(func(params managementService.CreateServiceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServicePatchServiceHandler = managementService.PatchServiceHandlerFunc(func(params managementService.PatchServiceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceListServiceServiceEdgeRouterPoliciesHandler = managementService.ListServiceServiceEdgeRouterPoliciesHandlerFunc(func(params managementService.ListServiceServiceEdgeRouterPoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listServiceEdgeRouterPolicies, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceListServiceEdgeRoutersHandler = managementService.ListServiceEdgeRoutersHandlerFunc(func(params managementService.ListServiceEdgeRoutersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listEdgeRouters, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceListServiceServicePoliciesHandler = managementService.ListServiceServicePoliciesHandlerFunc(func(params managementService.ListServiceServicePoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listServicePolicies, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceListServiceIdentitiesHandler = managementService.ListServiceIdentitiesHandlerFunc(func(params managementService.ListServiceIdentitiesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listIdentities, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceListServiceConfigHandler = managementService.ListServiceConfigHandlerFunc(func(params managementService.ListServiceConfigParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listConfigs, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.ServiceListServiceTerminatorsHandler = managementService.ListServiceTerminatorsHandlerFunc(func(params managementService.ListServiceTerminatorsParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.listManagementTerminators, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *ServiceRouter) ListManagementServices(ae *env.AppEnv, rc *response.RequestContext) {
	//always admin
	List(rc, func(rc *response.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {
		identity := rc.Identity
		if asId := rc.Request.URL.Query().Get("asIdentity"); asId != "" {
			var err error
			identity, err = ae.Managers.Identity.ReadOneByQuery(fmt.Sprintf(`id = "%v" or name = "%v"`, asId, asId))
			if err != nil {
				return nil, err
			}
			if identity == nil {
				return nil, boltz.NewNotFoundError("identity", "id or name", asId)
			}
		}

		// allow overriding config types
		configTypes := rc.ApiSession.ConfigTypes
		if requestedConfigTypes := rc.Request.URL.Query().Get("configTypes"); requestedConfigTypes != "" {
			configTypes = ae.Managers.ConfigType.MapConfigTypeNamesToIds(strings.Split(requestedConfigTypes, ","), identity.Id)
		}

		query, err := queryOptions.getFullQuery(ae.Managers.EdgeService.GetStore())
		if err != nil {
			return nil, err
		}

		roleFilters := rc.Request.URL.Query()["roleFilter"]
		roleSemantic := rc.Request.URL.Query().Get("roleSemantic")

		var apiEntities []interface{}
		var qmd *models.QueryMetaData

		if len(roleFilters) > 0 {
			cursorProvider, err := ae.GetStores().EdgeService.GetRoleAttributesCursorProvider(roleFilters, roleSemantic)
			if err != nil {
				return nil, err
			}

			result, err := ae.Managers.EdgeService.BasePreparedListIndexed(cursorProvider, query)

			if err != nil {
				return nil, err
			}

			apiEntities, err = modelToApi(ae, rc, MapServiceToRestEntity, result.GetEntities())
			if err != nil {
				return nil, err
			}
			qmd = &result.QueryMetaData
		} else {
			result, err := ae.Managers.EdgeService.PublicQueryForIdentity(identity, configTypes, query)
			if err != nil {
				pfxlog.Logger().Errorf("error executing list query: %+v", err)
				return nil, err
			}
			apiEntities, err = MapServicesToRestEntity(ae, rc, result.Services)
			if err != nil {
				return nil, err
			}
			qmd = &result.QueryMetaData
		}

		return NewQueryResult(apiEntities, qmd), nil
	})
}

func (r *ServiceRouter) ListClientServices(ae *env.AppEnv, rc *response.RequestContext) {
	//never in an admin capacity
	start := time.Now()
	// ListWithHandler won't do search limiting by logged in user
	List(rc, func(rc *response.RequestContext, queryOptions *PublicQueryOptions) (*QueryResult, error) {

		// allow overriding config types
		configTypes := rc.ApiSession.ConfigTypes
		if requestedConfigTypes := rc.Request.URL.Query().Get("configTypes"); requestedConfigTypes != "" {
			configTypes = ae.Managers.ConfigType.MapConfigTypeNamesToIds(strings.Split(requestedConfigTypes, ","), rc.Identity.Id)
		}

		query, err := queryOptions.getFullQuery(ae.Managers.EdgeService.GetStore())
		if err != nil {
			return nil, err
		}

		result, err := ae.Managers.EdgeService.PublicQueryForIdentity(rc.Identity, configTypes, query)
		if err != nil {
			pfxlog.Logger().Errorf("error executing list query: %+v", err)
			return nil, err
		}
		apiEntities, err := MapServicesToRestEntity(ae, rc, result.Services)
		if err != nil {
			return nil, err
		}
		qmd := &result.QueryMetaData

		return NewQueryResult(apiEntities, qmd), nil
	})
	r.listTimer.UpdateSince(start)
}

func (r *ServiceRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	// DetailWithHandler won't do search limiting by logged in user
	Detail(rc, func(rc *response.RequestContext, id string) (interface{}, error) {
		svc, err := ae.Managers.EdgeService.ReadForIdentity(id, rc.ApiSession.IdentityId, rc.ApiSession.ConfigTypes)
		if err != nil {
			return nil, err
		}
		return MapServiceToRestEntity(ae, rc, svc)
	})
}

func (r *ServiceRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params managementService.CreateServiceParams) {
	Create(rc, rc, ServiceLinkFactory, func() (string, error) {
		return ae.Managers.EdgeService.Create(MapCreateServiceToModel(params.Service))
	})
}

func (r *ServiceRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.EdgeService)
}

func (r *ServiceRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params managementService.UpdateServiceParams) {
	Update(rc, func(id string) error {
		return ae.Managers.EdgeService.Update(MapUpdateServiceToModel(params.ID, params.Service))
	})
}

func (r *ServiceRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params managementService.PatchServiceParams) {
	Patch(rc, func(id string, fields api.JsonFields) error {
		return ae.Managers.EdgeService.Patch(MapPatchServiceToModel(params.ID, params.Service), fields.ConcatNestedNames().FilterMaps("tags"))
	})
}

func (r *ServiceRouter) listServiceEdgeRouterPolicies(ae *env.AppEnv, rc *response.RequestContext) {
	r.listAssociations(ae, rc, ae.Managers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToRestEntity)
}

func (r *ServiceRouter) listServicePolicies(ae *env.AppEnv, rc *response.RequestContext) {
	r.listAssociations(ae, rc, ae.Managers.ServicePolicy, MapServicePolicyToRestEntity)
}

func (r *ServiceRouter) listConfigs(ae *env.AppEnv, rc *response.RequestContext) {
	r.listAssociations(ae, rc, ae.Managers.Config, MapConfigToRestEntity)
}

func (r *ServiceRouter) listManagementTerminators(ae *env.AppEnv, rc *response.RequestContext) {
	r.listAssociations(ae, rc, ae.Managers.Terminator, MapTerminatorToRestEntity)
}

func (r *ServiceRouter) listClientTerminators(ae *env.AppEnv, rc *response.RequestContext) {
	serviceId, err := rc.GetEntityId()

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	svc, err := ae.Managers.EdgeService.ReadForIdentity(serviceId, rc.Identity.Id, nil)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RespondWithNotFound()
		} else {
			rc.RespondWithError(err)
		}
		return
	}

	if svc == nil {
		rc.RespondWithNotFound()
		return
	}

	r.listAssociations(ae, rc, ae.Managers.Terminator, MapClientTerminatorToRestEntity)
}

func (r *ServiceRouter) listAssociations(ae *env.AppEnv, rc *response.RequestContext, associationLoader models.EntityRetriever, mapper ModelToApiMapper) {
	ListAssociationWithHandler(ae, rc, ae.Managers.EdgeService, associationLoader, mapper)
}

func (r *ServiceRouter) listIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	filterTemplate := `not isEmpty(from servicePolicies where anyOf(services) = "%v")`
	ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Managers.Identity, MapIdentityToRestEntity)
}

func (r *ServiceRouter) listEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	filterTemplate := `not isEmpty(from serviceEdgeRouterPolicies where anyOf(services) = "%v")`
	ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Managers.EdgeRouter, MapEdgeRouterToRestEntity)
}
