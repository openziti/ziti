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

func Test_Permissions_EdgeRouterPolicy(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all edge-router-policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterPolicyAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read edge-router-policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterPolicyAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level edge-router-policy permission allows all CRUD for edge-router-policies", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterPolicyEntityLevelPermissions(ctx)
	})

	t.Run("edge-router-policy-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterPolicyEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all edge-router-policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterPolicyNoPermissions(ctx)
	})

	t.Run("edge-router-policy list operations require specific permissions", func(t *testing.T) {
		ctx.testContextChanged(t)
		testEdgeRouterPolicyListOperationsPermissions(ctx)
	})
}

// testEdgeRouterPolicyAdminPermissions tests that admin permission allows all edge-router-policy operations
func testEdgeRouterPolicyAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testPolicyId := testHelper.createEdgeRouterPolicy(ctx, adminSession, "admin-test-edge-router-policy", http.StatusCreated)

	// Test read operation
	testHelper.getEdgeRouterPolicy(ctx, adminSession, testPolicyId, http.StatusOK)

	// Test list operation
	testHelper.listEdgeRouterPolicies(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchEdgeRouterPolicy(ctx, adminSession, testPolicyId, &rest_model.EdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteEdgeRouterPolicy(ctx, adminSession, testPolicyId, http.StatusOK)
}

// testEdgeRouterPolicyAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testEdgeRouterPolicyAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test edge-router-policy as admin for readonly user to read
	testPolicyId := testHelper.createEdgeRouterPolicy(ctx, ctx.AdminManagementSession, "readonly-test-edge-router-policy", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getEdgeRouterPolicy(ctx, readonlySession, testPolicyId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listEdgeRouterPolicies(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createEdgeRouterPolicy(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchEdgeRouterPolicy(ctx, readonlySession, testPolicyId, &rest_model.EdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteEdgeRouterPolicy(ctx, readonlySession, testPolicyId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteEdgeRouterPolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
}

// testEdgeRouterPolicyEntityLevelPermissions tests that entity-level edge-router-policy permission allows all CRUD for edge-router-policies
func testEdgeRouterPolicyEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with edge-router-policy entity permission
	policyPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"edge-router-policy"})

	// Test edge-router-policy CRUD - should all succeed
	testPolicyId := testHelper.createEdgeRouterPolicy(ctx, policyPermSession, "entity-perm-test-edge-router-policy", http.StatusCreated)
	testHelper.getEdgeRouterPolicy(ctx, policyPermSession, testPolicyId, http.StatusOK)
	testHelper.patchEdgeRouterPolicy(ctx, policyPermSession, testPolicyId, &rest_model.EdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)
	testHelper.deleteEdgeRouterPolicy(ctx, policyPermSession, testPolicyId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, policyPermSession, "edge-router-policy-perm-identity", http.StatusUnauthorized)
}

// testEdgeRouterPolicyEntityActionPermissions tests that entity-action permissions allow specific operations
func testEdgeRouterPolicyEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test edge-router-policy as admin
	testPolicyId := testHelper.createEdgeRouterPolicy(ctx, ctx.AdminManagementSession, "action-test-edge-router-policy", http.StatusCreated)

	// Test edge-router-policy.read permission
	readSession := testHelper.newIdentityWithPermissions(ctx, []string{"edge-router-policy.read"})

	testHelper.getEdgeRouterPolicy(ctx, readSession, testPolicyId, http.StatusOK)
	testHelper.listEdgeRouterPolicies(ctx, readSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createEdgeRouterPolicy(ctx, readSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchEdgeRouterPolicy(ctx, readSession, testPolicyId, &rest_model.EdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"read-only": true}}),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteEdgeRouterPolicy(ctx, readSession, testPolicyId, http.StatusUnauthorized)

	// Test edge-router-policy.create permission
	createSession := testHelper.newIdentityWithPermissions(ctx, []string{"edge-router-policy.create"})

	newPolicyId := testHelper.createEdgeRouterPolicy(ctx, createSession, "create-only-edge-router-policy", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getEdgeRouterPolicy(ctx, createSession, newPolicyId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchEdgeRouterPolicy(ctx, createSession, newPolicyId, &rest_model.EdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"create-only": true}}),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteEdgeRouterPolicy(ctx, createSession, newPolicyId, http.StatusUnauthorized)

	// Test edge-router-policy.update permission
	updateSession := testHelper.newIdentityWithPermissions(ctx, []string{"edge-router-policy.update"})
	testHelper.patchEdgeRouterPolicy(ctx, updateSession, newPolicyId, &rest_model.EdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"update-only": true}}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getEdgeRouterPolicy(ctx, updateSession, testPolicyId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createEdgeRouterPolicy(ctx, updateSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteEdgeRouterPolicy(ctx, updateSession, testPolicyId, http.StatusUnauthorized)

	// Test edge-router-policy.delete permission
	deleteSession := testHelper.newIdentityWithPermissions(ctx, []string{"edge-router-policy.delete"})

	testHelper.deleteEdgeRouterPolicy(ctx, deleteSession, testPolicyId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getEdgeRouterPolicy(ctx, deleteSession, testPolicyId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createEdgeRouterPolicy(ctx, deleteSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteEdgeRouterPolicy(ctx, ctx.AdminManagementSession, newPolicyId, http.StatusOK)
}

// testEdgeRouterPolicyNoPermissions tests that no permissions deny all operations
func testEdgeRouterPolicyNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test edge-router-policy as admin
	testPolicyId := testHelper.createEdgeRouterPolicy(ctx, ctx.AdminManagementSession, "no-perm-test-edge-router-policy", http.StatusCreated)

	// All operations should fail with Unauthorized

	// EdgeRouterPolicy operations
	testHelper.getEdgeRouterPolicy(ctx, noPermSession, testPolicyId, http.StatusUnauthorized)

	testHelper.listEdgeRouterPolicies(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createEdgeRouterPolicy(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	testHelper.patchEdgeRouterPolicy(ctx, noPermSession, testPolicyId, &rest_model.EdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"no-perm": true}}),
	}, http.StatusUnauthorized)

	testHelper.deleteEdgeRouterPolicy(ctx, noPermSession, testPolicyId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteEdgeRouterPolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
}

// testEdgeRouterPolicyListOperationsPermissions tests that edge-router-policy list operations require specific permissions
func testEdgeRouterPolicyListOperationsPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test edge-router-policy as admin
	testPolicyId := testHelper.createEdgeRouterPolicy(ctx, ctx.AdminManagementSession, "list-ops-test-edge-router-policy", http.StatusCreated)

	// Test listing edge routers for edge-router-policy - requires router.read, NOT edge-router-policy.read
	policyReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"edge-router-policy.read"})

	// Negative test: edge-router-policy.read alone should NOT allow listing edge routers
	testHelper.listEdgeRoutersForEdgeRouterPolicy(ctx, policyReadSession, testPolicyId, http.StatusUnauthorized)

	// Positive test: router.read should allow listing edge routers
	routerReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.read"})
	testHelper.listEdgeRoutersForEdgeRouterPolicy(ctx, routerReadSession, testPolicyId, http.StatusOK)

	// Test listing identities for edge-router-policy - requires identity.read, NOT edge-router-policy.read

	// Negative test: edge-router-policy.read alone should NOT allow listing identities
	testHelper.listIdentitiesForEdgeRouterPolicy(ctx, policyReadSession, testPolicyId, http.StatusUnauthorized)

	// Positive test: identity.read should allow listing identities
	identityReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"identity.read"})
	testHelper.listIdentitiesForEdgeRouterPolicy(ctx, identityReadSession, testPolicyId, http.StatusOK)

	// Test with admin - should succeed for both
	testHelper.listEdgeRoutersForEdgeRouterPolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
	testHelper.listIdentitiesForEdgeRouterPolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)

	// Cleanup
	testHelper.deleteEdgeRouterPolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
}
