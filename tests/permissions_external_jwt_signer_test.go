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

func Test_Permissions_ExternalJwtSigner(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("admin permission allows all external-jwt-signer operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testExternalJwtSignerAdminPermissions(ctx)
	})

	t.Run("admin_readonly permission allows only read external-jwt-signer operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testExternalJwtSignerAdminReadOnlyPermissions(ctx)
	})

	t.Run("entity-level external-jwt-signer permission allows all CRUD for external-jwt-signers", func(t *testing.T) {
		ctx.testContextChanged(t)
		testExternalJwtSignerEntityLevelPermissions(ctx)
	})

	t.Run("external-jwt-signer-action permissions allow specific operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testExternalJwtSignerEntityActionPermissions(ctx)
	})

	t.Run("no permissions deny all external-jwt-signer operations", func(t *testing.T) {
		ctx.testContextChanged(t)
		testExternalJwtSignerNoPermissions(ctx)
	})
}

// testExternalJwtSignerAdminPermissions tests that admin permission allows all external-jwt-signer operations
func testExternalJwtSignerAdminPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Use admin session
	adminSession := ctx.AdminManagementSession

	// Test create operation
	testSignerId := testHelper.createExternalJwtSigner(ctx, adminSession, "admin-test-external-jwt-signer", http.StatusCreated)

	// Test read operation
	testHelper.getExternalJwtSigner(ctx, adminSession, testSignerId, http.StatusOK)

	// Test list operation
	testHelper.listExternalJwtSigners(ctx, adminSession, http.StatusOK)

	// Test update operation
	testHelper.patchExternalJwtSigner(ctx, adminSession, testSignerId, &rest_model.ExternalJWTSignerPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)

	// Test delete operation
	testHelper.deleteExternalJwtSigner(ctx, adminSession, testSignerId, http.StatusOK)
}

// testExternalJwtSignerAdminReadOnlyPermissions tests that admin_readonly allows only read operations
func testExternalJwtSignerAdminReadOnlyPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with admin_readonly permission
	readonlySession := testHelper.newIdentityWithPermissions(ctx, []string{permissions.AdminReadOnlyPermission})

	// Create test external-jwt-signer as admin for readonly user to read
	testSignerId := testHelper.createExternalJwtSigner(ctx, ctx.AdminManagementSession, "readonly-test-external-jwt-signer", http.StatusCreated)

	// Test read operations - should succeed
	testHelper.getExternalJwtSigner(ctx, readonlySession, testSignerId, http.StatusOK)

	// Test list operations - should succeed
	testHelper.listExternalJwtSigners(ctx, readonlySession, http.StatusOK)

	// Test create operations - should fail with Unauthorized
	testHelper.createExternalJwtSigner(ctx, readonlySession, "readonly-create-test", http.StatusUnauthorized)

	// Test update operations - should fail with Unauthorized
	testHelper.patchExternalJwtSigner(ctx, readonlySession, testSignerId, &rest_model.ExternalJWTSignerPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusUnauthorized)

	// Test delete operations - should fail with Unauthorized
	testHelper.deleteExternalJwtSigner(ctx, readonlySession, testSignerId, http.StatusUnauthorized)

	// Cleanup as admin
	testHelper.deleteExternalJwtSigner(ctx, ctx.AdminManagementSession, testSignerId, http.StatusOK)
}

// testExternalJwtSignerEntityLevelPermissions tests that entity-level external-jwt-signer permission allows all CRUD for external-jwt-signers
func testExternalJwtSignerEntityLevelPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with external-jwt-signer entity permission
	signerPermSession := testHelper.newIdentityWithPermissions(ctx, []string{"external-jwt-signer"})

	// Test external-jwt-signer CRUD - should all succeed
	testSignerId := testHelper.createExternalJwtSigner(ctx, signerPermSession, "entity-perm-test-external-jwt-signer", http.StatusCreated)
	testHelper.getExternalJwtSigner(ctx, signerPermSession, testSignerId, http.StatusOK)
	testHelper.patchExternalJwtSigner(ctx, signerPermSession, testSignerId, &rest_model.ExternalJWTSignerPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"updated": true}}),
	}, http.StatusOK)
	testHelper.deleteExternalJwtSigner(ctx, signerPermSession, testSignerId, http.StatusOK)

	// Test identity operations - should fail (no identity permission)
	testHelper.createIdentity(ctx, signerPermSession, "external-jwt-signer-perm-identity", http.StatusUnauthorized)
}

