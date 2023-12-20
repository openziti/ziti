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
	"github.com/openziti/ziti/controller/api"
	"github.com/openziti/ziti/controller/network"

	"github.com/openziti/ziti/controller/rest_model"
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

func MapCircuitToRestModel(n *network.Network, _ api.RequestContext, circuit *network.Circuit) (*rest_model.CircuitDetail, error) {
	path := &rest_model.Path{}
	for _, node := range circuit.Path.Nodes {
		path.Nodes = append(path.Nodes, ToEntityRef(node.Name, node, RouterLinkFactory))
	}
	for _, link := range circuit.Path.Links {
		path.Links = append(path.Links, ToEntityRef(link.Id, link, LinkLinkFactory))
	}

	svc, err := n.Services.Read(circuit.ServiceId)
	if err != nil {
		return nil, err
	}
	ret := &rest_model.CircuitDetail{
		BaseEntity: BaseEntityToRestModel(circuit, CircuitLinkFactory),
		ClientID:   circuit.ClientId,
		Path:       path,
		Service:    ToEntityRef(svc.Name, svc, ServiceLinkFactory),
		Terminator: ToEntityRef(circuit.Terminator.GetId(), circuit.Terminator, TerminatorLinkFactory),
	}

	return ret, nil
}
