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
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
)

func init() {
	r := NewCurrentIdentityAuthenticatorRouter()
	env.AddRouter(r)
}

type CurrentIdentityAuthenticatorRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewCurrentIdentityAuthenticatorRouter() *CurrentIdentityAuthenticatorRouter {
	return &CurrentIdentityAuthenticatorRouter{
		BasePath: "/" + EntityNameAuthenticator,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *CurrentIdentityAuthenticatorRouter) Register(ae *env.AppEnv) {
	registerReadUpdateRouter(ae, ae.CurrentIdentityRouter, ir.BasePath, ir, permissions.IsAuthenticated())
}

func (ir *CurrentIdentityAuthenticatorRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	List(rc, func(rc *response.RequestContext, queryOptions *QueryOptions) (*QueryResult, error) {
		query, err := queryOptions.getFullQuery(ae.Handlers.Authenticator.GetStore())
		if err != nil {
			return nil, err
		}

		result, err := ae.Handlers.Authenticator.ListForIdentity(rc.Identity.Id, query)
		if err != nil {
			pfxlog.Logger().Errorf("error executing list query: %+v", err)
			return nil, err
		}

		apiAuthenticators, err := MapAuthenticatorsToApiEntities(ae, rc, result.Authenticators)
		if err != nil {
			return nil, err
		}
		return NewQueryResult(apiAuthenticators, result.GetMetaData()), nil
	})
}

func (ir *CurrentIdentityAuthenticatorRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	Detail(rc, ir.IdType, func(rc *response.RequestContext, id string) (entity interface{}, err error) {
		authenticator, err := ae.GetHandlers().Authenticator.ReadForIdentity(rc.Identity.Id, id)
		if err != nil {
			return nil, err
		}

		if authenticator == nil {
			return nil, boltz.NewNotFoundError(ae.GetHandlers().Authenticator.GetStore().GetSingularEntityType(), "id", id)
		}

		apiAuthenticator, err := MapAuthenticatorToApiList(authenticator)

		if err != nil {
			return nil, err
		}

		return apiAuthenticator, nil
	})
}

func (ir *CurrentIdentityAuthenticatorRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &AuthenticatorSelfUpdateApi{}
	Update(rc, ae.Schemes.AuthenticatorSelf.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.Authenticator.UpdateSelf(apiEntity.ToModel(id, rc.Identity.Id))
	})
}

func (ir *CurrentIdentityAuthenticatorRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &AuthenticatorSelfUpdateApi{}
	Patch(rc, ae.Schemes.AuthenticatorSelf.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.Authenticator.PatchSelf(apiEntity.ToModel(id, rc.Identity.Id), fields.FilterMaps("tags"))
	})
}
