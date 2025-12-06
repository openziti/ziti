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

func Test_Permissions_Ca(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all ca operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testCaAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read ca operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testCaAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level ca permission allows all CRUD for cas", func(t *testing.T) {
		ctx.testContextChanged(t)
		testCaEntityLevelPermissions(ctx)
	})

	t.Run("ca-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testCaEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all ca operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testCaNoPermissions(ctx)
	})
}

// testCaAdminPermissions tests that admin permission allows all ca operations
func testCaAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testCaId := testHelper.createCa(ctx, adminSession, "admin-test-ca", http.StatusCreated)

	// Test read operation
	testHelper.getCa(ctx, adminSession, testCaId, http.StatusOK)

	// Test list operation
	testHelper.listCas(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchCa(ctx, adminSession, testCaId, &rest_model.CaPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteCa(ctx, adminSession, testCaId, http.StatusOK)
}

// testCaAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testCaAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test ca as admin for readonly user to read
	testCaId := testHelper.createCa(ctx, ctx.AdminManagementSession, "readonly-test-ca", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getCa(ctx, readonlySession, testCaId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listCas(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createCa(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchCa(ctx, readonlySession, testCaId, &rest_model.CaPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteCa(ctx, readonlySession, testCaId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteCa(ctx, ctx.AdminManagementSession, testCaId, http.StatusOK)
}

// testCaEntityLevelPermissions tests that entity-level ca permission allows all CRUD for cas
func testCaEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with ca entity permission
	caPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"ca"})

	// Test ca CRUD - should all succeed
	testCaId := testHelper.createCa(ctx, caPermSession, "entity-perm-test-ca", http.StatusCreated)
	testHelper.getCa(ctx, caPermSession, testCaId, http.StatusOK)
	testHelper.patchCa(ctx, caPermSession, testCaId, &rest_model.CaPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)
	testHelper.deleteCa(ctx, caPermSession, testCaId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, caPermSession, "ca-perm-identity", http.StatusUnauthorized)
}

// testCaEntityActionPermissions tests that entity-action permissions allow specific operations
func testCaEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test ca as admin
	testCaId := testHelper.createCa(ctx, ctx.AdminManagementSession, "action-test-ca", http.StatusCreated)

	// Test ca.read permission
	readCaSession := testHelper.newIdentityWithPermissions(ctx, []string{"ca.read"})

	testHelper.getCa(ctx, readCaSession, testCaId, http.StatusOK)
	testHelper.listCas(ctx, readCaSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createCa(ctx, readCaSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchCa(ctx, readCaSession, testCaId, &rest_model.CaPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"read-only": true}}),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteCa(ctx, readCaSession, testCaId, http.StatusUnauthorized)

	// Test ca.create permission
	createCaSession := testHelper.newIdentityWithPermissions(ctx, []string{"ca.create"})

	newCaId := testHelper.createCa(ctx, createCaSession, "create-only-ca", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getCa(ctx, createCaSession, newCaId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchCa(ctx, createCaSession, newCaId, &rest_model.CaPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"create-only": true}}),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteCa(ctx, createCaSession, newCaId, http.StatusUnauthorized)

	// Test ca.update permission
	updateCaSession := testHelper.newIdentityWithPermissions(ctx, []string{"ca.update"})
	testHelper.patchCa(ctx, updateCaSession, newCaId, &rest_model.CaPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"update-only": true}}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getCa(ctx, updateCaSession, testCaId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createCa(ctx, updateCaSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteCa(ctx, updateCaSession, testCaId, http.StatusUnauthorized)

	// Test ca.delete permission
	deleteCaSession := testHelper.newIdentityWithPermissions(ctx, []string{"ca.delete"})

	testHelper.deleteCa(ctx, deleteCaSession, testCaId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getCa(ctx, deleteCaSession, testCaId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createCa(ctx, deleteCaSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteCa(ctx, ctx.AdminManagementSession, newCaId, http.StatusOK)
}

// testCaNoPermissions tests that no permissions deny all operations
func testCaNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test ca as admin
	testCaId := testHelper.createCa(ctx, ctx.AdminManagementSession, "no-perm-test-ca", http.StatusCreated)

	// All operations should fail with Unauthorized

	// Ca operations
	testHelper.getCa(ctx, noPermSession, testCaId, http.StatusUnauthorized)

	testHelper.listCas(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createCa(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	testHelper.patchCa(ctx, noPermSession, testCaId, &rest_model.CaPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"no-perm": true}}),
	}, http.StatusUnauthorized)

	testHelper.deleteCa(ctx, noPermSession, testCaId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteCa(ctx, ctx.AdminManagementSession, testCaId, http.StatusOK)
}
