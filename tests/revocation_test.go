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
	"testing"

	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
)

// Test_Revocation tests the revocation Management API endpoints: create, list, detail,
// input validation, and end-to-end token enforcement for JTI, identity, and api-session
// revocation types.
func Test_Revocation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminClient := ctx.NewEdgeManagementApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	apiSession, err := adminClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(apiSession)

	t.Run("create revocations via the management API", func(t *testing.T) {
		ctx.testContextChanged(t)

		t.Run("JTI type", func(t *testing.T) {
			ctx.testContextChanged(t)

			jti := uuid.NewString()
			loc, err := adminClient.CreateRevocation(jti, rest_model.RevocationTypeEnumJTI)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(loc)
			ctx.Req.Equal(jti, loc.ID)

			t.Run("detail is retrievable by ID", func(t *testing.T) {
				ctx.testContextChanged(t)

				detail, err := adminClient.GetRevocation(jti)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(detail)
				ctx.Req.Equal(jti, *detail.ID)
				ctx.Req.NotNil(detail.Type)
				ctx.Req.Equal(rest_model.RevocationTypeEnumJTI, *detail.Type)
				ctx.Req.NotNil(detail.ExpiresAt)
				ctx.Req.False((*detail.ExpiresAt).IsZero())
			})
		})

		t.Run("IDENTITY type", func(t *testing.T) {
			ctx.testContextChanged(t)

			identityLoc, err := adminClient.CreateIdentity(uuid.NewString(), false)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(identityLoc)

			loc, err := adminClient.CreateRevocation(identityLoc.ID, rest_model.RevocationTypeEnumIDENTITY)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(loc)
			ctx.Req.Equal(identityLoc.ID, loc.ID)

			t.Run("detail is retrievable by ID", func(t *testing.T) {
				ctx.testContextChanged(t)

				detail, err := adminClient.GetRevocation(identityLoc.ID)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(detail)
				ctx.Req.NotNil(detail.Type)
				ctx.Req.Equal(rest_model.RevocationTypeEnumIDENTITY, *detail.Type)
			})
		})

		t.Run("API_SESSION type", func(t *testing.T) {
			ctx.testContextChanged(t)

			apiSessionId := uuid.NewString()
			loc, err := adminClient.CreateRevocation(apiSessionId, rest_model.RevocationTypeEnumAPISESSION)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(loc)
			ctx.Req.Equal(apiSessionId, loc.ID)

			t.Run("detail is retrievable by ID", func(t *testing.T) {
				ctx.testContextChanged(t)

				detail, err := adminClient.GetRevocation(apiSessionId)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(detail)
				ctx.Req.NotNil(detail.Type)
				ctx.Req.Equal(rest_model.RevocationTypeEnumAPISESSION, *detail.Type)
			})
		})

		t.Run("list includes created revocations", func(t *testing.T) {
			ctx.testContextChanged(t)

			jti := uuid.NewString()
			_, err := adminClient.CreateRevocation(jti, rest_model.RevocationTypeEnumJTI)
			ctx.Req.NoError(err)

			revocations, err := adminClient.ListRevocations()
			ctx.Req.NoError(err)
			ctx.Req.NotNil(revocations)

			found := false
			for _, r := range revocations {
				if r.ID != nil && *r.ID == jti {
					found = true
					break
				}
			}
			ctx.Req.True(found, "created JTI revocation not found in list response")
		})
	})

	t.Run("create validation", func(t *testing.T) {
		ctx.testContextChanged(t)

		t.Run("JTI rejects non-UUID id", func(t *testing.T) {
			ctx.testContextChanged(t)

			_, err := adminClient.CreateRevocation("not-a-valid-uuid", rest_model.RevocationTypeEnumJTI)
			ctx.Req.Error(err)
		})

		t.Run("API_SESSION rejects non-UUID id", func(t *testing.T) {
			ctx.testContextChanged(t)

			_, err := adminClient.CreateRevocation("not-a-valid-uuid", rest_model.RevocationTypeEnumAPISESSION)
			ctx.Req.Error(err)
		})

		t.Run("IDENTITY rejects non-existent identity", func(t *testing.T) {
			ctx.testContextChanged(t)

			// A valid UUID that does not correspond to any identity
			_, err := adminClient.CreateRevocation(uuid.NewString(), rest_model.RevocationTypeEnumIDENTITY)
			ctx.Req.Error(err)
		})
	})

	t.Run("token enforcement", func(t *testing.T) {
		ctx.testContextChanged(t)

		clientHelper := ctx.NewEdgeClientApi(nil)

		t.Run("revoke by JTI rejects that specific access token", func(t *testing.T) {
			ctx.testContextChanged(t)

			creds := ctx.NewAdminCredentials()
			creds.CaPool = ctx.ControllerCaPool()
			tokenStr, claims, err := clientHelper.OidcAccessToken(creds)
			ctx.Req.NoError(err)
			c := ctx.NewEdgeManagementApiWithStaticToken(tokenStr)

			// Token should be valid before revocation.
			_, err = c.GetCurrentApiSessionDetail()
			ctx.Req.NoError(err)

			_, err = adminClient.CreateRevocation(claims.JWTID, rest_model.RevocationTypeEnumJTI)
			ctx.Req.NoError(err)

			// Same token must now be rejected.
			_, err = c.GetCurrentApiSessionDetail()
			ctx.Req.Error(err, "expected token to be rejected after JTI revocation")
		})

		t.Run("revoke by api-session rejects all tokens for that session", func(t *testing.T) {
			ctx.testContextChanged(t)

			creds := ctx.NewAdminCredentials()
			creds.CaPool = ctx.ControllerCaPool()
			tokenStr, claims, err := clientHelper.OidcAccessToken(creds)
			ctx.Req.NoError(err)
			c := ctx.NewEdgeManagementApiWithStaticToken(tokenStr)

			// Token should be valid before revocation.
			_, err = c.GetCurrentApiSessionDetail()
			ctx.Req.NoError(err)

			_, err = adminClient.CreateRevocation(claims.ApiSessionId, rest_model.RevocationTypeEnumAPISESSION)
			ctx.Req.NoError(err)

			// Same token must now be rejected.
			_, err = c.GetCurrentApiSessionDetail()
			ctx.Req.Error(err, "expected token to be rejected after api-session revocation")
		})

		t.Run("revoke by identity rejects tokens issued before the revocation", func(t *testing.T) {
			ctx.testContextChanged(t)

			// Use a freshly created admin identity so that revoking it does not affect the
			// shared adminClient (authenticated as the default admin).
			freshIdentity, freshCreds, err := adminClient.CreateAndEnrollOttIdentity(true)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(freshIdentity)
			ctx.Req.NotNil(freshCreds)

			// Authenticate as the fresh identity in OIDC mode via the management API.
			freshManClient := ctx.NewEdgeManagementApi(nil)
			freshCreds.CaPool = ctx.ControllerCaPool()
			freshManClient.SetUseOidc(true)
			_, err = freshManClient.Authenticate(freshCreds, nil)
			ctx.Req.NoError(err)

			// Extract the access token from the authenticated OIDC session.
			accessToken, freshAccessClaims, err := freshManClient.GetOidcAccessToken()
			ctx.Req.NoError(err)

			// Use a static-token client so the token cannot be silently refreshed.
			c := ctx.NewEdgeManagementApiWithStaticToken(accessToken)

			// Token should be valid before revocation.
			_, err = c.GetCurrentApiSessionDetail()
			ctx.Req.NoError(err)

			// Revoke the fresh identity (not the default admin identity).
			_, err = adminClient.CreateRevocation(freshAccessClaims.Subject, rest_model.RevocationTypeEnumIDENTITY)
			ctx.Req.NoError(err)

			// Token issued before the revocation must now be rejected.
			_, err = c.GetCurrentApiSessionDetail()
			ctx.Req.Error(err, "expected token to be rejected after identity revocation")

			// Verify the shared adminClient still works.
			_, err = adminClient.GetCurrentApiSessionDetail()
			ctx.Req.NoError(err)
		})
	})
}
