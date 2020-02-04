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
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-foundation/util/stringz"
)

const EntityNameServiceEdgeRouterPolicy = "service-edge-router-policies"

type ServiceEdgeRouterPolicyApi struct {
	Tags            map[string]interface{} `json:"tags"`
	Name            *string                `json:"name"`
	Semantic        *string                `json:"semantic"`
	EdgeRouterRoles []string               `json:"edgeRouterRoles"`
	ServiceRoles    []string               `json:"serviceRoles"`
}

func (i *ServiceEdgeRouterPolicyApi) ToModel(id string) *model.ServiceEdgeRouterPolicy {
	result := &model.ServiceEdgeRouterPolicy{}
	result.Id = id
	result.Name = stringz.OrEmpty(i.Name)
	result.Semantic = stringz.OrEmpty(i.Semantic)
	result.EdgeRouterRoles = i.EdgeRouterRoles
	result.ServiceRoles = i.ServiceRoles
	result.Tags = i.Tags
	return result
}

type ServiceEdgeRouterPolicyApiList struct {
	*env.BaseApi
	Name            string   `json:"name"`
	Semantic        string   `json:"semantic"`
	EdgeRouterRoles []string `json:"edgeRouterRoles"`
	ServiceRoles    []string `json:"serviceRoles"`
}

func (c *ServiceEdgeRouterPolicyApiList) GetSelfLink() *response.Link {
	return c.BuildSelfLink(c.Id)
}

func (ServiceEdgeRouterPolicyApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameServiceEdgeRouterPolicy, id))
}

func (c *ServiceEdgeRouterPolicyApiList) PopulateLinks() {
	if c.Links == nil {
		self := c.GetSelfLink()
		c.Links = &response.Links{
			EntityNameSelf:       self,
			EntityNameEdgeRouter: response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameEdgeRouter)),
			EntityNameService:    response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameService)),
		}
	}
}

func (c *ServiceEdgeRouterPolicyApiList) ToEntityApiRef() *EntityApiRef {
	c.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameServiceEdgeRouterPolicy,
		Name:   &c.Name,
		Id:     c.Id,
		Links:  c.Links,
	}
}

func MapServiceEdgeRouterPolicyToApiEntity(_ *env.AppEnv, _ *response.RequestContext, e model.BaseModelEntity) (BaseApiEntity, error) {
	i, ok := e.(*model.ServiceEdgeRouterPolicy)

	if !ok {
		err := fmt.Errorf("entity is not a service edge router policy \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapServiceEdgeRouterPolicyToApiList(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapServiceEdgeRouterPolicyToApiList(i *model.ServiceEdgeRouterPolicy) (*ServiceEdgeRouterPolicyApiList, error) {
	ret := &ServiceEdgeRouterPolicyApiList{
		BaseApi:         env.FromBaseModelEntity(i),
		Name:            i.Name,
		Semantic:        i.Semantic,
		EdgeRouterRoles: i.EdgeRouterRoles,
		ServiceRoles:    i.ServiceRoles,
	}

	ret.PopulateLinks()

	return ret, nil
}
