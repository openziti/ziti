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

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/controller/change"
)

// Test_EdgeRouterCapabilities locks in that when an edge router connects, the controller persists
// the capabilities bitmask and binary version it reports, and renders them on the client
// current-identity/edge-routers endpoint as the capability string list plus a version string.
func Test_EdgeRouterCapabilities(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	// Arrange: a running edge router reports its capabilities + version on connect, and an
	// #all/#all policy lets the test identity see it via current-identity/edge-routers.
	ctx.CreateEnrollAndStartEdgeRouter()
	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	_, identityCreds, err := mgmtClient.CreateAndEnrollOttIdentity(false)
	ctx.Req.NoError(err)

	clientApi := ctx.NewEdgeClientApi(nil)
	_, err = clientApi.Authenticate(identityCreds, nil)
	ctx.Req.NoError(err)

	// Act: the capabilities are persisted after the router's hello is processed, which trails the
	// router start, so poll the list until they land.
	var routers []*rest_model.CurrentIdentityEdgeRouterDetail
	ctx.Req.Eventually(func() bool {
		var listErr error
		routers, listErr = clientApi.ListCurrentIdentityEdgeRouters()
		return listErr == nil && len(routers) == 1 && len(routers[0].Capabilities) > 0
	}, 15*time.Second, 200*time.Millisecond, "edge router capabilities should be persisted and returned")

	// Assert: every advertised router bit was rendered to its capability string, and the
	// router-reported version was persisted.
	edgeRouter := routers[0]
	ctx.Req.Contains(edgeRouter.Capabilities, string(rest_model.RouterCapabilitiesMULTICHANNEL))
	ctx.Req.Contains(edgeRouter.Capabilities, string(rest_model.RouterCapabilitiesCONNECTV2))
	ctx.Req.Contains(edgeRouter.Capabilities, string(rest_model.RouterCapabilitiesSERVICESUBSCRIPTIONS))
	ctx.Req.Contains(edgeRouter.Capabilities, string(rest_model.RouterCapabilitiesPOSTURECHECKS))
	// RouterDataModel is side-local (controller<->router only, negative bit position) and is
	// deliberately absent from the rendered capability strings.
	ctx.Req.NotContains(edgeRouter.Capabilities, "RDM_SUPPORTED")
	ctx.Req.Contains(edgeRouter.Capabilities, string(rest_model.RouterCapabilitiesBINDSUCCESS))
	ctx.Req.Equal("v0.0.0", edgeRouter.Version, "router-reported test version should be persisted")

	// The management detail exposes the raw capabilities bitmask alongside the decoded strings.
	// Every rendered string corresponds to a set bit, so a non-empty list implies a non-zero mask.
	mgmtDetail, err := mgmtClient.GetEdgeRouter(ctx.edgeRouterEntity.id)
	ctx.Req.NoError(err)
	ctx.Req.NotZero(mgmtDetail.CapabilitiesMask, "management detail should expose the raw capabilities bitmask")
	ctx.Req.Equal(edgeRouter.Capabilities, mgmtDetail.Capabilities, "management detail should decode the same capabilities as the client list")

	t.Run("enumerated-router-capabilities lists every capability the controller knows", func(t *testing.T) {
		ctx.testContextChanged(t)

		enumerated, err := mgmtClient.ListEnumeratedRouterCapabilities()
		ctx.Req.NoError(err)
		ctx.Req.ElementsMatch([]rest_model.RouterCapabilities{
			rest_model.RouterCapabilitiesMULTICHANNEL,
			rest_model.RouterCapabilitiesCONNECTV2,
			rest_model.RouterCapabilitiesSERVICESUBSCRIPTIONS,
			rest_model.RouterCapabilitiesPOSTURECHECKS,
			rest_model.RouterCapabilitiesBINDSUCCESS,
		}, enumerated)
	})

	t.Run("unknown and control-plane-only bits are omitted from rendering", func(t *testing.T) {
		ctx.testContextChanged(t)

		// bit 1 = MultiChannel (known); bit 30 = an unmapped positive capability; bit 34 = the
		// control-plane-only -30 slot (bitPosition of -30 is maskBits-30 = 34). Neither unknown bit
		// has an edge-api string, so both must be dropped from the rendered list while the raw mask
		// is preserved verbatim.
		craftedMask := int64(1)<<int(capabilities.RouterMultiChannel) | int64(1)<<30 | int64(1)<<34
		ctx.Req.NoError(ctx.EdgeController.AppEnv.Managers.EdgeRouter.UpdateRouterReportedState(
			ctx.edgeRouterEntity.id, craftedMask, "v0.0.0", change.New()))

		detail, err := mgmtClient.GetEdgeRouter(ctx.edgeRouterEntity.id)
		ctx.Req.NoError(err)
		ctx.Req.Equal(craftedMask, detail.CapabilitiesMask, "raw mask should be preserved verbatim")
		ctx.Req.Equal([]string{string(rest_model.RouterCapabilitiesMULTICHANNEL)}, detail.Capabilities,
			"only the known bit should render; unknown and control-plane-only bits are omitted")
	})
}
