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

func Test_Permissions_PostureCheck(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all posture-check operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testPostureCheckAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read posture-check operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testPostureCheckAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level posture-check permission allows all CRUD for posture-checks", func(t *testing.T) {
		ctx.testContextChanged(t)
		testPostureCheckEntityLevelPermissions(ctx)
	})

	t.Run("posture-check-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testPostureCheckEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all posture-check operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testPostureCheckNoPermissions(ctx)
	})
}

// testPostureCheckAdminPermissions tests that admin permission allows all posture-check operations
func testPostureCheckAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testCheckId := testHelper.createPostureCheck(ctx, adminSession, "admin-test-posture-check", http.StatusCreated)

	// Test read operation
	testHelper.getPostureCheck(ctx, adminSession, testCheckId, http.StatusOK)

	// Test list operation
	testHelper.listPostureChecks(ctx, adminSession, http.StatusOK)

	// Test update operation
	patch := &rest_model.PostureCheckMfaPatch{}
	patch.SetTags(ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}))
	testHelper.patchPostureCheck(ctx, adminSession, testCheckId, patch, http.StatusOK)

	// Test delete operation
	testHelper.deletePostureCheck(ctx, adminSession, testCheckId, http.StatusOK)
}

// testPostureCheckAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testPostureCheckAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test posture-check as admin for readonly user to read
	testCheckId := testHelper.createPostureCheck(ctx, ctx.AdminManagementSession, "readonly-test-posture-check", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getPostureCheck(ctx, readonlySession, testCheckId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listPostureChecks(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createPostureCheck(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	patch := &rest_model.PostureCheckMfaPatch{}
	patch.SetTags(ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}))
	testHelper.patchPostureCheck(ctx, readonlySession, testCheckId, patch, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deletePostureCheck(ctx, readonlySession, testCheckId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deletePostureCheck(ctx, ctx.AdminManagementSession, testCheckId, http.StatusOK)
}

// testPostureCheckEntityLevelPermissions tests that entity-level posture-check permission allows all CRUD for posture-checks
func testPostureCheckEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with posture-check entity permission
	postureCheckPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"posture-check"})

	// Test posture-check CRUD - should all succeed
	testCheckId := testHelper.createPostureCheck(ctx, postureCheckPermSession, "entity-perm-test-posture-check", http.StatusCreated)
	testHelper.getPostureCheck(ctx, postureCheckPermSession, testCheckId, http.StatusOK)

	patch := &rest_model.PostureCheckMfaPatch{}
	patch.SetTags(ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}))
	testHelper.patchPostureCheck(ctx, postureCheckPermSession, testCheckId, patch, http.StatusOK)
	testHelper.deletePostureCheck(ctx, postureCheckPermSession, testCheckId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, postureCheckPermSession, "posture-check-perm-identity", http.StatusUnauthorized)
}

// testPostureCheckEntityActionPermissions tests that entity-action permissions allow specific operations
func testPostureCheckEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test posture-check as admin
	testCheckId := testHelper.createPostureCheck(ctx, ctx.AdminManagementSession, "action-test-posture-check", http.StatusCreated)

	// Test posture-check.read permission
	readSession := testHelper.newIdentityWithPermissions(ctx, []string{"posture-check.read"})

	testHelper.getPostureCheck(ctx, readSession, testCheckId, http.StatusOK)
	testHelper.listPostureChecks(ctx, readSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createPostureCheck(ctx, readSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	patch := &rest_model.PostureCheckMfaPatch{}
	patch.SetTags(ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}))
	testHelper.patchPostureCheck(ctx, readSession, testCheckId, patch, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deletePostureCheck(ctx, readSession, testCheckId, http.StatusUnauthorized)

	// Test posture-check.create permission
	createSession := testHelper.newIdentityWithPermissions(ctx, []string{"posture-check.create"})

	newCheckId := testHelper.createPostureCheck(ctx, createSession, "create-only-posture-check", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getPostureCheck(ctx, createSession, newCheckId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchPostureCheck(ctx, createSession, newCheckId, patch, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deletePostureCheck(ctx, createSession, newCheckId, http.StatusUnauthorized)

	// Test posture-check.update permission
	updateSession := testHelper.newIdentityWithPermissions(ctx, []string{"posture-check.update"})
	testHelper.patchPostureCheck(ctx, updateSession, newCheckId, patch, http.StatusOK)

	// Update-only should not allow read
	testHelper.getPostureCheck(ctx, updateSession, testCheckId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createPostureCheck(ctx, updateSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deletePostureCheck(ctx, updateSession, testCheckId, http.StatusUnauthorized)

	// Test posture-check.delete permission
	deleteSession := testHelper.newIdentityWithPermissions(ctx, []string{"posture-check.delete"})

	testHelper.deletePostureCheck(ctx, deleteSession, testCheckId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getPostureCheck(ctx, deleteSession, testCheckId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createPostureCheck(ctx, deleteSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deletePostureCheck(ctx, ctx.AdminManagementSession, newCheckId, http.StatusOK)
}

// testPostureCheckNoPermissions tests that no permissions deny all operations
func testPostureCheckNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test posture-check as admin
	testCheckId := testHelper.createPostureCheck(ctx, ctx.AdminManagementSession, "no-perm-test-posture-check", http.StatusCreated)

	// All operations should fail with Unauthorized

	// Posture check operations
	testHelper.getPostureCheck(ctx, noPermSession, testCheckId, http.StatusUnauthorized)

	testHelper.listPostureChecks(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createPostureCheck(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	patch := &rest_model.PostureCheckMfaPatch{}
	patch.SetTags(ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}))
	testHelper.patchPostureCheck(ctx, noPermSession, testCheckId, patch, http.StatusUnauthorized)

	testHelper.deletePostureCheck(ctx, noPermSession, testCheckId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deletePostureCheck(ctx, ctx.AdminManagementSession, testCheckId, http.StatusOK)
}
