/*
	Copyright 2020 Netfoundry, Inc.

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
	r := NewConfigTypeRouter()
	env.AddRouter(r)
}

type ConfigTypeRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewConfigTypeRouter() *ConfigTypeRouter {
	return &ConfigTypeRouter{
		BasePath: "/" + EntityNameConfigType,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *ConfigTypeRouter) Register(ae *env.AppEnv) {
	registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())
}

func (ir *ConfigTypeRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.ConfigType, MapConfigTypeToApiEntity)
}

func (ir *ConfigTypeRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.ConfigType, MapConfigTypeToApiEntity, ir.IdType)
}

func (ir *ConfigTypeRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ConfigTypeApi{}
	Create(rc, rc.RequestResponder, ae.Schemes.ConfigType.Post, apiEntity, (&ConfigTypeApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.ConfigType.Create(apiEntity.ToModel(""))
	})
}

func (ir *ConfigTypeRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.ConfigType)
}

func (ir *ConfigTypeRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ConfigTypeApi{}
	Update(rc, ae.Schemes.ConfigType.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.ConfigType.Update(apiEntity.ToModel(id))
	})
}

func (ir *ConfigTypeRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ConfigTypeApi{}
	Patch(rc, ae.Schemes.ConfigType.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.ConfigType.Patch(apiEntity.ToModel(id), fields.FilterMaps("tags", "data"))
	})
}
