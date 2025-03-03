//go:build apitests
// +build apitests

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

package tests

import (
	"github.com/openziti/edge-api/rest_management_api_client/settings"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/foundation/v2/stringz"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/controller/db"
	"sort"
	"testing"
)

func Test_ControllerSettings(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	managementClient := ctx.NewEdgeManagementApi(func(strings chan string) {
		strings <- ""
	})

	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	apiSession, err := managementClient.Authenticate(adminCreds, nil)

	ctx.Req.NoError(err)
	ctx.Req.NotNil(apiSession)

	defaultRedirects := []string{
		"openziti://auth/callback",
		"https://127.0.0.1:*/auth/callback",
		"http://127.0.0.1:*/auth/callback",
		"https://localhost:*/auth/callback",
		"http://localhost:*/auth/callback",
		"http://[::1]:*/auth/callback",
		"https://[::1]:*/auth/callback",
	}

	defaultPostLogouts := []string{
		"openziti://auth/logout",
		"https://127.0.0.1:*/auth/logout",
		"http://127.0.0.1:*/auth/logout",
		"https://localhost:*/auth/logout",
		"http://localhost:*/auth/logout",
		"http://[::1]:*/auth/logout",
		"https://[::1]:*/auth/logout",
	}

	sort.Strings(defaultPostLogouts)
	sort.Strings(defaultRedirects)

	t.Run("can retrieve the default global settings", func(t *testing.T) {
		ctx.testContextChanged(t)

		params := settings.NewDetailControllerSettingParams()
		params.ID = "global"

		resp, err := managementClient.API.Settings.DetailControllerSetting(params, nil)
		ctx.Req.NoError(rest_util.WrapErr(err))

		t.Run("has default redirects", func(t *testing.T) {
			ctx.testContextChanged(t)
			sort.Strings(resp.Payload.Data.Oidc.RedirectUris)
			ctx.Req.True(stringz.EqualSlices(resp.Payload.Data.Oidc.RedirectUris, defaultRedirects), "default global redirects did not match, expected: %s, got: %s", defaultRedirects, resp.Payload.Data.Oidc.RedirectUris)
		})

		t.Run("has default post logout redirects", func(t *testing.T) {
			ctx.testContextChanged(t)
			sort.Strings(resp.Payload.Data.Oidc.PostLogoutUris)
			ctx.Req.True(stringz.EqualSlices(resp.Payload.Data.Oidc.PostLogoutUris, defaultPostLogouts), "default global post logouts did not match, expected: %s, got: %s", defaultRedirects, resp.Payload.Data.Oidc.PostLogoutUris)
		})
	})

	t.Run("cannot delete the global settings", func(t *testing.T) {
		ctx.testContextChanged(t)

		params := settings.NewDeleteControllerSettingParams()
		params.ID = "global"

		resp, err := managementClient.API.Settings.DeleteControllerSetting(params, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(resp)
	})

	t.Run("global settings can be updated", func(t *testing.T) {
		ctx.testContextChanged(t)

		type event struct {
			Setting      string
			ControllerId string
			Event        *db.ControllerSettingsEvent
		}
		oidcAnyEvent := event{}
		ctx.EdgeController.AppEnv.Stores.ControllerSetting.Watch(db.FieldControllerSettingOidc, func(setting string, controllerId string, settingEvent *db.ControllerSettingsEvent) {
			oidcAnyEvent.Setting = setting
			oidcAnyEvent.ControllerId = controllerId
			oidcAnyEvent.Event = settingEvent
		}, db.ControllerSettingAny)

		oidcAnyRedirectEvent := event{}
		ctx.EdgeController.AppEnv.Stores.ControllerSetting.Watch(db.FieldControllerSettingOidcRedirectUris, func(setting string, controllerId string, settingEvent *db.ControllerSettingsEvent) {
			oidcAnyRedirectEvent.Setting = setting
			oidcAnyRedirectEvent.ControllerId = controllerId
			oidcAnyRedirectEvent.Event = settingEvent
		}, db.ControllerSettingAny)

		oidcAnyPostLogoutEvent := event{}
		ctx.EdgeController.AppEnv.Stores.ControllerSetting.Watch(db.FieldControllerSettingOidcPostLogoutUris, func(setting string, controllerId string, settingEvent *db.ControllerSettingsEvent) {
			oidcAnyPostLogoutEvent.Setting = setting
			oidcAnyPostLogoutEvent.ControllerId = controllerId
			oidcAnyPostLogoutEvent.Event = settingEvent
		}, db.ControllerSettingAny)

		oidcGlobalEvent := event{}
		ctx.EdgeController.AppEnv.Stores.ControllerSetting.Watch(db.FieldControllerSettingOidc, func(setting string, controllerId string, settingEvent *db.ControllerSettingsEvent) {
			oidcGlobalEvent.Setting = setting
			oidcGlobalEvent.ControllerId = controllerId
			oidcGlobalEvent.Event = settingEvent
		}, db.ControllerSettingGlobalId)

		oidcGlobalRedirectEvent := event{}
		ctx.EdgeController.AppEnv.Stores.ControllerSetting.Watch(db.FieldControllerSettingOidcRedirectUris, func(setting string, controllerId string, settingEvent *db.ControllerSettingsEvent) {
			oidcGlobalRedirectEvent.Setting = setting
			oidcGlobalRedirectEvent.ControllerId = controllerId
			oidcGlobalRedirectEvent.Event = settingEvent
		}, db.ControllerSettingGlobalId)

		oidcGlobalPostLogoutEvent := event{}
		ctx.EdgeController.AppEnv.Stores.ControllerSetting.Watch(db.FieldControllerSettingOidcPostLogoutUris, func(setting string, controllerId string, settingEvent *db.ControllerSettingsEvent) {
			oidcGlobalPostLogoutEvent.Setting = setting
			oidcGlobalPostLogoutEvent.ControllerId = controllerId
			oidcGlobalPostLogoutEvent.Event = settingEvent
		}, db.ControllerSettingGlobalId)

		params := settings.NewPatchControllerSettingParams()
		params.ID = "global"
		params.ControllerSetting = &rest_model.ControllerSettingPatch{
			ControllerSettings: rest_model.ControllerSettings{
				Oidc: &rest_model.ControllerSettingsOidc{
					PostLogoutUris: []string{
						"http://127.0.0.1:*/auth/logout",
					},
					RedirectUris: []string{
						"http://127.0.0.1:*/auth/callback",
					},
				},
			},
		}

		resp, err := managementClient.API.Settings.PatchControllerSetting(params, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(resp)

		t.Run("patching global setting triggers any oidc event", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.NotEmpty(oidcAnyEvent.Setting)
			ctx.Req.NotEmpty(oidcAnyEvent.ControllerId)
			ctx.Req.NotNil(oidcAnyEvent.Event)
			ctx.Req.NotNil(oidcAnyEvent.Event.Global)
			ctx.Req.NotNil(oidcAnyEvent.Event.Effective)
		})

		t.Run("patching global setting triggers any oidc redirect event", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.NotEmpty(oidcAnyRedirectEvent.Setting)
			ctx.Req.NotEmpty(oidcAnyRedirectEvent.ControllerId)
			ctx.Req.NotNil(oidcAnyRedirectEvent.Event)
			ctx.Req.NotNil(oidcAnyRedirectEvent.Event.Global)
			ctx.Req.NotNil(oidcAnyRedirectEvent.Event.Effective)
		})

		t.Run("patching global setting triggers any oidc post logout event", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.NotEmpty(oidcAnyPostLogoutEvent.Setting)
			ctx.Req.NotEmpty(oidcAnyPostLogoutEvent.ControllerId)
			ctx.Req.NotNil(oidcAnyPostLogoutEvent.Event)
			ctx.Req.NotNil(oidcAnyPostLogoutEvent.Event.Global)
			ctx.Req.NotNil(oidcAnyPostLogoutEvent.Event.Effective)
		})

		t.Run("patching global setting triggers global oidc event", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.NotEmpty(oidcGlobalEvent.Setting)
			ctx.Req.NotEmpty(oidcGlobalEvent.ControllerId)
			ctx.Req.NotNil(oidcGlobalEvent.Event)
			ctx.Req.NotNil(oidcGlobalEvent.Event.Global)
			ctx.Req.NotNil(oidcGlobalEvent.Event.Effective)
		})

		t.Run("patching global setting triggers global oidc redirect event", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.NotEmpty(oidcGlobalRedirectEvent.Setting)
			ctx.Req.NotEmpty(oidcGlobalRedirectEvent.ControllerId)
			ctx.Req.NotNil(oidcGlobalRedirectEvent.Event)
			ctx.Req.NotNil(oidcGlobalRedirectEvent.Event.Global)
			ctx.Req.NotNil(oidcGlobalRedirectEvent.Event.Effective)
		})

		t.Run("patching global setting triggers global oidc post logout event", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.NotEmpty(oidcGlobalPostLogoutEvent.Setting)
			ctx.Req.NotEmpty(oidcGlobalPostLogoutEvent.ControllerId)
			ctx.Req.NotNil(oidcGlobalPostLogoutEvent.Event)
			ctx.Req.NotNil(oidcGlobalPostLogoutEvent.Event.Global)
			ctx.Req.NotNil(oidcGlobalPostLogoutEvent.Event.Effective)
		})
	})
}
