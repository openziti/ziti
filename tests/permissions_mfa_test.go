//go:build apitests

package tests

import (
	"net/http"
	"testing"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/eid"
)

// Test_Permissions_WaitForMfa: an identity's granular permissions are not usable on a session
// whose MFA challenge is still pending, and become usable once it is answered.
func Test_Permissions_WaitForMfa(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	username := eid.New()
	password := eid.New()
	perms := []string{"identity"}
	identityCreate := &rest_model.IdentityCreate{
		AuthPolicyID: ToPtr("default"),
		Enrollment:   &rest_model.IdentityCreateEnrollment{Updb: username},
		IsAdmin:      ToPtr(false),
		Name:         ToPtr(eid.New()),
		Permissions:  (*rest_model.Permissions)(&perms),
		Type:         ToPtr(rest_model.IdentityTypeUser),
	}
	created := &rest_model.CreateEnvelope{}
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(created).SetBody(identityCreate).Post("/identities")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
	ctx.completeUpdbEnrollment(created.Data.ID, password)

	auth := &updbAuthenticator{Username: username, Password: password}
	enrollingSession, err := auth.AuthenticateManagementApi(ctx)
	ctx.Req.NoError(err)

	var secret string
	t.Run("enroll and verify MFA on a fully authenticated session", func(t *testing.T) {
		ctx.testContextChanged(t)
		resp, err := enrollingSession.newAuthenticatedRequest().Post("/current-identity/mfa")
		ctx.Req.NoError(err)
		standardJsonResponseTests(resp, http.StatusCreated, t)

		resp, err = enrollingSession.newAuthenticatedRequest().Get("/current-identity/mfa")
		ctx.Req.NoError(err)
		standardJsonResponseTests(resp, http.StatusOK, t)
		mfa := ctx.parseJson(resp.Body())
		secret, err = parseSecretFromProvisioningUrl(ctx.RequireGetNonNilPathValue(mfa, "data", "provisioningUrl").Data().(string))
		ctx.Req.NoError(err)

		resp, err = enrollingSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(computeMFACode(secret))).Post("/current-identity/mfa/verify")
		ctx.Req.NoError(err)
		standardJsonResponseTests(resp, http.StatusOK, t)
	})

	pendingSession, err := auth.AuthenticateManagementApi(ctx)
	ctx.Req.NoError(err)

	t.Run("a fresh session has the MFA challenge pending", func(t *testing.T) {
		ctx.testContextChanged(t)
		resp, err := pendingSession.newAuthenticatedRequest().Get("/current-api-session")
		ctx.Req.NoError(err)
		standardJsonResponseTests(resp, http.StatusOK, t)
		authQueries := ctx.RequireGetNonNilPathValue(ctx.parseJson(resp.Body()), "data", "authQueries").Data().([]interface{})
		ctx.Req.Len(authQueries, 1)
	})

	t.Run("granular permissions are refused while the challenge is pending", func(t *testing.T) {
		ctx.testContextChanged(t)
		resp, err := pendingSession.newAuthenticatedRequest().Get("/identities")
		ctx.Req.NoError(err)
		standardErrorJsonResponseTests(resp, errorz.UnauthorizedCode, http.StatusUnauthorized, t)

		resp, err = pendingSession.newAuthenticatedRequest().Delete("/identities/" + created.Data.ID)
		ctx.Req.NoError(err)
		standardErrorJsonResponseTests(resp, errorz.UnauthorizedCode, http.StatusUnauthorized, t)
	})

	t.Run("granular permissions work once the challenge is answered", func(t *testing.T) {
		ctx.testContextChanged(t)
		resp, err := pendingSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(computeMFACode(secret))).Post("/authenticate/mfa")
		ctx.Req.NoError(err)
		standardJsonResponseTests(resp, http.StatusOK, t)

		resp, err = pendingSession.newAuthenticatedRequest().Get("/identities")
		ctx.Req.NoError(err)
		standardJsonResponseTests(resp, http.StatusOK, t)
	})
}
