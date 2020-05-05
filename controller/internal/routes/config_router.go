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
	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_server/operations/config"
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

func (r *ConfigRouter) Register(ae *env.AppEnv) {
	ae.Api.ConfigDeleteConfigHandler = config.DeleteConfigHandlerFunc(func(params config.DeleteConfigParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ConfigDetailConfigHandler = config.DetailConfigHandlerFunc(func(params config.DetailConfigParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ConfigListConfigsHandler = config.ListConfigsHandlerFunc(func(params config.ListConfigsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.Api.ConfigUpdateConfigHandler = config.UpdateConfigHandlerFunc(func(params config.UpdateConfigParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Update(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.Api.ConfigCreateConfigHandler = config.CreateConfigHandlerFunc(func(params config.CreateConfigParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Create(ae, rc, params) }, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.Api.ConfigPatchConfigHandler = config.PatchConfigHandlerFunc(func(params config.PatchConfigParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *ConfigRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Config, MapConfigToRestEntity)
}

func (r *ConfigRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Config, MapConfigToRestEntity)
}

func (r *ConfigRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params config.CreateConfigParams) {
	if params.Body.Data == nil {
		ctx := middleware.MatchedRouteFrom(rc.Request)
		ae.Api.ServeErrorFor(ctx.Operation.ID)(rc.ResponseWriter, rc.Request, errors.Required("data", "body"))
		return
	}

	Create(rc, rc, ConfigLinkFactory, func() (string, error) {
		return ae.Handlers.Config.Create(MapCreateConfigToModel(params.Body))
	})
}

func (r *ConfigRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Handlers.Config)
}

func (r *ConfigRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params config.UpdateConfigParams) {
	if params.Body.Data == nil {
		ctx := middleware.MatchedRouteFrom(rc.Request)
		ae.Api.ServeErrorFor(ctx.Operation.ID)(rc.ResponseWriter, rc.Request, errors.Required("data", "body"))
		return
	}

	Update(rc, func(id string) error {
		return ae.Handlers.Config.Update(MapUpdateConfigToModel(params.ID, params.Body))
	})
}

func (r *ConfigRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params config.PatchConfigParams) {
	Patch(rc, func(id string, fields JsonFields) error {
		return ae.Handlers.Config.Patch(MapPatchConfigToModel(params.ID, params.Body), fields.FilterMaps("tags", "data"))
	})
}
