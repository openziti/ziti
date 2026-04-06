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
	"sort"
	"testing"

	"github.com/openziti/edge-api/rest_management_api_client/edge_router"
	"github.com/openziti/edge-api/rest_management_api_client/role_attributes"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/common/eid"
)

func Test_EdgeRouter(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	_, err := mgmtClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)

	t.Run("role attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		roleAttrs := rest_model.Attributes{role1, role2}

		createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{
				Name:           util.Ptr(eid.New()),
				RoleAttributes: &roleAttrs,
			},
		}, nil)
		ctx.Req.NoError(err)
		id := createResp.Payload.Data.ID

		detailResp, err := mgmtClient.API.EdgeRouter.DetailEdgeRouter(&edge_router.DetailEdgeRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(detailResp.Payload.Data.RoleAttributes)
		actual := []string(*detailResp.Payload.Data.RoleAttributes)
		sort.Strings(actual)
		expected := []string{role1, role2}
		sort.Strings(expected)
		ctx.Req.Equal(expected, actual)
	})

	t.Run("role attributes should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		roleAttrs := rest_model.Attributes{role1, role2}

		createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{
				Name:           util.Ptr(eid.New()),
				RoleAttributes: &roleAttrs,
			},
		}, nil)
		ctx.Req.NoError(err)
		id := createResp.Payload.Data.ID

		role3 := eid.New()
		updatedAttrs := rest_model.Attributes{role2, role3}
		_, err = mgmtClient.API.EdgeRouter.PatchEdgeRouter(&edge_router.PatchEdgeRouterParams{
			ID: id,
			EdgeRouter: &rest_model.EdgeRouterPatch{
				RoleAttributes: &updatedAttrs,
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.EdgeRouter.DetailEdgeRouter(&edge_router.DetailEdgeRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(detailResp.Payload.Data.RoleAttributes)
		actual := []string(*detailResp.Payload.Data.RoleAttributes)
		sort.Strings(actual)
		expected := []string{role2, role3}
		sort.Strings(expected)
		ctx.Req.Equal(expected, actual)
	})

	t.Run("role attributes should be queryable", func(t *testing.T) {
		ctx.testContextChanged(t)
		prefix := "rol3-attribut3-qu3ry-t3st-"
		role1 := prefix + "sales"
		role2 := prefix + "support"
		role3 := prefix + "engineering"
		role4 := prefix + "field-ops"
		role5 := prefix + "executive"

		createWithRoles := func(roles ...string) string {
			attrs := rest_model.Attributes(roles)
			var attrsPtr *rest_model.Attributes
			if len(roles) > 0 {
				attrsPtr = &attrs
			}
			resp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
				EdgeRouter: &rest_model.EdgeRouterCreate{
					Name:           util.Ptr(eid.New()),
					RoleAttributes: attrsPtr,
				},
			}, nil)
			ctx.Req.NoError(err)
			return resp.Payload.Data.ID
		}

		createWithRoles(role1, role2)
		createWithRoles(role2, role3)
		createWithRoles(role3, role4)
		role5RouterID := createWithRoles(role5)
		createWithRoles()

		// list all role attributes
		listResp, err := mgmtClient.API.RoleAttributes.ListEdgeRouterRoleAttributes(&role_attributes.ListEdgeRouterRoleAttributesParams{}, nil)
		ctx.Req.NoError(err)
		list := []string(listResp.Payload.Data)
		ctx.Req.True(len(list) >= 5)
		ctx.Req.True(stringz.ContainsAll(list, role1, role2, role3, role4, role5))

		// list with filter
		filter := `id contains "e" and id contains "` + prefix + `" sort by id`
		listResp, err = mgmtClient.API.RoleAttributes.ListEdgeRouterRoleAttributes(&role_attributes.ListEdgeRouterRoleAttributesParams{
			Filter: &filter,
		}, nil)
		ctx.Req.NoError(err)
		list = []string(listResp.Payload.Data)
		ctx.Req.Equal(4, len(list))

		expected := []string{role1, role3, role4, role5}
		sort.Strings(expected)
		ctx.Req.Equal(expected, list)

		// remove role5 by patching its router to have no role attributes
		emptyAttrs := rest_model.Attributes{}
		_, err = mgmtClient.API.EdgeRouter.PatchEdgeRouter(&edge_router.PatchEdgeRouterParams{
			ID: role5RouterID,
			EdgeRouter: &rest_model.EdgeRouterPatch{
				RoleAttributes: &emptyAttrs,
			},
		}, nil)
		ctx.Req.NoError(err)

		listResp, err = mgmtClient.API.RoleAttributes.ListEdgeRouterRoleAttributes(&role_attributes.ListEdgeRouterRoleAttributesParams{}, nil)
		ctx.Req.NoError(err)
		list = []string(listResp.Payload.Data)
		ctx.Req.True(len(list) >= 4)
		ctx.Req.True(stringz.ContainsAll(list, role1, role2, role3, role4))
		ctx.Req.False(stringz.Contains(list, role5))
	})

	t.Run("ctrlChanListeners can be created with an empty list", func(t *testing.T) {
		ctx.testContextChanged(t)
		createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{
				Name: util.Ptr(eid.New()),
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.EdgeRouter.DetailEdgeRouter(&edge_router.DetailEdgeRouterParams{
			ID: createResp.Payload.Data.ID,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Empty(detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("ctrlChanListeners can be set on create and retrieved", func(t *testing.T) {
		ctx.testContextChanged(t)
		listeners := map[string][]string{"tls:1.2.3.4:6262": {"group1"}, "tls:5.6.7.8:6262": {}}

		createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{
				Name:              util.Ptr(eid.New()),
				CtrlChanListeners: listeners,
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.EdgeRouter.DetailEdgeRouter(&edge_router.DetailEdgeRouterParams{
			ID: createResp.Payload.Data.ID,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(listeners, detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("ctrlChanListeners can be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		name := eid.New()
		createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{
				Name:              &name,
				CtrlChanListeners: map[string][]string{"tls:1.2.3.4:6262": {}},
			},
		}, nil)
		ctx.Req.NoError(err)
		id := createResp.Payload.Data.ID

		// update to more addresses
		_, err = mgmtClient.API.EdgeRouter.PatchEdgeRouter(&edge_router.PatchEdgeRouterParams{
			ID: id,
			EdgeRouter: &rest_model.EdgeRouterPatch{
				CtrlChanListeners: map[string][]string{"tls:10.0.0.1:6262": {}, "tls:10.0.0.2:6262": {}, "tls:10.0.0.3:6262": {}},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.EdgeRouter.DetailEdgeRouter(&edge_router.DetailEdgeRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(map[string][]string{"tls:10.0.0.1:6262": {}, "tls:10.0.0.2:6262": {}, "tls:10.0.0.3:6262": {}}, detailResp.Payload.Data.CtrlChanListeners)

		// update to fewer addresses
		_, err = mgmtClient.API.EdgeRouter.PatchEdgeRouter(&edge_router.PatchEdgeRouterParams{
			ID: id,
			EdgeRouter: &rest_model.EdgeRouterPatch{
				CtrlChanListeners: map[string][]string{"tls:10.0.0.1:6262": {}},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err = mgmtClient.API.EdgeRouter.DetailEdgeRouter(&edge_router.DetailEdgeRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(map[string][]string{"tls:10.0.0.1:6262": {}}, detailResp.Payload.Data.CtrlChanListeners)

		// clear all addresses via PUT
		_, err = mgmtClient.API.EdgeRouter.UpdateEdgeRouter(&edge_router.UpdateEdgeRouterParams{
			ID: id,
			EdgeRouter: &rest_model.EdgeRouterUpdate{
				Name: &name,
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err = mgmtClient.API.EdgeRouter.DetailEdgeRouter(&edge_router.DetailEdgeRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Empty(detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("ctrlChanListeners can be set and cleared via patch", func(t *testing.T) {
		ctx.testContextChanged(t)
		createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{
				Name: util.Ptr(eid.New()),
			},
		}, nil)
		ctx.Req.NoError(err)
		id := createResp.Payload.Data.ID

		// set listeners via patch
		_, err = mgmtClient.API.EdgeRouter.PatchEdgeRouter(&edge_router.PatchEdgeRouterParams{
			ID: id,
			EdgeRouter: &rest_model.EdgeRouterPatch{
				CtrlChanListeners: map[string][]string{"tls:1.2.3.4:6262": {"west"}, "tls:5.6.7.8:6262": {}},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.EdgeRouter.DetailEdgeRouter(&edge_router.DetailEdgeRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(map[string][]string{"tls:1.2.3.4:6262": {"west"}, "tls:5.6.7.8:6262": {}}, detailResp.Payload.Data.CtrlChanListeners)

		// clear listeners via patch with empty map
		_, err = mgmtClient.API.EdgeRouter.PatchEdgeRouter(&edge_router.PatchEdgeRouterParams{
			ID: id,
			EdgeRouter: &rest_model.EdgeRouterPatch{
				CtrlChanListeners: map[string][]string{},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err = mgmtClient.API.EdgeRouter.DetailEdgeRouter(&edge_router.DetailEdgeRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Empty(detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("newly created edge routers that is deleted", func(t *testing.T) {
		ctx.testContextChanged(t)
		createResp, err := mgmtClient.API.EdgeRouter.CreateEdgeRouter(&edge_router.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{
				Name: util.Ptr(eid.New()),
			},
		}, nil)
		ctx.Req.NoError(err)

		_, err = mgmtClient.API.EdgeRouter.DeleteEdgeRouter(&edge_router.DeleteEdgeRouterParams{
			ID: createResp.Payload.Data.ID,
		}, nil)
		ctx.Req.NoError(err)
	})
}

func Test_EdgeRouterConfigs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("create with router-target config should succeed", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("router")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		er := newTestEdgeRouter()
		er.configs = s(config.Id)
		er.id = ctx.AdminManagementSession.requireCreateEntity(er)
		ctx.AdminManagementSession.validateEntityWithQuery(er)
	})

	t.Run("create with service-target config should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("service")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		er := newTestEdgeRouter()
		er.configs = s(config.Id)
		resp := ctx.AdminManagementSession.createEntity(er)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "configs")
	})

	t.Run("create with nil-target config should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = nil
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		er := newTestEdgeRouter()
		er.configs = s(config.Id)
		resp := ctx.AdminManagementSession.createEntity(er)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "configs")
	})

	t.Run("create with duplicate config types should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("router")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config1 := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})
		config2 := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		er := newTestEdgeRouter()
		er.configs = s(config1.Id, config2.Id)
		resp := ctx.AdminManagementSession.createEntity(er)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "configs")
	})

	t.Run("update should add configs", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("router")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config1 := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		ct2 := ctx.newConfigType()
		ct2.Target = util.Ptr("router")
		ct2.Id = ctx.AdminManagementSession.requireCreateEntity(ct2)
		config2 := ctx.AdminManagementSession.requireCreateNewConfig(ct2.Id, map[string]interface{}{"key": "value"})

		er := newTestEdgeRouter()
		er.configs = s(config1.Id)
		er.id = ctx.AdminManagementSession.requireCreateEntity(er)
		ctx.AdminManagementSession.validateEntityWithQuery(er)

		er.configs = s(config1.Id, config2.Id)
		ctx.AdminManagementSession.requireUpdateEntity(er)
		ctx.AdminManagementSession.validateUpdate(er)
	})

	t.Run("patch should update configs", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("router")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		er := newTestEdgeRouter()
		er.id = ctx.AdminManagementSession.requireCreateEntity(er)

		er.configs = s(config.Id)
		ctx.AdminManagementSession.requirePatchEntity(er, "configs")
		ctx.AdminManagementSession.validateUpdate(er)
	})

	t.Run("update should clear configs", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("router")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		er := newTestEdgeRouter()
		er.configs = s(config.Id)
		er.id = ctx.AdminManagementSession.requireCreateEntity(er)
		ctx.AdminManagementSession.validateEntityWithQuery(er)

		er.configs = nil
		ctx.AdminManagementSession.requireUpdateEntity(er)
		ctx.AdminManagementSession.validateUpdate(er)
	})
}

func Test_TransitRouterConfigs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("create with router-target config should succeed", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("router")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		tr := newTestTransitRouter()
		tr.configs = s(config.Id)
		tr.id = ctx.AdminManagementSession.requireCreateEntity(tr)
		ctx.AdminManagementSession.validateEntityWithQuery(tr)
	})

	t.Run("create with service-target config should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("service")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		tr := newTestTransitRouter()
		tr.configs = s(config.Id)
		resp := ctx.AdminManagementSession.createEntity(tr)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "configs")
	})

	t.Run("create with nil-target config should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = nil
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		tr := newTestTransitRouter()
		tr.configs = s(config.Id)
		resp := ctx.AdminManagementSession.createEntity(tr)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "configs")
	})

	t.Run("create with duplicate config types should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("router")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config1 := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})
		config2 := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		tr := newTestTransitRouter()
		tr.configs = s(config1.Id, config2.Id)
		resp := ctx.AdminManagementSession.createEntity(tr)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "configs")
	})

	t.Run("update should add configs", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("router")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config1 := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		ct2 := ctx.newConfigType()
		ct2.Target = util.Ptr("router")
		ct2.Id = ctx.AdminManagementSession.requireCreateEntity(ct2)
		config2 := ctx.AdminManagementSession.requireCreateNewConfig(ct2.Id, map[string]interface{}{"key": "value"})

		tr := newTestTransitRouter()
		tr.configs = s(config1.Id)
		tr.id = ctx.AdminManagementSession.requireCreateEntity(tr)
		ctx.AdminManagementSession.validateEntityWithQuery(tr)

		tr.configs = s(config1.Id, config2.Id)
		ctx.AdminManagementSession.requireUpdateEntity(tr)
		ctx.AdminManagementSession.validateUpdate(tr)
	})

	t.Run("patch should update configs", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("router")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		tr := newTestTransitRouter()
		tr.id = ctx.AdminManagementSession.requireCreateEntity(tr)

		tr.configs = s(config.Id)
		ctx.AdminManagementSession.requirePatchEntity(tr, "configs")
		ctx.AdminManagementSession.validateUpdate(tr)
	})

	t.Run("update should clear configs", func(t *testing.T) {
		ctx.testContextChanged(t)
		ct := ctx.newConfigType()
		ct.Target = util.Ptr("router")
		ct.Id = ctx.AdminManagementSession.requireCreateEntity(ct)
		config := ctx.AdminManagementSession.requireCreateNewConfig(ct.Id, map[string]interface{}{"key": "value"})

		tr := newTestTransitRouter()
		tr.configs = s(config.Id)
		tr.id = ctx.AdminManagementSession.requireCreateEntity(tr)
		ctx.AdminManagementSession.validateEntityWithQuery(tr)

		tr.configs = nil
		ctx.AdminManagementSession.requireUpdateEntity(tr)
		ctx.AdminManagementSession.validateUpdate(tr)
	})
}
