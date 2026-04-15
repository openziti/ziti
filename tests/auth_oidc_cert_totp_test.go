//go:build apitests

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// Test_Authenticate_OIDC_Cert_Totp_Refresh verifies that refreshing an OIDC access token
// that was issued after cert+TOTP authentication preserves the original AMR in the new
// access token. It also confirms that the refreshed token works on authenticated endpoints
// and maintains session continuity (same API session ID).
func Test_Authenticate_OIDC_Cert_Totp_Refresh(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementHelper := ctx.NewEdgeManagementApi(nil)
	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminCreds.CaPool = ctx.ControllerCaPool()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	clientHelper := ctx.NewEdgeClientApi(nil)

	// Shared state populated by sequential sub-tests.
	var certCreds *edge_apis.CertCredentials
	var enrolledTotpSecret string
	var authedTokens *oidc.Tokens[*oidc.IDTokenClaims]

	t.Run("sets up identity with cert auth and TOTP enrolled under a TOTP-required policy", func(t *testing.T) {
		ctx.NextTest(t)

		identityDetail, creds, err := managementHelper.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityDetail)

		identityId := *identityDetail.ID
		certCreds = creds
		certCreds.CaPool = ctx.ControllerCaPool()

		// Get an initial cert-only token to enroll TOTP against.
		initialTokens, _, err := clientHelper.RawOidcAuthRequest(certCreds)
		ctx.Req.NoError(err)

		// Configure the client transport with the cert credentials so that subsequent
		// API calls present the client certificate (required for cert PoP enforcement).
		tlsCfg := clientHelper.Components.TlsAwareTransport.GetTlsClientConfig()
		tlsCfg.Certificates = certCreds.TlsCerts()
		clientHelper.Components.TlsAwareTransport.SetTlsClientConfig(tlsCfg)
		clientHelper.Components.HttpClient.CloseIdleConnections()

		var apiSession edge_apis.ApiSession = &edge_apis.ApiSessionOidc{OidcTokens: initialTokens}
		clientHelper.ApiSession.Store(&apiSession)

		totpProvider, _, err := clientHelper.EnrollTotpMfa()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(totpProvider)
		enrolledTotpSecret = totpProvider.Secret

		authPolicy, err := managementHelper.CreateAuthPolicy(&rest_model.AuthPolicyCreate{
			Name: ToPtr("cert-totp-refresh-" + identityId),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: ToPtr(false),
					Allowed:           ToPtr(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        ToPtr(false),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                ToPtr(false),
					LockoutDurationMinutes: ToPtr(int64(0)),
					MaxAttempts:            ToPtr(int64(5)),
					MinPasswordLength:      ToPtr(int64(5)),
					RequireMixedCase:       ToPtr(false),
					RequireNumberChar:      ToPtr(false),
					RequireSpecialChar:     ToPtr(false),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireTotp: ToPtr(true),
			},
		})
		ctx.Req.NoError(err)
		ctx.Req.NotNil(authPolicy)

		_, err = managementHelper.PatchIdentity(identityId, &rest_model.IdentityPatch{
			AuthPolicyID: authPolicy.ID,
		})
		ctx.Req.NoError(err)
	})

	t.Run("re-authenticating with cert and TOTP issues tokens with correct AMR", func(t *testing.T) {
		ctx.NextTest(t)

		secret := enrolledTotpSecret
		codeProvider := edge_apis.TotpCodeProviderFunc(func() <-chan edge_apis.TotpCodeResult {
			ch := make(chan edge_apis.TotpCodeResult, 1)
			ch <- edge_apis.TotpCodeResult{Code: computeMFACode(secret)}
			return ch
		})

		tokens, _, err := clientHelper.RawOidcAuthRequestWithProviders(certCreds, nil, codeProvider)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)
		ctx.Req.NotEmpty(tokens.RefreshToken)
		authedTokens = tokens

		parser := jwt.NewParser()
		claims := &common.AccessClaims{}
		_, _, err = parser.ParseUnverified(tokens.AccessToken, claims)
		ctx.Req.NoError(err)
		ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodCert)
		ctx.Req.True(claims.TotpComplete())
		ctx.Req.True(claims.IsCertExtendable)
		ctx.Req.NotEmpty(claims.ApiSessionId)
		ctx.Req.NotZero(claims.AuthTime)
		ctx.Req.Equal(common.TokenTypeAccess, claims.Type)

		t.Run("authenticated token works on /services", func(t *testing.T) {
			ctx.NextTest(t)

			resp, err := ctx.newRequestWithTlsCerts(certCreds.TlsCerts()).
				SetHeader("Authorization", "Bearer "+tokens.AccessToken).
				Get("https://" + ctx.ApiHost + EdgeClientApiPath + "/services")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())
		})
	})

	t.Run("refreshed access token preserves AMR", func(t *testing.T) {
		ctx.NextTest(t)

		parser := jwt.NewParser()
		origClaims := &common.AccessClaims{}
		_, _, err := parser.ParseUnverified(authedTokens.AccessToken, origClaims)
		ctx.Req.NoError(err)

		req := &oidc.RefreshTokenRequest{
			RefreshToken: authedTokens.RefreshToken,
			ClientID:     authedTokens.IDTokenClaims.ClientID,
			Scopes:       []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess},
		}

		enc := oidc.NewEncoder()
		dst := map[string][]string{}
		ctx.Req.NoError(enc.Encode(req, dst))
		dst["grant_type"] = []string{string(req.GrantType())}

		newTokens := &oidc.TokenExchangeResponse{}
		resp, err := ctx.newRequestWithTlsCerts(certCreds.TlsCerts()).
			SetHeader("content-type", oidc_auth.FormContentType).
			SetMultiValueFormData(dst).
			SetResult(newTokens).
			Post("https://" + ctx.ApiHost + "/oidc/oauth/token")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())
		ctx.Req.NotEmpty(newTokens.AccessToken)
		ctx.Req.NotEmpty(newTokens.RefreshToken)

		refreshedClaims := &common.AccessClaims{}
		_, _, err = parser.ParseUnverified(newTokens.AccessToken, refreshedClaims)
		ctx.Req.NoError(err)

		ctx.Req.Contains(refreshedClaims.AuthenticationMethodsReferences, oidc_auth.AuthMethodCert)
		ctx.Req.True(refreshedClaims.TotpComplete(), "refreshed token must retain TOTP AMR")
		ctx.Req.True(refreshedClaims.IsCertExtendable)
		ctx.Req.False(refreshedClaims.IsAdmin)
		ctx.Req.Equal(common.TokenTypeAccess, refreshedClaims.Type)
		ctx.Req.Equal(origClaims.ApiSessionId, refreshedClaims.ApiSessionId, "refresh must not create a new API session")
		ctx.Req.Equal(origClaims.AuthTime, refreshedClaims.AuthTime, "auth_time must reflect original authentication, not refresh time")

		t.Run("refreshed token works on /services", func(t *testing.T) {
			ctx.NextTest(t)

			resp, err := ctx.newRequestWithTlsCerts(certCreds.TlsCerts()).
				SetHeader("Authorization", "Bearer "+newTokens.AccessToken).
				Get("https://" + ctx.ApiHost + EdgeClientApiPath + "/services")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())
		})
	})
}

