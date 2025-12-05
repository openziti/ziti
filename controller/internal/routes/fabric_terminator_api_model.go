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
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/rest_model"
	"github.com/openziti/ziti/controller/xt"
)

var FabricTerminatorLinkFactory = NewBasicFabricLinkFactory(EntityNameTerminator)

func MapCreateFabricTerminatorToModel(terminator *rest_model.TerminatorCreate) *model.Terminator {
	ret := &model.Terminator{
		BaseEntity: models.BaseEntity{
			Tags: FabricTagsOrDefault(terminator.Tags),
		},
		Service:        stringz.OrEmpty(terminator.Service),
		Router:         stringz.OrEmpty(terminator.Router),
		Binding:        stringz.OrEmpty(terminator.Binding),
		Address:        stringz.OrEmpty(terminator.Address),
		InstanceId:     terminator.InstanceID,
		InstanceSecret: terminator.InstanceSecret,
		Precedence:     xt.GetPrecedenceForName(string(terminator.Precedence)),
		HostId:         terminator.HostID,
	}

	if terminator.Cost != nil {
		ret.Cost = uint16(*terminator.Cost)
	}

	return ret
}

func MapUpdateFabricTerminatorToModel(id string, terminator *rest_model.TerminatorUpdate) *model.Terminator {
	ret := &model.Terminator{
		BaseEntity: models.BaseEntity{
			Tags: FabricTagsOrDefault(terminator.Tags),
			Id:   id,
		},
		Service:    stringz.OrEmpty(terminator.Service),
		Router:     stringz.OrEmpty(terminator.Router),
		Binding:    stringz.OrEmpty(terminator.Binding),
		Address:    stringz.OrEmpty(terminator.Address),
		Precedence: xt.GetPrecedenceForName(string(terminator.Precedence)),
		HostId:     terminator.HostID,
	}

	if terminator.Cost != nil {
		ret.Cost = uint16(*terminator.Cost)
	}

	return ret
}

func MapPatchFabricTerminatorToModel(id string, terminator *rest_model.TerminatorPatch) *model.Terminator {
	ret := &model.Terminator{
		BaseEntity: models.BaseEntity{
			Tags: FabricTagsOrDefault(terminator.Tags),
			Id:   id,
		},
		Service:    terminator.Service,
		Router:     terminator.Router,
		Binding:    terminator.Binding,
		Address:    terminator.Address,
		Precedence: xt.GetPrecedenceForName(string(terminator.Precedence)),
		HostId:     terminator.HostID,
	}

	if terminator.Cost != nil {
		ret.Cost = uint16(*terminator.Cost)
	}

	return ret
}

func MapFabricTerminatorToRestEntity(ae *env.AppEnv, _ *response.RequestContext, terminator *model.Terminator) (interface{}, error) {
	return MapFabricTerminatorToRestModel(ae, terminator)
}

func MapFabricTerminatorToRestModel(ae *env.AppEnv, terminator *model.Terminator) (*rest_model.TerminatorDetail, error) {
	service, err := ae.Managers.Service.Read(terminator.Service)
	if err != nil {
		return nil, err
	}

	router, err := ae.Managers.Router.Read(terminator.Router)
	if err != nil {
		return nil, err
	}

	cost := rest_model.TerminatorCost(int64(terminator.Cost))
	dynamicCost := rest_model.TerminatorCost(xt.GlobalCosts().GetDynamicCost(terminator.Id))

	ret := &rest_model.TerminatorDetail{
		BaseEntity:  FabricEntityToRestModel(terminator, FabricTerminatorLinkFactory),
		ServiceID:   &terminator.Service,
		Service:     ToFabricEntityRef(service.Name, service, FabricServiceLinkFactory),
		RouterID:    &terminator.Router,
		Router:      ToFabricEntityRef(router.Name, router, FabricRouterLinkFactory),
		Binding:     &terminator.Binding,
		Address:     &terminator.Address,
		InstanceID:  &terminator.InstanceId,
		Cost:        &cost,
		DynamicCost: &dynamicCost,
		HostID:      &terminator.HostId,
	}

	precedence := terminator.Precedence

	resultPrecedence := rest_model.TerminatorPrecedenceDefault

	if precedence.IsRequired() {
		resultPrecedence = rest_model.TerminatorPrecedenceRequired
	} else if precedence.IsFailed() {
		resultPrecedence = rest_model.TerminatorPrecedenceFailed
	}

	ret.Precedence = &resultPrecedence

	return ret, nil
}
