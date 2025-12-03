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

package routes

import (
	"time"

	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/idgen"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/rest_model"

	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/models"
)

var FabricServiceLinkFactory = NewFabricServiceLinkFactory()

type FabricServiceLinkFactoryIml struct {
	BasicFabricLinkFactory
}

func NewFabricServiceLinkFactory() *FabricServiceLinkFactoryIml {
	return &FabricServiceLinkFactoryIml{
		BasicFabricLinkFactory: *NewBasicFabricLinkFactory(EntityNameService),
	}
}

func (factory *FabricServiceLinkFactoryIml) Links(entity LinkEntity) rest_model.Links {
	links := factory.BasicFabricLinkFactory.Links(entity)
	links[EntityNameTerminator] = factory.NewNestedLink(entity, EntityNameTerminator)
	return links
}

func MapCreateFabricServiceToModel(service *rest_model.ServiceCreate) *model.Service {
	ret := &model.Service{
		BaseEntity: models.BaseEntity{
			Tags: FabricTagsOrDefault(service.Tags),
		},
		Name:               stringz.OrEmpty(service.Name),
		TerminatorStrategy: service.TerminatorStrategy,
		MaxIdleTime:        time.Duration(service.MaxIdleTimeMillis) * time.Millisecond,
	}

	if ret.Id == "" {
		ret.Id = idgen.New()
	}

	if ret.Id != "" && ret.Name == "" {
		ret.Name = ret.Id
	}

	return ret
}

func MapUpdateFabricServiceToModel(id string, service *rest_model.ServiceUpdate) *model.Service {
	ret := &model.Service{
		BaseEntity: models.BaseEntity{
			Tags: FabricTagsOrDefault(service.Tags),
			Id:   id,
		},
		Name:               stringz.OrEmpty(service.Name),
		TerminatorStrategy: service.TerminatorStrategy,
		MaxIdleTime:        time.Duration(service.MaxIdleTimeMillis) * time.Millisecond,
	}

	return ret
}

func MapPatchFabricServiceToModel(id string, service *rest_model.ServicePatch) *model.Service {
	ret := &model.Service{
		BaseEntity: models.BaseEntity{
			Tags: FabricTagsOrDefault(service.Tags),
			Id:   id,
		},
		Name:               service.Name,
		TerminatorStrategy: service.TerminatorStrategy,
		MaxIdleTime:        time.Duration(service.MaxIdleTimeMillis) * time.Millisecond,
	}

	return ret
}

type FabricServiceModelMapper struct{}

func (FabricServiceModelMapper) ToApi(_ *env.AppEnv, _ *response.RequestContext, service *model.Service) (interface{}, error) {
	maxIdleTime := service.MaxIdleTime.Milliseconds()
	return &rest_model.ServiceDetail{
		BaseEntity:         FabricEntityToRestModel(service, FabricServiceLinkFactory),
		Name:               &service.Name,
		TerminatorStrategy: &service.TerminatorStrategy,
		MaxIdleTimeMillis:  &maxIdleTime,
	}, nil
}
