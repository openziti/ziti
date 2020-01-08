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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/migration"
	"github.com/netfoundry/ziti-foundation/util/stringz"
)

const (
	EntityNameEdgeRouter = "edge-routers"
	EntityNameGateway    = "gateways"
)

type EdgeRouterEntityApiRef struct {
	*EntityApiRef
	Url *string `json:"url"`
}

type EdgeRouterApi struct {
	Tags           *migration.PropertyMap `json:"tags"`
	Name           *string                `json:"name"`
	RoleAttributes []string               `json:"roleAttributes"`
}

func (i *EdgeRouterApi) ToModel(id string) *model.EdgeRouter {
	result := &model.EdgeRouter{}
	result.Id = id
	result.Name = stringz.OrEmpty(i.Name)
	result.RoleAttributes = i.RoleAttributes
	if i.Tags != nil {
		result.Tags = *i.Tags
	}
	return result
}

type EdgeRouterApiList struct {
	*env.BaseApi
	Name                *string           `json:"name"`
	Fingerprint         *string           `json:"fingerprint"`
	RoleAttributes      []string          `json:"roleAttributes"`
	IsVerified          *bool             `json:"isVerified"`
	IsOnline            *bool             `json:"isOnline"`
	EnrollmentToken     *string           `json:"enrollmentToken"`
	EnrollmentJwt       *string           `json:"enrollmentJwt"`
	EnrollmentCreatedAt *time.Time        `json:"enrollmentCreatedAt"`
	EnrollmentExpiresAt *time.Time        `json:"enrollmentExpiresAt"`
	Hostname            *string           `json:"hostname"`
	SupportedProtocols  map[string]string `json:"supportedProtocols"`
}

func (EdgeRouterApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameEdgeRouter, id))
}

func (e *EdgeRouterApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (e *EdgeRouterApiList) PopulateLinks() {
	if e.Links == nil {
		self := e.GetSelfLink()
		e.Links = &response.Links{
			EntityNameSelf:             self,
			EntityNameEdgeRouterPolicy: response.NewLink(fmt.Sprintf(self.Href + "/" + EntityNameEdgeRouter)),
		}
	}
}

func (e *EdgeRouterApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameEdgeRouter,
		Name:   e.Name,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func MapEdgeRouterToApiEntity(ae *env.AppEnv, _ *response.RequestContext, e model.BaseModelEntity) (BaseApiEntity, error) {
	i, ok := e.(*model.EdgeRouter)

	if !ok {
		err := fmt.Errorf("entity is not an edge router \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapEdgeRouterToApiList(ae, i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapEdgeRouterToApiList(ae *env.AppEnv, i *model.EdgeRouter) (*EdgeRouterApiList, error) {
	hostname := ""
	protocols := map[string]string{}

	onlineEdgeRouter := ae.Broker.GetOnlineEdgeRouter(i.Id)

	isOnline := onlineEdgeRouter != nil

	if isOnline {
		hostname = *onlineEdgeRouter.Hostname
		protocols = onlineEdgeRouter.EdgeRouterProtocols
	}

	ret := &EdgeRouterApiList{
		BaseApi:             env.FromBaseModelEntity(i),
		Name:                &i.Name,
		RoleAttributes:      i.RoleAttributes,
		EnrollmentToken:     i.EnrollmentToken,
		EnrollmentCreatedAt: i.EnrollmentCreatedAt,
		EnrollmentExpiresAt: i.EnrollmentExpiresAt,
		IsOnline:            &isOnline,
		IsVerified:          &i.IsVerified,
		Fingerprint:         i.Fingerprint,
		EnrollmentJwt:       i.EnrollmentJwt,
		Hostname:            &hostname,
		SupportedProtocols:  protocols,
	}

	ret.PopulateLinks()

	return ret, nil
}
