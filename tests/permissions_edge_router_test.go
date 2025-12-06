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

func Test_Permissions_EdgeRouter(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all edge router operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read edge router operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level router permission allows all CRUD for edge routers", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterEntityLevelPermissions(ctx)
	})

	t.Run("router-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all edge router operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterNoPermissions(ctx)
	})

	t.Run("edge router list operations require specific permissions", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterListOperationsPermissions(ctx)
	})
}

// testEdgeRouterAdminPermissions tests that admin permission allows all edge router operations
func testEdgeRouterAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testEdgeRouterId := testHelper.createEdgeRouter(ctx, adminSession, "admin-test-edge-router", http.StatusCreated)

	// Test read operation
	testHelper.getEdgeRouter(ctx, adminSession, testEdgeRouterId, http.StatusOK)

	// Test list operation
	testHelper.listEdgeRouters(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchEdgeRouter(ctx, adminSession, testEdgeRouterId, &rest_model.EdgeRouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteEdgeRouter(ctx, adminSession, testEdgeRouterId, http.StatusOK)
}

// testEdgeRouterAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testEdgeRouterAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test edge router as admin for readonly user to read
	testEdgeRouterId := testHelper.createEdgeRouter(ctx, ctx.AdminManagementSession, "readonly-test-edge-router", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getEdgeRouter(ctx, readonlySession, testEdgeRouterId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listEdgeRouters(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createEdgeRouter(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchEdgeRouter(ctx, readonlySession, testEdgeRouterId, &rest_model.EdgeRouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteEdgeRouter(ctx, readonlySession, testEdgeRouterId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteEdgeRouter(ctx, ctx.AdminManagementSession, testEdgeRouterId, http.StatusOK)
}

// testEdgeRouterEntityLevelPermissions tests that entity-level router permission allows all CRUD for edge routers
func testEdgeRouterEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with router entity permission
	routerPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"router"})

	// Test edge router CRUD - should all succeed
	testEdgeRouterId := testHelper.createEdgeRouter(ctx, routerPermSession, "entity-perm-test-edge-router", http.StatusCreated)
	testHelper.getEdgeRouter(ctx, routerPermSession, testEdgeRouterId, http.StatusOK)
	testHelper.patchEdgeRouter(ctx, routerPermSession, testEdgeRouterId, &rest_model.EdgeRouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)
	testHelper.deleteEdgeRouter(ctx, routerPermSession, testEdgeRouterId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, routerPermSession, "router-perm-identity", http.StatusUnauthorized)
}

// testEdgeRouterEntityActionPermissions tests that entity-action permissions allow specific operations
func testEdgeRouterEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test edge router as admin
	testEdgeRouterId := testHelper.createEdgeRouter(ctx, ctx.AdminManagementSession, "action-test-edge-router", http.StatusCreated)

	// Test router.read permission
	readRouterSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.read"})

	testHelper.getEdgeRouter(ctx, readRouterSession, testEdgeRouterId, http.StatusOK)
	testHelper.listEdgeRouters(ctx, readRouterSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createEdgeRouter(ctx, readRouterSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchEdgeRouter(ctx, readRouterSession, testEdgeRouterId, &rest_model.EdgeRouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"read-only": true}}),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteEdgeRouter(ctx, readRouterSession, testEdgeRouterId, http.StatusUnauthorized)

	// Test router.create permission
	createRouterSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.create"})

	newEdgeRouterId := testHelper.createEdgeRouter(ctx, createRouterSession, "create-only-edge-router", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getEdgeRouter(ctx, createRouterSession, newEdgeRouterId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchEdgeRouter(ctx, createRouterSession, newEdgeRouterId, &rest_model.EdgeRouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"create-only": true}}),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteEdgeRouter(ctx, createRouterSession, newEdgeRouterId, http.StatusUnauthorized)

	// Test router.update permission
	updateRouterSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.update"})
	testHelper.patchEdgeRouter(ctx, updateRouterSession, newEdgeRouterId, &rest_model.EdgeRouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"update-only": true}}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getEdgeRouter(ctx, updateRouterSession, testEdgeRouterId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createEdgeRouter(ctx, updateRouterSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteEdgeRouter(ctx, updateRouterSession, testEdgeRouterId, http.StatusUnauthorized)

	// Test router.delete permission
	deleteRouterSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.delete"})

	testHelper.deleteEdgeRouter(ctx, deleteRouterSession, testEdgeRouterId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getEdgeRouter(ctx, deleteRouterSession, testEdgeRouterId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createEdgeRouter(ctx, deleteRouterSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteEdgeRouter(ctx, ctx.AdminManagementSession, newEdgeRouterId, http.StatusOK)
}

// testEdgeRouterNoPermissions tests that no permissions deny all operations
func testEdgeRouterNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test edge router as admin
	testEdgeRouterId := testHelper.createEdgeRouter(ctx, ctx.AdminManagementSession, "no-perm-test-edge-router", http.StatusCreated)

	// All operations should fail with Unauthorized

	// Edge router operations
	testHelper.getEdgeRouter(ctx, noPermSession, testEdgeRouterId, http.StatusUnauthorized)

	testHelper.listEdgeRouters(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createEdgeRouter(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	testHelper.patchEdgeRouter(ctx, noPermSession, testEdgeRouterId, &rest_model.EdgeRouterPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"no-perm": true}}),
	}, http.StatusUnauthorized)

	testHelper.deleteEdgeRouter(ctx, noPermSession, testEdgeRouterId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteEdgeRouter(ctx, ctx.AdminManagementSession, testEdgeRouterId, http.StatusOK)
}

// testEdgeRouterListOperationsPermissions tests that edge router list operations require specific permissions
func testEdgeRouterListOperationsPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test edge router as admin
	testEdgeRouterId := testHelper.createEdgeRouter(ctx, ctx.AdminManagementSession, "list-ops-test-edge-router", http.StatusCreated)

	// Test listing edge router policies - requires edge-router-policy.read
	routerReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.read"})
	testHelper.listEdgeRouterPoliciesForEdgeRouter(ctx, routerReadSession, testEdgeRouterId, http.StatusUnauthorized)

	edgeRouterPolicyReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"edge-router-policy.read"})
	testHelper.listEdgeRouterPoliciesForEdgeRouter(ctx, edgeRouterPolicyReadSession, testEdgeRouterId, http.StatusOK)

	// Test listing service edge router policies - requires service-edge-router-policy.read
	testHelper.listServiceEdgeRouterPoliciesForEdgeRouter(ctx, routerReadSession, testEdgeRouterId, http.StatusUnauthorized)

	serviceEdgeRouterPolicyReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-edge-router-policy.read"})
	testHelper.listServiceEdgeRouterPoliciesForEdgeRouter(ctx, serviceEdgeRouterPolicyReadSession, testEdgeRouterId, http.StatusOK)

	// Test listing identities - requires identity.read
	testHelper.listIdentitiesForEdgeRouter(ctx, routerReadSession, testEdgeRouterId, http.StatusUnauthorized)

	identityReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"identity.read"})
	testHelper.listIdentitiesForEdgeRouter(ctx, identityReadSession, testEdgeRouterId, http.StatusOK)

	// Test listing services - requires service.read
	testHelper.listServicesForEdgeRouter(ctx, routerReadSession, testEdgeRouterId, http.StatusUnauthorized)

	serviceReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.read"})
	testHelper.listServicesForEdgeRouter(ctx, serviceReadSession, testEdgeRouterId, http.StatusOK)

	// Cleanup
	testHelper.deleteEdgeRouter(ctx, ctx.AdminManagementSession, testEdgeRouterId, http.StatusOK)
}
