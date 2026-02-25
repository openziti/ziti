//go:build apitests

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	authenticator2 "github.com/openziti/edge-api/rest_management_api_client/authenticator"
	identity2 "github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
)

// Test_MFA_TOTP_Enrollment_During_OIDC covers when an auth policy requires TOTP
// and an identity has not yet enrolled, the identity must be able to enroll mid-OIDC-flow
// and have the flow complete (tokens issued) after successful enrollment.
func Test_MFA_TOTP_Enrollment_During_OIDC(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	// Authenticate as admin to create test resources
	managementClient := ctx.NewEdgeManagementApi(nil)
	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	apiSession, err := managementClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))
	ctx.Req.NotNil(apiSession)

	// Auth policy: UPDB primary, TOTP secondary required, no ext-jwt
	authPolicyDetail, err := managementClient.CreateAuthPolicy(&rest_model.AuthPolicyCreate{
		Name: ToPtr("totp-required-no-ext-jwt-" + eid.New()),
		Primary: &rest_model.AuthPolicyPrimary{
			Cert: &rest_model.AuthPolicyPrimaryCert{
				AllowExpiredCerts: ToPtr(false),
				Allowed:           ToPtr(false),
			},
			ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
				Allowed:        ToPtr(false),
				AllowedSigners: []string{},
			},
			Updb: &rest_model.AuthPolicyPrimaryUpdb{
				Allowed:                ToPtr(true),
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
	authPolicyId := *authPolicyDetail.ID

	// Identity with UPDB credentials, bound to the TOTP-required policy, no prior TOTP enrollment
	identityName := eid.New()
	identityPassword := eid.New()

	createIdentityParams := identity2.NewCreateIdentityParams()
	createIdentityParams.Identity = &rest_model.IdentityCreate{
		AuthPolicyID: ToPtr(authPolicyId),
		IsAdmin:      ToPtr(false),
		Name:         ToPtr(identityName),
		Type:         ToPtr(rest_model.IdentityTypeDefault),
	}
	createIdentityResp, err := managementClient.API.Identity.CreateIdentity(createIdentityParams, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))
	ctx.Req.NotNil(createIdentityResp)

	createAuthenticatorParams := authenticator2.NewCreateAuthenticatorParams()
	createAuthenticatorParams.Authenticator = &rest_model.AuthenticatorCreate{
		IdentityID: ToPtr(createIdentityResp.Payload.Data.ID),
		Method:     ToPtr("updb"),
		Password:   identityPassword,
		Username:   identityName,
	}
	createAuthenticatorResp, err := managementClient.API.Authenticator.CreateAuthenticator(createAuthenticatorParams, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))
	ctx.Req.NotNil(createAuthenticatorResp)

	identityCreds := edge_apis.NewUpdbCredentials(identityName, identityPassword)
	identityCreds.CaPool = ctx.ControllerCaPool()

	clientHelper := ctx.NewEdgeClientApi(nil)

	t.Run("primary auth returns totp auth query with IsTotpEnrolled false when not yet enrolled", func(t *testing.T) {
		ctx.NextTest(t)

		// Attempt auth with no enrollment provider — expect failure, but inspect the primary response
		_, oidcResponses, authErr := clientHelper.RawOidcAuthRequest(identityCreds)
		ctx.Req.Error(authErr)
		ctx.Req.NotNil(oidcResponses)
		ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)

		primaryResp := oidcResponses.PrimaryCredentialResponse
		ctx.Req.Equal(http.StatusOK, primaryResp.StatusCode())
		ctx.Req.Equal("true", primaryResp.Header().Get(oidc_auth.TotpRequiredHeader))

		type authResp struct {
			AuthQueries []*rest_model.AuthQueryDetail `json:"authQueries"`
		}
		parsed := &authResp{}
		ctx.Req.NoError(json.Unmarshal(primaryResp.Body(), parsed))
		ctx.Req.Len(parsed.AuthQueries, 1)
		ctx.Req.Equal(rest_model.AuthQueryTypeTOTP, parsed.AuthQueries[0].TypeID)
		ctx.Req.False(parsed.AuthQueries[0].IsTotpEnrolled, "expected IsTotpEnrolled=false for an identity that has never enrolled TOTP")
	})

	// enrolledTotpSecret is set by the enrollment sub-test and read by subsequent sub-tests.
	var enrolledTotpSecret string

	t.Run("totp enrollment during oidc flow completes and issues tokens", func(t *testing.T) {
		ctx.NextTest(t)

		var capturedProvider *TotpProvider

		enrollProvider := edge_apis.TotpEnrollmentProviderFunc(func(provisioningUrl string) <-chan edge_apis.TotpEnrollmentResult {
			ch := make(chan edge_apis.TotpEnrollmentResult, 1)
			p := &TotpProvider{}
			if applyErr := p.ApplyProvisioningUrl(provisioningUrl); applyErr != nil {
				ch <- edge_apis.TotpEnrollmentResult{Err: applyErr}
				return ch
			}
			capturedProvider = p
			ch <- edge_apis.TotpEnrollmentResult{Code: p.Code()}
			return ch
		})

		tokens, oidcResponses, authErr := clientHelper.RawOidcAuthRequestWithProviders(identityCreds, enrollProvider, nil)
		ctx.Req.NoError(authErr)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)
		ctx.Req.NotEmpty(tokens.IDToken)
		ctx.Req.NotEmpty(tokens.RefreshToken)

		// The enroll-verify response must be a redirect — confirming the fix that
		// completes the OIDC flow after enrollment rather than returning a bare 200.
		ctx.Req.NotNil(oidcResponses.TotpEnrollVerifyResponse)
		ctx.Req.Equal(http.StatusFound, oidcResponses.TotpEnrollVerifyResponse.StatusCode())

		ctx.Req.NotNil(capturedProvider)
		enrolledTotpSecret = capturedProvider.Secret

		t.Run("issued tokens contain correct AMR after enrollment", func(t *testing.T) {
			ctx.NextTest(t)
			parser := jwt.NewParser()
			claims := &common.AccessClaims{}
			_, _, err := parser.ParseUnverified(tokens.AccessToken, claims)
			ctx.Req.NoError(err)
			ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodPassword)
			ctx.Req.True(claims.TotpComplete(), "enrolled TOTP should appear in AMR after mid-flow enrollment")
			ctx.Req.NotZero(claims.AuthTime)
		})
	})

	t.Run("re-auth after enrollment shows IsTotpEnrolled true and can complete flow via totp check", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.Req.NotEmpty(enrolledTotpSecret, "prerequisite: enrolledTotpSecret must be set by enrollment test")

		secret := enrolledTotpSecret
		codeProvider := edge_apis.TotpCodeProviderFunc(func() <-chan edge_apis.TotpCodeResult {
			ch := make(chan edge_apis.TotpCodeResult, 1)
			ch <- edge_apis.TotpCodeResult{Code: computeMFACode(secret)}
			return ch
		})

		tokens, oidcResponses, authErr := clientHelper.RawOidcAuthRequestWithProviders(identityCreds, nil, codeProvider)
		ctx.Req.NoError(authErr)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)
		ctx.Req.NotEmpty(tokens.IDToken)
		ctx.Req.NotEmpty(tokens.RefreshToken)

		// Primary response must show IsTotpEnrolled=true now that enrollment is complete
		ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
		type authResp struct {
			AuthQueries []*rest_model.AuthQueryDetail `json:"authQueries"`
		}
		parsed := &authResp{}
		ctx.Req.NoError(json.Unmarshal(oidcResponses.PrimaryCredentialResponse.Body(), parsed))
		ctx.Req.Len(parsed.AuthQueries, 1)
		ctx.Req.Equal(rest_model.AuthQueryTypeTOTP, parsed.AuthQueries[0].TypeID)
		ctx.Req.True(parsed.AuthQueries[0].IsTotpEnrolled, "expected IsTotpEnrolled=true after prior enrollment")

		t.Run("re-auth tokens contain correct AMR", func(t *testing.T) {
			ctx.NextTest(t)
			parser := jwt.NewParser()
			claims := &common.AccessClaims{}
			_, _, err := parser.ParseUnverified(tokens.AccessToken, claims)
			ctx.Req.NoError(err)
			ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodPassword)
			ctx.Req.True(claims.TotpComplete(), "TOTP should appear in AMR on re-authentication")
			ctx.Req.NotZero(claims.AuthTime)
		})
	})
}