// Test_Authenticate_OIDC_Cert_With_Required_Totp covers the full lifecycle of an identity
// that authenticates via OIDC cert auth and is subsequently placed under an auth policy that
// requires TOTP. It verifies that:
//   - initial cert-only tokens are invalidated after the TOTP policy is applied
//   - re-authentication with cert triggers a TOTP secondary auth challenge
//   - completing TOTP yields tokens with the correct AMR for both cert and TOTP
//   - the new tokens can access authenticated endpoints
func Test_Authenticate_OIDC_Cert_With_Required_Totp(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementHelper := ctx.NewEdgeManagementApi(nil)
	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminCreds.CaPool = ctx.ControllerCaPool()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	clientHelper := ctx.NewEdgeClientApi(nil)

	// Shared state populated by sequential sub-tests.
	var identityId string
	var certCreds *edge_apis.CertCredentials
	var initialTokens *oidc.Tokens[*oidc.IDTokenClaims]
	var enrolledTotpSecret string

	t.Run("identity enrolls and can authenticate via OIDC cert", func(t *testing.T) {
		ctx.NextTest(t)

		identityDetail, creds, err := managementHelper.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityDetail)

		identityId = *identityDetail.ID
		certCreds = creds
		certCreds.CaPool = ctx.ControllerCaPool()

		tokens, _, err := clientHelper.RawOidcAuthRequest(certCreds)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)
		initialTokens = tokens

		// Configure the client transport with the cert credentials so that subsequent
		// API calls present the client certificate (required for cert PoP enforcement).
		tlsCfg := clientHelper.Components.TlsAwareTransport.GetTlsClientConfig()
		tlsCfg.Certificates = certCreds.TlsCerts()
		clientHelper.Components.TlsAwareTransport.SetTlsClientConfig(tlsCfg)
		clientHelper.Components.HttpClient.CloseIdleConnections()

		parser := jwt.NewParser()
		claims := &common.AccessClaims{}
		_, _, err = parser.ParseUnverified(tokens.AccessToken, claims)
		ctx.Req.NoError(err)
		ctx.Req.True(claims.IsCertExtendable)
		ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodCert)
		ctx.Req.False(claims.TotpComplete(), "expected no TOTP AMR before policy requires it")
	})

	t.Run("identity enrolls in TOTP MFA", func(t *testing.T) {
		ctx.NextTest(t)

		var apiSession edge_apis.ApiSession = &edge_apis.ApiSessionOidc{OidcTokens: initialTokens}
		clientHelper.ApiSession.Store(&apiSession)

		totpProvider, _, err := clientHelper.EnrollTotpMfa()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(totpProvider)

		enrolledTotpSecret = totpProvider.Secret
		ctx.Req.NotEmpty(enrolledTotpSecret)
	})

	t.Run("auth policy requiring cert and TOTP is assigned to identity", func(t *testing.T) {
		ctx.NextTest(t)

		authPolicy, err := managementHelper.CreateAuthPolicy(&rest_model.AuthPolicyCreate{
			Name: ToPtr("cert-totp-required-" + identityId),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: ToPtr(false),
					Allowed:           ToPtr(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        ToPtr(false),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                ToPtr(false),
					LockoutDurationMinutes: ToPtr(int64(0)),
					MaxAttempts:            ToPtr(int64(5)),
					MinPasswordLength:      ToPtr(int64(5)),
					RequireMixedCase:       ToPtr(false),
					RequireNumberChar:      ToPtr(false),
					RequireSpecialChar:     ToPtr(false),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireTotp: ToPtr(true),
			},
		})
		ctx.Req.NoError(err)
		ctx.Req.NotNil(authPolicy)

		_, err = managementHelper.PatchIdentity(identityId, &rest_model.IdentityPatch{
			AuthPolicyID: authPolicy.ID,
		})
		ctx.Req.NoError(err)

		t.Run("old tokens are rejected after TOTP policy is applied", func(t *testing.T) {
			ctx.NextTest(t)

			resp, err := ctx.newAnonymousClientApiRequest().
				SetHeader("Authorization", "Bearer "+initialTokens.AccessToken).
				Get("https://" + ctx.ApiHost + EdgeClientApiPath + "/services")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
		})
	})

	t.Run("re-authenticating with cert challenges for TOTP", func(t *testing.T) {
		ctx.NextTest(t)

		_, oidcResponses, err := clientHelper.RawOidcAuthRequest(certCreds)
		ctx.Req.Error(err, "expected TOTP to be required but unsatisfied")
		ctx.Req.NotNil(oidcResponses)
		ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)

		primaryResp := oidcResponses.PrimaryCredentialResponse
		ctx.Req.Equal(http.StatusOK, primaryResp.StatusCode())
		ctx.Req.Equal("true", primaryResp.Header().Get(oidc_auth.TotpRequiredHeader))

		type authQueriesBody struct {
			AuthQueries []*rest_model.AuthQueryDetail `json:"authQueries"`
		}
		parsed := &authQueriesBody{}
		ctx.Req.NoError(json.Unmarshal(primaryResp.Body(), parsed))
		ctx.Req.Len(parsed.AuthQueries, 1)
		ctx.Req.Equal(rest_model.AuthQueryTypeTOTP, parsed.AuthQueries[0].TypeID)
		ctx.Req.True(parsed.AuthQueries[0].IsTotpEnrolled, "expected IsTotpEnrolled=true for a previously enrolled identity")
	})

	t.Run("providing TOTP code completes the OIDC flow and issues new tokens", func(t *testing.T) {
		ctx.NextTest(t)

		secret := enrolledTotpSecret
		codeProvider := edge_apis.TotpCodeProviderFunc(func() <-chan edge_apis.TotpCodeResult {
			ch := make(chan edge_apis.TotpCodeResult, 1)
			ch <- edge_apis.TotpCodeResult{Code: computeMFACode(secret)}
			return ch
		})

		newTokens, _, err := clientHelper.RawOidcAuthRequestWithProviders(certCreds, nil, codeProvider)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newTokens)
		ctx.Req.NotEmpty(newTokens.AccessToken)
		ctx.Req.NotEmpty(newTokens.IDToken)
		ctx.Req.NotEmpty(newTokens.RefreshToken)

		t.Run("new tokens contain the correct AMR for cert and TOTP", func(t *testing.T) {
			ctx.NextTest(t)

			parser := jwt.NewParser()
			newClaims := &common.AccessClaims{}
			_, _, err := parser.ParseUnverified(newTokens.AccessToken, newClaims)
			ctx.Req.NoError(err)

			ctx.Req.Contains(newClaims.AuthenticationMethodsReferences, oidc_auth.AuthMethodCert)
			ctx.Req.True(newClaims.TotpComplete(), "expected TOTP AMR in new access token")
			ctx.Req.True(newClaims.IsCertExtendable)
			ctx.Req.False(newClaims.IsAdmin)
			ctx.Req.NotEmpty(newClaims.ApiSessionId)
			ctx.Req.Equal(common.TokenTypeAccess, newClaims.Type)
		})

		t.Run("new tokens can be used on authenticated endpoints", func(t *testing.T) {
			ctx.NextTest(t)

			var apiSession edge_apis.ApiSession = &edge_apis.ApiSessionOidc{OidcTokens: newTokens}
			clientHelper.ApiSession.Store(&apiSession)

			sessionDetail, err := clientHelper.GetCurrentApiSessionDetail()
			ctx.Req.NoError(err)
			ctx.Req.NotNil(sessionDetail)
		})
	})
}
