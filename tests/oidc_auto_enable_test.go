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

	"github.com/openziti/edge-api/rest_model"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
)

// Test_OIDC_Auto_Enable verifies that the controller automatically adds the edge-oidc API
// binding to the web listener that hosts the edge-client API when edge-oidc is not explicitly
// present in the configuration. The config set used here omits the edge-oidc binding from the
// web section; the ensureOidcOnClientApiServer instance validator is expected to add it.
func Test_OIDC_Auto_Enable(t *testing.T) {
	ctx := NewTestContextWithConfigSet(t, NoExplicitOIDC)
	defer ctx.Teardown()
	ctx.StartServer()

	clientApi := ctx.NewEdgeClientApi(nil)

	t.Run("controller capabilities reflect that OIDC was auto-enabled", func(t *testing.T) {
		ctx.testContextChanged(t)

		version, err := clientApi.GetVersion()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(version)
		ctx.Req.Contains(version.Capabilities, string(rest_model.CapabilitiesOIDCAUTH))
		ctx.Req.Contains(version.Capabilities, string(rest_model.CapabilitiesOIDCAUTHWITHCSR))
		ctx.Req.Contains(version.APIVersions, "edge-oidc")
	})

	managementApi := ctx.NewEdgeManagementApi(nil)
	defaultAdminCreds := ctx.NewAdminCredentials()

	adminApiSession, err := managementApi.Authenticate(defaultAdminCreds, nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(adminApiSession)

	t.Run("non-admins in the client api", func(t *testing.T) {
		ctx.testContextChanged(t)

		nonAdminIdentity, nonAdminCreds, err := managementApi.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(nonAdminIdentity)
		ctx.Req.NotNil(nonAdminCreds)

		t.Run("can authenticate via oidc when oidc is not explicitly configured", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApiOidc := ctx.NewEdgeClientApi(nil)
			clientApiOidc.SetUseOidc(true)
			clientApiOidc.SetAllowOidcDynamicallyEnabled(true)

			oidcSession, err := clientApiOidc.Authenticate(nonAdminCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(oidcSession)
			ctx.Req.Equal(edge_apis.ApiSessionTypeOidc, oidcSession.GetType())
		})

		t.Run("can still authenticate via legacy auth when oidc is not explicitly configured", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApiLegacy := ctx.NewEdgeClientApi(nil)
			clientApiLegacy.SetUseOidc(false)
			clientApiLegacy.SetAllowOidcDynamicallyEnabled(false)

			legacySession, err := clientApiLegacy.Authenticate(nonAdminCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(legacySession)
			ctx.Req.Equal(edge_apis.ApiSessionTypeLegacy, legacySession.GetType())
		})
	})

	t.Run("admins in the management api", func(t *testing.T) {
		ctx.testContextChanged(t)

		adminIdentity, adminCreds, err := managementApi.CreateAndEnrollOttIdentity(true)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminIdentity)
		ctx.Req.NotNil(adminCreds)

		t.Run("can authenticate via oidc when oidc is not explicitly configured", func(t *testing.T) {
			ctx.testContextChanged(t)

			managementClient := ctx.NewEdgeManagementApi(nil)
			managementClient.SetUseOidc(true)
			managementClient.SetAllowOidcDynamicallyEnabled(true)

			oidcSession, err := managementClient.Authenticate(adminCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(oidcSession)
			ctx.Req.Equal(edge_apis.ApiSessionTypeOidc, oidcSession.GetType())
		})

		t.Run("can still authenticate via legacy auth when oidc is not explicitly configured", func(t *testing.T) {
			ctx.testContextChanged(t)

			managementClient := ctx.NewEdgeManagementApi(nil)
			managementClient.SetUseOidc(false)
			managementClient.SetAllowOidcDynamicallyEnabled(false)

			legacySession, err := managementClient.Authenticate(adminCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(legacySession)
			ctx.Req.Equal(edge_apis.ApiSessionTypeLegacy, legacySession.GetType())
		})
	})
}

// Test_OIDC_Auto_Binding_Disabled verifies that setting disableOidcAutoBinding: true in the
// edge section of the controller config suppresses the automatic addition of the edge-oidc
// binding, leaving OIDC absent from the running controller even when it is not explicitly
// configured elsewhere.
func Test_OIDC_Auto_Binding_Disabled(t *testing.T) {
	ctx := NewTestContextWithConfigSet(t, DisabledOidcAutoBinding)
	defer ctx.Teardown()
	ctx.StartServer()

	clientApi := ctx.NewEdgeClientApi(nil)

	t.Run("controller capabilities do not include OIDC when auto-binding is disabled", func(t *testing.T) {
		ctx.testContextChanged(t)

		version, err := clientApi.GetVersion()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(version)
		ctx.Req.NotContains(version.Capabilities, string(rest_model.CapabilitiesOIDCAUTH))
		ctx.Req.NotContains(version.Capabilities, string(rest_model.CapabilitiesOIDCAUTHWITHCSR))
		ctx.Req.NotContains(version.APIVersions, "edge-oidc")
	})

	managementApi := ctx.NewEdgeManagementApi(nil)
	defaultAdminCreds := ctx.NewAdminCredentials()

	adminApiSession, err := managementApi.Authenticate(defaultAdminCreds, nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(adminApiSession)

	t.Run("non-admins in the client api", func(t *testing.T) {
		ctx.testContextChanged(t)

		nonAdminIdentity, nonAdminCreds, err := managementApi.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(nonAdminIdentity)
		ctx.Req.NotNil(nonAdminCreds)

		t.Run("can still authenticate via legacy auth when oidc auto-binding is disabled", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApiLegacy := ctx.NewEdgeClientApi(nil)
			clientApiLegacy.SetUseOidc(false)
			clientApiLegacy.SetAllowOidcDynamicallyEnabled(false)

			legacySession, err := clientApiLegacy.Authenticate(nonAdminCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(legacySession)
			ctx.Req.Equal(edge_apis.ApiSessionTypeLegacy, legacySession.GetType())
		})
	})
}
