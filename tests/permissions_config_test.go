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

func Test_Permissions_Config(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all config operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read config operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level config permission allows all CRUD for configs", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigEntityLevelPermissions(ctx)
	})

	t.Run("config-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all config operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigNoPermissions(ctx)
	})

	t.Run("listing config services requires service.read permission", func(t *testing.T) {
		ctx.testContextChanged(t)
		testConfigListServicesPermission(ctx)
	})
}

// testConfigAdminPermissions tests that admin permission allows all config operations
func testConfigAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testConfigEnv := testHelper.createConfig(ctx, adminSession, "admin-test-config")

	// Test read operation
	testHelper.getConfig(ctx, adminSession, testConfigEnv.Data.ID)

	// Test list operation
	testHelper.listConfigs(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchConfig(ctx, adminSession, testConfigEnv.Data.ID, &rest_model.ConfigPatch{
		Data: map[string]interface{}{"updated": true},
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteConfig(ctx, adminSession, testConfigEnv.Data.ID)
}

// testConfigAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testConfigAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test config as admin for readonly user to read
	testConfigEnv := testHelper.createConfig(ctx, ctx.AdminManagementSession, "readonly-test-config")

	// Test read operations - should succeed
	testHelper.getConfig(ctx, readonlySession, testConfigEnv.Data.ID)

	// Test list operations - should succeed
	testHelper.listConfigs(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createConfigExpectStatus(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchConfig(ctx, readonlySession, testConfigEnv.Data.ID, &rest_model.ConfigPatch{
		Data: map[string]interface{}{"updated": true},
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteConfigExpectStatus(ctx, readonlySession, testConfigEnv.Data.ID, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteConfig(ctx, ctx.AdminManagementSession, testConfigEnv.Data.ID)
}

// testConfigEntityLevelPermissions tests that entity-level config permission allows all CRUD for configs
func testConfigEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with config entity permission
	configPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"config"})

	// Test config CRUD - should all succeed
	testConfigEnv := testHelper.createConfig(ctx, configPermSession, "entity-perm-test-config")
	testHelper.getConfig(ctx, configPermSession, testConfigEnv.Data.ID)
	testHelper.patchConfig(ctx, configPermSession, testConfigEnv.Data.ID, &rest_model.ConfigPatch{
		Data: map[string]interface{}{"updated": true},
	}, http.StatusOK)
	testHelper.deleteConfig(ctx, configPermSession, testConfigEnv.Data.ID)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, configPermSession, "config-perm-identity", http.StatusUnauthorized)
}

// testConfigEntityActionPermissions tests that entity-action permissions allow specific operations
func testConfigEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test config as admin
	testConfigEnv := testHelper.createConfig(ctx, ctx.AdminManagementSession, "action-test-config")

	// Test config.read permission
	readConfigSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.read"})

	testHelper.getConfig(ctx, readConfigSession, testConfigEnv.Data.ID)
	testHelper.listConfigs(ctx, readConfigSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createConfigExpectStatus(ctx, readConfigSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchConfig(ctx, readConfigSession, testConfigEnv.Data.ID, &rest_model.ConfigPatch{
		Data: map[string]interface{}{"read-only": true},
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteConfigExpectStatus(ctx, readConfigSession, testConfigEnv.Data.ID, http.StatusUnauthorized)

	// Test config.create permission
	createConfigSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.create"})

	newConfigEnv := testHelper.createConfig(ctx, createConfigSession, "create-only-config")

	// Create-only should not allow read
	testHelper.getConfigExpectStatus(ctx, createConfigSession, newConfigEnv.Data.ID, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchConfig(ctx, createConfigSession, newConfigEnv.Data.ID, &rest_model.ConfigPatch{
		Data: map[string]interface{}{"create-only": true},
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteConfigExpectStatus(ctx, createConfigSession, newConfigEnv.Data.ID, http.StatusUnauthorized)

	// Test config.update permission
	updateConfigSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.update"})
	testHelper.patchConfig(ctx, updateConfigSession, newConfigEnv.Data.ID, &rest_model.ConfigPatch{
		Data: map[string]interface{}{"update-only": true},
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getConfigExpectStatus(ctx, updateConfigSession, testConfigEnv.Data.ID, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createConfigExpectStatus(ctx, updateConfigSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteConfigExpectStatus(ctx, updateConfigSession, testConfigEnv.Data.ID, http.StatusUnauthorized)

	// Test config.delete permission
	deleteConfigSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.delete"})

	testHelper.deleteConfig(ctx, deleteConfigSession, testConfigEnv.Data.ID)

	// Delete-only should not allow read
	testHelper.getConfigExpectStatus(ctx, deleteConfigSession, testConfigEnv.Data.ID, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createConfigExpectStatus(ctx, deleteConfigSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteConfig(ctx, ctx.AdminManagementSession, newConfigEnv.Data.ID)
}

// testConfigNoPermissions tests that no permissions deny all operations
func testConfigNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test config as admin
	testConfigEnv := testHelper.createConfig(ctx, ctx.AdminManagementSession, "no-perm-test-config")

	// All operations should fail with Unauthorized

	// Config operations
	testHelper.getConfigExpectStatus(ctx, noPermSession, testConfigEnv.Data.ID, http.StatusUnauthorized)

	testHelper.listConfigs(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createConfigExpectStatus(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	testHelper.patchConfig(ctx, noPermSession, testConfigEnv.Data.ID, &rest_model.ConfigPatch{
		Data: map[string]interface{}{"no-perm": true},
	}, http.StatusUnauthorized)

	testHelper.deleteConfigExpectStatus(ctx, noPermSession, testConfigEnv.Data.ID, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteConfig(ctx, ctx.AdminManagementSession, testConfigEnv.Data.ID)
}

// testConfigListServicesPermission tests that listing services for a config requires service.read permission
func testConfigListServicesPermission(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test config as admin
	testConfigEnv := testHelper.createConfig(ctx, ctx.AdminManagementSession, "service-list-test-config")

	// Test with config.read permission - should fail (needs service.read)
	configReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.read"})
	testHelper.listConfigServices(ctx, configReadSession, testConfigEnv.Data.ID, http.StatusUnauthorized)

	// Test with service.read permission - should succeed
	serviceReadSession := testHelper.newIdentityWithPermissions(ctx, []string{"service.read"})
	testHelper.listConfigServices(ctx, serviceReadSession, testConfigEnv.Data.ID, http.StatusOK)

	// Test with both config.read and service.read - should succeed
	bothPermsSession := testHelper.newIdentityWithPermissions(ctx, []string{"config.read", "service.read"})
	testHelper.listConfigServices(ctx, bothPermsSession, testConfigEnv.Data.ID, http.StatusOK)

	// Test with admin - should succeed
	testHelper.listConfigServices(ctx, ctx.AdminManagementSession, testConfigEnv.Data.ID, http.StatusOK)

	// Cleanup
	testHelper.deleteConfig(ctx, ctx.AdminManagementSession, testConfigEnv.Data.ID)
}
