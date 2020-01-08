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
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
)

const (
	EntityNameApiSession = "api-sessions"
)

func NewApiSessionEntityRef(s *model.ApiSession) *EntityApiRef {
	links := &response.Links{
		"self": NewApiSessionLink(s.Id),
	}

	return &EntityApiRef{
		Entity: EntityNameApiSession,
		Id:     s.Id,
		Name:   nil,
		Links:  links,
	}
}

func NewApiSessionLink(sessionId string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameApiSession, sessionId))
}

type ApiSessionApiList struct {
	*env.BaseApi
	Token    *string       `json:"token"`
	Identity *EntityApiRef `json:"identity"`
}

func (ApiSessionApiList) BuildSelfLink(id string) *response.Link {
	return NewApiSessionLink(id)
}

func (e *ApiSessionApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (e *ApiSessionApiList) PopulateLinks() {
	if e.Links == nil {
		e.Links = &response.Links{
			EntityNameSelf: e.GetSelfLink(),
		}
	}
}

func (e *ApiSessionApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameApiSession,
		Name:   nil,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func MapApiSessionToApiEntity(_ *env.AppEnv, _ *response.RequestContext, e model.BaseModelEntity) (BaseApiEntity, error) {
	i, ok := e.(*model.ApiSession)

	if !ok {
		err := fmt.Errorf("entity is not an ApiSession \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapApiSessionToApiList(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapApiSessionToApiList(i *model.ApiSession) (*ApiSessionApiList, error) {
	ret := &ApiSessionApiList{
		BaseApi:  env.FromBaseModelEntity(i),
		Token:    &i.Token,
		Identity: NewIdentityEntityRef(i.Identity),
	}

	ret.PopulateLinks()

	return ret, nil
}
