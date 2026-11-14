/*
	Copyright NetFoundry Inc.

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
	"path"

	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/permissions"
	"github.com/openziti/ziti/v2/controller/response"
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

func (factory *ApiSessionLinkFactoryImpl) NewNestedLink(entity models.Entity, elem ...string) rest_model.Link {
	elem = append([]string{EntityNameApiSession, entity.GetId()}, elem...)
	return NewLink("./" + path.Join(elem...))
}

func (factory *ApiSessionLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	return rest_model.Links{
		EntityNameSelf:    factory.SelfLink(entity),
		EntityNameSession: factory.NewNestedLink(entity, EntityNameSession),
	}
}

// MapApiSessionToRestInterface renders an api session for the management API. The token is the bearer
// credential of the session, so it is withheld from callers that are not admins: anyone who can read
// it can act as the session's identity.
func MapApiSessionToRestInterface(ae *env.AppEnv, rc *response.RequestContext, apiSession *model.ApiSession) (interface{}, error) {
	ret, err := MapApiSessionToRestModel(ae, apiSession)
	if err != nil {
		return nil, err
	}

	if rc == nil || !rc.HasPermission(permissions.AdminPermission) {
		token := ""
		ret.Token = &token
	}

	return ret, nil
}

func MapApiSessionToRestModel(ae *env.AppEnv, apiSession *model.ApiSession) (*rest_model.APISessionDetail, error) {

	lastActivityAt := strfmt.DateTime(apiSession.LastActivityAt)

	ret := &rest_model.APISessionDetail{
		BaseEntity:              BaseEntityToRestModel(apiSession, ApiSessionLinkFactory),
		IdentityID:              &apiSession.IdentityId,
		Identity:                ToEntityRef(apiSession.Identity.Name, apiSession.Identity, IdentityLinkFactory),
		Token:                   &apiSession.Token,
		IPAddress:               &apiSession.IPAddress,
		ConfigTypes:             stringz.SetToSlice(apiSession.ConfigTypes),
		AuthQueries:             rest_model.AuthQueryList{}, //not in a request context, can't fill
		IsMfaComplete:           &apiSession.TotpComplete,
		IsMfaRequired:           &apiSession.TotpRequired,
		LastActivityAt:          lastActivityAt,
		AuthenticatorID:         &apiSession.AuthenticatorId,
		IsCertExtendable:        &apiSession.IsCertExtendable,
		IsCertExtendRequested:   apiSession.IsCertExtendRequested,
		IsCertKeyRollRequested:  apiSession.IsCertKeyRollRequested,
		ImproperClientCertChain: apiSession.ImproperClientCertChain,
	}

	if ret.ConfigTypes == nil {
		ret.ConfigTypes = []string{}
	}

	if val, ok := ae.GetManagers().ApiSession.HeartbeatCollector.LastAccessedAt(apiSession.Id); ok {
		cachedActivityAt := strfmt.DateTime(*val)
		ret.CachedLastActivityAt = cachedActivityAt
	} else {
		ret.CachedLastActivityAt = lastActivityAt
	}

	return ret, nil
}
