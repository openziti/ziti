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

const EntityNameEventLog = "event-logs"

type EventLogApiList struct {
	*env.BaseApi
	Type             string                 `json:"type"`
	ActorType        string                 `json:"actorType"`
	ActorId          string                 `json:"actorId"`
	EntityType       string                 `json:"entityType"`
	EntityId         string                 `json:"entityId"`
	FormattedMessage string                 `json:"formattedMessage"`
	FormatString     string                 `json:"formatString"`
	FormatData       string                 `json:"formatData"`
	Data             map[string]interface{} `json:"data"`
}

func (EventLogApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameEventLog, id))
}

func (e *EventLogApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (e *EventLogApiList) PopulateLinks() {
	if e.Links == nil {
		e.Links = &response.Links{
			EntityNameSelf: e.GetSelfLink(),
		}
	}
}

func (e *EventLogApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameEventLog,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func MapEventLogToApiEntity(_ *env.AppEnv, _ *response.RequestContext, e models.Entity) (BaseApiEntity, error) {
	i, ok := e.(*model.EventLog)

	if !ok {
		err := fmt.Errorf("entity is not an event log entry \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapEventLogToApiList(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapEventLogToApiList(i *model.EventLog) (*EventLogApiList, error) {
	ret := &EventLogApiList{
		BaseApi:          env.FromBaseModelEntity(i),
		Data:             i.Data,
		Type:             i.Type,
		FormattedMessage: i.FormattedMessage,
		FormatString:     i.FormatString,
		FormatData:       i.FormatData,
		EntityType:       i.EntityType,
		EntityId:         i.EntityId,
		ActorType:        i.ActorType,
		ActorId:          i.ActorId,
	}

	ret.PopulateLinks()

	return ret, nil
}
