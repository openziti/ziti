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
	"github.com/go-openapi/strfmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/stringz"
	"net/http"
	"path"
)

const (
	EntityNameApiSession = "api-sessions"
)

var ApiSessionLinkFactory LinksFactory = NewApiSessionLinkFactory()

type ApiSessionLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewApiSessionLinkFactory() *ApiSessionLinkFactoryImpl {
	return &ApiSessionLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameApiSession),
	}
}

func (factory ApiSessionLinkFactoryImpl) NewNestedLink(entity models.Entity, elem ...string) rest_model.Link {
	elem = append([]string{EntityNameApiSession, entity.GetId()}, elem...)
	return NewLink("./" + path.Join(elem...))
}

func (factory *ApiSessionLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	return rest_model.Links{
		EntityNameSelf:    factory.SelfLink(entity),
		EntityNameSession: factory.NewNestedLink(entity, EntityNameSession),
	}
}

func MapApiSessionToRestInterface(ae *env.AppEnv, _ *response.RequestContext, apiSessionEntity models.Entity) (interface{}, error) {
	apiSession, ok := apiSessionEntity.(*model.ApiSession)

	if !ok {
		err := fmt.Errorf("entity is not an ApiSession \"%s\"", apiSessionEntity.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapApiSessionToRestModel(ae, apiSession)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", apiSessionEntity.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapApiSessionToRestModel(ae *env.AppEnv, apiSession *model.ApiSession) (*rest_model.APISessionDetail, error) {
	authQueries := rest_model.AuthQueryList{}

	if apiSession.MfaRequired && !apiSession.MfaComplete {
		authQueries = append(authQueries, newAuthCheckZitiMfa())
	}

	lastActivityAt := strfmt.DateTime(apiSession.LastActivityAt)

	ret := &rest_model.APISessionDetail{
		BaseEntity:     BaseEntityToRestModel(apiSession, ApiSessionLinkFactory),
		IdentityID:     &apiSession.IdentityId,
		Identity:       ToEntityRef(apiSession.Identity.Name, apiSession.Identity, IdentityLinkFactory),
		Token:          &apiSession.Token,
		IPAddress:      &apiSession.IPAddress,
		ConfigTypes:    stringz.SetToSlice(apiSession.ConfigTypes),
		AuthQueries:    authQueries,
		IsMfaComplete:  &apiSession.MfaComplete,
		IsMfaRequired:  &apiSession.MfaRequired,
		LastActivityAt: lastActivityAt,
	}

	if val, ok := ae.GetHandlers().ApiSession.HeartbeatCollector.LastAccessedAt(apiSession.Id); ok {
		cachedActivityAt := strfmt.DateTime(*val)
		ret.CachedLastActivityAt = cachedActivityAt
	} else {
		ret.CachedLastActivityAt = lastActivityAt
	}

	return ret, nil
}

func newAuthCheckZitiMfa() *rest_model.AuthQueryDetail {
	provider := rest_model.MfaProvidersZiti
	return &rest_model.AuthQueryDetail{
		TypeID:     "MFA",
		Format:     rest_model.MfaFormatsAlphaNumeric,
		HTTPMethod: http.MethodPost,
		HTTPURL:    "./authenticate/mfa",
		MaxLength:  model.TotpMaxLength,
		MinLength:  model.TotpMinLength,
		Provider:   &provider,
	}
}
