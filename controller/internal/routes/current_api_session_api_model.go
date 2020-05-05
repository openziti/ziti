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

package routes

import (
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/foundation/util/stringz"
	"time"
)

const EntityNameCurrentSession = "current-api-session"

var CurrentApiSessionLinkFactory LinksFactory = NewBasicLinkFactory(EntityNameCurrentSession)

func MapToCurrentApiSessionRestModel(s *model.ApiSession, sessionTimeout time.Duration) *rest_model.CurrentAPISessionDetail {
	expiresAt := strfmt.DateTime(s.UpdatedAt.Add(sessionTimeout))
	apiSession := &rest_model.CurrentAPISessionDetail{
		APISessionDetail: rest_model.APISessionDetail{
			BaseEntity:  BaseEntityToRestModel(s, CurrentApiSessionLinkFactory),
			Identity:    ToEntityRef(s.Identity.Name, s.Identity, IdentityLinkFactory),
			Token:       &s.Token,
			ConfigTypes: stringz.SetToSlice(s.ConfigTypes),
		},
		ExpiresAt: &expiresAt,
	}

	return apiSession
}
