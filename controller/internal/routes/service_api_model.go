/*
	Copyright NetFoundry Inc.

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
	"strings"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/response"
	"go.etcd.io/bbolt"
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

func MapCreateServiceToModel(service *rest_model.ServiceCreate) *model.EdgeService {
	ret := &model.EdgeService{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(service.Tags),
		},
		Name:               stringz.OrEmpty(service.Name),
		MaxIdleTime:        time.Duration(service.MaxIdleTimeMillis) * time.Millisecond,
		TerminatorStrategy: service.TerminatorStrategy,
		RoleAttributes:     service.RoleAttributes,
		Configs:            service.Configs,
		EncryptionRequired: *service.EncryptionRequired,
	}

	return ret
}

func MapUpdateServiceToModel(id string, service *rest_model.ServiceUpdate) *model.EdgeService {
	ret := &model.EdgeService{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(service.Tags),
			Id:   id,
		},
		Name:               stringz.OrEmpty(service.Name),
		MaxIdleTime:        time.Duration(service.MaxIdleTimeMillis) * time.Millisecond,
		TerminatorStrategy: service.TerminatorStrategy,
		RoleAttributes:     service.RoleAttributes,
		Configs:            service.Configs,
		EncryptionRequired: service.EncryptionRequired,
	}

	return ret
}

func MapPatchServiceToModel(id string, service *rest_model.ServicePatch) *model.EdgeService {
	ret := &model.EdgeService{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(service.Tags),
			Id:   id,
		},
		Name:               service.Name,
		MaxIdleTime:        time.Duration(service.MaxIdleTimeMillis) * time.Millisecond,
		TerminatorStrategy: service.TerminatorStrategy,
		RoleAttributes:     service.RoleAttributes,
		Configs:            service.Configs,
		EncryptionRequired: service.EncryptionRequired,
	}

	return ret
}

func GetServiceMapper(ae *env.AppEnv) func(ae *env.AppEnv, rc *response.RequestContext, service *model.ServiceDetail) (any, error) {
	fillPostureData := FillServicePostureCheckData(ae)
	return func(ae *env.AppEnv, rc *response.RequestContext, service *model.ServiceDetail) (any, error) {
		return MapServiceToRestEntity(ae, rc, service, fillPostureData)
	}
}

func MapServiceToRestEntity(ae *env.AppEnv, rc *response.RequestContext, service *model.ServiceDetail, fillPostureData bool) (interface{}, error) {
	return MapServiceToRestModel(ae, rc, service, fillPostureData)
}

func FillServicePostureCheckData(ae *env.AppEnv) bool {
	if ae.GetHostController().GetConfig().Edge.DisablePostureChecks {
		return false
	}

	arePostureChecksDefined := false
	_ = ae.GetDb().View(func(tx *bbolt.Tx) error {
		arePostureChecksDefined = !ae.GetStores().PostureCheck.IterateIds(tx, ast.BoolNodeTrue).IsValid()
		return nil
	})

	return arePostureChecksDefined
}

func MapServicesToRestEntity(ae *env.AppEnv, rc *response.RequestContext, es []*model.ServiceDetail) ([]interface{}, error) {
	// can't use modelToApi b/c it require list of network.Entity
	restModel := make([]interface{}, 0)

	fillPostureData := FillServicePostureCheckData(ae)
	for _, e := range es {
		al, err := MapServiceToRestEntity(ae, rc, e, fillPostureData)

		if err != nil {
			return nil, err
		}

		restModel = append(restModel, al)
	}

	return restModel, nil
}

func MapServiceToRestModel(ae *env.AppEnv, rc *response.RequestContext, service *model.ServiceDetail, fillPostureData bool) (*rest_model.ServiceDetail, error) {
	roleAttributes := rest_model.Attributes(service.RoleAttributes)

	maxIdleTime := service.MaxIdleTime.Milliseconds()
	ret := &rest_model.ServiceDetail{
		BaseEntity:         BaseEntityToRestModel(service, ServiceLinkFactory),
		Name:               &service.Name,
		MaxIdleTimeMillis:  &maxIdleTime,
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

	var policyPostureCheckMap map[string]*model.PolicyPostureChecks

	if !ae.GetHostController().GetConfig().Edge.DisablePostureChecks {
		policyPostureCheckMap = ae.GetManagers().EdgeService.GetPolicyPostureChecks(rc.Identity.Id, *ret.ID)
	}

	if len(policyPostureCheckMap) == 0 {
		for _, permission := range ret.Permissions {
			passing := true
			id := fmt.Sprintf("dummy %s policy: no posture checks defined", strings.ToLower(string(permission)))
			ret.PostureQueries = append(ret.PostureQueries, &rest_model.PostureQueries{
				PolicyID:       &id,
				PostureQueries: []*rest_model.PostureQuery{},
				PolicyType:     permission,
				IsPassing:      &passing,
			})
		}
	}

	for policyId, policyPostureChecks := range policyPostureCheckMap {
		isPolicyPassing := true
		policyIdCopy := policyId
		querySet := &rest_model.PostureQueries{
			PolicyID:       &policyIdCopy,
			PostureQueries: []*rest_model.PostureQuery{},
		}

		if policyPostureChecks.PolicyType == db.PolicyTypeBind {
			querySet.PolicyType = rest_model.DialBindBind
		} else if policyPostureChecks.PolicyType == db.PolicyTypeDial {
			querySet.PolicyType = rest_model.DialBindDial
		} else {
			pfxlog.Logger().Errorf("attempting to render API response for policy type [%s] for policy id [%s], unknown type expected dial/bind", policyPostureChecks.PolicyType, policyId)
		}

		for _, postureCheck := range policyPostureChecks.PostureChecks {
			query := PostureCheckToQueries(postureCheck)

			isCheckPassing := false
			found := false
			if isCheckPassing, found = validChecks[postureCheck.Id]; !found {
				isCheckPassing, _ = ae.Managers.PostureResponse.Evaluate(rc.Identity.Id, rc.ApiSession.Id, postureCheck)
				validChecks[postureCheck.Id] = isCheckPassing
			}

			var timeoutRemaining int64
			var timeoutAt *strfmt.DateTime

			ae.Managers.PostureResponse.WithPostureData(rc.Identity.Id, func(postureData *model.PostureData) {
				timeoutRemaining = postureCheck.TimeoutRemainingSeconds(rc.ApiSession.Id, postureData)
				if timeoutRemaining >= 0 {
					newTimeout := strfmt.DateTime(time.Now().Add(time.Duration(timeoutRemaining) * time.Second))
					timeoutAt = &newTimeout
				}

				//determine if updatedAt is provided by the source posture check or the posture state
				if lastUpdatedAt := postureCheck.LastUpdatedAt(rc.ApiSession.Id, postureData); lastUpdatedAt != nil {
					if lastUpdatedAt.After(postureCheck.UpdatedAt) {
						query.UpdatedAt = DateTimePtrOrNil(lastUpdatedAt)
					}
				}
			})

			timeout := postureCheck.TimeoutSeconds()
			query.IsPassing = &isCheckPassing
			query.TimeoutRemaining = &timeoutRemaining
			query.Timeout = &timeout
			query.TimeoutAt = timeoutAt
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
	case rest_model.PostureCheckTypeMFA:
		mfaCheck := check.SubType.(*model.PostureCheckMfa)
		ret.PromptOnWake = mfaCheck.PromptOnWake
		ret.PromptOnUnlock = mfaCheck.PromptOnUnlock
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

func GetNamedServiceRoles(serviceHandler *model.EdgeServiceManager, roles []string) rest_model.NamedRoles {
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
