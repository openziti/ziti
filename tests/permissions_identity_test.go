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

func Test_Permissions_Identity(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all identity operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testIdentityAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read identity operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testIdentityAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level identity permission allows all CRUD for identities", func(t *testing.T) {
		ctx.testContextChanged(t)
		testIdentityEntityLevelPermissions(ctx)
	})

	t.Run("identity-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testIdentityEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all identity operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testIdentityNoPermissions(ctx)
	})

	t.Run("non-admin cannot create or affect admin identities", func(t *testing.T) {
		ctx.testContextChanged(t)
		testIdentityNonAdminCannotAffectAdmins(ctx)
	})
}

// testIdentityAdminPermissions tests that admin permission allows all identity operations
func testIdentityAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin permission
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testIdentityId := testHelper.createIdentity(ctx, adminSession, "admin-test-identity", http.StatusCreated)

	// Test read operation
	detail := testHelper.getIdentity(ctx, adminSession, testIdentityId, http.StatusOK)

	// Test list operation
	testHelper.listIdentities(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.updateIdentity(ctx, adminSession, detail, http.StatusOK)

	// Test delete operation
	testHelper.deleteIdentity(ctx, adminSession, testIdentityId, http.StatusOK)

	// Test on other entity types (service)
	serviceId := testHelper.createService(ctx, adminSession, "admin-test-service", http.StatusCreated)
	testHelper.getService(ctx, adminSession, serviceId, http.StatusOK)
	testHelper.deleteService(ctx, adminSession, serviceId, http.StatusOK)

	// Test on config
	config := testHelper.createConfig(ctx, adminSession, "admin-test-config")

	testHelper.getConfig(ctx, adminSession, config.Data.ID)

	testHelper.deleteConfig(ctx, adminSession, config.Data.ID)
}

// testIdentityAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testIdentityAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test entities as admin for readonly user to read
	testIdentityId := testHelper.createIdentity(ctx, ctx.AdminManagementSession, "readonly-test-identity", http.StatusCreated)
	testServiceId := testHelper.createService(ctx, ctx.AdminManagementSession, "readonly-test-service", http.StatusCreated)
	testConfig := testHelper.createConfig(ctx, ctx.AdminManagementSession, "readonly-test-config")

	// Test read operations - should succeed
	testIdentity := testHelper.getIdentity(ctx, readonlySession, testIdentityId, http.StatusOK)
	testHelper.getService(ctx, readonlySession, testServiceId, http.StatusOK)
	testHelper.getConfig(ctx, readonlySession, testConfig.Data.ID)

	// Test list operations - should succeed
	testHelper.listIdentities(ctx, readonlySession, http.StatusOK)
	testHelper.listServices(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createIdentity(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.updateIdentity(ctx, readonlySession, testIdentity, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteIdentity(ctx, readonlySession, testIdentityId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, testIdentityId, http.StatusOK)
	testHelper.deleteService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)
	testHelper.deleteConfig(ctx, ctx.AdminManagementSession, testConfig.Data.ID)
}

// testIdentityEntityLevelPermissions tests that entity-level identity permission allows all CRUD for identities
func testIdentityEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with identity entity permission
	identityPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"identity"})

	// Test identity CRUD - should all succeed
	testIdentityId := testHelper.createIdentity(ctx, identityPermSession, "entity-perm-test-identity", http.StatusCreated)
	detail := testHelper.getIdentity(ctx, identityPermSession, testIdentityId, http.StatusOK)
	testHelper.patchIdentity(ctx, identityPermSession, *detail.ID, &rest_model.IdentityPatch{
		RoleAttributes: ToPtr(rest_model.Attributes{"updated"}),
	}, http.StatusOK)
	testHelper.deleteIdentity(ctx, identityPermSession, testIdentityId, http.StatusOK)

	// Test service operations - should fail (no service permission)
	testHelper.createService(ctx, identityPermSession, "identity-perm-service", http.StatusUnauthorized)

	// Create an identity with service entity permission
	servicePermSession := testHelper.newIdentityWithPermissions(ctx, []string{"service"})

	// Test service CRUD - should all succeed
	testServiceId := testHelper.createService(ctx, servicePermSession, "service-perm-test", http.StatusCreated)
	serviceDetail := testHelper.getService(ctx, servicePermSession, testServiceId, http.StatusOK)
	testHelper.updateService(ctx, servicePermSession, serviceDetail, http.StatusOK)
	testHelper.deleteService(ctx, servicePermSession, testServiceId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, servicePermSession, "service-perm-identity", http.StatusUnauthorized)

	// Create an identity with config entity permission
	configPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"config"})

	// Test config CRUD - should all succeed
	testConfig := testHelper.createConfig(ctx, configPermSession, "config-perm-test")
	configDetail := testHelper.getConfig(ctx, configPermSession, testConfig.Data.ID)
	testHelper.updateConfig(ctx, configPermSession, configDetail, http.StatusOK)
	testHelper.deleteConfig(ctx, configPermSession, testConfig.Data.ID)
}

