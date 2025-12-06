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

func Test_Permissions_Terminator(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all terminator operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testTerminatorAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read terminator operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testTerminatorAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level terminator permission allows all CRUD for terminators", func(t *testing.T) {
		ctx.testContextChanged(t)
		testTerminatorEntityLevelPermissions(ctx)
	})

	t.Run("terminator-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testTerminatorEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all terminator operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testTerminatorNoPermissions(ctx)
	})
}

// testTerminatorAdminPermissions tests that admin permission allows all terminator operations
func testTerminatorAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Create prerequisites: service and router
	serviceId := testHelper.createService(ctx, adminSession, "terminator-test-service", http.StatusCreated)
	routerId := testHelper.createTransitRouter(ctx, adminSession, "terminator-test-router", http.StatusCreated)

	// Test create operation
	testTerminatorId := testHelper.createTerminator(ctx, adminSession, serviceId, routerId, http.StatusCreated)

	// Test read operation
	testHelper.getTerminator(ctx, adminSession, testTerminatorId, http.StatusOK)

	// Test list operation
	testHelper.listTerminators(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchTerminator(ctx, adminSession, testTerminatorId, &rest_model.TerminatorPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteTerminator(ctx, adminSession, testTerminatorId, http.StatusOK)

	// Cleanup
	testHelper.deleteService(ctx, adminSession, serviceId, http.StatusOK)
	testHelper.deleteTransitRouter(ctx, adminSession, routerId, http.StatusOK)
}

// testTerminatorAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testTerminatorAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create prerequisites: service and router
	serviceId := testHelper.createService(ctx, ctx.AdminManagementSession, "readonly-terminator-test-service", http.StatusCreated)
	routerId := testHelper.createTransitRouter(ctx, ctx.AdminManagementSession, "readonly-terminator-test-router", http.StatusCreated)

	// Create test terminator as admin for readonly user to read
	testTerminatorId := testHelper.createTerminator(ctx, ctx.AdminManagementSession, serviceId, routerId, http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getTerminator(ctx, readonlySession, testTerminatorId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listTerminators(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createTerminator(ctx, readonlySession, serviceId, routerId, http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchTerminator(ctx, readonlySession, testTerminatorId, &rest_model.TerminatorPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteTerminator(ctx, readonlySession, testTerminatorId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteTerminator(ctx, ctx.AdminManagementSession, testTerminatorId, http.StatusOK)
	testHelper.deleteService(ctx, ctx.AdminManagementSession, serviceId, http.StatusOK)
	testHelper.deleteTransitRouter(ctx, ctx.AdminManagementSession, routerId, http.StatusOK)
}

// testTerminatorEntityLevelPermissions tests that entity-level terminator permission allows all CRUD for terminators
func testTerminatorEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with terminator entity permission
	terminatorPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"terminator"})

	// Create prerequisites: service and router (as admin, since terminator permission doesn't grant service/router creation)
	serviceId := testHelper.createService(ctx, ctx.AdminManagementSession, "entity-perm-terminator-test-service", http.StatusCreated)
	routerId := testHelper.createTransitRouter(ctx, ctx.AdminManagementSession, "entity-perm-terminator-test-router", http.StatusCreated)

	// Test terminator CRUD - should all succeed
	testTerminatorId := testHelper.createTerminator(ctx, terminatorPermSession, serviceId, routerId, http.StatusCreated)
	testHelper.getTerminator(ctx, terminatorPermSession, testTerminatorId, http.StatusOK)
	testHelper.patchTerminator(ctx, terminatorPermSession, testTerminatorId, &rest_model.TerminatorPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)
	testHelper.deleteTerminator(ctx, terminatorPermSession, testTerminatorId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, terminatorPermSession, "terminator-perm-identity", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteService(ctx, ctx.AdminManagementSession, serviceId, http.StatusOK)
	testHelper.deleteTransitRouter(ctx, ctx.AdminManagementSession, routerId, http.StatusOK)
}

// testTerminatorEntityActionPermissions tests that entity-action permissions allow specific operations
func testTerminatorEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create prerequisites: service and router
	serviceId := testHelper.createService(ctx, ctx.AdminManagementSession, "action-terminator-test-service", http.StatusCreated)
	routerId := testHelper.createTransitRouter(ctx, ctx.AdminManagementSession, "action-terminator-test-router", http.StatusCreated)

	// Create test terminator as admin
	testTerminatorId := testHelper.createTerminator(ctx, ctx.AdminManagementSession, serviceId, routerId, http.StatusCreated)

	// Test terminator.read permission
	readSession := testHelper.newIdentityWithPermissions(ctx, []string{"terminator.read"})

	testHelper.getTerminator(ctx, readSession, testTerminatorId, http.StatusOK)
	testHelper.listTerminators(ctx, readSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createTerminator(ctx, readSession, serviceId, routerId, http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchTerminator(ctx, readSession, testTerminatorId, &rest_model.TerminatorPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"read-only": true}}),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteTerminator(ctx, readSession, testTerminatorId, http.StatusUnauthorized)

	// Test terminator.create permission
	createSession := testHelper.newIdentityWithPermissions(ctx, []string{"terminator.create"})

	newTerminatorId := testHelper.createTerminator(ctx, createSession, serviceId, routerId, http.StatusCreated)

	// Create-only should not allow read
	testHelper.getTerminator(ctx, createSession, newTerminatorId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchTerminator(ctx, createSession, newTerminatorId, &rest_model.TerminatorPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"create-only": true}}),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteTerminator(ctx, createSession, newTerminatorId, http.StatusUnauthorized)

	// Test terminator.update permission
	updateSession := testHelper.newIdentityWithPermissions(ctx, []string{"terminator.update"})
	testHelper.patchTerminator(ctx, updateSession, newTerminatorId, &rest_model.TerminatorPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"update-only": true}}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getTerminator(ctx, updateSession, testTerminatorId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createTerminator(ctx, updateSession, serviceId, routerId, http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteTerminator(ctx, updateSession, testTerminatorId, http.StatusUnauthorized)

	// Test terminator.delete permission
	deleteSession := testHelper.newIdentityWithPermissions(ctx, []string{"terminator.delete"})

	testHelper.deleteTerminator(ctx, deleteSession, testTerminatorId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getTerminator(ctx, deleteSession, testTerminatorId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createTerminator(ctx, deleteSession, serviceId, routerId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteTerminator(ctx, ctx.AdminManagementSession, newTerminatorId, http.StatusOK)
	testHelper.deleteService(ctx, ctx.AdminManagementSession, serviceId, http.StatusOK)
	testHelper.deleteTransitRouter(ctx, ctx.AdminManagementSession, routerId, http.StatusOK)
}

// testTerminatorNoPermissions tests that no permissions deny all operations
func testTerminatorNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create prerequisites: service and router
	serviceId := testHelper.createService(ctx, ctx.AdminManagementSession, "no-perm-terminator-test-service", http.StatusCreated)
	routerId := testHelper.createTransitRouter(ctx, ctx.AdminManagementSession, "no-perm-terminator-test-router", http.StatusCreated)

	// Create test terminator as admin
	testTerminatorId := testHelper.createTerminator(ctx, ctx.AdminManagementSession, serviceId, routerId, http.StatusCreated)

	// All operations should fail with Unauthorized
	testHelper.getTerminator(ctx, noPermSession, testTerminatorId, http.StatusUnauthorized)
	testHelper.listTerminators(ctx, noPermSession, http.StatusUnauthorized)
	testHelper.createTerminator(ctx, noPermSession, serviceId, routerId, http.StatusUnauthorized)
	testHelper.patchTerminator(ctx, noPermSession, testTerminatorId, &rest_model.TerminatorPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"no-perm": true}}),
	}, http.StatusUnauthorized)
	testHelper.deleteTerminator(ctx, noPermSession, testTerminatorId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteTerminator(ctx, ctx.AdminManagementSession, testTerminatorId, http.StatusOK)
	testHelper.deleteService(ctx, ctx.AdminManagementSession, serviceId, http.StatusOK)
	testHelper.deleteTransitRouter(ctx, ctx.AdminManagementSession, routerId, http.StatusOK)
}
