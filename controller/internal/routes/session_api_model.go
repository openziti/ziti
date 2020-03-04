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
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-fabric/controller/models"
)

const EntityNameSession = "sessions"

type SessionApiPost struct {
	ServiceId *string                `json:"serviceId"`
	Type      *string                `json:"type"`
	Tags      map[string]interface{} `json:"tags"`
}

func (i *SessionApiPost) ToModel(rc *response.RequestContext) *model.Session {
	sessionType := "Dial"
	if i.Type != nil {
		sessionType = *i.Type
	}
	return &model.Session{
		BaseEntity: models.BaseEntity{
			Tags: i.Tags,
		},
		Token:        uuid.New().String(),
		ServiceId:    *i.ServiceId,
		ApiSessionId: rc.ApiSession.Id,
		Type:         sessionType,
	}
}

type NewSession struct {
	*SessionApiList
	Token string `json:"token"`
}

type SessionApiList struct {
	*env.BaseApi
	Type        string               `json:"type"`
	ApiSession  *EntityApiRef        `json:"apiSession"`
	Service     *EntityApiRef        `json:"service"`
	EdgeRouters []*SessionEdgeRouter `json:"edgeRouters"`
}

func (SessionApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameSession, id))
}

func (e *SessionApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (e *SessionApiList) PopulateLinks() {
	if e.Links == nil {
		e.Links = &response.Links{
			EntityNameSelf: e.GetSelfLink(),
		}
	}
}

func (e *SessionApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameSession,
		Name:   nil,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func MapSessionsToApiEntities(ae *env.AppEnv, rc *response.RequestContext, es []*model.Session) ([]BaseApiEntity, error) {
	// can't use modelToApi b/c it require list of network.Entity
	apiEntities := make([]BaseApiEntity, 0)

	for _, e := range es {
		al, err := MapSessionToApiEntity(ae, rc, e)

		if err != nil {
			return nil, err
		}

		apiEntities = append(apiEntities, al)
	}

	return apiEntities, nil
}

func MapSessionToApiEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (BaseApiEntity, error) {
	i, ok := e.(*model.Session)

	if !ok {
		err := fmt.Errorf("entity is not a session \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapSessionToApiList(ae, i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapSessionToApiList(ae *env.AppEnv, i *model.Session) (*SessionApiList, error) {
	service, err := ae.Handlers.EdgeService.Read(i.ServiceId)
	if err != nil {
		return nil, err
	}

	edgeRouters, err := getSessionEdgeRouters(ae, i)
	if err != nil {
		return nil, err
	}

	apiSession, err := ae.Handlers.ApiSession.Read(i.ApiSessionId)
	if err != nil {
		return nil, err
	}

	ret := &SessionApiList{
		BaseApi:     env.FromBaseModelEntity(i),
		Type:        i.Type,
		Service:     NewServiceEntityRef(service),
		ApiSession:  NewApiSessionEntityRef(apiSession),
		EdgeRouters: edgeRouters,
	}

	ret.PopulateLinks()

	return ret, nil
}
