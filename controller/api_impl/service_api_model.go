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

package api_impl

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/idgen"
	"github.com/openziti/fabric/controller/network"

	"github.com/openziti/fabric/rest_model"

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

func (factory *ServiceLinkFactoryIml) Links(entity LinkEntity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameTerminator] = factory.NewNestedLink(entity, EntityNameTerminator)
	return links
}

func MapCreateServiceToModel(service *rest_model.ServiceCreate) *network.Service {
	ret := &network.Service{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(service.Tags),
		},
		Name:               stringz.OrEmpty(service.Name),
		TerminatorStrategy: service.TerminatorStrategy,
	}

	if ret.Id == "" {
		ret.Id = idgen.New()
	}

	if ret.Id != "" && ret.Name == "" {
		ret.Name = ret.Id
	}

	return ret
}

func MapUpdateServiceToModel(id string, service *rest_model.ServiceUpdate) *network.Service {
	ret := &network.Service{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(service.Tags),
			Id:   id,
		},
		Name:               stringz.OrEmpty(service.Name),
		TerminatorStrategy: service.TerminatorStrategy,
	}

	return ret
}

func MapPatchServiceToModel(id string, service *rest_model.ServicePatch) *network.Service {
	ret := &network.Service{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(service.Tags),
			Id:   id,
		},
		Name:               service.Name,
		TerminatorStrategy: service.TerminatorStrategy,
	}

	return ret
}

func MapServiceToRestEntity(n *network.Network, rc api.RequestContext, e models.Entity) (interface{}, error) {
	service, ok := e.(*network.Service)

	if !ok {
		err := fmt.Errorf("entity is not a Service \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapServiceToRestModel(n, rc, service)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapServiceToRestModel(_ *network.Network, _ api.RequestContext, service *network.Service) (*rest_model.ServiceDetail, error) {
	ret := &rest_model.ServiceDetail{
		BaseEntity:         BaseEntityToRestModel(service, ServiceLinkFactory),
		Name:               &service.Name,
		TerminatorStrategy: &service.TerminatorStrategy,
	}

	return ret, nil
}
