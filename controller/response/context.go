/*
	Copyright NetFoundry, Inc.

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

package response

import (
	"errors"
	"github.com/openziti/edge/controller/model"
	"net/http"
	"time"
)

const (
	IdPropertyName    = "id"
	SubIdPropertyName = "subId"
)

type RequestContext struct {
	Responder
	Id                string
	ApiSession        *model.ApiSession
	Identity          *model.Identity
	ActivePermissions []string
	ResponseWriter    http.ResponseWriter
	Request           *http.Request
	SessionToken      string
	entityId          string
	entitySubId       string
	Body              []byte
	StartTime         time.Time
}

func (rc *RequestContext) GetId() string {
	return rc.Id
}

func (rc *RequestContext) GetBody() []byte {
	return rc.Body
}

func (rc *RequestContext) GetRequest() *http.Request {
	return rc.Request
}

func (rc *RequestContext) GetResponseWriter() http.ResponseWriter {
	return rc.ResponseWriter
}

type EventLogger interface {
	Log(actorType, actorId, eventType, entityType, entityId, formatString string, formatData []string, data map[interface{}]interface{})
}

func (rc *RequestContext) SetEntityId(id string) {
	rc.entityId = id
}

func (rc *RequestContext) SetEntitySubId(id string) {
	rc.entitySubId = id
}

func (rc *RequestContext) GetEntityId() (string, error) {
	if rc.entityId == "" {
		return "", errors.New("id not found")
	}
	return rc.entityId, nil
}

func (rc *RequestContext) GetEntitySubId() (string, error) {
	if rc.entitySubId == "" {
		return "", errors.New("subId not found")
	}

	return rc.entitySubId, nil
}
