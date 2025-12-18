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

func Test_Permissions_TransitRouter(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all transit router operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testTransitRouterAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read transit router operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testTransitRouterAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level router permission allows all CRUD for transit routers", func(t *testing.T) {
		ctx.testContextChanged(t)
		testTransitRouterEntityLevelPermissions(ctx)
	})

	t.Run("router-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testTransitRouterEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all transit router operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testTransitRouterNoPermissions(ctx)
	})
}

// testTransitRouterAdminPermissions tests that admin permission allows all transit router operations
func testTransitRouterAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testRouterId := testHelper.createTransitRouter(ctx, adminSession, "admin-test-transit-router", http.StatusCreated)

	// Test read operation
	testHelper.getTransitRouter(ctx, adminSession, testRouterId, http.StatusOK)

	// Test list operation
	testHelper.listTransitRouters(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchTransitRouter(ctx, adminSession, testRouterId, &rest_model.RouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteTransitRouter(ctx, adminSession, testRouterId, http.StatusOK)
}

// testTransitRouterAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testTransitRouterAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test transit router as admin for readonly user to read
	testRouterId := testHelper.createTransitRouter(ctx, ctx.AdminManagementSession, "readonly-test-transit-router", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getTransitRouter(ctx, readonlySession, testRouterId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listTransitRouters(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createTransitRouter(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchTransitRouter(ctx, readonlySession, testRouterId, &rest_model.RouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteTransitRouter(ctx, readonlySession, testRouterId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteTransitRouter(ctx, ctx.AdminManagementSession, testRouterId, http.StatusOK)
}

// testTransitRouterEntityLevelPermissions tests that entity-level router permission allows all CRUD for transit routers
func testTransitRouterEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with router entity permission
	routerPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"router"})

	// Test transit router CRUD - should all succeed
	testRouterId := testHelper.createTransitRouter(ctx, routerPermSession, "entity-perm-test-transit-router", http.StatusCreated)
	testHelper.getTransitRouter(ctx, routerPermSession, testRouterId, http.StatusOK)
	testHelper.patchTransitRouter(ctx, routerPermSession, testRouterId, &rest_model.RouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)
	testHelper.deleteTransitRouter(ctx, routerPermSession, testRouterId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, routerPermSession, "router-perm-identity", http.StatusUnauthorized)
}

// testTransitRouterEntityActionPermissions tests that entity-action permissions allow specific operations
func testTransitRouterEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test transit router as admin
	testRouterId := testHelper.createTransitRouter(ctx, ctx.AdminManagementSession, "action-test-transit-router", http.StatusCreated)

	// Test router.read permission
	readRouterSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.read"})

	testHelper.getTransitRouter(ctx, readRouterSession, testRouterId, http.StatusOK)
	testHelper.listTransitRouters(ctx, readRouterSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createTransitRouter(ctx, readRouterSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchTransitRouter(ctx, readRouterSession, testRouterId, &rest_model.RouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"read-only": true}}),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteTransitRouter(ctx, readRouterSession, testRouterId, http.StatusUnauthorized)

	// Test router.create permission
	createRouterSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.create"})

	newRouterId := testHelper.createTransitRouter(ctx, createRouterSession, "create-only-transit-router", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getTransitRouter(ctx, createRouterSession, newRouterId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchTransitRouter(ctx, createRouterSession, newRouterId, &rest_model.RouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"create-only": true}}),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteTransitRouter(ctx, createRouterSession, newRouterId, http.StatusUnauthorized)

	// Test router.update permission
	updateRouterSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.update"})
	testHelper.patchTransitRouter(ctx, updateRouterSession, newRouterId, &rest_model.RouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"update-only": true}}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getTransitRouter(ctx, updateRouterSession, testRouterId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createTransitRouter(ctx, updateRouterSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteTransitRouter(ctx, updateRouterSession, testRouterId, http.StatusUnauthorized)

	// Test router.delete permission
	deleteRouterSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.delete"})

	testHelper.deleteTransitRouter(ctx, deleteRouterSession, testRouterId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getTransitRouter(ctx, deleteRouterSession, testRouterId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createTransitRouter(ctx, deleteRouterSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteTransitRouter(ctx, ctx.AdminManagementSession, newRouterId, http.StatusOK)
}

// testTransitRouterNoPermissions tests that no permissions deny all operations
func testTransitRouterNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test transit router as admin
	testRouterId := testHelper.createTransitRouter(ctx, ctx.AdminManagementSession, "no-perm-test-transit-router", http.StatusCreated)

	// All operations should fail with Unauthorized

	// Transit router operations
	testHelper.getTransitRouter(ctx, noPermSession, testRouterId, http.StatusUnauthorized)

	testHelper.listTransitRouters(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createTransitRouter(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	testHelper.patchTransitRouter(ctx, noPermSession, testRouterId, &rest_model.RouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"no-perm": true}}),
	}, http.StatusUnauthorized)

	testHelper.deleteTransitRouter(ctx, noPermSession, testRouterId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteTransitRouter(ctx, ctx.AdminManagementSession, testRouterId, http.StatusOK)
}