// testIdentityEntityActionPermissions tests that entity-action permissions allow specific operations
func testIdentityEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test entities as admin
	testIdentityId := testHelper.createIdentity(ctx, ctx.AdminManagementSession, "action-test-identity", http.StatusCreated)
	testServiceId := testHelper.createService(ctx, ctx.AdminManagementSession, "action-test-service", http.StatusCreated)
	testConfig := testHelper.createConfig(ctx, ctx.AdminManagementSession, "action-test-config")

	// Test identity.read permission
	readIdentitySession := testHelper.newIdentityWithPermissions(ctx, []string{"identity.read"})

	detail := testHelper.getIdentity(ctx, readIdentitySession, testIdentityId, http.StatusOK)
	testHelper.listIdentities(ctx, readIdentitySession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createIdentity(ctx, readIdentitySession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.updateIdentity(ctx, readIdentitySession, detail, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteIdentity(ctx, readIdentitySession, testIdentityId, http.StatusUnauthorized)

	// Test identity.create permission
	createIdentitySession := testHelper.newIdentityWithPermissions(ctx, []string{"identity.create"})

	newIdentityId := testHelper.createIdentity(ctx, createIdentitySession, "create-only-identity", http.StatusCreated)
	detail = testHelper.getIdentity(ctx, ctx.AdminManagementSession, newIdentityId, http.StatusOK)

	// Create-only should not allow read
	testHelper.getIdentity(ctx, createIdentitySession, newIdentityId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.updateIdentity(ctx, createIdentitySession, detail, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteIdentity(ctx, createIdentitySession, newIdentityId, http.StatusUnauthorized)

	// Test identity.update permission
	updateIdentitySession := testHelper.newIdentityWithPermissions(ctx, []string{"identity.update"})
	testHelper.patchIdentity(ctx, updateIdentitySession, *detail.ID, &rest_model.IdentityPatch{
		RoleAttributes: ToPtr(rest_model.Attributes{"updated"}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getIdentity(ctx, updateIdentitySession, testIdentityId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createIdentity(ctx, updateIdentitySession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteIdentity(ctx, updateIdentitySession, testIdentityId, http.StatusUnauthorized)

	// Test identity.delete permission
	deleteIdentitySession := testHelper.newIdentityWithPermissions(ctx, []string{"identity.delete"})

	testHelper.deleteIdentity(ctx, deleteIdentitySession, testIdentityId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getIdentity(ctx, deleteIdentitySession, testIdentityId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createIdentity(ctx, deleteIdentitySession, "delete-only-create", http.StatusUnauthorized)

	// Test service.read permission
	readServiceSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.read"})
	serviceDetail := testHelper.getService(ctx, readServiceSession, testServiceId, http.StatusOK)

	// Test service.create permission
	createServiceSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.create"})
	newServiceId := testHelper.createService(ctx, createServiceSession, "create-only-service", http.StatusCreated)

	// Test service.update permission
	updateServiceSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.update"})
	testHelper.updateService(ctx, updateServiceSession, serviceDetail, http.StatusOK)

	// Test service.delete permission
	deleteServiceSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.delete"})
	testHelper.deleteService(ctx, deleteServiceSession, testServiceId, http.StatusOK)

	// Test config permissions
	readConfigSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.read"})
	configDetail := testHelper.getConfig(ctx, readConfigSession, testConfig.Data.ID)

	createConfigSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.create"})
	newConfig := testHelper.createConfig(ctx, createConfigSession, "create-only-config")

	updateConfigSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.update"})
	testHelper.updateConfig(ctx, updateConfigSession, configDetail, http.StatusOK)

	deleteConfigSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.delete"})
	testHelper.deleteConfig(ctx, deleteConfigSession, testConfig.Data.ID)

	// Cleanup
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, newIdentityId, http.StatusOK)
	testHelper.deleteService(ctx, ctx.AdminManagementSession, newServiceId, http.StatusOK)
	testHelper.deleteConfig(ctx, ctx.AdminManagementSession, newConfig.Data.ID)
}

// testIdentityNoPermissions tests that no permissions deny all operations
func testIdentityNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test entities as admin
	testIdentityId := testHelper.createIdentity(ctx, ctx.AdminManagementSession, "no-perm-test-identity", http.StatusCreated)
	testServiceId := testHelper.createService(ctx, ctx.AdminManagementSession, "no-perm-test-service", http.StatusCreated)
	testConfig := testHelper.createConfig(ctx, ctx.AdminManagementSession, "no-perm-test-config")
	identityDetail := testHelper.getIdentity(ctx, ctx.AdminManagementSession, testIdentityId, http.StatusOK)

	// All operations should fail with Unauthorized

	// Identity operations
	testHelper.getIdentity(ctx, noPermSession, testIdentityId, http.StatusUnauthorized)

	testHelper.listIdentities(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createIdentity(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	testHelper.updateIdentity(ctx, noPermSession, identityDetail, http.StatusUnauthorized)

	testHelper.deleteIdentity(ctx, noPermSession, testIdentityId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, testIdentityId, http.StatusOK)
	testHelper.deleteService(ctx, ctx.AdminManagementSession, testServiceId, http.StatusOK)
	testHelper.deleteConfig(ctx, ctx.AdminManagementSession, testConfig.Data.ID)
}

// testIdentityNonAdminCannotAffectAdmins tests that non-admin users cannot create or modify admin identities
func testIdentityNonAdminCannotAffectAdmins(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create a user with full identity permissions but not admin
	identityPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"identity"})

	// Test 1: Non-admin cannot create an admin identity
	testHelper.createAdminIdentity(ctx, identityPermSession, "non-admin-create-admin", http.StatusUnauthorized)

	// Test 2: Non-admin cannot create an identity with permissions
	testHelper.createIdentityWithPermissions(ctx, identityPermSession, "non-admin-create-with-perms", []string{"service.read"}, http.StatusUnauthorized)

	// Create an admin identity as admin for testing updates/deletes
	adminIdentityId := testHelper.createAdminIdentity(ctx, ctx.AdminManagementSession, "test-admin-identity", http.StatusCreated)

	// Test 3: Non-admin cannot update an admin identity using PATCH
	testHelper.patchIdentity(ctx, identityPermSession, adminIdentityId, &rest_model.IdentityPatch{
		RoleAttributes: ToPtr(rest_model.Attributes{"patched"}),
	}, http.StatusUnauthorized)

	// Test 4: Non-admin cannot delete an admin identity
	testHelper.deleteIdentity(ctx, identityPermSession, adminIdentityId, http.StatusUnauthorized)

	// Create a non-admin identity for testing field restrictions
	regularIdentityId := testHelper.createIdentity(ctx, ctx.AdminManagementSession, "regular-identity", http.StatusCreated)
	regularIdentityDetail := testHelper.getIdentity(ctx, ctx.AdminManagementSession, regularIdentityId, http.StatusOK)

	// Test 5: Non-admin cannot modify permissions field on a non-admin identity
	perms := []string{"service.read"}
	testHelper.patchIdentity(ctx, identityPermSession, regularIdentityId, &rest_model.IdentityPatch{
		Permissions: (*rest_model.Permissions)(&perms),
	}, http.StatusUnauthorized)

	// Test 6: Non-admin cannot modify isAdmin field on a non-admin identity
	testHelper.patchIdentity(ctx, identityPermSession, regularIdentityId, &rest_model.IdentityPatch{
		IsAdmin: ToPtr(true),
	}, http.StatusUnauthorized)

	// Test 7: Non-admin cannot use PUT method (Update) - only PATCH is allowed
	testHelper.updateIdentity(ctx, identityPermSession, regularIdentityDetail, http.StatusUnauthorized)

	// Verify that admin can still do all these operations
	testHelper.patchIdentity(ctx, ctx.AdminManagementSession, adminIdentityId, &rest_model.IdentityPatch{
		RoleAttributes: ToPtr(rest_model.Attributes{"patched"}),
	}, http.StatusOK)

	adminPerms := []string{"service.read"}
	testHelper.patchIdentity(ctx, ctx.AdminManagementSession, regularIdentityId, &rest_model.IdentityPatch{
		Permissions: (*rest_model.Permissions)(&adminPerms),
	}, http.StatusOK)

	testHelper.patchIdentity(ctx, ctx.AdminManagementSession, regularIdentityId, &rest_model.IdentityPatch{
		IsAdmin: ToPtr(true),
	}, http.StatusOK)

	// Cleanup
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, adminIdentityId, http.StatusOK)
	testHelper.deleteIdentity(ctx, ctx.AdminManagementSession, regularIdentityId, http.StatusOK)
}
