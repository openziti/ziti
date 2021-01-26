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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/stringz"
	"net/http"
)

const (
	EntityNameApiSession = "api-sessions"
)

var ApiSessionLinkFactory = NewBasicLinkFactory(EntityNameApiSession)

func MapApiSessionToRestInterface(_ *env.AppEnv, _ *response.RequestContext, apiSessionEntity models.Entity) (interface{}, error) {
	apiSession, ok := apiSessionEntity.(*model.ApiSession)

	if !ok {
		err := fmt.Errorf("entity is not an ApiSession \"%s\"", apiSessionEntity.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapApiSessionToRestModel(apiSession)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", apiSessionEntity.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapApiSessionToRestModel(apiSession *model.ApiSession) (*rest_model.APISessionDetail, error) {
	authQueries := rest_model.AuthQueryList{}

	if apiSession.MfaRequired && !apiSession.MfaComplete {
		authQueries = append(authQueries, newAuthCheckZitiMfa())
	}

	ret := &rest_model.APISessionDetail{
		BaseEntity:  BaseEntityToRestModel(apiSession, ApiSessionLinkFactory),
		IdentityID:  &apiSession.IdentityId,
		Identity:    ToEntityRef(apiSession.Identity.Name, apiSession.Identity, IdentityLinkFactory),
		Token:       &apiSession.Token,
		IPAddress:   &apiSession.IPAddress,
		ConfigTypes: stringz.SetToSlice(apiSession.ConfigTypes),
		AuthQueries: authQueries,
	}
	return ret, nil
}

func newAuthCheckZitiMfa() *rest_model.AuthQueryDetail {
	return &rest_model.AuthQueryDetail{
		TypeID:     "MFA",
		Format:     rest_model.MfaFormatsAlphaNumeric,
		HTTPMethod: http.MethodPost,
		HTTPURL:    "./authenticate/mfa",
		MaxLength:  model.TotpMaxLength,
		MinLength:  model.TotpMinLength,
		Provider:   rest_model.MfaProvidersZiti,
	}
}
