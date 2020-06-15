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

const EntityNameService = "services"

var ServiceLinkFactory = NewServiceLinkFactory()

type ServiceLinkFactoryIml struct {
	BasicLinkFactory
}

func NewServiceLinkFactory() *ServiceLinkFactoryIml {
	return &ServiceLinkFactoryIml{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameService),
	}
}

func (factory *ServiceLinkFactoryIml) Links(entity models.Entity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameServiceEdgeRouterPolicy] = factory.NewNestedLink(entity, EntityNameServiceEdgeRouterPolicy)
	links[EntityNameServicePolicy] = factory.NewNestedLink(entity, EntityNameServicePolicy)
	links[EntityNameTerminator] = factory.NewNestedLink(entity, EntityNameTerminator)
	links[EntityNameConfig] = factory.NewNestedLink(entity, EntityNameConfig)

	return links
}

func MapCreateServiceToModel(service *rest_model.ServiceCreate) *model.Service {
	ret := &model.Service{
		BaseEntity: models.BaseEntity{
			Tags: service.Tags,
		},
		Name:               stringz.OrEmpty(service.Name),
		TerminatorStrategy: service.TerminatorStrategy,
		RoleAttributes:     service.RoleAttributes,
		Configs:            service.Configs,
	}

	return ret
}

func MapUpdateServiceToModel(id string, service *rest_model.ServiceUpdate) *model.Service {
	ret := &model.Service{
		BaseEntity: models.BaseEntity{
			Tags: service.Tags,
			Id:   id,
		},
		Name:               stringz.OrEmpty(service.Name),
		TerminatorStrategy: service.TerminatorStrategy,
		RoleAttributes:     service.RoleAttributes,
		Configs:            service.Configs,
	}

	return ret
}

func MapPatchServiceToModel(id string, service *rest_model.ServicePatch) *model.Service {
	ret := &model.Service{
		BaseEntity: models.BaseEntity{
			Tags: service.Tags,
			Id:   id,
		},
		Name:               service.Name,
		TerminatorStrategy: service.TerminatorStrategy,
		RoleAttributes:     service.RoleAttributes,
		Configs:            service.Configs,
	}

	return ret
}

func MapServiceToRestEntity(_ *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	service, ok := e.(*model.ServiceDetail)

	if !ok {
		err := fmt.Errorf("entity is not a Service \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapServiceToRestModel(service)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapServicesToRestEntity(ae *env.AppEnv, rc *response.RequestContext, es []*model.ServiceDetail) ([]interface{}, error) {
	// can't use modelToApi b/c it require list of network.Entity
	restModel := make([]interface{}, 0)

	for _, e := range es {
		al, err := MapServiceToRestEntity(ae, rc, e)

		if err != nil {
			return nil, err
		}

		restModel = append(restModel, al)
	}

	return restModel, nil
}

func MapServiceToRestModel(service *model.ServiceDetail) (*rest_model.ServiceDetail, error) {
	ret := &rest_model.ServiceDetail{
		BaseEntity:         BaseEntityToRestModel(service, ServiceLinkFactory),
		Name:               &service.Name,
		TerminatorStrategy: &service.TerminatorStrategy,
		RoleAttributes:     service.RoleAttributes,
		Configs:            service.Configs,
		Config:             service.Config,
	}

	for _, permission := range service.Permissions {
		ret.Permissions = append(ret.Permissions, rest_model.DialBind(permission))
	}

	return ret, nil
}
