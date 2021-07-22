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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/stringz"
	"strings"
)

const EntityNameService = "services"

var ServiceLinkFactory = NewServiceLinkFactory()

type ServiceLinkFactoryIml struct {
	BasicLinkFactory
}

func NewServiceLinkFactory() *ServiceLinkFactoryIml {
	return &ServiceLinkFactoryIml{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameService),
	}
}

func (factory *ServiceLinkFactoryIml) Links(entity models.Entity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameServiceEdgeRouterPolicy] = factory.NewNestedLink(entity, EntityNameServiceEdgeRouterPolicy)
	links[EntityNameServicePolicy] = factory.NewNestedLink(entity, EntityNameServicePolicy)
	links[EntityNameTerminator] = factory.NewNestedLink(entity, EntityNameTerminator)
	links[EntityNameConfig] = factory.NewNestedLink(entity, EntityNameConfig)

	return links
}

func MapCreateServiceToModel(service *rest_model.ServiceCreate) *model.Service {
	ret := &model.Service{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(service.Tags),
		},
		Name:               stringz.OrEmpty(service.Name),
		TerminatorStrategy: service.TerminatorStrategy,
		RoleAttributes:     service.RoleAttributes,
		Configs:            service.Configs,
		EncryptionRequired: *service.EncryptionRequired,
	}

	return ret
}

func MapUpdateServiceToModel(id string, service *rest_model.ServiceUpdate) *model.Service {
	ret := &model.Service{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(service.Tags),
			Id:   id,
		},
		Name:               stringz.OrEmpty(service.Name),
		TerminatorStrategy: service.TerminatorStrategy,
		RoleAttributes:     service.RoleAttributes,
		Configs:            service.Configs,
		EncryptionRequired: service.EncryptionRequired,
	}

	return ret
}

func MapPatchServiceToModel(id string, service *rest_model.ServicePatch) *model.Service {
	ret := &model.Service{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(service.Tags),
			Id:   id,
		},
		Name:               service.Name,
		TerminatorStrategy: service.TerminatorStrategy,
		RoleAttributes:     service.RoleAttributes,
		Configs:            service.Configs,
		EncryptionRequired: service.EncryptionRequired,
	}

	return ret
}

