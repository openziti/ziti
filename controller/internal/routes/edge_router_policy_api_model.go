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

const EntityNameEdgeRouterPolicy = "edge-router-policies"

var EdgeRouterPolicyLinkFactory = NewEdgeRouterPolicyLinkFactory()

type EdgeRouterPolicyLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewEdgeRouterPolicyLinkFactory() *EdgeRouterPolicyLinkFactoryImpl {
	return &EdgeRouterPolicyLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameEdgeRouterPolicy),
	}
}

func (factory *EdgeRouterPolicyLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameEdgeRouter] = factory.NewNestedLink(entity, EntityNameEdgeRouter)
	links[EntityNameIdentity] = factory.NewNestedLink(entity, EntityNameIdentity)

	return links
}

func MapCreateEdgeRouterPolicyToModel(policy *rest_model.EdgeRouterPolicyCreate) *model.EdgeRouterPolicy {
	semantic := ""
	if policy.Semantic != nil {
		semantic = string(*policy.Semantic)
	}
	ret := &model.EdgeRouterPolicy{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(policy.Tags),
		},
		Name:            stringz.OrEmpty(policy.Name),
		Semantic:        semantic,
		EdgeRouterRoles: policy.EdgeRouterRoles,
		IdentityRoles:   policy.IdentityRoles,
	}

	return ret
}

func MapUpdateEdgeRouterPolicyToModel(id string, policy *rest_model.EdgeRouterPolicyUpdate) *model.EdgeRouterPolicy {
	semantic := ""
	if policy.Semantic != nil {
		semantic = string(*policy.Semantic)
	}

	ret := &model.EdgeRouterPolicy{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(policy.Tags),
			Id:   id,
		},
		Name:            stringz.OrEmpty(policy.Name),
		Semantic:        semantic,
		EdgeRouterRoles: policy.EdgeRouterRoles,
		IdentityRoles:   policy.IdentityRoles,
	}

	return ret
}

func MapPatchEdgeRouterPolicyToModel(id string, policy *rest_model.EdgeRouterPolicyPatch) *model.EdgeRouterPolicy {
	ret := &model.EdgeRouterPolicy{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(policy.Tags),
			Id:   id,
		},
		Name:            policy.Name,
		Semantic:        string(policy.Semantic),
		EdgeRouterRoles: policy.EdgeRouterRoles,
		IdentityRoles:   policy.IdentityRoles,
	}

	return ret
}

func MapEdgeRouterPolicyToRestEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	policy, ok := e.(*model.EdgeRouterPolicy)

	if !ok {
		err := fmt.Errorf("entity is not a EdgeRouterPolicy \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapEdgeRouterPolicyToRestModel(ae, policy)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapEdgeRouterPolicyToRestModel(ae *env.AppEnv, policy *model.EdgeRouterPolicy) (*rest_model.EdgeRouterPolicyDetail, error) {
	semantic := rest_model.Semantic(policy.Semantic)
	ret := &rest_model.EdgeRouterPolicyDetail{
		BaseEntity:             BaseEntityToRestModel(policy, EdgeRouterPolicyLinkFactory),
		EdgeRouterRoles:        policy.EdgeRouterRoles,
		EdgeRouterRolesDisplay: GetNamedEdgeRouterRoles(ae.GetHandlers().EdgeRouter, policy.EdgeRouterRoles),
		IdentityRoles:          policy.IdentityRoles,
		IdentityRolesDisplay:   GetNamedIdentityRoles(ae.GetHandlers().Identity, policy.IdentityRoles),
		Name:                   &policy.Name,
		Semantic:               &semantic,
		IsSystem:               &policy.IsSystem,
	}

	return ret, nil
}
