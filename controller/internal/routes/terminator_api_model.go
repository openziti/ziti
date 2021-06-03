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
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/util/stringz"
)

const EntityNameTerminator = "terminators"

var TerminatorLinkFactory = NewBasicLinkFactory(EntityNameTerminator)

func MapCreateTerminatorToModel(terminator *rest_model.TerminatorCreate) *network.Terminator {
	ret := &network.Terminator{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(terminator.Tags),
		},
		Service:        stringz.OrEmpty(terminator.Service),
		Router:         stringz.OrEmpty(terminator.Router),
		Binding:        stringz.OrEmpty(terminator.Binding),
		Address:        stringz.OrEmpty(terminator.Address),
		Identity:       terminator.Identity,
		IdentitySecret: terminator.IdentitySecret,
		Precedence:     xt.GetPrecedenceForName(string(terminator.Precedence)),
	}

	if terminator.Cost != nil {
		ret.Cost = uint16(*terminator.Cost)
	}

	return ret
}

func MapUpdateTerminatorToModel(id string, terminator *rest_model.TerminatorUpdate) *network.Terminator {
	ret := &network.Terminator{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(terminator.Tags),
			Id:   id,
		},
		Service:    stringz.OrEmpty(terminator.Service),
		Router:     stringz.OrEmpty(terminator.Router),
		Binding:    stringz.OrEmpty(terminator.Binding),
		Address:    stringz.OrEmpty(terminator.Address),
		Precedence: xt.GetPrecedenceForName(string(terminator.Precedence)),
	}

	if terminator.Cost != nil {
		ret.Cost = uint16(*terminator.Cost)
	}

	return ret
}

func MapPatchTerminatorToModel(id string, terminator *rest_model.TerminatorPatch) *network.Terminator {
	ret := &network.Terminator{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(terminator.Tags),
			Id:   id,
		},
		Service:    terminator.Service,
		Router:     terminator.Router,
		Binding:    terminator.Binding,
		Address:    terminator.Address,
		Precedence: xt.GetPrecedenceForName(string(terminator.Precedence)),
	}

	if terminator.Cost != nil {
		ret.Cost = uint16(*terminator.Cost)
	}

	return ret
}

func MapTerminatorToRestEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	terminator, ok := e.(*network.Terminator)

	if !ok {
		err := fmt.Errorf("entity is not a Terminator \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapTerminatorToRestModel(ae, terminator)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapTerminatorToRestModel(ae *env.AppEnv, terminator *network.Terminator) (*rest_model.TerminatorDetail, error) {

	service, err := ae.Handlers.Service.Read(terminator.Service)
	if err != nil {
		return nil, err
	}

	router, err := ae.Handlers.TransitRouter.Read(terminator.Router)
	if err != nil {
		return nil, err
	}

	cost := rest_model.TerminatorCost(int64(terminator.Cost))
	dynamicCost := rest_model.TerminatorCost(xt.GlobalCosts().GetDynamicCost(terminator.Id))

	ret := &rest_model.TerminatorDetail{
		BaseEntity:  BaseEntityToRestModel(terminator, TerminatorLinkFactory),
		ServiceID:   &terminator.Service,
		Service:     ToEntityRef(service.Name, service, ServiceLinkFactory),
		RouterID:    &terminator.Router,
		Router:      ToEntityRef(router.Name, router, TransitRouterLinkFactory),
		Binding:     &terminator.Binding,
		Address:     &terminator.Address,
		Identity:    &terminator.Identity,
		Cost:        &cost,
		DynamicCost: &dynamicCost,
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

func MapClientTerminatorToRestEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	terminator, ok := e.(*network.Terminator)

	if !ok {
		err := fmt.Errorf("entity is not a Terminator \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapLimitedTerminatorToRestModel(ae, terminator)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapLimitedTerminatorToRestModel(ae *env.AppEnv, terminator *network.Terminator) (*rest_model.TerminatorClientDetail, error) {
	service, err := ae.Handlers.EdgeService.Read(terminator.Service)
	if err != nil {
		return nil, err
	}

	ret := &rest_model.TerminatorClientDetail{
		BaseEntity: BaseEntityToRestModel(terminator, TerminatorLinkFactory),
		ServiceID:  &terminator.Service,
		Service:    ToEntityRef(service.Name, service, ServiceLinkFactory),
		RouterID:   &terminator.Router,
		Identity:   &terminator.Identity,
	}

	return ret, nil
}
