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

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/controller/permissions"
)

func Test_Permissions_Authenticator(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all authenticator operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testAuthenticatorAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read authenticator operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testAuthenticatorAdminReadOnlyPermissions(ctx)
	})

	t.Run("non-admin permissions are denied all authenticator operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testAuthenticatorNonAdminPermissionsDenied(ctx)
	})
}

// testAuthenticatorAdminPermissions tests that admin permission allows all authenticator operations
func testAuthenticatorAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Create an identity to attach authenticator to
	identityId := testHelper.createIdentity(ctx, adminSession, "admin-test-identity", http.StatusCreated)

	// Test create operation
	testAuthenticatorId := testHelper.createAuthenticator(ctx, adminSession, identityId, http.StatusCreated)

	// Test read operation
	testHelper.getAuthenticator(ctx, adminSession, testAuthenticatorId, http.StatusOK)

	// Test list operation
	testHelper.listAuthenticators(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchAuthenticator(ctx, adminSession, testAuthenticatorId, &rest_model.AuthenticatorPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteAuthenticator(ctx, adminSession, testAuthenticatorId, http.StatusOK)

	// Cleanup
	testHelper.deleteIdentity(ctx, adminSession, identityId, http.StatusOK)
}

// testAuthenticatorAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testAuthenticatorAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test identity and authenticator as admin for readonly user to read
	identityId := testHelper.createIdentity(ctx, ctx.AdminManagementSession, "readonly-test-identity", http.StatusCreated)
	testAuthenticatorId := testHelper.createAuthenticator(ctx, ctx.AdminManagementSession, identityId, http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getAuthenticator(ctx, readonlySession, testAuthenticatorId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listAuthenticators(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createAuthenticator(ctx, readonlySession, identityId, http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchAuthenticator(ctx, readonlySession, testAuthenticatorId, &rest_model.AuthenticatorPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteAuthenticator(ctx, readonlySession, testAuthenticatorId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteAuthenticator(ctx, ctx.AdminManagementSession, testAuthenticatorId, http.StatusOK)
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, identityId, http.StatusOK)
}

// testAuthenticatorNonAdminPermissionsDenied tests that "authenticator" is an invalid permission
func testAuthenticatorNonAdminPermissionsDenied(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Test that "authenticator" is an invalid permission - attempting to create an identity with it should fail
	testHelper.createIdentityWithPermissions(ctx, ctx.AdminManagementSession, "invalid-authenticator-perm", []string{"authenticator"}, http.StatusBadRequest)

	// Test that "authenticator.read" is an invalid permission
	testHelper.createIdentityWithPermissions(ctx, ctx.AdminManagementSession, "invalid-authenticator-read-perm", []string{"authenticator.read"}, http.StatusBadRequest)

	// Test that "authenticator.create" is an invalid permission
	testHelper.createIdentityWithPermissions(ctx, ctx.AdminManagementSession, "invalid-authenticator-create-perm", []string{"authenticator.create"}, http.StatusBadRequest)

	// Test that "authenticator.update" is an invalid permission
	testHelper.createIdentityWithPermissions(ctx, ctx.AdminManagementSession, "invalid-authenticator-update-perm", []string{"authenticator.update"}, http.StatusBadRequest)

	// Test that "authenticator.delete" is an invalid permission
	testHelper.createIdentityWithPermissions(ctx, ctx.AdminManagementSession, "invalid-authenticator-delete-perm", []string{"authenticator.delete"}, http.StatusBadRequest)
}
