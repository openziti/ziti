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

func Test_Permissions_AuthPolicy(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all auth policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testAuthPolicyAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read auth policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testAuthPolicyAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level auth-policy permission allows all CRUD for auth policies", func(t *testing.T) {
		ctx.testContextChanged(t)
		testAuthPolicyEntityLevelPermissions(ctx)
	})

	t.Run("auth-policy-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testAuthPolicyEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all auth policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testAuthPolicyNoPermissions(ctx)
	})
}

// testAuthPolicyAdminPermissions tests that admin permission allows all auth policy operations
func testAuthPolicyAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testAuthPolicyId := testHelper.createAuthPolicy(ctx, adminSession, "admin-test-auth-policy", http.StatusCreated)

	// Test read operation
	testHelper.getAuthPolicy(ctx, adminSession, testAuthPolicyId, http.StatusOK)

	// Test list operation
	testHelper.listAuthPolicies(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchAuthPolicy(ctx, adminSession, testAuthPolicyId, &rest_model.AuthPolicyPatch{
		Name: ToPtr("admin-test-updated"),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteAuthPolicy(ctx, adminSession, testAuthPolicyId, http.StatusOK)
}

// testAuthPolicyAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testAuthPolicyAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test auth policy as admin for readonly user to read
	testAuthPolicyId := testHelper.createAuthPolicy(ctx, ctx.AdminManagementSession, "readonly-test-auth-policy", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getAuthPolicy(ctx, readonlySession, testAuthPolicyId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listAuthPolicies(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createAuthPolicy(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchAuthPolicy(ctx, readonlySession, testAuthPolicyId, &rest_model.AuthPolicyPatch{
		Name: ToPtr("readonly-update-test"),
	}, http.StatusUnauthorized)

	testHelper.patchAuthPolicy(ctx, readonlySession, testAuthPolicyId, &rest_model.AuthPolicyPatch{
		Name: ToPtr("readonly-update-test"),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteAuthPolicy(ctx, readonlySession, testAuthPolicyId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteAuthPolicy(ctx, ctx.AdminManagementSession, testAuthPolicyId, http.StatusOK)
}

// testAuthPolicyEntityLevelPermissions tests that entity-level auth-policy permission allows all CRUD for auth policies
func testAuthPolicyEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with auth-policy entity permission
	authPolicyPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"auth-policy"})

	// Test auth policy CRUD - should all succeed
	testAuthPolicyId := testHelper.createAuthPolicy(ctx, authPolicyPermSession, "entity-perm-test-auth-policy", http.StatusCreated)
	testHelper.getAuthPolicy(ctx, authPolicyPermSession, testAuthPolicyId, http.StatusOK)
	testHelper.patchAuthPolicy(ctx, authPolicyPermSession, testAuthPolicyId, &rest_model.AuthPolicyPatch{
		Name: ToPtr("entity-perm-updated"),
	}, http.StatusOK)
	testHelper.deleteAuthPolicy(ctx, authPolicyPermSession, testAuthPolicyId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, authPolicyPermSession, "auth-policy-perm-identity", http.StatusUnauthorized)
}

// testAuthPolicyEntityActionPermissions tests that entity-action permissions allow specific operations
func testAuthPolicyEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test auth policy as admin
	testAuthPolicyId := testHelper.createAuthPolicy(ctx, ctx.AdminManagementSession, "action-test-auth-policy", http.StatusCreated)

	// Test auth-policy.read permission
	readAuthPolicySession := testHelper.newIdentityWithPermissions(ctx, []string{"auth-policy.read"})

	testHelper.getAuthPolicy(ctx, readAuthPolicySession, testAuthPolicyId, http.StatusOK)
	testHelper.listAuthPolicies(ctx, readAuthPolicySession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createAuthPolicy(ctx, readAuthPolicySession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchAuthPolicy(ctx, readAuthPolicySession, testAuthPolicyId, &rest_model.AuthPolicyPatch{
		Name: ToPtr("read-only-update"),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteAuthPolicy(ctx, readAuthPolicySession, testAuthPolicyId, http.StatusUnauthorized)

	// Test auth-policy.create permission
	createAuthPolicySession := testHelper.newIdentityWithPermissions(ctx, []string{"auth-policy.create"})

	newAuthPolicyId := testHelper.createAuthPolicy(ctx, createAuthPolicySession, "create-only-auth-policy", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getAuthPolicy(ctx, createAuthPolicySession, newAuthPolicyId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchAuthPolicy(ctx, createAuthPolicySession, newAuthPolicyId, &rest_model.AuthPolicyPatch{
		Name: ToPtr("create-only-update"),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteAuthPolicy(ctx, createAuthPolicySession, newAuthPolicyId, http.StatusUnauthorized)

	// Test auth-policy.update permission
	updateAuthPolicySession := testHelper.newIdentityWithPermissions(ctx, []string{"auth-policy.update"})
	testHelper.patchAuthPolicy(ctx, updateAuthPolicySession, newAuthPolicyId, &rest_model.AuthPolicyPatch{
		Name: ToPtr("update-only-updated"),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getAuthPolicy(ctx, updateAuthPolicySession, testAuthPolicyId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createAuthPolicy(ctx, updateAuthPolicySession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteAuthPolicy(ctx, updateAuthPolicySession, testAuthPolicyId, http.StatusUnauthorized)

	// Test auth-policy.delete permission
	deleteAuthPolicySession := testHelper.newIdentityWithPermissions(ctx, []string{"auth-policy.delete"})

	testHelper.deleteAuthPolicy(ctx, deleteAuthPolicySession, testAuthPolicyId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getAuthPolicy(ctx, deleteAuthPolicySession, testAuthPolicyId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createAuthPolicy(ctx, deleteAuthPolicySession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteAuthPolicy(ctx, ctx.AdminManagementSession, newAuthPolicyId, http.StatusOK)
}

// testAuthPolicyNoPermissions tests that no permissions deny all operations
func testAuthPolicyNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test auth policy as admin
	testAuthPolicyId := testHelper.createAuthPolicy(ctx, ctx.AdminManagementSession, "no-perm-test-auth-policy", http.StatusCreated)

	// All operations should fail with Unauthorized

	// Auth policy operations
	testHelper.getAuthPolicy(ctx, noPermSession, testAuthPolicyId, http.StatusUnauthorized)

	testHelper.listAuthPolicies(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createAuthPolicy(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	testHelper.patchAuthPolicy(ctx, noPermSession, testAuthPolicyId, &rest_model.AuthPolicyPatch{
		Name: ToPtr("no-perm-update"),
	}, http.StatusUnauthorized)

	testHelper.deleteAuthPolicy(ctx, noPermSession, testAuthPolicyId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteAuthPolicy(ctx, ctx.AdminManagementSession, testAuthPolicyId, http.StatusOK)
}
