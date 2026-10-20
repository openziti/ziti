//go:build apitests

package tests

import (
	"testing"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
)

// Test_DefaultAdminStaysAdmin: the default admin's isAdmin flag cannot be cleared.
func Test_DefaultAdminStaysAdmin(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	current := ctx.AdminManagementSession.requireQuery("current-identity")
	defaultAdminId := ctx.RequireGetNonNilPathValue(current, "data", "id").Data().(string)
	ctx.Req.True(ctx.RequireGetNonNilPathValue(current, "data", "isDefaultAdmin").Data().(bool), "the admin session belongs to the default admin")

	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
		SetBody(&rest_model.IdentityPatch{IsAdmin: ToPtr(false)}).
		Patch("/identities/" + defaultAdminId)
	ctx.Req.NoError(err)
	ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "isAdmin")

	detail := ctx.AdminManagementSession.requireQuery("identities/" + defaultAdminId)
	ctx.Req.True(ctx.RequireGetNonNilPathValue(detail, "data", "isAdmin").Data().(bool), "the default admin is still an admin")
}
