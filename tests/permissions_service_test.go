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

func Test_Permissions_Service(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all service operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read service operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level service permission allows all CRUD for services", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceEntityLevelPermissions(ctx)
	})

	t.Run("service-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all service operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceNoPermissions(ctx)
	})

	t.Run("service list operations require specific permissions", func(t *testing.T) {
		ctx.testContextChanged(t)
		testServiceListOperationsPermissions(ctx)
	})
}

// testServiceAdminPermissions tests that admin permission allows all service operations
func testServiceAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testServiceId := testHelper.createService(ctx, adminSession, "admin-test-service", http.StatusCreated)

	// Test read operation
	testHelper.getService(ctx, adminSession, testServiceId, http.StatusOK)

	// Test list operation
	testHelper.listServices(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchService(ctx, adminSession, testServiceId, &rest_model.ServicePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteService(ctx, adminSession, testServiceId, http.StatusOK)
}

// testServiceAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testServiceAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test service as admin for readonly user to read
	testServiceId := testHelper.createService(ctx, ctx.AdminManagementSession, "readonly-test-service", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getService(ctx, readonlySession, testServiceId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listServices(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createService(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchService(ctx, readonlySession, testServiceId, &rest_model.ServicePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteService(ctx, readonlySession, testServiceId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)
}

// testServiceEntityLevelPermissions tests that entity-level service permission allows all CRUD for services
func testServiceEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with service entity permission
	servicePermSession := testHelper.newIdentityWithPermissions(ctx, []string{"service"})

	// Test service CRUD - should all succeed
	testServiceId := testHelper.createService(ctx, servicePermSession, "entity-perm-test-service", http.StatusCreated)
	testHelper.getService(ctx, servicePermSession, testServiceId, http.StatusOK)
	testHelper.patchService(ctx, servicePermSession, testServiceId, &rest_model.ServicePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)
	testHelper.deleteService(ctx, servicePermSession, testServiceId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, servicePermSession, "service-perm-identity", http.StatusUnauthorized)
}

// testServiceEntityActionPermissions tests that entity-action permissions allow specific operations
func testServiceEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test service as admin
	testServiceId := testHelper.createService(ctx, ctx.AdminManagementSession, "action-test-service", http.StatusCreated)

	// Test service.read permission
	readSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.read"})

	testHelper.getService(ctx, readSession, testServiceId, http.StatusOK)
	testHelper.listServices(ctx, readSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createService(ctx, readSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchService(ctx, readSession, testServiceId, &rest_model.ServicePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"read-only": true}}),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteService(ctx, readSession, testServiceId, http.StatusUnauthorized)

	// Test service.create permission
	createSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.create"})

	newServiceId := testHelper.createService(ctx, createSession, "create-only-service", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getService(ctx, createSession, newServiceId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchService(ctx, createSession, newServiceId, &rest_model.ServicePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"create-only": true}}),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteService(ctx, createSession, newServiceId, http.StatusUnauthorized)

	// Test service.update permission
	updateSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.update"})
	testHelper.patchService(ctx, updateSession, newServiceId, &rest_model.ServicePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"update-only": true}}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getService(ctx, updateSession, testServiceId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createService(ctx, updateSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteService(ctx, updateSession, testServiceId, http.StatusUnauthorized)

	// Test service.delete permission
	deleteSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.delete"})

	testHelper.deleteService(ctx, deleteSession, testServiceId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getService(ctx, deleteSession, testServiceId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createService(ctx, deleteSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteService(ctx, ctx.AdminManagementSession, newServiceId, http.StatusOK)
}

// testServiceNoPermissions tests that no permissions deny all operations
func testServiceNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test service as admin
	testServiceId := testHelper.createService(ctx, ctx.AdminManagementSession, "no-perm-test-service", http.StatusCreated)

	// All operations should fail with Unauthorized

	// Service operations
	testHelper.getService(ctx, noPermSession, testServiceId, http.StatusUnauthorized)

	testHelper.listServices(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createService(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	testHelper.patchService(ctx, noPermSession, testServiceId, &rest_model.ServicePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"no-perm": true}}),
	}, http.StatusUnauthorized)

	testHelper.deleteService(ctx, noPermSession, testServiceId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)
}

// testServiceListOperationsPermissions tests that service list operations require specific permissions
func testServiceListOperationsPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test service as admin
	testServiceId := testHelper.createService(ctx, ctx.AdminManagementSession, "list-ops-test-service", http.StatusCreated)

	// Test listing configs for service - requires config.read, NOT service.read
	serviceReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.read"})

	// Negative test: service.read alone should NOT allow listing configs
	testHelper.listConfigsForService(ctx, serviceReadSession, testServiceId, http.StatusUnauthorized)

	// Positive test: config.read should allow listing configs
	configReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.read"})
	testHelper.listConfigsForService(ctx, configReadSession, testServiceId, http.StatusOK)

	// Test listing service edge router policies - requires service-edge-router-policy.read
	testHelper.listServiceEdgeRouterPoliciesForService(ctx, serviceReadSession, testServiceId, http.StatusUnauthorized)

	serviceEdgeRouterPolicyReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-edge-router-policy.read"})
	testHelper.listServiceEdgeRouterPoliciesForService(ctx, serviceEdgeRouterPolicyReadSession, testServiceId, http.StatusOK)

	// Test listing edge routers - requires router.read
	testHelper.listEdgeRoutersForService(ctx, serviceReadSession, testServiceId, http.StatusUnauthorized)

	routerReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"router.read"})
	testHelper.listEdgeRoutersForService(ctx, routerReadSession, testServiceId, http.StatusOK)

	// Test listing service policies - requires service-policy.read
	testHelper.listServicePoliciesForService(ctx, serviceReadSession, testServiceId, http.StatusUnauthorized)

	servicePolicyReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"service-policy.read"})
	testHelper.listServicePoliciesForService(ctx, servicePolicyReadSession, testServiceId, http.StatusOK)

	// Test listing identities - requires identity.read
	testHelper.listIdentitiesForService(ctx, serviceReadSession, testServiceId, http.StatusUnauthorized)

	identityReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"identity.read"})
	testHelper.listIdentitiesForService(ctx, identityReadSession, testServiceId, http.StatusOK)

	// Test listing terminators - requires terminator.read
	testHelper.listTerminatorsForService(ctx, serviceReadSession, testServiceId, http.StatusUnauthorized)

	terminatorReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"terminator.read"})
	testHelper.listTerminatorsForService(ctx, terminatorReadSession, testServiceId, http.StatusOK)

	// Test with admin - should succeed for all
	testHelper.listConfigsForService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)
	testHelper.listServiceEdgeRouterPoliciesForService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)
	testHelper.listEdgeRoutersForService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)
	testHelper.listServicePoliciesForService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)
	testHelper.listIdentitiesForService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)
	testHelper.listTerminatorsForService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)

	// Cleanup
	testHelper.deleteService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)
}
