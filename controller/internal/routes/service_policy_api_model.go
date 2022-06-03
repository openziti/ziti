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
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/stringz"
)

const EntityNameServicePolicy = "service-policies"

var ServicePolicyLinkFactory = NewServicePolicyLinkFactory()

type ServicePolicyLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewServicePolicyLinkFactory() *ServicePolicyLinkFactoryImpl {
	return &ServicePolicyLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameServicePolicy),
	}
}

func (factory *ServicePolicyLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameService] = factory.NewNestedLink(entity, EntityNameService)
	links[EntityNameIdentity] = factory.NewNestedLink(entity, EntityNameIdentity)
	links[EntityNamePostureCheck] = factory.NewNestedLink(entity, EntityNamePostureCheck)

	return links
}

func MapCreateServicePolicyToModel(policy *rest_model.ServicePolicyCreate) *model.ServicePolicy {
	semantic := ""
	if policy.Semantic != nil {
		semantic = string(*policy.Semantic)
	}

	ret := &model.ServicePolicy{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(policy.Tags),
		},
		Name:              stringz.OrEmpty(policy.Name),
		PolicyType:        string(*policy.Type),
		Semantic:          semantic,
		ServiceRoles:      policy.ServiceRoles,
		IdentityRoles:     policy.IdentityRoles,
		PostureCheckRoles: policy.PostureCheckRoles,
	}

	return ret
}

func MapUpdateServicePolicyToModel(id string, policy *rest_model.ServicePolicyUpdate) *model.ServicePolicy {
	semantic := ""
	if policy.Semantic != nil {
		semantic = string(*policy.Semantic)
	}

	ret := &model.ServicePolicy{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(policy.Tags),
			Id:   id,
		},
		Name:              stringz.OrEmpty(policy.Name),
		PolicyType:        string(*policy.Type),
		Semantic:          semantic,
		ServiceRoles:      policy.ServiceRoles,
		IdentityRoles:     policy.IdentityRoles,
		PostureCheckRoles: policy.PostureCheckRoles,
	}

	return ret
}

func MapPatchServicePolicyToModel(id string, policy *rest_model.ServicePolicyPatch) *model.ServicePolicy {
	ret := &model.ServicePolicy{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(policy.Tags),
			Id:   id,
		},
		Name:              policy.Name,
		PolicyType:        string(policy.Type),
		Semantic:          string(policy.Semantic),
		ServiceRoles:      policy.ServiceRoles,
		IdentityRoles:     policy.IdentityRoles,
		PostureCheckRoles: policy.PostureCheckRoles,
	}

	return ret
}

func MapServicePolicyToRestEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	policy, ok := e.(*model.ServicePolicy)

	if !ok {
		err := fmt.Errorf("entity is not a ServicePolicy \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapServicePolicyToRestModel(ae, policy)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapServicePolicyToRestModel(ae *env.AppEnv, policy *model.ServicePolicy) (*rest_model.ServicePolicyDetail, error) {
	semantic := rest_model.Semantic(policy.Semantic)
	dialBindType := rest_model.DialBind(policy.PolicyType)

	ret := &rest_model.ServicePolicyDetail{
		BaseEntity:               BaseEntityToRestModel(policy, ServicePolicyLinkFactory),
		IdentityRoles:            policy.IdentityRoles,
		IdentityRolesDisplay:     GetNamedIdentityRoles(ae.GetManagers().Identity, policy.IdentityRoles),
		Name:                     &policy.Name,
		Semantic:                 &semantic,
		ServiceRoles:             policy.ServiceRoles,
		ServiceRolesDisplay:      GetNamedServiceRoles(ae.GetManagers().EdgeService, policy.ServiceRoles),
		Type:                     &dialBindType,
		PostureCheckRoles:        policy.PostureCheckRoles,
		PostureCheckRolesDisplay: GetNamedPostureCheckRoles(ae.GetManagers().PostureCheck, policy.PostureCheckRoles),
	}

	return ret, nil
}
