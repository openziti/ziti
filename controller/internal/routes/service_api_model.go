/*
	Copyright 2020 NetFoundry, Inc.

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

const EntityNameService = "services"

type ServiceApiCreate struct {
	Name               *string                `json:"name"`
	TerminatorStrategy *string                `json:"terminatorStrategy"`
	Tags               map[string]interface{} `json:"tags"`
	RoleAttributes     []string               `json:"roleAttributes"`
	Configs            []string               `json:"configs"`
}

func (i *ServiceApiCreate) ToModel() *model.Service {
	result := &model.Service{}
	result.Name = stringz.OrEmpty(i.Name)
	result.TerminatorStrategy = stringz.OrEmpty(i.TerminatorStrategy)
	result.RoleAttributes = i.RoleAttributes
	result.Tags = i.Tags
	result.Configs = i.Configs
	return result
}

type ServiceApiUpdate struct {
	Name               *string                `json:"name"`
	TerminatorStrategy *string                `json:"terminatorStrategy"`
	Tags               map[string]interface{} `json:"tags"`
	RoleAttributes     []string               `json:"roleAttributes"`
	Configs            []string               `json:"configs"`
}

func (i *ServiceApiUpdate) ToModel(id string) *model.Service {
	result := &model.Service{}
	result.Id = id
	result.Name = stringz.OrEmpty(i.Name)
	result.TerminatorStrategy = stringz.OrEmpty(i.TerminatorStrategy)
	result.Tags = i.Tags
	result.RoleAttributes = i.RoleAttributes
	result.Configs = i.Configs
	return result
}

func NewServiceEntityRef(s *model.Service) *EntityApiRef {
	links := &response.Links{
		"self": NewServiceLink(s.Id),
	}

	return &EntityApiRef{
		Entity: EntityNameService,
		Id:     s.Id,
		Name:   &s.Name,
		Links:  links,
	}
}

func NewServiceLink(sessionId string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameService, sessionId))
}

type ServiceApiList struct {
	*env.BaseApi
	Name               *string                           `json:"name"`
	TerminatorStrategy *string                           `json:"terminatorStrategy"`
	RoleAttributes     []string                          `json:"roleAttributes"`
	Permissions        []string                          `json:"permissions"`
	Configs            []string                          `json:"configs"`
	Config             map[string]map[string]interface{} `json:"config"`
}

func (e *ServiceApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (ServiceApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameService, id))
}

func (e *ServiceApiList) PopulateLinks() {
	if e.Links == nil {
		self := e.GetSelfLink()
		e.Links = &response.Links{
			EntityNameSelf:                    self,
			EntityNameServiceEdgeRouterPolicy: response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameServiceEdgeRouterPolicy)),
			EntityNameServicePolicy:           response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameServicePolicy)),
			EntityNameTerminator:              response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameTerminator)),
			EntityNameConfig:                  response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameConfig)),
		}
	}
}

func (e *ServiceApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameService,
		Name:   e.Name,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func MapServicesToApiEntities(ae *env.AppEnv, rc *response.RequestContext, es []*model.ServiceDetail) ([]BaseApiEntity, error) {
	// can't use modelToApi b/c it require list of network.Entity
	apiEntities := make([]BaseApiEntity, 0)

	for _, e := range es {
		al, err := MapToServiceApiList(ae, rc, e)

		if err != nil {
			return nil, err
		}

		apiEntities = append(apiEntities, al)
	}

	return apiEntities, nil
}

func MapServiceToApiEntity(ae *env.AppEnv, rc *response.RequestContext, e models.Entity) (BaseApiEntity, error) {
	i, ok := e.(*model.ServiceDetail)

	if !ok {
		err := fmt.Errorf("entity is not a service \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapToServiceApiList(ae, rc, i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapToServiceApiList(_ *env.AppEnv, _ *response.RequestContext, i *model.ServiceDetail) (*ServiceApiList, error) {
	ret := &ServiceApiList{
		BaseApi:            env.FromBaseModelEntity(i),
		Name:               &i.Name,
		TerminatorStrategy: &i.TerminatorStrategy,
		RoleAttributes:     i.RoleAttributes,
		Permissions:        i.Permissions,
		Configs:            i.Configs,
		Config:             i.Config,
	}

	ret.PopulateLinks()
	return ret, nil
}
