/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/util/stringz"
)

const EntityNameEdgeRouterPolicy = "edge-router-policies"

type EdgeRouterPolicyApi struct {
	Tags            map[string]interface{} `json:"tags"`
	Name            *string                `json:"name"`
	Semantic        *string                `json:"semantic"`
	EdgeRouterRoles []string               `json:"edgeRouterRoles"`
	IdentityRoles   []string               `json:"identityRoles"`
}

func (i *EdgeRouterPolicyApi) ToModel(id string) *model.EdgeRouterPolicy {
	result := &model.EdgeRouterPolicy{}
	result.Id = id
	result.Name = stringz.OrEmpty(i.Name)
	result.Semantic = stringz.OrEmpty(i.Semantic)
	result.EdgeRouterRoles = i.EdgeRouterRoles
	result.IdentityRoles = i.IdentityRoles
	result.Tags = i.Tags
	return result
}

type EdgeRouterPolicyApiList struct {
	*env.BaseApi
	Name            string   `json:"name"`
	Semantic        string   `json:"semantic"`
	EdgeRouterRoles []string `json:"edgeRouterRoles"`
	IdentityRoles   []string `json:"identityRoles"`
}

func (c *EdgeRouterPolicyApiList) GetSelfLink() *response.Link {
	return c.BuildSelfLink(c.Id)
}

func (EdgeRouterPolicyApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameEdgeRouterPolicy, id))
}

func (c *EdgeRouterPolicyApiList) PopulateLinks() {
	if c.Links == nil {
		self := c.GetSelfLink()
		c.Links = &response.Links{
			EntityNameSelf:       self,
			EntityNameEdgeRouter: response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameEdgeRouter)),
			EntityNameIdentity:   response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameIdentity)),
		}
	}
}

func (c *EdgeRouterPolicyApiList) ToEntityApiRef() *EntityApiRef {
	c.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameEdgeRouterPolicy,
		Name:   &c.Name,
		Id:     c.Id,
		Links:  c.Links,
	}
}

func MapEdgeRouterPolicyToApiEntity(_ *env.AppEnv, _ *response.RequestContext, e models.Entity) (BaseApiEntity, error) {
	i, ok := e.(*model.EdgeRouterPolicy)

	if !ok {
		err := fmt.Errorf("entity is not an edge router policy \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapEdgeRouterPolicyToApiList(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapEdgeRouterPolicyToApiList(i *model.EdgeRouterPolicy) (*EdgeRouterPolicyApiList, error) {
	ret := &EdgeRouterPolicyApiList{
		BaseApi:         env.FromBaseModelEntity(i),
		Name:            i.Name,
		Semantic:        i.Semantic,
		EdgeRouterRoles: i.EdgeRouterRoles,
		IdentityRoles:   i.IdentityRoles,
	}

	ret.PopulateLinks()

	return ret, nil
}