func MapServiceToRestEntity(ae *env.AppEnv, rc *response.RequestContext, e models.Entity) (interface{}, error) {
	service, ok := e.(*model.ServiceDetail)

	if !ok {
		err := fmt.Errorf("entity is not a Service \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapServiceToRestModel(ae, rc, service)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapServicesToRestEntity(ae *env.AppEnv, rc *response.RequestContext, es []*model.ServiceDetail) ([]interface{}, error) {
	// can't use modelToApi b/c it require list of network.Entity
	restModel := make([]interface{}, 0)

	for _, e := range es {
		al, err := MapServiceToRestEntity(ae, rc, e)

		if err != nil {
			return nil, err
		}

		restModel = append(restModel, al)
	}

	return restModel, nil
}

func MapServiceToRestModel(ae *env.AppEnv, rc *response.RequestContext, service *model.ServiceDetail) (*rest_model.ServiceDetail, error) {
	roleAttributes := rest_model.Attributes(service.RoleAttributes)

	ret := &rest_model.ServiceDetail{
		BaseEntity:         BaseEntityToRestModel(service, ServiceLinkFactory),
		Name:               &service.Name,
		TerminatorStrategy: &service.TerminatorStrategy,
		RoleAttributes:     &roleAttributes,
		Configs:            service.Configs,
		Config:             service.Config,
		EncryptionRequired: &service.EncryptionRequired,
		PostureQueries:     []*rest_model.PostureQueries{},
	}

	for _, permission := range service.Permissions {
		ret.Permissions = append(ret.Permissions, rest_model.DialBind(permission))
	}

	validChecks := map[string]bool{} //cache individual check status

	policyPostureCheckMap := ae.GetHandlers().EdgeService.GetPolicyPostureChecks(rc.Identity.Id, *ret.ID)

	for policyId, policyPostureChecks := range policyPostureCheckMap {

		isPolicyPassing := true
		policyIdCopy := policyId
		querySet := &rest_model.PostureQueries{
			PolicyID:       &policyIdCopy,
			PostureQueries: []*rest_model.PostureQuery{},
		}

		if policyPostureChecks.PolicyType == persistence.PolicyTypeBind {
			querySet.PolicyType = rest_model.DialBindBind
		} else if policyPostureChecks.PolicyType == persistence.PolicyTypeDial {
			querySet.PolicyType = rest_model.DialBindDial
		} else {
			pfxlog.Logger().Errorf("attempting to render API response for policy type [%s] for policy id [%s], unknown type expected dial/bind", policyPostureChecks.PolicyType, policyId)
		}

		if policyPostureChecks == nil {
			pfxlog.Logger().Errorf("unexpected nil policyPostureCheck attempting to render posture queries for service [%s]", service.Id)
			continue
		}

		for _, postureCheck := range policyPostureChecks.PostureChecks {
			query := PostureCheckToQueries(postureCheck)

			isCheckPassing := false
			found := false
			if isCheckPassing, found = validChecks[postureCheck.Id]; !found {
				isCheckPassing, _ = ae.Handlers.PostureResponse.Evaluate(rc.Identity.Id, rc.ApiSession.Id, postureCheck)
				validChecks[postureCheck.Id] = isCheckPassing
			}
			timeout := postureCheck.TimeoutSeconds()
			timeoutRemaining := postureCheck.TimeoutRemainingSeconds(rc.ApiSession.Id, ae.Handlers.PostureResponse.PostureData(rc.Identity.Id))

			query.IsPassing = &isCheckPassing
			query.TimeoutRemaining = &timeoutRemaining
			query.Timeout = &timeout
			querySet.PostureQueries = append(querySet.PostureQueries, query)

			if !isCheckPassing {
				isPolicyPassing = false
			}
		}
		querySet.IsPassing = &isPolicyPassing
		ret.PostureQueries = append(ret.PostureQueries, querySet)
	}

	return ret, nil
}

func PostureCheckToQueries(check *model.PostureCheck) *rest_model.PostureQuery {
	isPassing := false
	queryType := rest_model.PostureCheckType(check.TypeId)
	ret := &rest_model.PostureQuery{
		BaseEntity: BaseEntityToRestModel(check, PostureCheckLinkFactory),
		IsPassing:  &isPassing,
		QueryType:  &queryType,
	}

	switch *ret.QueryType {
	case rest_model.PostureCheckTypePROCESS:
		processCheck := check.SubType.(*model.PostureCheckProcess)
		ret.Process = &rest_model.PostureQueryProcess{
			OsType: rest_model.OsType(processCheck.OsType),
			Path:   processCheck.Path,
		}
	case rest_model.PostureCheckTypePROCESSMULTI:
		processCheck := check.SubType.(*model.PostureCheckProcessMulti)
		for _, process := range processCheck.Processes {
			ret.Processes = append(ret.Processes, &rest_model.PostureQueryProcess{
				OsType: rest_model.OsType(process.OsType),
				Path:   process.Path,
			})
		}
	}

	return ret
}

func GetNamedServiceRoles(serviceHandler *model.EdgeServiceHandler, roles []string) rest_model.NamedRoles {
	result := rest_model.NamedRoles{}
	for _, role := range roles {
		if strings.HasPrefix(role, "@") {

			service, err := serviceHandler.Read(role[1:])
			if err != nil {
				pfxlog.Logger().Errorf("error converting service role [%s] to a named role: %v", role, err)
				continue
			}

			result = append(result, &rest_model.NamedRole{
				Role: role,
				Name: "@" + service.Name,
			})
		} else {
			result = append(result, &rest_model.NamedRole{
				Role: role,
				Name: role,
			})
		}
	}
	return result
}
