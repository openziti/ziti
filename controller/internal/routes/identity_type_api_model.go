/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-fabric/controller/models"
)

const EntityNameIdentityType = "identity-types"

type IdentityTypeApiList struct {
	*env.BaseApi
	Name *string `json:"name"`
}

func (IdentityTypeApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameIdentityType, id))
}

func (e *IdentityTypeApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (e *IdentityTypeApiList) PopulateLinks() {
	if e.Links == nil {
		e.Links = &response.Links{
			EntityNameSelf: e.GetSelfLink(),
		}
	}
}

func (e *IdentityTypeApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameIdentityType,
		Name:   e.Name,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func MapIdentityTypeToApiEntity(_ *env.AppEnv, _ *response.RequestContext, e models.Entity) (BaseApiEntity, error) {
	i, ok := e.(*model.IdentityType)

	if !ok {
		err := fmt.Errorf("entity is not an identity type \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapIdentityTypeToApiList(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapIdentityTypeToApiList(i *model.IdentityType) (*IdentityTypeApiList, error) {
	ret := &IdentityTypeApiList{
		BaseApi: env.FromBaseModelEntity(i),
		Name:    &i.Name,
	}

	ret.PopulateLinks()

	return ret, nil
}
