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

package response

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/model"
	"net/http"
)

type IdType int

const (
	IdTypeString IdType = iota
	IdTypeUuid

	IdPropertyName    = "id"
	SubIdPropertyName = "subId"
)

type RequestContext struct {
	Id                uuid.UUID
	Session           *model.ApiSession
	Identity          *model.Identity
	ActivePermissions []string
	ResponseWriter    http.ResponseWriter
	Request           *http.Request
	RequestResponder  RequestResponder
	EventLogger       EventLogger
}

type EventLogger interface {
	Log(actorType, actorId, eventType, entityType, entityId, formatString string, formatData []string, data map[interface{}]interface{})
}

func (rc *RequestContext) GetIdFromRequest(idType IdType) (string, error) {
	vars := mux.Vars(rc.Request)

	id, ok := vars[IdPropertyName]

	if !ok {
		return "", fmt.Errorf("id property '%s' not found in request", IdPropertyName)
	}

	if idType == IdTypeUuid {
		_, err := uuid.Parse(id)

		if err != nil {
			return "", apierror.NewInvalidUuid(id)
		}
	}

	return id, nil
}

func (rc *RequestContext) GetSubIdFromRequest() (string, error) {
	v := mux.Vars(rc.Request)
	id, ok := v[SubIdPropertyName]

	if !ok {
		return "", fmt.Errorf("subId not found")
	}

	return id, nil
}
