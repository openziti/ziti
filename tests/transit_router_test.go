//go:build apitests
// +build apitests

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

package tests

import (
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/network"
	"testing"
	"time"
)

func Test_TransitRouters(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("transit routers can be created and enrolled", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.createAndEnrollTransitRouter()
	})

	t.Run("transit routers can be created, enrolled, and started", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.createEnrollAndStartTransitRouter()
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
		router := ctx.AdminManagementSession.requireNewTransitRouter()
		ctx.AdminManagementSession.requireDeleteEntity(router)
	})

	t.Run("create & enroll transit router, then delete", func(t *testing.T) {
		ctx.testContextChanged(t)
		router := ctx.createAndEnrollTransitRouter()
		ctx.AdminManagementSession.requireDeleteEntity(router)
	})

	t.Run("can list transit routers created in fabric", func(t *testing.T) {
		ctx.testContextChanged(t)

		fp := "f6fc1c03175f674f1f0b505a9ff930e5"
		fabTxRouter := &network.Router{
			BaseEntity: models.BaseEntity{
				Id:        "uMvqq",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Tags:      nil,
			},
			Name:        "uMvqq",
			Fingerprint: &fp,
		}
		err := ctx.fabricController.GetNetwork().Routers.Create(fabTxRouter)
		ctx.Req.NoError(err, "could not create router at fabric level")

		body := ctx.AdminManagementSession.requireQuery("transit-routers")
		ctx.logJson(body.Bytes())
	})
}
