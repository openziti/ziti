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

func Test_Permissions_ConfigType(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all config-type operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigTypeAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read config-type operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigTypeAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level config-type permission allows all CRUD for config-types", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigTypeEntityLevelPermissions(ctx)
	})

	t.Run("config-type-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigTypeEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all config-type operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigTypeNoPermissions(ctx)
	})

	t.Run("listing configs for config-type requires config.read permission", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigTypeListConfigsPermission(ctx)
	})
}

// testConfigTypeAdminPermissions tests that admin permission allows all config-type operations
func testConfigTypeAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testConfigTypeId := testHelper.createConfigType(ctx, adminSession, "admin-test-config-type", http.StatusCreated)

	// Test read operation
	testHelper.getConfigType(ctx, adminSession, testConfigTypeId, http.StatusOK)

	// Test list operation
	testHelper.listConfigTypes(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchConfigType(ctx, adminSession, testConfigTypeId, &rest_model.ConfigTypePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteConfigType(ctx, adminSession, testConfigTypeId, http.StatusOK)
}

// testConfigTypeAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testConfigTypeAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test config-type as admin for readonly user to read
	testConfigTypeId := testHelper.createConfigType(ctx, ctx.AdminManagementSession, "readonly-test-config-type", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getConfigType(ctx, readonlySession, testConfigTypeId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listConfigTypes(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createConfigType(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchConfigType(ctx, readonlySession, testConfigTypeId, &rest_model.ConfigTypePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteConfigType(ctx, readonlySession, testConfigTypeId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteConfigType(ctx, ctx.AdminManagementSession, testConfigTypeId, http.StatusOK)
}

// testConfigTypeEntityLevelPermissions tests that entity-level config-type permission allows all CRUD for config-types
func testConfigTypeEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with config-type entity permission
	configTypePermSession := testHelper.newIdentityWithPermissions(ctx, []string{"config-type"})

	// Test config-type CRUD - should all succeed
	testConfigTypeId := testHelper.createConfigType(ctx, configTypePermSession, "entity-perm-test-config-type", http.StatusCreated)
	testHelper.getConfigType(ctx, configTypePermSession, testConfigTypeId, http.StatusOK)
	testHelper.patchConfigType(ctx, configTypePermSession, testConfigTypeId, &rest_model.ConfigTypePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)
	testHelper.deleteConfigType(ctx, configTypePermSession, testConfigTypeId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, configTypePermSession, "config-type-perm-identity", http.StatusUnauthorized)
}

// testConfigTypeEntityActionPermissions tests that entity-action permissions allow specific operations
func testConfigTypeEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test config-type as admin
	testConfigTypeId := testHelper.createConfigType(ctx, ctx.AdminManagementSession, "action-test-config-type", http.StatusCreated)

	// Test config-type.read permission
	readConfigTypeSession := testHelper.newIdentityWithPermissions(ctx, []string{"config-type.read"})

	testHelper.getConfigType(ctx, readConfigTypeSession, testConfigTypeId, http.StatusOK)
	testHelper.listConfigTypes(ctx, readConfigTypeSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createConfigType(ctx, readConfigTypeSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchConfigType(ctx, readConfigTypeSession, testConfigTypeId, &rest_model.ConfigTypePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"read-only": true}}),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteConfigType(ctx, readConfigTypeSession, testConfigTypeId, http.StatusUnauthorized)

	// Test config-type.create permission
	createConfigTypeSession := testHelper.newIdentityWithPermissions(ctx, []string{"config-type.create"})

	newConfigTypeId := testHelper.createConfigType(ctx, createConfigTypeSession, "create-only-config-type", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getConfigType(ctx, createConfigTypeSession, newConfigTypeId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchConfigType(ctx, createConfigTypeSession, newConfigTypeId, &rest_model.ConfigTypePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"create-only": true}}),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteConfigType(ctx, createConfigTypeSession, newConfigTypeId, http.StatusUnauthorized)

	// Test config-type.update permission
	updateConfigTypeSession := testHelper.newIdentityWithPermissions(ctx, []string{"config-type.update"})
	testHelper.patchConfigType(ctx, updateConfigTypeSession, newConfigTypeId, &rest_model.ConfigTypePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"update-only": true}}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getConfigType(ctx, updateConfigTypeSession, testConfigTypeId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createConfigType(ctx, updateConfigTypeSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteConfigType(ctx, updateConfigTypeSession, testConfigTypeId, http.StatusUnauthorized)

	// Test config-type.delete permission
	deleteConfigTypeSession := testHelper.newIdentityWithPermissions(ctx, []string{"config-type.delete"})

	testHelper.deleteConfigType(ctx, deleteConfigTypeSession, testConfigTypeId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getConfigType(ctx, deleteConfigTypeSession, testConfigTypeId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createConfigType(ctx, deleteConfigTypeSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteConfigType(ctx, ctx.AdminManagementSession, newConfigTypeId, http.StatusOK)
}

// testConfigTypeNoPermissions tests that no permissions deny all operations
func testConfigTypeNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test config-type as admin
	testConfigTypeId := testHelper.createConfigType(ctx, ctx.AdminManagementSession, "no-perm-test-config-type", http.StatusCreated)

	// All operations should fail with Unauthorized

	// ConfigType operations
	testHelper.getConfigType(ctx, noPermSession, testConfigTypeId, http.StatusUnauthorized)

	testHelper.listConfigTypes(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createConfigType(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	testHelper.patchConfigType(ctx, noPermSession, testConfigTypeId, &rest_model.ConfigTypePatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"no-perm": true}}),
	}, http.StatusUnauthorized)

	testHelper.deleteConfigType(ctx, noPermSession, testConfigTypeId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteConfigType(ctx, ctx.AdminManagementSession, testConfigTypeId, http.StatusOK)
}

// testConfigTypeListConfigsPermission tests that listing configs for a config-type requires config.read permission
func testConfigTypeListConfigsPermission(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test config-type as admin
	testConfigTypeId := testHelper.createConfigType(ctx, ctx.AdminManagementSession, "config-list-test-config-type", http.StatusCreated)

	// Test with config-type.read permission - should fail (needs config.read)
	configTypeReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"config-type.read"})
	testHelper.listConfigsForConfigType(ctx, configTypeReadSession, testConfigTypeId, http.StatusUnauthorized)

	// Test with config.read permission - should succeed
	configReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.read"})
	testHelper.listConfigsForConfigType(ctx, configReadSession, testConfigTypeId, http.StatusOK)

	// Test with both config-type.read and config.read - should succeed
	bothPermsSession := testHelper.newIdentityWithPermissions(ctx, []string{"config-type.read", "config.read"})
	testHelper.listConfigsForConfigType(ctx, bothPermsSession, testConfigTypeId, http.StatusOK)

	// Test with admin - should succeed
	testHelper.listConfigsForConfigType(ctx, ctx.AdminManagementSession, testConfigTypeId, http.StatusOK)

	// Cleanup
	testHelper.deleteConfigType(ctx, ctx.AdminManagementSession, testConfigTypeId, http.StatusOK)
}
