//go:build apitests

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
	"testing"

	"github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_management_api_client/router"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/common/eid"
)

func Test_FabricRouters(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	_, err := mgmtClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)

	t.Run("ctrlChanListeners can be created with an empty list", func(t *testing.T) {
		ctx.testContextChanged(t)

		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name: util.Ptr(eid.New()),
			},
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(createResp)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: createResp.Payload.Data.ID,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(detailResp)
		ctx.Req.Empty(detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("ctrlChanListeners can be set on create and retrieved", func(t *testing.T) {
		ctx.testContextChanged(t)

		listeners := map[string][]string{
			"tls:1.2.3.4:6262": {"group1"},
			"tls:5.6.7.8:6262": {},
		}

		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name:              util.Ptr(eid.New()),
				CtrlChanListeners: listeners,
			},
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(createResp)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: createResp.Payload.Data.ID,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(detailResp)
		ctx.Req.Equal(listeners, detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("ctrlChanListeners can be updated via patch", func(t *testing.T) {
		ctx.testContextChanged(t)

		name := eid.New()
		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name:              &name,
				CtrlChanListeners: map[string][]string{"tls:1.2.3.4:6262": {}},
			},
		}, nil)
		ctx.Req.NoError(err)
		id := createResp.Payload.Data.ID

		// update to more addresses
		_, err = mgmtClient.API.Router.PatchRouter(&router.PatchRouterParams{
			ID: id,
			Router: &rest_model.RouterPatch{
				CtrlChanListeners: map[string][]string{
					"tls:10.0.0.1:6262": {"group1"},
					"tls:10.0.0.2:6262": {},
					"tls:10.0.0.3:6262": {"group2", "group3"},
				},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(map[string][]string{
			"tls:10.0.0.1:6262": {"group1"},
			"tls:10.0.0.2:6262": {},
			"tls:10.0.0.3:6262": {"group2", "group3"},
		}, detailResp.Payload.Data.CtrlChanListeners)

		// update to fewer addresses
		_, err = mgmtClient.API.Router.PatchRouter(&router.PatchRouterParams{
			ID: id,
			Router: &rest_model.RouterPatch{
				CtrlChanListeners: map[string][]string{"tls:10.0.0.1:6262": {}},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err = mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(map[string][]string{"tls:10.0.0.1:6262": {}}, detailResp.Payload.Data.CtrlChanListeners)

		// clear all addresses via PUT
		_, err = mgmtClient.API.Router.UpdateRouter(&router.UpdateRouterParams{
			ID: id,
			Router: &rest_model.RouterUpdate{
				Name: &name,
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err = mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Empty(detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("ctrlChanListeners can be set and cleared via patch", func(t *testing.T) {
		ctx.testContextChanged(t)

		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name: util.Ptr(eid.New()),
			},
		}, nil)
		ctx.Req.NoError(err)
		id := createResp.Payload.Data.ID

		// set listeners via patch
		_, err = mgmtClient.API.Router.PatchRouter(&router.PatchRouterParams{
			ID: id,
			Router: &rest_model.RouterPatch{
				CtrlChanListeners: map[string][]string{"tls:1.2.3.4:6262": {"west"}, "tls:5.6.7.8:6262": {}},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(map[string][]string{"tls:1.2.3.4:6262": {"west"}, "tls:5.6.7.8:6262": {}}, detailResp.Payload.Data.CtrlChanListeners)

		// clear listeners via patch with empty map
		_, err = mgmtClient.API.Router.PatchRouter(&router.PatchRouterParams{
			ID: id,
			Router: &rest_model.RouterPatch{
				CtrlChanListeners: map[string][]string{},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err = mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Empty(detailResp.Payload.Data.CtrlChanListeners)
	})
}

func Test_FabricRouterConfigs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	_, err := mgmtClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)

	// Helper: create a config type with the given target and return its ID.
	createConfigType := func(t *testing.T, target *string) string {
		t.Helper()
		resp, err := mgmtClient.API.Config.CreateConfigType(&config.CreateConfigTypeParams{
			ConfigType: &rest_model.ConfigTypeCreate{
				Name:   util.Ptr(eid.New()),
				Target: target,
			},
		}, nil)
		ctx.Req.NoError(err)
		return resp.Payload.Data.ID
	}

	// Helper: create a config for the given config type and return its ID.
	createConfig := func(t *testing.T, configTypeId string) string {
		t.Helper()
		resp, err := mgmtClient.API.Config.CreateConfig(&config.CreateConfigParams{
			Config: &rest_model.ConfigCreate{
				Name:         util.Ptr(eid.New()),
				ConfigTypeID: &configTypeId,
				Data:         map[string]interface{}{},
			},
		}, nil)
		ctx.Req.NoError(err)
		return resp.Payload.Data.ID
	}

	t.Run("create with router-target config should succeed", func(t *testing.T) {
		ctx.testContextChanged(t)
		configTypeId := createConfigType(t, util.Ptr("router"))
		configId := createConfig(t, configTypeId)

		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name:    util.Ptr(eid.New()),
				Configs: []string{configId},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: createResp.Payload.Data.ID,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal([]string{configId}, detailResp.Payload.Data.Configs)
	})

	t.Run("create with service-target config should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configTypeId := createConfigType(t, util.Ptr("service"))
		configId := createConfig(t, configTypeId)

		_, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name:    util.Ptr(eid.New()),
				Configs: []string{configId},
			},
		}, nil)
		ctx.Req.Error(err)
		var badReq *router.CreateRouterBadRequest
		ctx.Req.ErrorAs(err, &badReq)
	})

	t.Run("create with nil-target config should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configTypeId := createConfigType(t, nil)
		configId := createConfig(t, configTypeId)

		_, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name:    util.Ptr(eid.New()),
				Configs: []string{configId},
			},
		}, nil)
		ctx.Req.Error(err)
		var badReq *router.CreateRouterBadRequest
		ctx.Req.ErrorAs(err, &badReq)
	})

	t.Run("create with duplicate config types should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configTypeId := createConfigType(t, util.Ptr("router"))
		configId1 := createConfig(t, configTypeId)
		configId2 := createConfig(t, configTypeId)

		_, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name:    util.Ptr(eid.New()),
				Configs: []string{configId1, configId2},
			},
		}, nil)
		ctx.Req.Error(err)
		var badReq *router.CreateRouterBadRequest
		ctx.Req.ErrorAs(err, &badReq)
	})

	t.Run("update should add configs", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct1 := createConfigType(t, util.Ptr("router"))
		config1 := createConfig(t, ct1)

		ct2 := createConfigType(t, util.Ptr("router"))
		config2 := createConfig(t, ct2)

		name := eid.New()
		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name:    &name,
				Configs: []string{config1},
			},
		}, nil)
		ctx.Req.NoError(err)
		id := createResp.Payload.Data.ID

		_, err = mgmtClient.API.Router.UpdateRouter(&router.UpdateRouterParams{
			ID: id,
			Router: &rest_model.RouterUpdate{
				Name:    &name,
				Configs: []string{config1, config2},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{ID: id}, nil)
		ctx.Req.NoError(err)
		ctx.Req.ElementsMatch([]string{config1, config2}, detailResp.Payload.Data.Configs)
	})

	t.Run("patch should update configs", func(t *testing.T) {
		ctx.testContextChanged(t)
		configTypeId := createConfigType(t, util.Ptr("router"))
		configId := createConfig(t, configTypeId)

		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name: util.Ptr(eid.New()),
			},
		}, nil)
		ctx.Req.NoError(err)
		id := createResp.Payload.Data.ID

		_, err = mgmtClient.API.Router.PatchRouter(&router.PatchRouterParams{
			ID: id,
			Router: &rest_model.RouterPatch{
				Configs: []string{configId},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{ID: id}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal([]string{configId}, detailResp.Payload.Data.Configs)
	})

	t.Run("update should clear configs", func(t *testing.T) {
		ctx.testContextChanged(t)
		configTypeId := createConfigType(t, util.Ptr("router"))
		configId := createConfig(t, configTypeId)

		name := eid.New()
		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name:    &name,
				Configs: []string{configId},
			},
		}, nil)
		ctx.Req.NoError(err)
		id := createResp.Payload.Data.ID

		_, err = mgmtClient.API.Router.UpdateRouter(&router.UpdateRouterParams{
			ID: id,
			Router: &rest_model.RouterUpdate{
				Name: &name,
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{ID: id}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Empty(detailResp.Payload.Data.Configs)
	})
}
