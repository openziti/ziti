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
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/foundation/v2/stringz"
)

const EntityNameServiceEdgeRouterPolicy = "service-edge-router-policies"

var ServiceEdgeRouterPolicyLinkFactory = NewServiceEdgeRouterPolicyLinkFactory()

type ServiceEdgeRouterPolicyLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewServiceEdgeRouterPolicyLinkFactory() *ServiceEdgeRouterPolicyLinkFactoryImpl {
	return &ServiceEdgeRouterPolicyLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameServiceEdgeRouterPolicy),
	}
}

func (factory *ServiceEdgeRouterPolicyLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameEdgeRouter] = factory.NewNestedLink(entity, EntityNameEdgeRouter)
	links[EntityNameService] = factory.NewNestedLink(entity, EntityNameService)
	return links
}

func MapCreateServiceEdgeRouterPolicyToModel(policy *rest_model.ServiceEdgeRouterPolicyCreate) *model.ServiceEdgeRouterPolicy {
	semantic := ""
	if policy.Semantic != nil {
		semantic = string(*policy.Semantic)
	}

	ret := &model.ServiceEdgeRouterPolicy{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(policy.Tags),
		},
		Name:            stringz.OrEmpty(policy.Name),
		Semantic:        semantic,
		EdgeRouterRoles: policy.EdgeRouterRoles,
		ServiceRoles:    policy.ServiceRoles,
	}

	return ret
}

func MapUpdateServiceEdgeRouterPolicyToModel(id string, policy *rest_model.ServiceEdgeRouterPolicyUpdate) *model.ServiceEdgeRouterPolicy {
	semantic := ""
	if policy.Semantic != nil {
		semantic = string(*policy.Semantic)
	}

	ret := &model.ServiceEdgeRouterPolicy{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(policy.Tags),
			Id:   id,
		},
		Name:            stringz.OrEmpty(policy.Name),
		Semantic:        semantic,
		EdgeRouterRoles: policy.EdgeRouterRoles,
		ServiceRoles:    policy.ServiceRoles,
	}

	return ret
}

func MapPatchServiceEdgeRouterPolicyToModel(id string, policy *rest_model.ServiceEdgeRouterPolicyPatch) *model.ServiceEdgeRouterPolicy {
	ret := &model.ServiceEdgeRouterPolicy{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(policy.Tags),
			Id:   id,
		},
		Name:            policy.Name,
		Semantic:        string(policy.Semantic),
		EdgeRouterRoles: policy.EdgeRouterRoles,
		ServiceRoles:    policy.ServiceRoles,
	}

	return ret
}

func MapServiceEdgeRouterPolicyToRestEntity(ae *env.AppEnv, _ *response.RequestContext, policy *model.ServiceEdgeRouterPolicy) (interface{}, error) {
	return MapServiceEdgeRouterPolicyToRestModel(ae, policy)
}

func MapServiceEdgeRouterPolicyToRestModel(ae *env.AppEnv, policy *model.ServiceEdgeRouterPolicy) (*rest_model.ServiceEdgeRouterPolicyDetail, error) {
	semantic := rest_model.Semantic(policy.Semantic)

	ret := &rest_model.ServiceEdgeRouterPolicyDetail{
		BaseEntity:             BaseEntityToRestModel(policy, ServiceEdgeRouterPolicyLinkFactory),
		EdgeRouterRoles:        policy.EdgeRouterRoles,
		EdgeRouterRolesDisplay: GetNamedEdgeRouterRoles(ae.GetManagers().EdgeRouter, policy.EdgeRouterRoles),
		Name:                   &policy.Name,
		Semantic:               &semantic,
		ServiceRoles:           policy.ServiceRoles,
		ServiceRolesDisplay:    GetNamedServiceRoles(ae.GetManagers().EdgeService, policy.ServiceRoles),
	}

	return ret, nil
}
