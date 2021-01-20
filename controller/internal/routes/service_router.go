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
	"github.com/openziti/edge/rest_server/operations/service"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/storage/boltz"
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
	IdType    response.IdType
	listTimer metrics.Timer
}

func NewServiceRouter() *ServiceRouter {
	return &ServiceRouter{
		BasePath: "/" + EntityNameService,
	}
}

func (r *ServiceRouter) Register(ae *env.AppEnv) {
	r.listTimer = ae.GetHostController().GetNetwork().GetMetricsRegistry().Timer("services.list")

	ae.Api.ServiceDeleteServiceHandler = service.DeleteServiceHandlerFunc(func(params service.DeleteServiceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ServiceDetailServiceHandler = service.DetailServiceHandlerFunc(func(params service.DetailServiceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated()) //filter restricted
	})

	ae.Api.ServiceListServicesHandler = service.ListServicesHandlerFunc(func(params service.ListServicesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAuthenticated()) //filter restricted
	})

	ae.Api.ServiceUpdateServiceHandler = service.UpdateServiceHandlerFunc(func(params service.UpdateServiceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ServiceCreateServiceHandler = service.CreateServiceHandlerFunc(func(params service.CreateServiceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.Api.ServicePatchServiceHandler = service.PatchServiceHandlerFunc(func(params service.PatchServiceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ServiceListServiceServiceEdgeRouterPoliciesHandler = service.ListServiceServiceEdgeRouterPoliciesHandlerFunc(func(params service.ListServiceServiceEdgeRouterPoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listServiceEdgeRouterPolicies, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ServiceListServiceEdgeRoutersHandler = service.ListServiceEdgeRoutersHandlerFunc(func(params service.ListServiceEdgeRoutersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listEdgeRouters, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ServiceListServiceServicePoliciesHandler = service.ListServiceServicePoliciesHandlerFunc(func(params service.ListServiceServicePoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listServicePolicies, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ServiceListServiceIdentitiesHandler = service.ListServiceIdentitiesHandlerFunc(func(params service.ListServiceIdentitiesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listIdentities, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ServiceListServiceConfigHandler = service.ListServiceConfigHandlerFunc(func(params service.ListServiceConfigParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listConfigs, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ServiceListServiceTerminatorsHandler = service.ListServiceTerminatorsHandlerFunc(func(params service.ListServiceTerminatorsParams, i interface{}) middleware.Responder {
		return ae.IsAllowed(r.listTerminators, params.HTTPRequest, params.ID, "", permissions.IsAuthenticated())
	})
}

func (r *ServiceRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	start := time.Now()
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

		roleFilters := rc.Request.URL.Query()["roleFilter"]
		roleSemantic := rc.Request.URL.Query().Get("roleSemantic")

		var apiEntities []interface{}
		var qmd *models.QueryMetaData
		if rc.Identity.IsAdmin && len(roleFilters) > 0 {
			cursorProvider, err := ae.GetStores().EdgeService.GetRoleAttributesCursorProvider(roleFilters, roleSemantic)
			if err != nil {
				return nil, err
			}

			result, err := ae.Handlers.EdgeService.BasePreparedListIndexed(cursorProvider, query)

			if err != nil {
				return nil, err
			}

			apiEntities, err = modelToApi(ae, rc, MapServiceToRestEntity, result.GetEntities())
			if err != nil {
				return nil, err
			}
			qmd = &result.QueryMetaData
		} else {
			result, err := ae.Handlers.EdgeService.PublicQueryForIdentity(identity, configTypes, query)
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
	r.listTimer.UpdateSince(start)
}

func (r *ServiceRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	// DetailWithHandler won't do search limiting by logged in user
	Detail(rc, func(rc *response.RequestContext, id string) (interface{}, error) {
		svc, err := ae.Handlers.EdgeService.ReadForIdentity(id, rc.ApiSession.IdentityId, rc.ApiSession.ConfigTypes)
		if err != nil {
			return nil, err
		}
		return MapServiceToRestEntity(ae, rc, svc)
	})
}

func (r *ServiceRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params service.CreateServiceParams) {
	Create(rc, rc, ServiceLinkFactory, func() (string, error) {
		return ae.Handlers.EdgeService.Create(MapCreateServiceToModel(params.Body))
	})
}

func (r *ServiceRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.EdgeService)
}

func (r *ServiceRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params service.UpdateServiceParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.EdgeService.Update(MapUpdateServiceToModel(params.ID, params.Body))
	})
}

func (r *ServiceRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params service.PatchServiceParams) {
	Patch(rc, func(id string, fields JsonFields) error {
		return ae.Handlers.EdgeService.Patch(MapPatchServiceToModel(params.ID, params.Body), fields.ConcatNestedNames().FilterMaps("tags"))
	})
}

func (r *ServiceRouter) listServiceEdgeRouterPolicies(ae *env.AppEnv, rc *response.RequestContext) {
	r.listAssociations(ae, rc, ae.Handlers.ServiceEdgeRouterPolicy, MapServiceEdgeRouterPolicyToRestEntity)
}

func (r *ServiceRouter) listServicePolicies(ae *env.AppEnv, rc *response.RequestContext) {
	r.listAssociations(ae, rc, ae.Handlers.ServicePolicy, MapServicePolicyToRestEntity)
}

func (r *ServiceRouter) listConfigs(ae *env.AppEnv, rc *response.RequestContext) {
	r.listAssociations(ae, rc, ae.Handlers.Config, MapConfigToRestEntity)
}

func (r *ServiceRouter) listTerminators(ae *env.AppEnv, rc *response.RequestContext) {
	if rc.Identity.IsAdmin {
		r.listAssociations(ae, rc, ae.Handlers.Terminator, MapTerminatorToRestEntity)
		return
	} else {
		serviceId, err := rc.GetEntityId()

		if err != nil {
			rc.RespondWithError(err)
			return
		}

		svc, err := ae.Handlers.EdgeService.ReadForIdentity(serviceId, rc.Identity.Id, nil)

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

		r.listAssociations(ae, rc, ae.Handlers.Terminator, MapLimitedTerminatorToRestEntity)
		return
	}
}

func (r *ServiceRouter) listAssociations(ae *env.AppEnv, rc *response.RequestContext, associationLoader models.EntityRetriever, mapper ModelToApiMapper) {
	ListAssociationWithHandler(ae, rc, ae.Handlers.EdgeService, associationLoader, mapper)
}

func (r *ServiceRouter) listIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	filterTemplate := `not isEmpty(from servicePolicies where anyOf(services) = "%v")`
	ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Handlers.Identity, MapIdentityToRestEntity)
}

func (r *ServiceRouter) listEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	filterTemplate := `not isEmpty(from serviceEdgeRouterPolicies where anyOf(services) = "%v")`
	ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Handlers.EdgeRouter, MapEdgeRouterToRestEntity)
}
