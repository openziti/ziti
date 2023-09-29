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
	"github.com/go-openapi/strfmt"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/network"

	"github.com/openziti/fabric/controller/rest_model"
)

const EntityNameCircuit = "circuits"

var CircuitLinkFactory = NewCircuitLinkFactory()

type CircuitLinkFactoryIml struct {
	BasicLinkFactory
}

func NewCircuitLinkFactory() *CircuitLinkFactoryIml {
	return &CircuitLinkFactoryIml{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameCircuit),
	}
}

func (factory *CircuitLinkFactoryIml) Links(entity LinkEntity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameTerminator] = factory.NewNestedLink(entity, EntityNameTerminator)
	links[EntityNameService] = factory.NewNestedLink(entity, EntityNameService)
	return links
}

func MapCircuitToRestModel(_ *network.Network, _ api.RequestContext, circuit *network.Circuit) (*rest_model.CircuitDetail, error) {
	path := &rest_model.CircuitDetailPath{}
	for _, node := range circuit.Path.Nodes {
		path.Nodes = append(path.Nodes, ToEntityRef(node.Name, node, RouterLinkFactory))
	}
	for _, link := range circuit.Path.Links {
		path.Links = append(path.Links, ToEntityRef(link.Id, link, LinkLinkFactory))
	}

	createdAt := strfmt.DateTime(circuit.CreatedAt)
	ret := &rest_model.CircuitDetail{
		ID:         &circuit.Id,
		ClientID:   circuit.ClientId,
		Path:       path,
		Service:    ToEntityRef(circuit.Service.Name, circuit.Service, ServiceLinkFactory),
		Terminator: ToEntityRef(circuit.Terminator.GetId(), circuit.Terminator, TerminatorLinkFactory),
		CreatedAt:  &createdAt,
	}

	return ret, nil
}
