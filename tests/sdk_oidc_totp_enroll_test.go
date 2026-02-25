//go:build apitests

package tests

import (
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
)

// TestSdkOidcTotpEnrollment covers TOTP enrollment during OIDC authentication.
// When an auth policy requires TOTP and an identity has not yet enrolled, the SDK
// must surface an enrollment event so the application can present the provisioning
// QR code to the user and collect their first TOTP code to complete enrollment.
func TestSdkOidcTotpEnrollment(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementClient := ctx.NewEdgeManagementApi(nil)
	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	_, err := managementClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	authPolicyDetail, err := managementClient.CreateAuthPolicy(&rest_model.AuthPolicyCreate{
		Name: ToPtr("totp-required-oidc-sdk-" + eid.New()),
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

	t.Run("enrollment provider completes enrollment and authentication succeeds", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds, err := managementClient.CreateUpdbIdentityWithAuthPolicy(*authPolicyDetail.ID)
		ctx.Req.NoError(err)

		var enrolledProvider *TotpProvider

		clientHelper := ctx.NewEdgeClientApi(nil)
		clientHelper.API.TotpEnrollmentProvider = edge_apis.TotpEnrollmentProviderFunc(func(provisioningUrl string) <-chan edge_apis.TotpEnrollmentResult {
			ch := make(chan edge_apis.TotpEnrollmentResult, 1)
			totpProvider := &TotpProvider{}
			if applyErr := totpProvider.ApplyProvisioningUrl(provisioningUrl); applyErr != nil {
				ch <- edge_apis.TotpEnrollmentResult{Err: applyErr}
				return ch
			}
			enrolledProvider = totpProvider
			ch <- edge_apis.TotpEnrollmentResult{Code: totpProvider.Code()}
			return ch
		})
		clientHelper.SetUseOidc(true)

		apiSession, err := clientHelper.Authenticate(creds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotEmpty(apiSession.GetToken())
		ctx.Req.NotNil(enrolledProvider)

		t.Run("issued tokens contain correct AMR after enrollment", func(t *testing.T) {
			ctx.testContextChanged(t)
			oidcSession := apiSession.(*edge_apis.ApiSessionOidc)
			parser := jwt.NewParser()
			claims := &common.AccessClaims{}
			_, _, err := parser.ParseUnverified(oidcSession.OidcTokens.AccessToken, claims)
			ctx.Req.NoError(err)
			ctx.Req.Contains(claims.AuthenticationMethodsReferences, oidc_auth.AuthMethodPassword)
			ctx.Req.True(claims.TotpComplete(), "enrolled TOTP should appear in AMR after mid-flow enrollment")
			ctx.Req.NotZero(claims.AuthTime)
		})

		t.Run("re-authentication after enrollment uses totp code provider", func(t *testing.T) {
			ctx.testContextChanged(t)

			capturedProvider := enrolledProvider
			reAuthClient := ctx.NewEdgeClientApi(func(ch chan string) {
				ch <- capturedProvider.Code()
			})
			reAuthClient.SetUseOidc(true)

			apiSession, err := reAuthClient.Authenticate(creds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(apiSession)
			ctx.Req.NotEmpty(apiSession.GetToken())
		})
	})

	t.Run("enrollment cancelled by provider returns error", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds, err := managementClient.CreateUpdbIdentityWithAuthPolicy(*authPolicyDetail.ID)
		ctx.Req.NoError(err)

		clientHelper := ctx.NewEdgeClientApi(nil)
		clientHelper.API.TotpEnrollmentProvider = edge_apis.TotpEnrollmentProviderFunc(func(provisioningUrl string) <-chan edge_apis.TotpEnrollmentResult {
			ch := make(chan edge_apis.TotpEnrollmentResult, 1)
			ch <- edge_apis.TotpEnrollmentResult{Err: errors.New("enrollment cancelled by user")}
			return ch
		})
		clientHelper.SetUseOidc(true)

		apiSession, err := clientHelper.Authenticate(creds, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(apiSession)
	})

	t.Run("enrollment with empty code returns error", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds, err := managementClient.CreateUpdbIdentityWithAuthPolicy(*authPolicyDetail.ID)
		ctx.Req.NoError(err)

		clientHelper := ctx.NewEdgeClientApi(nil)
		clientHelper.API.TotpEnrollmentProvider = edge_apis.TotpEnrollmentProviderFunc(func(provisioningUrl string) <-chan edge_apis.TotpEnrollmentResult {
			ch := make(chan edge_apis.TotpEnrollmentResult, 1)
			ch <- edge_apis.TotpEnrollmentResult{Code: ""}
			return ch
		})
		clientHelper.SetUseOidc(true)

		apiSession, err := clientHelper.Authenticate(creds, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(apiSession)
	})

	t.Run("enrollment with alpha code returns error", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds, err := managementClient.CreateUpdbIdentityWithAuthPolicy(*authPolicyDetail.ID)
		ctx.Req.NoError(err)

		clientHelper := ctx.NewEdgeClientApi(nil)
		clientHelper.API.TotpEnrollmentProvider = edge_apis.TotpEnrollmentProviderFunc(func(provisioningUrl string) <-chan edge_apis.TotpEnrollmentResult {
			ch := make(chan edge_apis.TotpEnrollmentResult, 1)
			ch <- edge_apis.TotpEnrollmentResult{Code: "abcdef"}
			return ch
		})
		clientHelper.SetUseOidc(true)

		apiSession, err := clientHelper.Authenticate(creds, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(apiSession)
	})

	t.Run("enrollment with wrong numeric code returns error", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds, err := managementClient.CreateUpdbIdentityWithAuthPolicy(*authPolicyDetail.ID)
		ctx.Req.NoError(err)

		clientHelper := ctx.NewEdgeClientApi(nil)
		clientHelper.API.TotpEnrollmentProvider = edge_apis.TotpEnrollmentProviderFunc(func(provisioningUrl string) <-chan edge_apis.TotpEnrollmentResult {
			ch := make(chan edge_apis.TotpEnrollmentResult, 1)
			ch <- edge_apis.TotpEnrollmentResult{Code: "000000"}
			return ch
		})
		clientHelper.SetUseOidc(true)

		apiSession, err := clientHelper.Authenticate(creds, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(apiSession)
	})

	t.Run("no enrollment provider returns error", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds, err := managementClient.CreateUpdbIdentityWithAuthPolicy(*authPolicyDetail.ID)
		ctx.Req.NoError(err)

		clientHelper := ctx.NewEdgeClientApi(nil)
		clientHelper.SetUseOidc(true)

		apiSession, err := clientHelper.Authenticate(creds, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(apiSession)
	})
}
