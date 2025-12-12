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

func Test_Permissions_Enrollment(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all enrollment operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEnrollmentAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read enrollment operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEnrollmentAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level enrollment permission allows all CRUD for enrollments", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEnrollmentEntityLevelPermissions(ctx)
	})

	t.Run("enrollment-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEnrollmentEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all enrollment operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEnrollmentNoPermissions(ctx)
	})
}

// testEnrollmentAdminPermissions tests that admin permission allows all enrollment operations
func testEnrollmentAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Create a test identity without enrollment
	testIdentityId := testHelper.createIdentityWithoutEnrollment(ctx, adminSession, "enrollment-test-identity", http.StatusCreated)

	// Test create operation
	enrollmentId := testHelper.createEnrollment(ctx, adminSession, testIdentityId, http.StatusCreated)

	// Test read operation
	testHelper.getEnrollment(ctx, adminSession, enrollmentId, http.StatusOK)

	// Test list operation
	testHelper.listEnrollments(ctx, adminSession, http.StatusOK)

	// Test refresh operation
	testHelper.refreshEnrollment(ctx, adminSession, enrollmentId, http.StatusOK)

	// Test delete operation
	testHelper.deleteEnrollment(ctx, adminSession, enrollmentId, http.StatusOK)

	// Cleanup
	testHelper.deleteIdentity(ctx, adminSession, testIdentityId, http.StatusOK)
}

// testEnrollmentAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testEnrollmentAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test identity as admin without enrollment
	testIdentityId := testHelper.createIdentityWithoutEnrollment(ctx, ctx.AdminManagementSession, "readonly-enrollment-test-identity", http.StatusCreated)

	// Create enrollment as admin
	enrollmentId := testHelper.createEnrollment(ctx, ctx.AdminManagementSession, testIdentityId, http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getEnrollment(ctx, readonlySession, enrollmentId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listEnrollments(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createEnrollment(ctx, readonlySession, testIdentityId, http.StatusUnauthorized)

	// Test refresh operations - should fail with Unauthorized
	testHelper.refreshEnrollment(ctx, readonlySession, enrollmentId, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteEnrollment(ctx, readonlySession, enrollmentId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteEnrollment(ctx, ctx.AdminManagementSession, enrollmentId, http.StatusOK)
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, testIdentityId, http.StatusOK)
}

// testEnrollmentEntityLevelPermissions tests that entity-level enrollment permission allows all CRUD for enrollments
func testEnrollmentEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with enrollment entity permission
	enrollmentPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"enrollment"})

	// Create test identity as admin without enrollment (entity-level enrollment permission doesn't grant identity creation)
	testIdentityId := testHelper.createIdentityWithoutEnrollment(ctx, ctx.AdminManagementSession, "entity-perm-enrollment-test-identity", http.StatusCreated)

	// Test enrollment CRUD - should all succeed
	enrollmentId := testHelper.createEnrollment(ctx, enrollmentPermSession, testIdentityId, http.StatusCreated)
	testHelper.getEnrollment(ctx, enrollmentPermSession, enrollmentId, http.StatusOK)
	testHelper.listEnrollments(ctx, enrollmentPermSession, http.StatusOK)
	testHelper.refreshEnrollment(ctx, enrollmentPermSession, enrollmentId, http.StatusOK)
	testHelper.deleteEnrollment(ctx, enrollmentPermSession, enrollmentId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, enrollmentPermSession, "enrollment-perm-identity", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, testIdentityId, http.StatusOK)
}

