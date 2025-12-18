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

func Test_Permissions_ServicePolicy(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all service-policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServicePolicyAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read service-policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServicePolicyAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level service-policy permission allows all CRUD for service-policies", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServicePolicyEntityLevelPermissions(ctx)
	})

	t.Run("service-policy-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServicePolicyEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all service-policy operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServicePolicyNoPermissions(ctx)
	})

	t.Run("service-policy list operations require specific permissions", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServicePolicyListOperationsPermissions(ctx)
	})
}

// testServicePolicyAdminPermissions tests that admin permission allows all service-policy operations
func testServicePolicyAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testPolicyId := testHelper.createServicePolicy(ctx, adminSession, "admin-test-service-policy", http.StatusCreated)

	// Test read operation
	testHelper.getServicePolicy(ctx, adminSession, testPolicyId, http.StatusOK)

	// Test list operation
	testHelper.listServicePolicies(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchServicePolicy(ctx, adminSession, testPolicyId, &rest_model.ServicePolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteServicePolicy(ctx, adminSession, testPolicyId, http.StatusOK)
}

// testServicePolicyAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testServicePolicyAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test service-policy as admin for readonly user to read
	testPolicyId := testHelper.createServicePolicy(ctx, ctx.AdminManagementSession, "readonly-test-service-policy", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getServicePolicy(ctx, readonlySession, testPolicyId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listServicePolicies(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createServicePolicy(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchServicePolicy(ctx, readonlySession, testPolicyId, &rest_model.ServicePolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteServicePolicy(ctx, readonlySession, testPolicyId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteServicePolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
}

// testServicePolicyEntityLevelPermissions tests that entity-level service-policy permission allows all CRUD for service-policies
func testServicePolicyEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with service-policy entity permission
	policyPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-policy"})

	// Test service-policy CRUD - should all succeed
	testPolicyId := testHelper.createServicePolicy(ctx, policyPermSession, "entity-perm-test-service-policy", http.StatusCreated)
	testHelper.getServicePolicy(ctx, policyPermSession, testPolicyId, http.StatusOK)
	testHelper.patchServicePolicy(ctx, policyPermSession, testPolicyId, &rest_model.ServicePolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)
	testHelper.deleteServicePolicy(ctx, policyPermSession, testPolicyId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, policyPermSession, "service-policy-perm-identity", http.StatusUnauthorized)
}

// testServicePolicyEntityActionPermissions tests that entity-action permissions allow specific operations
func testServicePolicyEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test service-policy as admin
	testPolicyId := testHelper.createServicePolicy(ctx, ctx.AdminManagementSession, "action-test-service-policy", http.StatusCreated)

	// Test service-policy.read permission
	readSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-policy.read"})

	testHelper.getServicePolicy(ctx, readSession, testPolicyId, http.StatusOK)
	testHelper.listServicePolicies(ctx, readSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createServicePolicy(ctx, readSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchServicePolicy(ctx, readSession, testPolicyId, &rest_model.ServicePolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"read-only": true}}),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteServicePolicy(ctx, readSession, testPolicyId, http.StatusUnauthorized)

	// Test service-policy.create permission
	createSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-policy.create"})

	newPolicyId := testHelper.createServicePolicy(ctx, createSession, "create-only-service-policy", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getServicePolicy(ctx, createSession, newPolicyId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchServicePolicy(ctx, createSession, newPolicyId, &rest_model.ServicePolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"create-only": true}}),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteServicePolicy(ctx, createSession, newPolicyId, http.StatusUnauthorized)

	// Test service-policy.update permission
	updateSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-policy.update"})
	testHelper.patchServicePolicy(ctx, updateSession, newPolicyId, &rest_model.ServicePolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"update-only": true}}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getServicePolicy(ctx, updateSession, testPolicyId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createServicePolicy(ctx, updateSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteServicePolicy(ctx, updateSession, testPolicyId, http.StatusUnauthorized)

	// Test service-policy.delete permission
	deleteSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-policy.delete"})

	testHelper.deleteServicePolicy(ctx, deleteSession, testPolicyId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getServicePolicy(ctx, deleteSession, testPolicyId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createServicePolicy(ctx, deleteSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteServicePolicy(ctx, ctx.AdminManagementSession, newPolicyId, http.StatusOK)
}

// testServicePolicyNoPermissions tests that no permissions deny all operations
func testServicePolicyNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test service-policy as admin
	testPolicyId := testHelper.createServicePolicy(ctx, ctx.AdminManagementSession, "no-perm-test-service-policy", http.StatusCreated)

	// All operations should fail with Unauthorized
	testHelper.getServicePolicy(ctx, noPermSession, testPolicyId, http.StatusUnauthorized)
	testHelper.listServicePolicies(ctx, noPermSession, http.StatusUnauthorized)
	testHelper.createServicePolicy(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)
	testHelper.patchServicePolicy(ctx, noPermSession, testPolicyId, &rest_model.ServicePolicyPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"no-perm": true}}),
	}, http.StatusUnauthorized)
	testHelper.deleteServicePolicy(ctx, noPermSession, testPolicyId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteServicePolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
}

// testServicePolicyListOperationsPermissions tests that service-policy list operations require specific permissions
func testServicePolicyListOperationsPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test service-policy as admin
	testPolicyId := testHelper.createServicePolicy(ctx, ctx.AdminManagementSession, "list-ops-test-service-policy", http.StatusCreated)

	// Test listing services for service-policy - requires service.read
	policyReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-policy.read"})
	testHelper.listServicesForServicePolicy(ctx, policyReadSession, testPolicyId, http.StatusUnauthorized)

	serviceReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.read"})
	testHelper.listServicesForServicePolicy(ctx, serviceReadSession, testPolicyId, http.StatusOK)

	// Test listing identities for service-policy - requires identity.read
	testHelper.listIdentitiesForServicePolicy(ctx, policyReadSession, testPolicyId, http.StatusUnauthorized)

	identityReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"identity.read"})
	testHelper.listIdentitiesForServicePolicy(ctx, identityReadSession, testPolicyId, http.StatusOK)

	// Test listing posture checks for service-policy - requires posture-check.read
	testHelper.listPostureChecksForServicePolicy(ctx, policyReadSession, testPolicyId, http.StatusUnauthorized)

	postureCheckReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"posture-check.read"})
	testHelper.listPostureChecksForServicePolicy(ctx, postureCheckReadSession, testPolicyId, http.StatusOK)

	// Test with admin - should succeed for all
	testHelper.listServicesForServicePolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
	testHelper.listIdentitiesForServicePolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
	testHelper.listPostureChecksForServicePolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)

	// Cleanup
	testHelper.deleteServicePolicy(ctx, ctx.AdminManagementSession, testPolicyId, http.StatusOK)
}
