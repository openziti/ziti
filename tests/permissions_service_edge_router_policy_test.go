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

func Test_Permissions_ServiceEdgeRouterPolicy(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all service-edge-router-policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceEdgeRouterPolicyAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read service-edge-router-policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceEdgeRouterPolicyAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level service-edge-router-policy permission allows all CRUD for service-edge-router-policies", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceEdgeRouterPolicyEntityLevelPermissions(ctx)
	})

	t.Run("service-edge-router-policy-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceEdgeRouterPolicyEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all service-edge-router-policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceEdgeRouterPolicyNoPermissions(ctx)
	})

	t.Run("service-edge-router-policy list operations require specific permissions", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceEdgeRouterPolicyListOperationsPermissions(ctx)
	})
}

// testServiceEdgeRouterPolicyAdminPermissions tests that admin permission allows all service-edge-router-policy operations
func testServiceEdgeRouterPolicyAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testPolicyId := testHelper.createServiceEdgeRouterPolicy(ctx, adminSession, "admin-test-service-edge-router-policy", http.StatusCreated)

	// Test read operation
	testHelper.getServiceEdgeRouterPolicy(ctx, adminSession, testPolicyId, http.StatusOK)

	// Test list operation
	testHelper.listServiceEdgeRouterPolicies(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchServiceEdgeRouterPolicy(ctx, adminSession, testPolicyId, &rest_model.ServiceEdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteServiceEdgeRouterPolicy(ctx, adminSession, testPolicyId, http.StatusOK)
}

// testServiceEdgeRouterPolicyAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testServiceEdgeRouterPolicyAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test service-edge-router-policy as admin for readonly user to read
	testPolicyId := testHelper.createServiceEdgeRouterPolicy(ctx, ctx.AdminManagementSession, "readonly-test-service-edge-router-policy", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getServiceEdgeRouterPolicy(ctx, readonlySession, testPolicyId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listServiceEdgeRouterPolicies(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createServiceEdgeRouterPolicy(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchServiceEdgeRouterPolicy(ctx, readonlySession, testPolicyId, &rest_model.ServiceEdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteServiceEdgeRouterPolicy(ctx, readonlySession, testPolicyId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteServiceEdgeRouterPolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
}

// testServiceEdgeRouterPolicyEntityLevelPermissions tests that entity-level service-edge-router-policy permission allows all CRUD for service-edge-router-policies
func testServiceEdgeRouterPolicyEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with service-edge-router-policy entity permission
	policyPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-edge-router-policy"})

	// Test service-edge-router-policy CRUD - should all succeed
	testPolicyId := testHelper.createServiceEdgeRouterPolicy(ctx, policyPermSession, "entity-perm-test-service-edge-router-policy", http.StatusCreated)
	testHelper.getServiceEdgeRouterPolicy(ctx, policyPermSession, testPolicyId, http.StatusOK)
	testHelper.patchServiceEdgeRouterPolicy(ctx, policyPermSession, testPolicyId, &rest_model.ServiceEdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)
	testHelper.deleteServiceEdgeRouterPolicy(ctx, policyPermSession, testPolicyId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, policyPermSession, "service-edge-router-policy-perm-identity", http.StatusUnauthorized)
}

// testServiceEdgeRouterPolicyEntityActionPermissions tests that entity-action permissions allow specific operations
func testServiceEdgeRouterPolicyEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test service-edge-router-policy as admin
	testPolicyId := testHelper.createServiceEdgeRouterPolicy(ctx, ctx.AdminManagementSession, "action-test-service-edge-router-policy", http.StatusCreated)

	// Test service-edge-router-policy.read permission
	readSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-edge-router-policy.read"})

	testHelper.getServiceEdgeRouterPolicy(ctx, readSession, testPolicyId, http.StatusOK)
	testHelper.listServiceEdgeRouterPolicies(ctx, readSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createServiceEdgeRouterPolicy(ctx, readSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchServiceEdgeRouterPolicy(ctx, readSession, testPolicyId, &rest_model.ServiceEdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"read-only": true}}),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteServiceEdgeRouterPolicy(ctx, readSession, testPolicyId, http.StatusUnauthorized)

	// Test service-edge-router-policy.create permission
	createSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-edge-router-policy.create"})

	newPolicyId := testHelper.createServiceEdgeRouterPolicy(ctx, createSession, "create-only-service-edge-router-policy", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getServiceEdgeRouterPolicy(ctx, createSession, newPolicyId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchServiceEdgeRouterPolicy(ctx, createSession, newPolicyId, &rest_model.ServiceEdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"create-only": true}}),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteServiceEdgeRouterPolicy(ctx, createSession, newPolicyId, http.StatusUnauthorized)

	// Test service-edge-router-policy.update permission
	updateSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-edge-router-policy.update"})
	testHelper.patchServiceEdgeRouterPolicy(ctx, updateSession, newPolicyId, &rest_model.ServiceEdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"update-only": true}}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getServiceEdgeRouterPolicy(ctx, updateSession, testPolicyId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createServiceEdgeRouterPolicy(ctx, updateSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteServiceEdgeRouterPolicy(ctx, updateSession, testPolicyId, http.StatusUnauthorized)

	// Test service-edge-router-policy.delete permission
	deleteSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-edge-router-policy.delete"})

	testHelper.deleteServiceEdgeRouterPolicy(ctx, deleteSession, testPolicyId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getServiceEdgeRouterPolicy(ctx, deleteSession, testPolicyId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createServiceEdgeRouterPolicy(ctx, deleteSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteServiceEdgeRouterPolicy(ctx, ctx.AdminManagementSession, newPolicyId, http.StatusOK)
}

// testServiceEdgeRouterPolicyNoPermissions tests that no permissions deny all operations
func testServiceEdgeRouterPolicyNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test service-edge-router-policy as admin
	testPolicyId := testHelper.createServiceEdgeRouterPolicy(ctx, ctx.AdminManagementSession, "no-perm-test-service-edge-router-policy", http.StatusCreated)

	// All operations should fail with Unauthorized
	testHelper.getServiceEdgeRouterPolicy(ctx, noPermSession, testPolicyId, http.StatusUnauthorized)
	testHelper.listServiceEdgeRouterPolicies(ctx, noPermSession, http.StatusUnauthorized)
	testHelper.createServiceEdgeRouterPolicy(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)
	testHelper.patchServiceEdgeRouterPolicy(ctx, noPermSession, testPolicyId, &rest_model.ServiceEdgeRouterPolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"no-perm": true}}),
	}, http.StatusUnauthorized)
	testHelper.deleteServiceEdgeRouterPolicy(ctx, noPermSession, testPolicyId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteServiceEdgeRouterPolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
}

// testServiceEdgeRouterPolicyListOperationsPermissions tests that service-edge-router-policy list operations require specific permissions
func testServiceEdgeRouterPolicyListOperationsPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test service-edge-router-policy as admin
	testPolicyId := testHelper.createServiceEdgeRouterPolicy(ctx, ctx.AdminManagementSession, "list-ops-test-service-edge-router-policy", http.StatusCreated)

	// Test listing edge routers for service-edge-router-policy - requires router.read
	policyReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-edge-router-policy.read"})
	testHelper.listEdgeRoutersForServiceEdgeRouterPolicy(ctx, policyReadSession, testPolicyId, http.StatusUnauthorized)

	routerReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.read"})
	testHelper.listEdgeRoutersForServiceEdgeRouterPolicy(ctx, routerReadSession, testPolicyId, http.StatusOK)

	// Test listing services for service-edge-router-policy - requires service.read
	testHelper.listServicesForServiceEdgeRouterPolicy(ctx, policyReadSession, testPolicyId, http.StatusUnauthorized)

	serviceReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.read"})
	testHelper.listServicesForServiceEdgeRouterPolicy(ctx, serviceReadSession, testPolicyId, http.StatusOK)

	// Test with admin - should succeed for all
	testHelper.listEdgeRoutersForServiceEdgeRouterPolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
	testHelper.listServicesForServiceEdgeRouterPolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)

	// Cleanup
	testHelper.deleteServiceEdgeRouterPolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
}
