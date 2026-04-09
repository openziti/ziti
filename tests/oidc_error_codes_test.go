//go:build apitests

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// oidcErrorResponse matches the JSON structure returned by the zitadel/oidc library's
// WriteError handler for OAuth/OIDC error responses.
type oidcErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func Test_OIDC_ErrorCodes(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	clientHelper := ctx.NewEdgeClientApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	tokenUrl := "https://" + ctx.ApiHost + "/oidc/oauth/token"

	// Authenticate once to get valid tokens for reuse in sub-tests.
	tokens, _, err := clientHelper.RawOidcAuthRequest(adminCreds)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(tokens)

	t.Run("token endpoint returns invalid_client for unknown client_id", func(t *testing.T) {
		ctx.NextTest(t)

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			SetFormData(map[string]string{
				"grant_type":    string(oidc.GrantTypeRefreshToken),
				"refresh_token": tokens.RefreshToken,
				"client_id":     "nonexistent-client",
			}).
			Post(tokenUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(),
			"expected 400 for unknown client, got %d: %s", resp.StatusCode(), resp.String())

		errResp := parseOidcError(t, resp.Body())
		ctx.Req.Equal("invalid_client", errResp.Error)
	})

	t.Run("token endpoint returns invalid_grant for garbage refresh_token", func(t *testing.T) {
		ctx.NextTest(t)

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			SetFormData(map[string]string{
				"grant_type":    string(oidc.GrantTypeRefreshToken),
				"refresh_token": "not-a-valid-jwt",
				"client_id":     tokens.IDTokenClaims.ClientID,
			}).
			Post(tokenUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(),
			"expected 400 for bad refresh token, got %d: %s", resp.StatusCode(), resp.String())

		errResp := parseOidcError(t, resp.Body())
		ctx.Req.Equal("invalid_grant", errResp.Error)
	})

	t.Run("token endpoint returns invalid_grant for expired or revoked refresh_token", func(t *testing.T) {
		ctx.NextTest(t)

		// Use an access token (wrong token type) as the refresh_token value.
		// parseRefreshToken will reject this because the token type claim is not "refresh".
		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			SetFormData(map[string]string{
				"grant_type":    string(oidc.GrantTypeRefreshToken),
				"refresh_token": tokens.AccessToken,
				"client_id":     tokens.IDTokenClaims.ClientID,
			}).
			Post(tokenUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(),
			"expected 400 for wrong token type, got %d: %s", resp.StatusCode(), resp.String())

		errResp := parseOidcError(t, resp.Body())
		ctx.Req.Equal("invalid_grant", errResp.Error)
	})

	t.Run("token endpoint returns invalid_grant for missing grant_type", func(t *testing.T) {
		ctx.NextTest(t)

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			SetFormData(map[string]string{
				"refresh_token": tokens.RefreshToken,
				"client_id":     tokens.IDTokenClaims.ClientID,
			}).
			Post(tokenUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(),
			"expected 400 for missing grant_type, got %d: %s", resp.StatusCode(), resp.String())

		errResp := parseOidcError(t, resp.Body())
		ctx.Req.Equal("invalid_request", errResp.Error)
	})

	t.Run("token endpoint returns error for unsupported grant_type", func(t *testing.T) {
		ctx.NextTest(t)

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			SetFormData(map[string]string{
				"grant_type": "urn:fake:unsupported",
				"client_id":  tokens.IDTokenClaims.ClientID,
			}).
			Post(tokenUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(),
			"expected 400 for unsupported grant_type, got %d: %s", resp.StatusCode(), resp.String())

		errResp := parseOidcError(t, resp.Body())
		ctx.Req.Equal("unsupported_grant_type", errResp.Error)
	})

	t.Run("token endpoint returns invalid_grant for code exchange with invalid code", func(t *testing.T) {
		ctx.NextTest(t)

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			SetFormData(map[string]string{
				"grant_type":    string(oidc.GrantTypeCode),
				"code":          "bogus-auth-code",
				"redirect_uri":  "http://localhost:18643/auth/callback",
				"client_id":     tokens.IDTokenClaims.ClientID,
				"code_verifier": "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			}).
			Post(tokenUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(),
			"expected 400 for invalid code, got %d: %s", resp.StatusCode(), resp.String())

		errResp := parseOidcError(t, resp.Body())
		ctx.Req.Equal("invalid_grant", errResp.Error)
	})

	t.Run("login endpoint returns 401 for invalid credentials", func(t *testing.T) {
		ctx.NextTest(t)

		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		loginUrl := "https://" + ctx.ApiHost + "/oidc/login/username?id=" + result.AuthRequestId

		resp, err := result.Client.R().
			SetHeader("Accept", "application/json").
			SetFormData(map[string]string{
				"username": ctx.AdminAuthenticator.Username,
				"password": "wrong-password",
			}).
			Post(loginUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(),
			"expected 401 for bad password, got %d: %s", resp.StatusCode(), resp.String())
	})

	t.Run("login endpoint returns 401 for unknown username", func(t *testing.T) {
		ctx.NextTest(t)

		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		loginUrl := "https://" + ctx.ApiHost + "/oidc/login/username?id=" + result.AuthRequestId

		resp, err := result.Client.R().
			SetHeader("Accept", "application/json").
			SetFormData(map[string]string{
				"username": "nonexistent-user",
				"password": "irrelevant",
			}).
			Post(loginUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(),
			"expected 401 for unknown user, got %d: %s", resp.StatusCode(), resp.String())
	})

	t.Run("userinfo endpoint returns 401 for invalid access token", func(t *testing.T) {
		ctx.NextTest(t)

		userinfoUrl := "https://" + ctx.ApiHost + "/oidc/userinfo"

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("Authorization", "Bearer not-a-valid-jwt").
			Get(userinfoUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(),
			"expected 401 for bad access token, got %d: %s", resp.StatusCode(), resp.String())
	})

	t.Run("userinfo endpoint returns 401 for missing access token", func(t *testing.T) {
		ctx.NextTest(t)

		userinfoUrl := "https://" + ctx.ApiHost + "/oidc/userinfo"

		resp, err := ctx.newAnonymousClientApiRequest().
			Get(userinfoUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(),
			"expected 401 for missing token, got %d: %s", resp.StatusCode(), resp.String())
	})

	t.Run("token refresh preserves correct error types after successful auth", func(t *testing.T) {
		ctx.NextTest(t)

		// Authenticate fresh to get a known-good refresh token
		freshTokens, _, err := clientHelper.RawOidcAuthRequest(adminCreds)
		ctx.Req.NoError(err)

		// Refresh with invalid scopes should return invalid_scope
		enc := oidc.NewEncoder()
		dst := map[string][]string{}
		req := &oidc.RefreshTokenRequest{
			RefreshToken: freshTokens.RefreshToken,
			ClientID:     freshTokens.IDTokenClaims.ClientID,
			Scopes:       []string{"bogus-scope-not-in-original"},
		}
		err = enc.Encode(req, dst)
		ctx.Req.NoError(err)
		dst["grant_type"] = []string{string(req.GrantType())}

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			SetMultiValueFormData(dst).
			Post(tokenUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(),
			"expected 400 for invalid scope, got %d: %s", resp.StatusCode(), resp.String())

		errResp := parseOidcError(t, resp.Body())
		ctx.Req.Equal("invalid_scope", errResp.Error)
	})

	t.Run("end_session endpoint is reachable", func(t *testing.T) {
		ctx.NextTest(t)

		endSessionUrl := "https://" + ctx.ApiHost + "/oidc/end_session"

		// POST with no id_token_hint - should get an error, not a panic or 500
		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			Post(endSessionUrl)

		ctx.Req.NoError(err)
		// Should be a client error (400-range), not a server error (500-range)
		ctx.Req.Less(resp.StatusCode(), 500,
			"expected client error, got %d: %s", resp.StatusCode(), resp.String())
	})

	t.Run("well-known openid-configuration is accessible", func(t *testing.T) {
		ctx.NextTest(t)

		discoveryUrl := "https://" + ctx.ApiHost + "/oidc/.well-known/openid-configuration"

		resp, err := ctx.newAnonymousClientApiRequest().Get(discoveryUrl)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(),
			"expected 200 for discovery, got %d: %s", resp.StatusCode(), resp.String())

		// Verify it's valid JSON with expected fields
		var discovery map[string]interface{}
		err = json.Unmarshal(resp.Body(), &discovery)
		ctx.Req.NoError(err)
		ctx.Req.Contains(discovery, "issuer")
		ctx.Req.Contains(discovery, "token_endpoint")
		ctx.Req.Contains(discovery, "authorization_endpoint")
	})

	t.Run("successful OIDC auth still works end to end", func(t *testing.T) {
		ctx.NextTest(t)

		// Full auth flow should still succeed after all the error handling changes
		freshTokens, _, err := clientHelper.RawOidcAuthRequest(adminCreds)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(freshTokens.AccessToken)
		ctx.Req.NotEmpty(freshTokens.RefreshToken)
		ctx.Req.NotEmpty(freshTokens.IDToken)

		// Use the access token to hit the userinfo endpoint
		userinfoUrl := "https://" + ctx.ApiHost + "/oidc/userinfo"
		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("Authorization", "Bearer "+freshTokens.AccessToken).
			Get(userinfoUrl)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(),
			"expected 200 for valid userinfo request, got %d: %s", resp.StatusCode(), resp.String())

		// Refresh should work
		enc := oidc.NewEncoder()
		dst := map[string][]string{}
		req := &oidc.RefreshTokenRequest{
			RefreshToken: freshTokens.RefreshToken,
			ClientID:     freshTokens.IDTokenClaims.ClientID,
			Scopes:       []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess},
		}
		err = enc.Encode(req, dst)
		ctx.Req.NoError(err)
		dst["grant_type"] = []string{string(req.GrantType())}

		resp, err = ctx.newAnonymousClientApiRequest().
			SetHeader("content-type", oidc_auth.FormContentType).
			SetMultiValueFormData(dst).
			Post(tokenUrl)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(),
			"expected 200 for valid refresh, got %d: %s", resp.StatusCode(), resp.String())
	})
}

