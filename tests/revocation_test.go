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
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	managementCurrentApiSession "github.com/openziti/edge-api/rest_management_api_client/current_api_session"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/oidc_auth"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// Test_Revocation verifies the OIDC token revocation lifecycle: refresh-token
// rotation, explicit revocation via /oidc/revoke, session termination via
// /oidc/end_session, and periodic purge of expired revocation entries.
func Test_Revocation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()

	ctrl := ctx.StartServerWithConfigModifier(func(cfg *config.Config) {
		cfg.Edge.Oidc.AccessTokenDuration = 10 * time.Second
		cfg.Edge.Oidc.RefreshTokenDuration = 15 * time.Second
		cfg.Edge.Oidc.IdTokenDuration = 10 * time.Second
	})
	ctx.RequireAdminManagementApiLogin()
	router := ctx.CreateEnrollAndStartEdgeRouter()

	// Wait for the router to complete its initial data state sync with the
	// controller. Without this, the first revocation may be created before
	// the router is ready to receive change set events.
	ctx.Req.True(router.WaitForRouterSync(30*time.Second), "timed out waiting for router initial sync")

	jwtParser := jwt.NewParser()

	t.Run("refresh revokes old refresh token JWTID", func(t *testing.T) {
		ctx.testContextChanged(t)

		client := ctx.NewEdgeManagementApi(nil)
		oidcSession, err := client.AuthenticateOidc(ctx.NewAdminCredentials())
		ctx.Req.NoError(err)

		// Extract the JWTID from the old refresh token before exchanging it.
		oldRefreshClaims := &common.RefreshClaims{}
		_, _, err = jwtParser.ParseUnverified(oidcSession.OidcTokens.RefreshToken, oldRefreshClaims)
		ctx.Req.NoError(err)
		oldRefreshJTI := oldRefreshClaims.JWTID
		ctx.Req.NotEmpty(oldRefreshJTI)

		// Exchange the refresh token for a new token pair.
		req := &oidc.RefreshTokenRequest{
			RefreshToken: oidcSession.OidcTokens.RefreshToken,
			ClientID:     oidcSession.OidcTokens.IDTokenClaims.ClientID,
			Scopes:       []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess},
		}
		enc := oidc.NewEncoder()
		dst := map[string][]string{}
		ctx.Req.NoError(enc.Encode(req, dst))
		dst["grant_type"] = []string{string(req.GrantType())}

		newTokens := &oidc.TokenExchangeResponse{}
		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			SetMultiValueFormData(dst).
			SetResult(newTokens).
			Post("https://" + ctx.ApiHost + "/oidc/oauth/token")
		ctx.Req.NoError(err)
		ctx.Req.Equal(200, resp.StatusCode())

		t.Run("old refresh token JWTID is in the controller revocation DB", func(t *testing.T) {
			ctx.testContextChanged(t)
			rev, err := ctrl.ReadRevocation(oldRefreshJTI)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(rev, "expected revocation entry for old refresh JWTID %s", oldRefreshJTI)
		})

		t.Run("revocation propagates to the router RDM within 10s", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.True(router.WaitForRevocation(oldRefreshJTI, 10*time.Second),
				"timed out waiting for revocation %s to appear in router RDM", oldRefreshJTI)
		})
	})

	t.Run("explicit revocation revokes refresh token by JWTID", func(t *testing.T) {
		ctx.testContextChanged(t)

		client := ctx.NewEdgeManagementApi(nil)
		oidcSession, err := client.AuthenticateOidc(ctx.NewAdminCredentials())
		ctx.Req.NoError(err)

		// Extract the JWTID from the refresh token before revoking it.
		refreshClaims := &common.RefreshClaims{}
		_, _, err = jwtParser.ParseUnverified(oidcSession.OidcTokens.RefreshToken, refreshClaims)
		ctx.Req.NoError(err)
		refreshJTI := refreshClaims.JWTID
		ctx.Req.NotEmpty(refreshJTI)

		clientID := oidcSession.OidcTokens.IDTokenClaims.ClientID
		refreshToken := oidcSession.OidcTokens.RefreshToken

		// POST to the OIDC revocation endpoint with the refresh token.
		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			SetMultiValueFormData(map[string][]string{
				"token":           {refreshToken},
				"token_type_hint": {"refresh_token"},
				"client_id":       {clientID},
			}).
			Post("https://" + ctx.ApiHost + "/oidc/revoke")
		ctx.Req.NoError(err)
		ctx.Req.Equal(200, resp.StatusCode())

		t.Run("refresh token JWTID is in the controller revocation DB", func(t *testing.T) {
			ctx.testContextChanged(t)
			rev, err := ctrl.ReadRevocation(refreshJTI)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(rev, "expected revocation entry for refresh JWTID %s", refreshJTI)
		})

		t.Run("raw refresh token string is NOT stored as a separate revocation key", func(t *testing.T) {
			ctx.testContextChanged(t)
			// Verifies bug 1 fix: no double-write with the raw JWT string as key.
			rev, err := ctrl.ReadRevocation(refreshToken)
			ctx.Req.NoError(err)
			ctx.Req.Nil(rev, "expected no revocation keyed on raw token string")
		})

		t.Run("revocation propagates to the router RDM within 10s", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.True(router.WaitForRevocation(refreshJTI, 10*time.Second),
				"timed out waiting for revocation %s to appear in router RDM", refreshJTI)
		})
	})

	t.Run("session termination revokes all identity tokens by subject", func(t *testing.T) {
		ctx.testContextChanged(t)

		client := ctx.NewEdgeManagementApi(nil)
		oidcSession, err := client.AuthenticateOidc(ctx.NewAdminCredentials())
		ctx.Req.NoError(err)

		// Extract identityId (Subject) and IssuedAt from the access token.
		accessClaims := &common.AccessClaims{}
		_, _, err = jwtParser.ParseUnverified(oidcSession.OidcTokens.AccessToken, accessClaims)
		ctx.Req.NoError(err)
		identityId := accessClaims.Subject
		ctx.Req.NotEmpty(identityId)

		clientID := oidcSession.OidcTokens.IDTokenClaims.ClientID
		idToken := oidcSession.OidcTokens.IDToken
		oldAccessToken := oidcSession.OidcTokens.AccessToken

		// Wait one full second so that the revocation's CreatedAt lands in a
		// strictly later second than the old token's IssuedAt. JWT timestamps
		// have second-level precision and the revocation check uses truncated
		// comparison, so this sleep is required for a deterministic result.
		time.Sleep(time.Second)

		// Hit the OIDC end_session endpoint. The redirect destination uses a
		// custom scheme so the HTTP client will not be able to follow it; we
		// discard that error and rely on the server having processed the request.
		endSessionURL := "https://" + ctx.ApiHost + "/oidc/end_session?" + url.Values{
			"id_token_hint":            {idToken},
			"client_id":                {clientID},
			"post_logout_redirect_uri": {"openziti://auth/logout"},
		}.Encode()
		_, _ = ctx.newAnonymousClientApiRequest().Get(endSessionURL)

		t.Run("identityId is in the controller revocation DB", func(t *testing.T) {
			ctx.testContextChanged(t)
			rev, err := ctrl.ReadRevocation(identityId)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(rev, "expected high-water-mark revocation entry for identity %s", identityId)
		})

		t.Run("revocation CreatedAt is after the token IssuedAt", func(t *testing.T) {
			ctx.testContextChanged(t)
			rev, err := ctrl.ReadRevocation(identityId)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(rev)
			ctx.Req.True(rev.CreatedAt.After(accessClaims.IssuedAt.AsTime()),
				"expected revocation CreatedAt %v to be after token IssuedAt %v",
				rev.CreatedAt, accessClaims.IssuedAt.AsTime())
		})

		t.Run("revocation propagates to the router RDM within 10s", func(t *testing.T) {
			ctx.testContextChanged(t)
			// Verifies bug 2 fix: key is identityId alone, which is what both
			// TerminateSession now stores and the RDM lookup uses.
			ctx.Req.True(router.WaitForRevocation(identityId, 10*time.Second),
				"timed out waiting for revocation %s to appear in router RDM", identityId)
		})

		t.Run("old access token is rejected by the controller", func(t *testing.T) {
			ctx.testContextChanged(t)
			_, err := ctx.NewEdgeManagementApiWithToken(oldAccessToken).GetCurrentApiSessionDetailResponse()
			ctx.Req.Error(err)
			var unauthorizedErr *managementCurrentApiSession.GetCurrentAPISessionUnauthorized
			ctx.Req.True(errors.As(err, &unauthorizedErr), "expected 401 Unauthorized, got: %v", err)
		})

		t.Run("new token issued after termination is accepted", func(t *testing.T) {
			ctx.testContextChanged(t)
			// Re-authenticate as the same identity; the new token's IssuedAt is
			// after the revocation's CreatedAt so the high-water mark does not block it.
			newClient := ctx.NewEdgeManagementApi(nil)
			_, err := newClient.AuthenticateOidc(ctx.NewAdminCredentials())
			ctx.Req.NoError(err)
			_, err = newClient.GetCurrentApiSessionDetailResponse()
			ctx.Req.NoError(err)
		})
	})

	t.Run("DeleteExpired purges revocations from DB and router RDM", func(t *testing.T) {
		ctx.testContextChanged(t)

		const testRevocationId = "test-expired-revocation-id"

		// Plant an already-expired revocation directly.
		err := ctrl.CreateRevocation(testRevocationId, time.Now().Add(-1*time.Second))
		ctx.Req.NoError(err)

		t.Run("entry is visible in the controller DB before purge", func(t *testing.T) {
			ctx.testContextChanged(t)
			rev, err := ctrl.ReadRevocation(testRevocationId)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(rev)
		})

		t.Run("entry propagates to the router RDM before purge", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.True(router.WaitForRevocation(testRevocationId, 10*time.Second),
				"timed out waiting for planted revocation to appear in router RDM")
		})

		// Purge expired entries.
		n, err := ctrl.DeleteExpiredRevocations()
		ctx.Req.NoError(err)
		ctx.Req.GreaterOrEqual(n, 1, "expected at least 1 expired revocation to be deleted")

		t.Run("entry is gone from the controller DB after purge", func(t *testing.T) {
			ctx.testContextChanged(t)
			rev, err := ctrl.ReadRevocation(testRevocationId)
			ctx.Req.NoError(err)
			ctx.Req.Nil(rev, "expected revocation to be purged from DB")
		})

		t.Run("entry is evicted from the router RDM after purge within 10s", func(t *testing.T) {
			ctx.testContextChanged(t)
			// Verifies bug 3 fix: RevocationDelete now sends DataState_Delete so
			// the router removes the entry from its in-memory map.
			ctx.Req.True(router.WaitForRevocationGone(testRevocationId, 10*time.Second),
				"timed out waiting for purged revocation to be evicted from router RDM")
		})
	})
}
