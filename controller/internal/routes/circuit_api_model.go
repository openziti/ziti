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
	"github.com/openziti/ziti/controller/api_impl"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/rest_model"
)

const EntityNameCircuit = "circuits"

var CircuitLinkFactory = NewCircuitLinkFactory()

type CircuitLinkFactoryIml struct {
	api_impl.BasicLinkFactory
}

func NewCircuitLinkFactory() *CircuitLinkFactoryIml {
	return &CircuitLinkFactoryIml{
		BasicLinkFactory: *api_impl.NewBasicLinkFactory(EntityNameCircuit),
	}
}

func (factory *CircuitLinkFactoryIml) Links(entity api_impl.LinkEntity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[api_impl.EntityNameTerminator] = factory.NewNestedLink(entity, api_impl.EntityNameTerminator)
	links[api_impl.EntityNameService] = factory.NewNestedLink(entity, api_impl.EntityNameService)
	return links
}

func MapCircuitToRestModel(ae *env.AppEnv, _ *response.RequestContext, circuit *model.Circuit) (interface{}, error) {
	path := &rest_model.Path{}
	for _, node := range circuit.Path.Nodes {
		path.Nodes = append(path.Nodes, api_impl.ToEntityRef(node.Name, node, api_impl.RouterLinkFactory))
	}
	for _, link := range circuit.Path.Links {
		path.Links = append(path.Links, api_impl.ToEntityRef(link.Id, link, LinkLinkFactory))
	}

	var svcEntityRef *rest_model.EntityRef
	if svc, _ := ae.Managers.Service.Read(circuit.ServiceId); svc != nil {
		svcEntityRef = api_impl.ToEntityRef(svc.Name, svc, api_impl.ServiceLinkFactory)
	} else {
		svcEntityRef = api_impl.ToEntityRef("<deleted>", deletedEntity(circuit.ServiceId), api_impl.ServiceLinkFactory)
	}

	ret := &rest_model.CircuitDetail{
		BaseEntity: api_impl.BaseEntityToRestModel(circuit, CircuitLinkFactory),
		ClientID:   circuit.ClientId,
		Path:       path,
		Service:    svcEntityRef,
		Terminator: api_impl.ToEntityRef(circuit.Terminator.GetId(), circuit.Terminator, api_impl.TerminatorLinkFactory),
	}

	return ret, nil
}

type deletedEntity string

func (self deletedEntity) GetId() string {
	return string(self)
}
