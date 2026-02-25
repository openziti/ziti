package tests

import (
	"testing"

	edge_apis "github.com/openziti/sdk-golang/edge-apis"
)

// Test_OIDC_Auto_Enable verifies that the controller automatically adds the edge-oidc API
// binding to the web listener that hosts the edge-client API when edge-oidc is not explicitly
// present in the configuration. This exercises the ensureOidcOnClientApiServer instance
// validator introduced in issue #3597.
//
// The config set used here ("testdata/configs/no-explicit-oidc/ctrl.yml") is identical to
// the default ats-ctrl.yml except that the edge-oidc binding is omitted from the web section.
func Test_OIDC_Auto_Enable(t *testing.T) {
	ctx := NewTestContextWithConfigSet(t, NoExplicitOIDC)
	defer ctx.Teardown()
	ctx.StartServer()

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

			clientApi := ctx.NewEdgeClientApi(nil)
			clientApi.SetUseOidc(true)
			clientApi.SetAllowOidcDynamicallyEnabled(true)

			oidcSession, err := clientApi.Authenticate(nonAdminCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(oidcSession)
			ctx.Req.Equal(edge_apis.ApiSessionTypeOidc, oidcSession.GetType())
		})

		t.Run("can still authenticate via legacy auth when oidc is not explicitly configured", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)
			clientApi.SetUseOidc(false)
			clientApi.SetAllowOidcDynamicallyEnabled(false)

			legacySession, err := clientApi.Authenticate(nonAdminCreds, nil)
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
