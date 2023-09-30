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
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/foundation/v2/stringz"
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

func MapApiSessionToRestInterface(ae *env.AppEnv, _ *response.RequestContext, apiSession *model.ApiSession) (interface{}, error) {
	return MapApiSessionToRestModel(ae, apiSession)
}

func MapApiSessionToRestModel(ae *env.AppEnv, apiSession *model.ApiSession) (*rest_model.APISessionDetail, error) {

	lastActivityAt := strfmt.DateTime(apiSession.LastActivityAt)

	ret := &rest_model.APISessionDetail{
		BaseEntity:      BaseEntityToRestModel(apiSession, ApiSessionLinkFactory),
		IdentityID:      &apiSession.IdentityId,
		Identity:        ToEntityRef(apiSession.Identity.Name, apiSession.Identity, IdentityLinkFactory),
		Token:           &apiSession.Token,
		IPAddress:       &apiSession.IPAddress,
		ConfigTypes:     stringz.SetToSlice(apiSession.ConfigTypes),
		AuthQueries:     rest_model.AuthQueryList{}, //not in a request context, can't fill
		IsMfaComplete:   &apiSession.MfaComplete,
		IsMfaRequired:   &apiSession.MfaRequired,
		LastActivityAt:  lastActivityAt,
		AuthenticatorID: &apiSession.AuthenticatorId,
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
