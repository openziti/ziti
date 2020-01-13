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
	"time"

	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/response"
)

const EntityNameCurrentSession = "current-api-session"

type CurrentSessionApiList struct {
	*env.BaseApi
	Token       *string       `json:"token"`
	Identity    *EntityApiRef `json:"identity"`
	ExpiresAt   *time.Time    `json:"expiresAt"`
	ConfigTypes []string      `json:"configTypes"`
}

func (CurrentSessionApiList) BuildSelfLink(_ string) *response.Link {
	return response.NewLink("./" + EntityNameCurrentSession)
}

func (e *CurrentSessionApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (e *CurrentSessionApiList) PopulateLinks() {
	if e.Links == nil {
		e.Links = &response.Links{
			EntityNameSelf: e.GetSelfLink(),
		}
	}
}

func (e *CurrentSessionApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameCurrentSession,
		Name:   nil,
		Id:     e.Id,
		Links:  e.Links,
	}
}
