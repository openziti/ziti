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
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/edge/rest_server/operations/identity"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
)

func init() {
	r := NewIdentityRouter()
	env.AddRouter(r)
}

type IdentityRouter struct {
	BasePath string
}

func NewIdentityRouter() *IdentityRouter {
	return &IdentityRouter{
		BasePath: "/" + EntityNameIdentity,
	}
}

func (r *IdentityRouter) Register(ae *env.AppEnv) {

	//identity crud
	ae.Api.IdentityDeleteIdentityHandler = identity.DeleteIdentityHandlerFunc(func(params identity.DeleteIdentityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.IdentityDetailIdentityHandler = identity.DetailIdentityHandlerFunc(func(params identity.DetailIdentityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.IdentityListIdentitiesHandler = identity.ListIdentitiesHandlerFunc(func(params identity.ListIdentitiesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.Api.IdentityUpdateIdentityHandler = identity.UpdateIdentityHandlerFunc(func(params identity.UpdateIdentityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.IdentityCreateIdentityHandler = identity.CreateIdentityHandlerFunc(func(params identity.CreateIdentityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.Api.IdentityPatchIdentityHandler = identity.PatchIdentityHandlerFunc(func(params identity.PatchIdentityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	// edge router policies list
	ae.Api.IdentityListIdentitysEdgeRouterPoliciesHandler = identity.ListIdentitysEdgeRouterPoliciesHandlerFunc(func(params identity.ListIdentitysEdgeRouterPoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listEdgeRouterPolicies, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	// edge routers list
	ae.Api.IdentityListIdentityEdgeRoutersHandler = identity.ListIdentityEdgeRoutersHandlerFunc(func(params identity.ListIdentityEdgeRoutersParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listEdgeRouters, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	// service policies list
	ae.Api.IdentityListIdentityServicePoliciesHandler = identity.ListIdentityServicePoliciesHandlerFunc(func(params identity.ListIdentityServicePoliciesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listServicePolicies, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	// service list
	ae.Api.IdentityListIdentityServicesHandler = identity.ListIdentityServicesHandlerFunc(func(params identity.ListIdentityServicesParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listServices, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	// service configs crud
	ae.Api.IdentityListIdentitysServiceConfigsHandler = identity.ListIdentitysServiceConfigsHandlerFunc(func(params identity.ListIdentitysServiceConfigsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.listServiceConfigs, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.IdentityAssociateIdentitysServiceConfigsHandler = identity.AssociateIdentitysServiceConfigsHandlerFunc(func(params identity.AssociateIdentitysServiceConfigsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.assignServiceConfigs(ae, rc, params)
		}, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.IdentityDisassociateIdentitysServiceConfigsHandler = identity.DisassociateIdentitysServiceConfigsHandlerFunc(func(params identity.DisassociateIdentitysServiceConfigsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.removeServiceConfigs(ae, rc, params)
		}, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	// policy advice URL
	ae.Api.IdentityGetIdentityPolicyAdviceHandler = identity.GetIdentityPolicyAdviceHandlerFunc(func(params identity.GetIdentityPolicyAdviceParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.getPolicyAdvice, params.HTTPRequest, params.ID, params.ServiceID, permissions.IsAdmin())
	})

	// posture data
	ae.Api.IdentityGetIdentityPostureDataHandler = identity.GetIdentityPostureDataHandlerFunc(func(params identity.GetIdentityPostureDataParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.getPostureData, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *IdentityRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	roleFilters := rc.Request.URL.Query()["roleFilter"]
	roleSemantic := rc.Request.URL.Query().Get("roleSemantic")

	if len(roleFilters) > 0 {
		ListWithQueryF(ae, rc, ae.Handlers.EdgeRouter, MapIdentityToRestEntity, func(query ast.Query) (*models.EntityListResult, error) {
			cursorProvider, err := ae.GetStores().Identity.GetRoleAttributesCursorProvider(roleFilters, roleSemantic)
			if err != nil {
				return nil, err
			}
			return ae.Handlers.Identity.BasePreparedListIndexed(cursorProvider, query)
		})
	} else {
		ListWithHandler(ae, rc, ae.Handlers.Identity, MapIdentityToRestEntity)
	}
}

func (r *IdentityRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Identity, MapIdentityToRestEntity)
}

func getIdentityTypeId(ae *env.AppEnv, identityType rest_model.IdentityType) string {
	//todo: Remove this, should be identityTypeId coming in through the API so we can defer this lookup and subsequent checks to the handlers
	identityTypeId := ""
	if identityType, err := ae.Handlers.IdentityType.ReadByName(string(identityType)); identityType != nil && err == nil {
		identityTypeId = identityType.Id
	}

	return identityTypeId
}

func (r *IdentityRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params identity.CreateIdentityParams) {
	Create(rc, rc, IdentityLinkFactory, func() (string, error) {
		identityModel, enrollments := MapCreateIdentityToModel(params.Body, getIdentityTypeId(ae, params.Body.Type))
		identityId, _, err := ae.Handlers.Identity.CreateWithEnrollments(identityModel, enrollments)
		return identityId, err
	})
}

func (r *IdentityRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.Identity)
}

func (r *IdentityRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params identity.UpdateIdentityParams) {
	Update(rc, func(id string) error {
		return ae.Handlers.Identity.Update(MapUpdateIdentityToModel(params.ID, params.Body, getIdentityTypeId(ae, params.Body.Type)))
	})
}

func (r *IdentityRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params identity.PatchIdentityParams) {
	Patch(rc, func(id string, fields JsonFields) error {
		return ae.Handlers.Identity.Patch(MapPatchIdentityToModel(params.ID, params.Body, getIdentityTypeId(ae, params.Body.Type)), fields.FilterMaps("tags"))
	})
}

func (r *IdentityRouter) listEdgeRouterPolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Handlers.Identity, ae.Handlers.EdgeRouterPolicy, MapEdgeRouterPolicyToRestEntity)
}

func (r *IdentityRouter) listServicePolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociationWithHandler(ae, rc, ae.Handlers.Identity, ae.Handlers.ServicePolicy, MapServicePolicyToRestEntity)
}

func (r *IdentityRouter) listServices(ae *env.AppEnv, rc *response.RequestContext) {
	filterTemplate := `not isEmpty(from servicePolicies where anyOf(identities) = "%v")`
	ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Handlers.EdgeService, MapServiceToRestEntity)
}

func (r *IdentityRouter) listEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	filterTemplate := `not isEmpty(from edgeRouterPolicies where anyOf(identities) = "%v")`
	ListAssociationsWithFilter(ae, rc, filterTemplate, ae.Handlers.EdgeRouter, MapEdgeRouterToRestEntity)
}

func (r *IdentityRouter) listServiceConfigs(ae *env.AppEnv, rc *response.RequestContext) {
	listWithId(rc, func(id string) ([]interface{}, error) {
		serviceConfigs, err := ae.Handlers.Identity.GetServiceConfigs(id)
		if err != nil {
			return nil, err
		}
		result := make([]interface{}, 0)
		for _, serviceConfig := range serviceConfigs {
			service, err := ae.Handlers.EdgeService.Read(serviceConfig.Service)
			if err != nil {
				pfxlog.Logger().Debugf("listing service configs for identity [%s] could not find service [%s]: %v", id, serviceConfig.Service, err)
				continue
			}

			config, err := ae.Handlers.Config.Read(serviceConfig.Config)
			if err != nil {
				pfxlog.Logger().Debugf("listing service configs for identity [%s] could not find config [%s]: %v", id, serviceConfig.Config, err)
				continue
			}

			result = append(result, rest_model.ServiceConfigDetail{
				Config:    ToEntityRef(config.Name, config, ConfigLinkFactory),
				ConfigID:  &config.Id,
				Service:   ToEntityRef(service.Name, service, ServiceLinkFactory),
				ServiceID: &service.Id,
			})
		}
		return result, nil
	})
}

func (r *IdentityRouter) assignServiceConfigs(ae *env.AppEnv, rc *response.RequestContext, params identity.AssociateIdentitysServiceConfigsParams) {
	Update(rc, func(id string) error {
		var modelServiceConfigs []model.ServiceConfig
		for _, serviceConfig := range params.Body {
			modelServiceConfigs = append(modelServiceConfigs, MapServiceConfigToModel(*serviceConfig))
		}
		return ae.Handlers.Identity.AssignServiceConfigs(id, modelServiceConfigs)
	})
}

func (r *IdentityRouter) removeServiceConfigs(ae *env.AppEnv, rc *response.RequestContext, params identity.DisassociateIdentitysServiceConfigsParams) {
	UpdateAllowEmptyBody(rc, func(id string) error {
		var modelServiceConfigs []model.ServiceConfig
		for _, serviceConfig := range params.Body {
			modelServiceConfigs = append(modelServiceConfigs, MapServiceConfigToModel(*serviceConfig))
		}
		return ae.Handlers.Identity.RemoveServiceConfigs(id, modelServiceConfigs)
	})
}

func (r *IdentityRouter) getPolicyAdvice(ae *env.AppEnv, rc *response.RequestContext) {
	id, err := rc.GetEntityId()

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not find id property: %v", response.IdPropertyName)
		log.WithField("property", response.IdPropertyName).Error(logErr)
		rc.RespondWithError(err)
		return
	}

	serviceId, err := rc.GetEntitySubId()

	if err != nil {
		log := pfxlog.Logger()
		logErr := fmt.Errorf("could not find subId property: %v", response.SubIdPropertyName)
		log.WithField("property", response.SubIdPropertyName).Error(logErr)
		rc.RespondWithError(err)
		return
	}

	result, err := ae.Handlers.PolicyAdvisor.AnalyzeServiceReachability(id, serviceId)

	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			rc.RespondWithNotFoundWithCause(err)
			return
		}

		log := pfxlog.Logger()
		log.WithField("cause", err).Error("could not convert list")
		rc.RespondWithError(err)
		return
	}

	output := MapAdvisorServiceReachabilityToRestEntity(result)
	rc.RespondWithOk(output, nil)
}

func (r *IdentityRouter) getPostureData(ae *env.AppEnv, rc *response.RequestContext) {
	id, _ := rc.GetEntityId()
	postureData := ae.GetHandlers().PostureResponse.PostureData(id)

	rc.RespondWithOk(postureData, nil)
}
