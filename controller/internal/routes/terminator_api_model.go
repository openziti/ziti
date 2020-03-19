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
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/util/stringz"
)

const EntityNameTerminator = "terminators"

type TerminatorApi struct {
	Service *string `json:"service"`
	Router  *string `json:"router"`
	Binding *string `json:"binding"`
	Address *string `json:"address"`
}

func (i *TerminatorApi) ToModel(id string) *network.Terminator {
	result := &network.Terminator{}
	result.Id = id
	result.Service = stringz.OrEmpty(i.Service)
	result.Router = stringz.OrEmpty(i.Router)
	result.Binding = stringz.OrEmpty(i.Binding)
	result.Address = stringz.OrEmpty(i.Address)

	return result
}

type TerminatorApiList struct {
	*env.BaseApi
	ServiceId string        `json:"serviceId"`
	Service   *EntityApiRef `json:"service"`
	RouterId  string        `json:"routerId"`
	Router    *EntityApiRef `json:"router"`
	Binding   string        `json:"binding"`
	Address   string        `json:"address"`
}

func (c *TerminatorApiList) GetSelfLink() *response.Link {
	return c.BuildSelfLink(c.Id)
}

func (TerminatorApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameTerminator, id))
}

func (c *TerminatorApiList) PopulateLinks() {
	if c.Links == nil {
		self := c.GetSelfLink()
		c.Links = &response.Links{
			EntityNameSelf: self,
		}
	}
}

func (c *TerminatorApiList) ToEntityApiRef() *EntityApiRef {
	c.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameTerminator,
		Name:   nil,
		Id:     c.Id,
		Links:  c.Links,
	}
}

func MapTerminatorToApiEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (BaseApiEntity, error) {
	i, ok := e.(*network.Terminator)

	if !ok {
		err := fmt.Errorf("entity is not a terminator \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapTerminatorToApiList(ae, i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapTerminatorToApiList(ae *env.AppEnv, i *network.Terminator) (*TerminatorApiList, error) {
	service, err := ae.Handlers.EdgeService.Read(i.Service)
	if err != nil {
		return nil, err
	}

	router, err := ae.Handlers.EdgeRouter.Read(i.Router)
	if err != nil {
		return nil, err
	}

	ret := &TerminatorApiList{
		BaseApi:   env.FromBaseModelEntity(i),
		ServiceId: i.Service,
		Service:   NewServiceEntityRef(service),
		RouterId:  i.Router,
		Router:    NewEdgeRouterEntityRef(router),
		Binding:   i.Binding,
		Address:   i.Address,
	}

	ret.PopulateLinks()

	return ret, nil
}