// testEnrollmentEntityActionPermissions tests that entity-action permissions allow specific operations
func testEnrollmentEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test identities as admin without enrollments
	testIdentityId1 := testHelper.createIdentityWithoutEnrollment(ctx, ctx.AdminManagementSession, "action-test-identity-1", http.StatusCreated)
	testIdentityId2 := testHelper.createIdentityWithoutEnrollment(ctx, ctx.AdminManagementSession, "action-test-identity-2", http.StatusCreated)
	testIdentityId3 := testHelper.createIdentityWithoutEnrollment(ctx, ctx.AdminManagementSession, "action-test-identity-3", http.StatusCreated)

	// Create enrollments as admin
	enrollmentId1 := testHelper.createEnrollment(ctx, ctx.AdminManagementSession, testIdentityId1, http.StatusCreated)
	enrollmentId2 := testHelper.createEnrollment(ctx, ctx.AdminManagementSession, testIdentityId2, http.StatusCreated)

	// Test enrollment.read permission
	readEnrollmentSession := testHelper.newIdentityWithPermissions(ctx, []string{"enrollment.read"})

	testHelper.getEnrollment(ctx, readEnrollmentSession, enrollmentId1, http.StatusOK)
	testHelper.listEnrollments(ctx, readEnrollmentSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createEnrollment(ctx, readEnrollmentSession, testIdentityId3, http.StatusUnauthorized)

	// Read-only should not allow refresh
	testHelper.refreshEnrollment(ctx, readEnrollmentSession, enrollmentId1, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteEnrollment(ctx, readEnrollmentSession, enrollmentId1, http.StatusUnauthorized)

	// Test enrollment.create permission
	createEnrollmentSession := testHelper.newIdentityWithPermissions(ctx, []string{"enrollment.create"})

	newEnrollmentId := testHelper.createEnrollment(ctx, createEnrollmentSession, testIdentityId3, http.StatusCreated)

	// Create-only should not allow read
	testHelper.getEnrollment(ctx, createEnrollmentSession, newEnrollmentId, http.StatusUnauthorized)

	// Create-only should not allow refresh
	testHelper.refreshEnrollment(ctx, createEnrollmentSession, newEnrollmentId, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteEnrollment(ctx, createEnrollmentSession, newEnrollmentId, http.StatusUnauthorized)

	// Test enrollment.update permission (for refresh operation)
	updateEnrollmentSession := testHelper.newIdentityWithPermissions(ctx, []string{"enrollment.update"})

	// Update-only should allow refresh
	testHelper.refreshEnrollment(ctx, updateEnrollmentSession, enrollmentId1, http.StatusOK)

	// Update-only should not allow read
	testHelper.getEnrollment(ctx, updateEnrollmentSession, enrollmentId1, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createEnrollment(ctx, updateEnrollmentSession, testIdentityId3, http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteEnrollment(ctx, updateEnrollmentSession, enrollmentId1, http.StatusUnauthorized)

	// Test enrollment.delete permission
	deleteEnrollmentSession := testHelper.newIdentityWithPermissions(ctx, []string{"enrollment.delete"})

	testHelper.deleteEnrollment(ctx, deleteEnrollmentSession, enrollmentId1, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getEnrollment(ctx, deleteEnrollmentSession, enrollmentId1, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createEnrollment(ctx, deleteEnrollmentSession, testIdentityId1, http.StatusUnauthorized)

	// Delete-only should not allow refresh
	testHelper.refreshEnrollment(ctx, deleteEnrollmentSession, enrollmentId2, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteEnrollment(ctx, ctx.AdminManagementSession, enrollmentId2, http.StatusOK)
	testHelper.deleteEnrollment(ctx, ctx.AdminManagementSession, newEnrollmentId, http.StatusOK)
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, testIdentityId1, http.StatusOK)
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, testIdentityId2, http.StatusOK)
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, testIdentityId3, http.StatusOK)
}

// testEnrollmentNoPermissions tests that no permissions deny all operations
func testEnrollmentNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test identity as admin without enrollment
	testIdentityId := testHelper.createIdentityWithoutEnrollment(ctx, ctx.AdminManagementSession, "no-perm-enrollment-test-identity", http.StatusCreated)

	// Create enrollment as admin
	enrollmentId := testHelper.createEnrollment(ctx, ctx.AdminManagementSession, testIdentityId, http.StatusCreated)

	// All operations should fail with Unauthorized

	// Enrollment operations
	testHelper.getEnrollment(ctx, noPermSession, enrollmentId, http.StatusUnauthorized)

	testHelper.listEnrollments(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createEnrollment(ctx, noPermSession, testIdentityId, http.StatusUnauthorized)

	testHelper.refreshEnrollment(ctx, noPermSession, enrollmentId, http.StatusUnauthorized)

	testHelper.deleteEnrollment(ctx, noPermSession, enrollmentId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteEnrollment(ctx, ctx.AdminManagementSession, enrollmentId, http.StatusOK)
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, testIdentityId, http.StatusOK)
}
