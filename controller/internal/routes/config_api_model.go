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
	"math"
)

const EntityNameConfig = "configs"

type ConfigApi struct {
	Name *string                `json:"name"`
	Data map[string]interface{} `json:"data"`
	Tags map[string]interface{} `json:"tags"`
}

func (i *ConfigApi) ToModel(id string) *model.Config {
	result := &model.Config{}
	result.Id = id
	result.Name = stringz.OrEmpty(i.Name)
	result.Data = i.Data
	result.Tags = i.Tags

	narrowJsonTypes(result.Data)
	return result
}

type ConfigApiList struct {
	*env.BaseApi
	Name *string                `json:"name"`
	Data map[string]interface{} `json:"data"`
}

func (c *ConfigApiList) GetSelfLink() *response.Link {
	return c.BuildSelfLink(c.Id)
}

func (ConfigApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameConfig, id))
}

func (c *ConfigApiList) PopulateLinks() {
	if c.Links == nil {
		self := c.GetSelfLink()
		c.Links = &response.Links{
			EntityNameSelf: self,
		}
	}
}

func (c *ConfigApiList) ToEntityApiRef() *EntityApiRef {
	c.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameConfig,
		Name:   c.Name,
		Id:     c.Id,
		Links:  c.Links,
	}
}

func MapConfigToApiEntity(_ *env.AppEnv, _ *response.RequestContext, e model.BaseModelEntity) (BaseApiEntity, error) {
	i, ok := e.(*model.Config)

	if !ok {
		err := fmt.Errorf("entity is not a configuration \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapConfigToApiList(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapConfigToApiList(i *model.Config) (*ConfigApiList, error) {
	ret := &ConfigApiList{
		BaseApi: env.FromBaseModelEntity(i),
		Name:    &i.Name,
		Data:    i.Data,
	}

	ret.PopulateLinks()

	return ret, nil
}

func narrowJsonTypes(m map[string]interface{}) {
	for k, v := range m {
		switch val := v.(type) {
		case float64:
			intVal := math.Trunc(val)
			if intVal == val {
				m[k] = intVal
			}
		case map[string]interface{}:
			narrowJsonTypes(val)
		}
	}
}
