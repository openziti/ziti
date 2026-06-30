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
	ctx.Req.Contains(edgeRouter.Capabilities, string(rest_model.RouterCapabilitiesSERVICESUBSCRIPTIONS))
	ctx.Req.Contains(edgeRouter.Capabilities, string(rest_model.RouterCapabilitiesRDMSUPPORTED))
	ctx.Req.Equal("v0.0.0", edgeRouter.Version, "router-reported test version should be persisted")
}
