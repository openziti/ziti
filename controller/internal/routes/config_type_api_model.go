/*
	Copyright 2020 Netfoundry, Inc.

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

const EntityNameConfigType = "configTypes"

type ConfigTypeApi struct {
	Name *string                `json:"name"`
	Tags map[string]interface{} `json:"tags"`
}

func (i *ConfigTypeApi) ToModel(id string) *model.ConfigType {
	result := &model.ConfigType{}
	result.Id = id
	result.Name = stringz.OrEmpty(i.Name)
	result.Tags = i.Tags

	narrowJsonTypes(result.Data)
	return result
}

type ConfigTypeApiList struct {
	*env.BaseApi
	Name string `json:"name"`
}

func (c *ConfigTypeApiList) GetSelfLink() *response.Link {
	return c.BuildSelfLink(c.Id)
}

func (ConfigTypeApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameConfigType, id))
}

func (c *ConfigTypeApiList) PopulateLinks() {
	if c.Links == nil {
		self := c.GetSelfLink()
		c.Links = &response.Links{
			EntityNameSelf: self,
		}
	}
}

func (c *ConfigTypeApiList) ToEntityApiRef() *EntityApiRef {
	c.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameConfigType,
		Name:   &c.Name,
		Id:     c.Id,
		Links:  c.Links,
	}
}

func MapConfigTypeToApiEntity(_ *env.AppEnv, _ *response.RequestContext, e model.BaseModelEntity) (BaseApiEntity, error) {
	i, ok := e.(*model.ConfigType)

	if !ok {
		err := fmt.Errorf("entity is not a configuration type \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapConfigTypeToApiList(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapConfigTypeToApiList(i *model.ConfigType) (*ConfigTypeApiList, error) {
	ret := &ConfigTypeApiList{
		BaseApi: env.FromBaseModelEntity(i),
		Name:    i.Name,
	}

	ret.PopulateLinks()

	return ret, nil
}
