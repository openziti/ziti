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
	"github.com/netfoundry/ziti-edge/migration"
	"github.com/netfoundry/ziti-foundation/util/stringz"
)

const EntityNameServicePolicy = "service-policies"

type ServicePolicyApi struct {
	Tags          *migration.PropertyMap `json:"tags"`
	Name          *string                `json:"name"`
	PolicyType    *string                `json:"type"`
	ServiceRoles  []string               `json:"serviceRoles"`
	IdentityRoles []string               `json:"identityRoles"`
}

func (i *ServicePolicyApi) ToModel(id string) *model.ServicePolicy {
	result := &model.ServicePolicy{}
	result.Id = id
	result.Name = stringz.OrEmpty(i.Name)
	result.PolicyType = stringz.OrEmpty(i.PolicyType)
	result.ServiceRoles = i.ServiceRoles
	result.IdentityRoles = i.IdentityRoles
	if i.Tags != nil {
		result.Tags = *i.Tags
	}
	return result
}

type ServicePolicyApiList struct {
	*env.BaseApi
	Name          *string  `json:"name"`
	PolicyType    *string  `json:"type"`
	ServiceRoles  []string `json:"serviceRoles"`
	IdentityRoles []string `json:"identityRoles"`
}

func (c *ServicePolicyApiList) GetSelfLink() *response.Link {
	return c.BuildSelfLink(c.Id)
}

func (ServicePolicyApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameServicePolicy, id))
}

func (c *ServicePolicyApiList) PopulateLinks() {
	if c.Links == nil {
		self := c.GetSelfLink()
		c.Links = &response.Links{
			EntityNameSelf:     self,
			EntityNameService:  response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameService)),
			EntityNameIdentity: response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameIdentity)),
		}
	}
}

func (c *ServicePolicyApiList) ToEntityApiRef() *EntityApiRef {
	c.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameServicePolicy,
		Name:   c.Name,
		Id:     c.Id,
		Links:  c.Links,
	}
}

func MapServicePolicyToApiEntity(_ *env.AppEnv, _ *response.RequestContext, e model.BaseModelEntity) (BaseApiEntity, error) {
	i, ok := e.(*model.ServicePolicy)

	if !ok {
		err := fmt.Errorf("entity is not a service policy \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapServicePolicyToApiList(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapServicePolicyToApiList(i *model.ServicePolicy) (*ServicePolicyApiList, error) {
	ret := &ServicePolicyApiList{
		BaseApi:       env.FromBaseModelEntity(i),
		Name:          &i.Name,
		PolicyType:    &i.PolicyType,
		ServiceRoles:  i.ServiceRoles,
		IdentityRoles: i.IdentityRoles,
	}

	ret.PopulateLinks()

	return ret, nil
}
