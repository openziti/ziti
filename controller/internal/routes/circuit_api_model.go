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
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/rest_model"
)

const EntityNameCircuit = "circuits"

var CircuitLinkFactory = NewCircuitLinkFactory()

type CircuitLinkFactoryIml struct {
	BasicFabricLinkFactory
}

func NewCircuitLinkFactory() *CircuitLinkFactoryIml {
	return &CircuitLinkFactoryIml{
		BasicFabricLinkFactory: *NewBasicFabricLinkFactory(EntityNameCircuit),
	}
}

func (factory *CircuitLinkFactoryIml) Links(entity LinkEntity) rest_model.Links {
	links := factory.BasicFabricLinkFactory.Links(entity)
	links[EntityNameTerminator] = factory.NewNestedLink(entity, EntityNameTerminator)
	links[EntityNameService] = factory.NewNestedLink(entity, EntityNameService)
	return links
}

func MapCircuitToRestModel(ae *env.AppEnv, _ *response.RequestContext, circuit *model.Circuit) (interface{}, error) {
	path := &rest_model.Path{}
	for _, node := range circuit.Path.Nodes {
		path.Nodes = append(path.Nodes, ToFabricEntityRef(node.Name, node, FabricRouterLinkFactory))
	}
	for _, link := range circuit.Path.Links {
		path.Links = append(path.Links, ToFabricEntityRef(link.Id, link, LinkLinkFactory))
	}

	var svcEntityRef *rest_model.EntityRef
	if svc, _ := ae.Managers.Service.Read(circuit.ServiceId); svc != nil {
		svcEntityRef = ToFabricEntityRef(svc.Name, svc, FabricServiceLinkFactory)
	} else {
		svcEntityRef = ToFabricEntityRef("<deleted>", deletedEntity(circuit.ServiceId), FabricServiceLinkFactory)
	}

	ret := &rest_model.CircuitDetail{
		BaseEntity: FabricEntityToRestModel(circuit, CircuitLinkFactory),
		ClientID:   circuit.ClientId,
		Path:       path,
		Service:    svcEntityRef,
		Terminator: ToFabricEntityRef(circuit.Terminator.GetId(), circuit.Terminator, FabricTerminatorLinkFactory),
	}

	return ret, nil
}

type deletedEntity string

func (self deletedEntity) GetId() string {
	return string(self)
}
