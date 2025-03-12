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
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/response"
)

const EntityNameControllerSetting = "controller-settings"

var ControllerSettingLinkFactory = NewBasicLinkFactory(EntityNameControllerSetting)

func mapControllerSettingToRest(setting *model.ControllerSetting) *rest_model.ControllerSettings {
	if setting == nil {
		return nil
	}

	result := &rest_model.ControllerSettings{
		Oidc: &rest_model.ControllerSettingsOidc{
			RedirectUris:   setting.Oidc.RedirectUris,
			PostLogoutUris: setting.Oidc.PostLogoutUris,
		},
	}

	return result
}

func MapControllerSettingDetailRest(_ *env.AppEnv, _ *response.RequestContext, settings *model.ControllerSetting) (any, error) {
	ret := &rest_model.ControllerSettingDetail{
		BaseEntity:         BaseEntityToRestModel(settings, ControllerSettingLinkFactory),
		ControllerSettings: *mapControllerSettingToRest(settings),
	}

	return ret, nil
}

func MapControllerSettingEffectiveToRest(_ *env.AppEnv, _ *response.RequestContext, settings *model.ControllerSettingEffective) (*rest_model.ControllerSettingEffective, error) {
	ret := &rest_model.ControllerSettingEffective{
		BaseEntity: BaseEntityToRestModel(settings, ControllerSettingLinkFactory),
		Effective:  mapControllerSettingToRest(settings.Effective),
		Instance:   mapControllerSettingToRest(settings.Instance),
	}
	return ret, nil
}

func MapCreateControllerSettingToModel(setting *rest_model.ControllerSettingCreate) (*model.ControllerSetting, error) {
	result := &model.ControllerSetting{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(setting.Tags),
			Id:   stringz.OrEmpty(setting.ControllerID),
		},
		Oidc: nil,
	}

	if setting.Oidc != nil {
		result.Oidc = &model.OidcSettings{
			OidcSettingDef: &db.OidcSettingDef{
				RedirectUris:   setting.Oidc.RedirectUris,
				PostLogoutUris: setting.Oidc.PostLogoutUris,
			},
		}
	}

	return result, nil
}

func MapUpdateControllerSettingToModel(id string, setting *rest_model.ControllerSettingUpdate) (*model.ControllerSetting, error) {
	result := &model.ControllerSetting{
		BaseEntity: models.BaseEntity{
			Id: id,
		},
	}
	var err error

	result.Oidc, err = MapOidcSettingToModel(setting.Oidc)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func MapPatchControllerSettingToModel(id string, setting *rest_model.ControllerSettingPatch) (*model.ControllerSetting, error) {
	result := &model.ControllerSetting{
		BaseEntity: models.BaseEntity{
			Id: id,
		},
	}
	var err error

	result.Oidc, err = MapOidcSettingToModel(setting.Oidc)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func MapOidcSettingToModel(oidc *rest_model.ControllerSettingsOidc) (*model.OidcSettings, error) {
	var result *model.OidcSettings
	if oidc != nil {
		result = &model.OidcSettings{
			OidcSettingDef: &db.OidcSettingDef{
				RedirectUris:   oidc.RedirectUris,
				PostLogoutUris: oidc.PostLogoutUris,
			},
		}
	}

	return result, nil
}
