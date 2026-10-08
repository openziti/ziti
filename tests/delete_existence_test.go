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
	"net/http"
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/model"
)

// Edge routers and transit routers share the router store. A delete through one endpoint must
// only remove a record of that endpoint's type, and must leave the other type's record alone. A
// router created in the fabric, with no transit router record, still belongs to the router API.
func Test_DeleteRequiresTypedTarget(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("edge router delete with a transit router id is not found", func(t *testing.T) {
		ctx.testContextChanged(t)
		transitRouter := ctx.AdminManagementSession.requireNewTransitRouter()

		resp := ctx.AdminManagementSession.deleteEntityOfType("edge-routers", transitRouter.id)
		ctx.RequireNotFoundError(resp.StatusCode(), resp.Body())

		ctx.AdminManagementSession.requireQuery("transit-routers/" + transitRouter.id)
	})

	t.Run("transit router delete with an edge router id is not found", func(t *testing.T) {
		ctx.testContextChanged(t)
		edgeRouter := ctx.AdminManagementSession.requireNewEdgeRouter()

		resp := ctx.AdminManagementSession.deleteEntityOfType("transit-routers", edgeRouter.id)
		ctx.RequireNotFoundError(resp.StatusCode(), resp.Body())

		ctx.AdminManagementSession.requireQuery("edge-routers/" + edgeRouter.id)
	})

	t.Run("router created in the fabric is deleted through the router API", func(t *testing.T) {
		ctx.testContextChanged(t)
		fabricRouter := &model.Router{Name: eid.New()}
		ctx.Req.NoError(ctx.fabricController.GetNetwork().Router.Create(fabricRouter, change.New()))
		ctx.AdminManagementSession.requireQuery("routers/" + fabricRouter.Id)

		resp := ctx.AdminManagementSession.deleteEntityOfType("routers", fabricRouter.Id)
		standardJsonResponseTests(resp, http.StatusOK, t)

		status, body := ctx.AdminManagementSession.query("routers/" + fabricRouter.Id)
		ctx.RequireNotFoundError(status, body)
	})
}
