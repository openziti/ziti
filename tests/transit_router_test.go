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
	"time"

	"github.com/openziti/edge-api/rest_management_api_client/router"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
)

func Test_TransitRouters(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	_, err := mgmtClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)

	t.Run("transit routers can be created and enrolled", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.createAndEnrollTransitRouter()
	})

	t.Run("transit routers can be created, enrolled, and started", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.createEnrollAndStartTransitRouter()
		ctx.shutdownRouters()
	})

	t.Run("transit routers can be created, enrolled, and listed", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.AdminManagementSession.requireQuery("transit-routers")
	})

	t.Run("transit routers can be listed with enrolled and un-enrolled states", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.createAndEnrollTransitRouter()
		_ = ctx.AdminManagementSession.requireNewTransitRouter()

		body := ctx.AdminManagementSession.requireQuery("transit-routers")
		ctx.logJson(body.Bytes())

		t.Run("enrolled router is verified and has a fingerprint, un-enrolled router does not", func(t *testing.T) {
			ctx.testContextChanged(t)

			routers := body.Path("data")

			children, err := routers.Children()
			ctx.Req.NoError(err)

			ctx.Req.Len(children, 2, "two routers should have been returned")

			router0IsVerified, ok := children[0].Path("isVerified").Data().(bool)
			ctx.Req.True(ok, "issue getting transit router 0 isVerified state")

			router1IsVerified, ok := children[1].Path("isVerified").Data().(bool)
			ctx.Req.True(ok, "issue getting transit router 1 isVerified state")

			ctx.Req.True(router0IsVerified != router1IsVerified, "expected 1 enrolled transit router and 1 un-enrolled transit router")

			if router0IsVerified {
				ctx.requireEntityEnrolled("transit router 0", children[0])
			} else {
				ctx.requireEntityNotEnrolled("transit router 0", children[0])
			}

			if router1IsVerified {
				ctx.requireEntityEnrolled("transit router 1", children[1])
			} else {
				ctx.requireEntityNotEnrolled("transit router 1", children[1])
			}
		})
	})

	t.Run("create transit router, then delete", func(t *testing.T) {
		ctx.testContextChanged(t)
		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name: util.Ptr(eid.New()),
			},
		}, nil)
		ctx.Req.NoError(err)

		_, err = mgmtClient.API.Router.DeleteRouter(&router.DeleteRouterParams{
			ID: createResp.Payload.Data.ID,
		}, nil)
		ctx.Req.NoError(err)
	})

	t.Run("create & enroll transit router, then delete", func(t *testing.T) {
		ctx.testContextChanged(t)
		router := ctx.createAndEnrollTransitRouter()
		ctx.AdminManagementSession.requireDeleteEntity(router)
	})

	t.Run("ctrlChanListeners can be created with an empty list", func(t *testing.T) {
		ctx.testContextChanged(t)
		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name: util.Ptr(eid.New()),
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: createResp.Payload.Data.ID,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Empty(detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("ctrlChanListeners can be set on create and retrieved", func(t *testing.T) {
		ctx.testContextChanged(t)
		listeners := map[string][]string{"tls:1.2.3.4:6262": {"group1"}, "tls:5.6.7.8:6262": {}}

		createResp, err := mgmtClient.API.Router.CreateRouter(&router.CreateRouterParams{
			Router: &rest_model.RouterCreate{
				Name:              util.Ptr(eid.New()),
				CtrlChanListeners: listeners,
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: createResp.Payload.Data.ID,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(listeners, detailResp.Payload.Data.CtrlChanListeners)
	})

	t.Run("ctrlChanListeners can be updated", func(t *testing.T) {
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
				CtrlChanListeners: map[string][]string{"tls:10.0.0.1:6262": {}, "tls:10.0.0.2:6262": {}, "tls:10.0.0.3:6262": {}},
			},
		}, nil)
		ctx.Req.NoError(err)

		detailResp, err := mgmtClient.API.Router.DetailRouter(&router.DetailRouterParams{
			ID: id,
		}, nil)
		ctx.Req.NoError(err)
		ctx.Req.Equal(map[string][]string{"tls:10.0.0.1:6262": {}, "tls:10.0.0.2:6262": {}, "tls:10.0.0.3:6262": {}}, detailResp.Payload.Data.CtrlChanListeners)

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

	t.Run("can list transit routers created in fabric", func(t *testing.T) {
		ctx.testContextChanged(t)

		fp := "f6fc1c03175f674f1f0b505a9ff930e5"
		fabTxRouter := &model.Router{
			BaseEntity: models.BaseEntity{
				Id:        "uMvqq",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Tags:      nil,
			},
			Name:        "uMvqq",
			Fingerprint: &fp,
		}
		err := ctx.fabricController.GetNetwork().Router.Create(fabTxRouter, change.New())
		ctx.Req.NoError(err, "could not create router at fabric level")

		body := ctx.AdminManagementSession.requireQuery("transit-routers")
		ctx.logJson(body.Bytes())
	})
}
