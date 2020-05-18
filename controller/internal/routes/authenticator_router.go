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
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/response"
)

func init() {
	r := NewAuthenticatorRouter()
	env.AddRouter(r)
}

type AuthenticatorRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewAuthenticatorRouter() *AuthenticatorRouter {
	return &AuthenticatorRouter{
		BasePath: "/" + EntityNameAuthenticator,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *AuthenticatorRouter) Register(ae *env.AppEnv) {
	registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())
}

func (ir *AuthenticatorRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Authenticator, MapAuthenticatorToApiEntity)
}

func (ir *AuthenticatorRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Authenticator, MapAuthenticatorToApiEntity, ir.IdType)
}

func (ir *AuthenticatorRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	mapEntity := &map[string]interface{}{}
	Create(rc, rc.RequestResponder, ae.Schemes.Authenticator.Post, mapEntity, (&AuthenticatorApiList{}).BuildSelfLink, func() (string, error) {
		apiEntity := &AuthenticatorCreateApi{}
		apiEntity.FillFromMap(*mapEntity)
		return ae.Handlers.Authenticator.Create(apiEntity.ToModel(""))
	})
}

func (ir *AuthenticatorRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.Authenticator)
}

func (ir *AuthenticatorRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	mapEntity := &map[string]interface{}{}
	Update(rc, ae.Schemes.Authenticator.Put, ir.IdType, mapEntity, func(id string) error {
		apiEntity := &AuthenticatorUpdateApi{}
		apiEntity.FillFromMap(*mapEntity)
		return ae.Handlers.Authenticator.Update(apiEntity.ToModel(id))
	})
}

func (ir *AuthenticatorRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	mapEntity := &map[string]interface{}{}
	Patch(rc, ae.Schemes.Authenticator.Patch, ir.IdType, mapEntity, func(id string, fields JsonFields) error {
		apiEntity := &AuthenticatorUpdateApi{}
		apiEntity.FillFromMap(*mapEntity)

		if fields.IsUpdated("password") {
			fields.AddField("salt")
		}

		if fields.IsUpdated("certPem") {
			fields.AddField("fingerprint")
		}

		return ae.Handlers.Authenticator.Patch(apiEntity.ToModel(id), fields.FilterMaps("tags"))
	})
}
