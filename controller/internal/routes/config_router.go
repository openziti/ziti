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
	r := NewConfigRouter()
	env.AddRouter(r)
}

type ConfigRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewConfigRouter() *ConfigRouter {
	return &ConfigRouter{
		BasePath: "/" + EntityNameConfig,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *ConfigRouter) Register(ae *env.AppEnv) {
	registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())
}

func (ir *ConfigRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Config, MapConfigToApiEntity)
}

func (ir *ConfigRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Config, MapConfigToApiEntity, ir.IdType)
}

func (ir *ConfigRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ConfigCreateApi{}
	Create(rc, rc.RequestResponder, ae.Schemes.Config.Post, apiEntity, (&ConfigApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.Config.Create(apiEntity.ToModel(""))
	})
}

func (ir *ConfigRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.Config)
}

func (ir *ConfigRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ConfigUpdateApi{}
	Update(rc, ae.Schemes.Config.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.Config.Update(apiEntity.ToModel(id))
	})
}

func (ir *ConfigRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ConfigUpdateApi{}
	Patch(rc, ae.Schemes.Config.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.Config.Patch(apiEntity.ToModel(id), fields.FilterMaps("tags", "data"))
	})
}