// testExternalJwtSignerEntityActionPermissions tests that entity-action permissions allow specific operations
func testExternalJwtSignerEntityActionPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create test external-jwt-signer as admin
	testSignerId := testHelper.createExternalJwtSigner(ctx, ctx.AdminManagementSession, "action-test-external-jwt-signer", http.StatusCreated)

	// Test external-jwt-signer.read permission
	readSession := testHelper.newIdentityWithPermissions(ctx, []string{"external-jwt-signer.read"})

	testHelper.getExternalJwtSigner(ctx, readSession, testSignerId, http.StatusOK)
	testHelper.listExternalJwtSigners(ctx, readSession, http.StatusOK)

	// Read-only should not allow create
	testHelper.createExternalJwtSigner(ctx, readSession, "read-only-create", http.StatusUnauthorized)

	// Read-only should not allow update
	testHelper.patchExternalJwtSigner(ctx, readSession, testSignerId, &rest_model.ExternalJWTSignerPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"read-only": true}}),
	}, http.StatusUnauthorized)

	// Read-only should not allow delete
	testHelper.deleteExternalJwtSigner(ctx, readSession, testSignerId, http.StatusUnauthorized)

	// Test external-jwt-signer.create permission
	createSession := testHelper.newIdentityWithPermissions(ctx, []string{"external-jwt-signer.create"})

	newSignerId := testHelper.createExternalJwtSigner(ctx, createSession, "create-only-external-jwt-signer", http.StatusCreated)

	// Create-only should not allow read
	testHelper.getExternalJwtSigner(ctx, createSession, newSignerId, http.StatusUnauthorized)

	// Create-only should not allow update
	testHelper.patchExternalJwtSigner(ctx, createSession, newSignerId, &rest_model.ExternalJWTSignerPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"create-only": true}}),
	}, http.StatusUnauthorized)

	// Create-only should not allow delete
	testHelper.deleteExternalJwtSigner(ctx, createSession, newSignerId, http.StatusUnauthorized)

	// Test external-jwt-signer.update permission
	updateSession := testHelper.newIdentityWithPermissions(ctx, []string{"external-jwt-signer.update"})
	testHelper.patchExternalJwtSigner(ctx, updateSession, newSignerId, &rest_model.ExternalJWTSignerPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"update-only": true}}),
	}, http.StatusOK)

	// Update-only should not allow read
	testHelper.getExternalJwtSigner(ctx, updateSession, testSignerId, http.StatusUnauthorized)

	// Update-only should not allow create
	testHelper.createExternalJwtSigner(ctx, updateSession, "update-only-create", http.StatusUnauthorized)

	// Update-only should not allow delete
	testHelper.deleteExternalJwtSigner(ctx, updateSession, testSignerId, http.StatusUnauthorized)

	// Test external-jwt-signer.delete permission
	deleteSession := testHelper.newIdentityWithPermissions(ctx, []string{"external-jwt-signer.delete"})

	testHelper.deleteExternalJwtSigner(ctx, deleteSession, testSignerId, http.StatusOK)

	// Delete-only should not allow read
	testHelper.getExternalJwtSigner(ctx, deleteSession, testSignerId, http.StatusUnauthorized)

	// Delete-only should not allow create
	testHelper.createExternalJwtSigner(ctx, deleteSession, "delete-only-create", http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteExternalJwtSigner(ctx, ctx.AdminManagementSession, newSignerId, http.StatusOK)
}

// testExternalJwtSignerNoPermissions tests that no permissions deny all operations
func testExternalJwtSignerNoPermissions(ctx *TestContext) {
	testHelper := permissionTestHelper{}

	// Create an identity with no permissions
	noPermSession := testHelper.newIdentityWithPermissions(ctx, []string{})

	// Create test external-jwt-signer as admin
	testSignerId := testHelper.createExternalJwtSigner(ctx, ctx.AdminManagementSession, "no-perm-test-external-jwt-signer", http.StatusCreated)

	// All operations should fail with Unauthorized

	// External JWT Signer operations
	testHelper.getExternalJwtSigner(ctx, noPermSession, testSignerId, http.StatusUnauthorized)

	testHelper.listExternalJwtSigners(ctx, noPermSession, http.StatusUnauthorized)

	testHelper.createExternalJwtSigner(ctx, noPermSession, "no-perm-create", http.StatusUnauthorized)

	testHelper.patchExternalJwtSigner(ctx, noPermSession, testSignerId, &rest_model.ExternalJWTSignerPatch{
		Tags: ToPtr(rest_model.Tags{SubTags: rest_model.SubTags{"no-perm": true}}),
	}, http.StatusUnauthorized)

	testHelper.deleteExternalJwtSigner(ctx, noPermSession, testSignerId, http.StatusUnauthorized)

	// Cleanup
	testHelper.deleteExternalJwtSigner(ctx, ctx.AdminManagementSession, testSignerId, http.StatusOK)
}
