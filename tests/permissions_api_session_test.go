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

	"github.com/openziti/ziti/controller/permissions"
)

func Test_Permissions_ApiSession(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all api-session operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testApiSessionAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read api-session operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testApiSessionAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level ops permission allows all api-session operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testApiSessionEntityLevelPermissions(ctx)
	})

	t.Run("ops-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testApiSessionEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all api-session operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testApiSessionNoPermissions(ctx)
	})
}

// testApiSessionAdminPermissions tests that admin permission allows all api-session operations
func testApiSessionAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Create a test identity with a session
	testIdentitySession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Get the API session ID from the test identity's session
	apiSessionId := *testIdentitySession.AuthResponse.ID

	// Test read operation
	testHelper.getApiSession(ctx, adminSession, apiSessionId, http.StatusOK)

	// Test list operation
	testHelper.listApiSessions(ctx, adminSession, http.StatusOK)

	// Test delete operation
	testHelper.deleteApiSession(ctx, adminSession, apiSessionId, http.StatusOK)
}

// testApiSessionAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testApiSessionAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create a test identity with a session
	testIdentitySession := testHelper.newIdentityWithPermissions(ctx, []string{})
	apiSessionId := *testIdentitySession.AuthResponse.ID

	// Test read operations - should succeed
	testHelper.getApiSession(ctx, readonlySession, apiSessionId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listApiSessions(ctx, readonlySession, http.StatusOK)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteApiSession(ctx, readonlySession, apiSessionId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteApiSession(ctx, ctx.AdminManagementSession, apiSessionId, http.StatusOK)
}

// testApiSessionEntityLevelPermissions tests that entity-level ops permission allows all api-session operations
func testApiSessionEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with ops entity permission
	opsPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"ops"})

	// Create a test identity with a session
	testIdentitySession := testHelper.newIdentityWithPermissions(ctx, []string{})
	apiSessionId := *testIdentitySession.AuthResponse.ID

	// Test api-session operations - should all succeed
	testHelper.getApiSession(ctx, opsPermSession, apiSessionId, http.StatusOK)
	testHelper.listApiSessions(ctx, opsPermSession, http.StatusOK)
	testHelper.deleteApiSession(ctx, opsPermSession, apiSessionId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, opsPermSession, "ops-perm-identity", http.StatusUnauthorized)
}

// testApiSessionEntityActionPermissions tests that entity-action permissions allow specific operations
func testApiSessionEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test identity sessions for testing
	testIdentitySession1 := testHelper.newIdentityWithPermissions(ctx, []string{})
	apiSessionId1 := *testIdentitySession1.AuthResponse.ID

	testIdentitySession2 := testHelper.newIdentityWithPermissions(ctx, []string{})
	apiSessionId2 := *testIdentitySession2.AuthResponse.ID

	// Test ops.read permission
	readSession := testHelper.newIdentityWithPermissions(ctx, []string{"ops.read"})

	testHelper.getApiSession(ctx, readSession, apiSessionId1, http.StatusOK)
	testHelper.listApiSessions(ctx, readSession, http.StatusOK)

	// Read-only should not allow delete
	testHelper.deleteApiSession(ctx, readSession, apiSessionId1, http.StatusUnauthorized)

	// Test ops.delete permission
	deleteSession := testHelper.newIdentityWithPermissions(ctx, []string{"ops.delete"})

	testHelper.deleteApiSession(ctx, deleteSession, apiSessionId1, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getApiSession(ctx, deleteSession, apiSessionId2, http.StatusUnauthorized)

	// Delete-only should not allow list
	testHelper.listApiSessions(ctx, deleteSession, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteApiSession(ctx, ctx.AdminManagementSession, apiSessionId2, http.StatusOK)
}

// testApiSessionNoPermissions tests that no permissions deny all operations
func testApiSessionNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test identity with a session
	testIdentitySession := testHelper.newIdentityWithPermissions(ctx, []string{})
	apiSessionId := *testIdentitySession.AuthResponse.ID

	// All operations should fail with Unauthorized
	testHelper.getApiSession(ctx, noPermSession, apiSessionId, http.StatusUnauthorized)
	testHelper.listApiSessions(ctx, noPermSession, http.StatusUnauthorized)
	testHelper.deleteApiSession(ctx, noPermSession, apiSessionId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteApiSession(ctx, ctx.AdminManagementSession, apiSessionId, http.StatusOK)
}
