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

import "testing"

func Test_CurrentIdentity(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("edge routers endpoint", func(t *testing.T) {
		ctx.testContextChanged(t)

		er1 := ctx.createAndEnrollEdgeRouter(false, "test1")

		_ = ctx.AdminManagementSession.requireNewEdgeRouter("test1")

		_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment("testErAccess", false, "test1")

		identitySession, err := identityAuth.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)

		t.Run("returns empty list with no edge router policies", func(t *testing.T) {
			ctx.testContextChanged(t)

			erContainer := identitySession.requireQuery("/current-identity/edge-routers")

			ctx.Req.True(erContainer.ExistsP("data"), "has a data attribute")

			erArray, err := erContainer.Path("data").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(erArray, 0, "expect empty edge router list")
		})

		t.Run("returns a list of one with an er policy", func(t *testing.T) {
			ctx.testContextChanged(t)
			_ = ctx.AdminManagementSession.requireNewEdgeRouterPolicy([]string{"#test1"}, []string{"#test1"})

			erContainer := identitySession.requireQuery("/current-identity/edge-routers")

			ctx.Req.True(erContainer.ExistsP("data"), "has a data attribute")

			erArray, err := erContainer.Path("data").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(erArray, 1, "expected edge router list to have one edge router")

			erArray[0].ExistsP("id")
		})

		t.Run("returns empty list with if edge router is deleted", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.AdminManagementSession.requireDeleteEntity(er1)

			erContainer := identitySession.requireQuery("/current-identity/edge-routers")

			ctx.Req.True(erContainer.ExistsP("data"), "has a data attribute")

			erArray, err := erContainer.Path("data").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(erArray, 0, "expect empty edge router list")
		})
	})
}
