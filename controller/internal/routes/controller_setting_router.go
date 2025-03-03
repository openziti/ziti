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
	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/edge-api/rest_management_api_server/operations/settings"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/fields"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/response"
)

func init() {
	r := NewControllerSettingRouter()
	env.AddRouter(r)
}

type ControllerSettingRouter struct {
	BasePath    string
	createTimer metrics.Timer
}

func NewControllerSettingRouter() *ControllerSettingRouter {
	return &ControllerSettingRouter{
		BasePath: "/" + EntityNameControllerSetting,
	}
}

func (r *ControllerSettingRouter) Register(ae *env.AppEnv) {

	//Management
	ae.ManagementApi.SettingsListControllerSettingsHandler = settings.ListControllerSettingsHandlerFunc(func(params settings.ListControllerSettingsParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.List, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.SettingsDetailControllerSettingHandler = settings.DetailControllerSettingHandlerFunc(func(params settings.DetailControllerSettingParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})

	ae.ManagementApi.SettingsCreateControllerSettingHandler = settings.CreateControllerSettingHandlerFunc(func(params settings.CreateControllerSettingParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.Create(ae, rc, params)
		}, params.HTTPRequest, "", "", permissions.IsAdmin())
	})

	ae.ManagementApi.SettingsUpdateControllerSettingHandler = settings.UpdateControllerSettingHandlerFunc(func(params settings.UpdateControllerSettingParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.Update(ae, rc, params)
		}, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
	ae.ManagementApi.SettingsPatchControllerSettingHandler = settings.PatchControllerSettingHandlerFunc(func(params settings.PatchControllerSettingParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) {
			r.Patch(ae, rc, params)
		}, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
	ae.ManagementApi.SettingsDeleteControllerSettingHandler = settings.DeleteControllerSettingHandlerFunc(func(params settings.DeleteControllerSettingParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.IsAdmin())
	})
}

func (r *ControllerSettingRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.ControllerSetting](ae, rc, ae.Managers.ControllerSetting, MapControllerSettingDetailRest)
}

func (r *ControllerSettingRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler[*model.ControllerSetting](ae, rc, ae.Managers.ControllerSetting, MapControllerSettingDetailRest)
}

func (r *ControllerSettingRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ae.Managers.ControllerSetting)
}

func (r *ControllerSettingRouter) DetailEffective(ae *env.AppEnv, rc *response.RequestContext) {
	Detail(rc, func(rc *response.RequestContext, id string) (interface{}, error) {
		effective, err := ae.Managers.ControllerSetting.ReadEffective(id)
		if err != nil {
			return nil, err
		}
		return MapControllerSettingEffectiveToRest(ae, rc, effective)
	})
}

func (r *ControllerSettingRouter) Create(ae *env.AppEnv, rc *response.RequestContext, params settings.CreateControllerSettingParams) {
	if params.ControllerSetting == nil {
		ae.ManagementApi.ServeErrorFor("")(rc.ResponseWriter, rc.Request, errors.Required("data", "body", nil))
		return
	}

	Create(rc, rc, ConfigLinkFactory, func() (string, error) {
		entity, err := MapCreateControllerSettingToModel(params.ControllerSetting)
		if err != nil {
			return "", err
		}
		return MapCreate(ae.Managers.ControllerSetting.Create, entity, rc)
	})
}

func (r *ControllerSettingRouter) Update(ae *env.AppEnv, rc *response.RequestContext, params settings.UpdateControllerSettingParams) {
	Update(rc, func(id string) error {
		setting, err := MapUpdateControllerSettingToModel(params.ID, params.ControllerSetting)

		if err != nil {
			return err
		}

		return ae.Managers.ControllerSetting.Update(setting, nil, rc.NewChangeContext())
	})
}

func (r *ControllerSettingRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params settings.PatchControllerSettingParams) {
	Patch(rc, func(id string, fields fields.UpdatedFields) error {
		setting, err := MapPatchControllerSettingToModel(params.ID, params.ControllerSetting)

		if err != nil {
			return err
		}

		return ae.Managers.ControllerSetting.Update(setting, fields.FilterMaps("tags"), rc.NewChangeContext())
	})
}