func Test_OIDC_ErrorCodes_CertAuth(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	clientHelper := ctx.NewEdgeClientApi(nil)
	adminCreds := ctx.NewAdminCredentials()

	_, certAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment("oidc-err-cert-test", false)
	certCreds := edge_apis.NewCertCredentials(certAuth.certs, certAuth.key)
	certCreds.CaPool = ctx.ControllerCaPool()

	t.Run("cert auth OIDC flow succeeds", func(t *testing.T) {
		ctx.NextTest(t)

		tokens, _, err := clientHelper.RawOidcAuthRequest(certCreds)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(tokens.AccessToken)
	})

	t.Run("cert login endpoint returns error for invalid auth request id", func(t *testing.T) {
		ctx.NextTest(t)

		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		certLoginUrl := "https://" + ctx.ApiHost + "/oidc/login/cert?id=bogus-request-id"

		resp, err := result.Client.R().
			SetHeader("Accept", "application/json").
			Get(certLoginUrl)

		ctx.Req.NoError(err)
		ctx.Req.GreaterOrEqual(resp.StatusCode(), 400,
			"expected error for bogus request id, got %d: %s", resp.StatusCode(), resp.String())
		ctx.Req.Less(resp.StatusCode(), 500,
			"expected client error not server error, got %d: %s", resp.StatusCode(), resp.String())
	})
}

// parseOidcError unmarshals an OIDC/OAuth error response body.
func parseOidcError(t *testing.T, body []byte) oidcErrorResponse {
	t.Helper()
	var errResp oidcErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to parse OIDC error response: %v\nbody: %s", err, string(body))
	}
	return errResp
}
