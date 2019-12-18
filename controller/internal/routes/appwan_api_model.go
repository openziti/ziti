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

const EntityNameAppWan = "app-wans"

type AppWanApiUpdate struct {
	Tags *map[string]interface{} `json:"tags"`
	Name *string                 `json:"name"`
}

func (i *AppWanApiUpdate) ToModel(id string) *model.Appwan {
	result := &model.Appwan{}
	result.Id = id
	result.Name = stringz.OrEmpty(i.Name)
	if i.Tags != nil {
		result.Tags = *i.Tags
	}
	return result
}

type AppWanApiCreate struct {
	Name       *string                 `json:"name"`
	Identities []string                `json:"identities"`
	Services   []string                `json:"services"`
	Tags       *map[string]interface{} `json:"tags"`
}

func NewAppWanApiCreate() *AppWanApiCreate {
	return &AppWanApiCreate{
		Identities: []string{},
		Services:   []string{},
	}
}

func (i *AppWanApiCreate) ToModel() *model.Appwan {
	result := &model.Appwan{}
	result.Name = stringz.OrEmpty(i.Name)
	if i.Tags != nil {
		result.Tags = *i.Tags
	}
	result.Identities = i.Identities
	result.Services = i.Services
	return result
}

type AppWanApiList struct {
	*env.BaseApi
	Name *string `json:"name"`
}

func (AppWanApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameAppWan, id))
}

func (e *AppWanApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (e *AppWanApiList) GetIdentitiesLink() *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s/%s", EntityNameAppWan, e.Id, EntityNameIdentity))
}

func (e *AppWanApiList) GetServicesLink() *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s/%s", EntityNameAppWan, e.Id, EntityNameService))
}

func (e *AppWanApiList) PopulateLinks() {
	if e.Links == nil {
		e.Links = &response.Links{
			EntityNameSelf:     e.GetSelfLink(),
			EntityNameService:  e.GetServicesLink(),
			EntityNameIdentity: e.GetIdentitiesLink(),
		}
	}
}

func (e *AppWanApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameAppWan,
		Name:   e.Name,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func MapAppWanToApiEntity(_ *env.AppEnv, _ *response.RequestContext, e model.BaseModelEntity) (BaseApiEntity, error) {
	i, ok := e.(*model.Appwan)

	if !ok {
		err := fmt.Errorf("entity is not an appwan \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapAppWanToApiList(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapAppWanToApiList(i *model.Appwan) (*AppWanApiList, error) {
	ret := &AppWanApiList{
		BaseApi: env.FromBaseModelEntity(i),
		Name:    &i.Name,
	}

	ret.PopulateLinks()

	return ret, nil
}
