/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-edge/edge/controller/env"
	"github.com/netfoundry/ziti-edge/edge/controller/model"
	"github.com/netfoundry/ziti-edge/edge/controller/response"
	"github.com/netfoundry/ziti-foundation/util/stringz"
)

const EntityNameService = "services"

type ServiceDnsApiPost struct {
	Hostname *string `json:"hostname"`
	Port     *uint16 `json:"port"`
}

type ServiceApiCreate struct {
	Dns             *ServiceDnsApiPost     `json:"dns"`
	Name            *string                `json:"name"`
	HostIds         []string               `json:"hostIds"`
	Tags            map[string]interface{} `json:"tags"`
	EgressRouter    *string                `json:"egressRouter"`
	EndpointAddress *string                `json:"endpointAddress"`
	EdgeRouterRoles []string               `json:"edgeRouterRoles"`
	RoleAttributes  []string               `json:"roleAttributes"`
}

// DnsHostname is used by deepcopy to copy the dnsHostname value into the target struct
func (i *ServiceApiCreate) DnsHostname() string {
	if i.Dns != nil && i.Dns.Hostname != nil {
		return *i.Dns.Hostname
	}
	return ""
}

// DnsPort is used by deepcopy to copy the dnsPort value into the target struct
func (i *ServiceApiCreate) DnsPort() uint16 {
	if i.Dns != nil && i.Dns.Port != nil {
		return *i.Dns.Port
	}
	return 0
}

func (i *ServiceApiCreate) ToModel() *model.Service {
	result := &model.Service{}
	result.Name = stringz.OrEmpty(i.Name)
	result.EgressRouter = stringz.OrEmpty(i.EgressRouter)
	result.EndpointAddress = stringz.OrEmpty(i.EndpointAddress)
	result.DnsHostname = i.DnsHostname()
	result.DnsPort = i.DnsPort()
	result.EdgeRouterRoles = i.EdgeRouterRoles
	result.HostIds = i.HostIds
	result.RoleAttributes = i.RoleAttributes
	result.Tags = i.Tags
	return result
}

type ServiceApiUpdate struct {
	Dns             *ServiceDnsApiPost     `json:"dns"`
	Name            *string                `json:"name"`
	Tags            map[string]interface{} `json:"tags"`
	EgressRouter    *string                `json:"egressRouter"`
	EndpointAddress *string                `json:"endpointAddress"`
	EdgeRouterRoles []string               `json:"edgeRouterRoles"`
	RoleAttributes  []string               `json:"roleAttributes"`
}

func (i *ServiceApiUpdate) DnsHostname() string {
	if i.Dns != nil && i.Dns.Hostname != nil {
		return *i.Dns.Hostname
	}
	return ""
}

func (i *ServiceApiUpdate) DnsPort() uint16 {
	if i.Dns != nil && i.Dns.Port != nil {
		return *i.Dns.Port
	}
	return 0
}

func (i *ServiceApiUpdate) ToModel(id string) *model.Service {
	result := &model.Service{}
	result.Id = id
	result.Name = stringz.OrEmpty(i.Name)
	result.EgressRouter = stringz.OrEmpty(i.EgressRouter)
	result.EndpointAddress = stringz.OrEmpty(i.EndpointAddress)
	result.DnsHostname = i.DnsHostname()
	result.DnsPort = i.DnsPort()
	result.Tags = i.Tags
	result.EdgeRouterRoles = i.EdgeRouterRoles
	result.RoleAttributes = i.RoleAttributes
	return result
}

func NewServiceEntityRef(s *model.Service) *EntityApiRef {
	links := &response.Links{
		"self": NewServiceLink(s.Id),
	}

	return &EntityApiRef{
		Id:    s.Id,
		Name:  &s.Name,
		Links: links,
	}
}

func NewServiceLink(sessionId string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameService, sessionId))
}

type ServiceApiList struct {
	*env.BaseApi
	Name            *string            `json:"name"`
	Dns             *ServiceDnsApiPost `json:"dns"`
	EndpointAddress *string            `json:"endpointAddress"`
	EgressRouter    *string            `json:"egressRouter"`
	Hostable        *bool              `json:"hostable"`
	EdgeRouterRoles []string           `json:"edgeRouterRoles"`
}

func (e *ServiceApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (ServiceApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameService, id))
}

func (e *ServiceApiList) PopulateLinks() {
	if e.Links == nil {
		e.Links = &response.Links{
			EntityNameSelf: e.GetSelfLink(),
			"edge-routers": e.GetEdgeRoutersLink(),
			"hosts":        e.GetHostsLink(),
		}
	}
}

func (e *ServiceApiList) GetEdgeRoutersLink() *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s/%s", EntityNameService, e.Id, EntityNameEdgeRouter))

}

func (e *ServiceApiList) GetHostsLink() *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s/%s", EntityNameService, e.Id, "hosts"))
}

func (e *ServiceApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameService,
		Name:   e.Name,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func MapServicesToApiEntities(ae *env.AppEnv, rc *response.RequestContext, es []*model.Service) ([]BaseApiEntity, error) {
	// can't use modelToApi b/c it require list of BaseModelEntity
	apiEntities := make([]BaseApiEntity, 0)

	for _, e := range es {
		al, err := MapServiceToApiEntity(ae, rc, e)

		if err != nil {
			return nil, err
		}

		apiEntities = append(apiEntities, al)
	}

	return apiEntities, nil
}

func MapServiceToApiEntity(ae *env.AppEnv, rc *response.RequestContext, e model.BaseModelEntity) (BaseApiEntity, error) {
	i, ok := e.(*model.Service)

	if !ok {
		err := fmt.Errorf("entity is not a service \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapToServiceApiList(ae, rc, i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapToServiceApiList(_ *env.AppEnv, rc *response.RequestContext, i *model.Service) (*ServiceApiList, error) {
	var hostable bool
	for _, hostId := range i.HostIds {
		if hostId == rc.Identity.Id {
			hostable = true
			break
		}
	}

	ret := &ServiceApiList{
		BaseApi:         env.FromBaseModelEntity(i),
		Name:            &i.Name,
		EndpointAddress: &i.EndpointAddress,
		EgressRouter:    &i.EgressRouter,
		Dns: &ServiceDnsApiPost{
			Port:     &i.DnsPort,
			Hostname: &i.DnsHostname,
		},
		Hostable:        &hostable,
		EdgeRouterRoles: i.EdgeRouterRoles,
	}

	ret.PopulateLinks()

	return ret, nil
}
