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
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// Test_OIDC_AuthTime_And_Amr verifies that auth_time is the time the last authentication factor
// completed, and that a token exchange carries the subject token's amr.
func Test_OIDC_AuthTime_And_Amr(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementClient := ctx.NewEdgeManagementApi(nil)
	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	_, err := managementClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	authPolicyDetail, err := managementClient.CreateAuthPolicy(&rest_model.AuthPolicyCreate{
		Name: ToPtr("auth-time-amr-" + eid.New()),
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

	t.Run("auth_time is not before TOTP enrollment completed during authentication", func(t *testing.T) {
		ctx.NextTest(t)

		creds, err := managementClient.CreateUpdbIdentityWithAuthPolicy(*authPolicyDetail.ID)
		ctx.Req.NoError(err)

		var totpProvider *TotpProvider
		var enrollmentSubmittedAt time.Time

		clientHelper := ctx.NewEdgeClientApi(nil)
		clientHelper.API.TotpEnrollmentProvider = edge_apis.TotpEnrollmentProviderFunc(func(provisioningUrl string) <-chan edge_apis.TotpEnrollmentResult {
			ch := make(chan edge_apis.TotpEnrollmentResult, 1)
			provider := &TotpProvider{}
			if applyErr := provider.ApplyProvisioningUrl(provisioningUrl); applyErr != nil {
				ch <- edge_apis.TotpEnrollmentResult{Err: applyErr}
				return ch
			}
			totpProvider = provider
			time.Sleep(1100 * time.Millisecond)
			enrollmentSubmittedAt = time.Now()
			ch <- edge_apis.TotpEnrollmentResult{Code: provider.Code()}
			return ch
		})
		clientHelper.SetUseOidc(true)

		apiSession, err := clientHelper.Authenticate(creds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(totpProvider)

		claims, err := parseAccessClaims(apiSession.(*edge_apis.ApiSessionOidc).OidcTokens.AccessToken)
		ctx.Req.NoError(err)
		ctx.Req.True(claims.TotpComplete(), "TOTP enrollment during authentication must add totp to amr")
		ctx.Req.False(claims.AuthTime.AsTime().Before(enrollmentSubmittedAt.Truncate(time.Second)),
			"auth_time %s must not be before the TOTP enrollment code was submitted at %s",
			claims.AuthTime.AsTime(), enrollmentSubmittedAt)

		t.Run("auth_time is not before TOTP verification", func(t *testing.T) {
			ctx.NextTest(t)

			var totpSubmittedAt time.Time

			reAuthClient := ctx.NewEdgeClientApi(func(ch chan string) {
				time.Sleep(1100 * time.Millisecond)
				totpSubmittedAt = time.Now()
				ch <- totpProvider.Code()
			})
			reAuthClient.SetUseOidc(true)

			apiSession, err := reAuthClient.Authenticate(creds, nil)
			ctx.Req.NoError(err)

			tokens := apiSession.(*edge_apis.ApiSessionOidc).OidcTokens
			claims, err := parseAccessClaims(tokens.AccessToken)
			ctx.Req.NoError(err)
			ctx.Req.True(claims.TotpComplete(), "TOTP verification must add totp to amr")
			ctx.Req.False(claims.AuthTime.AsTime().Before(totpSubmittedAt.Truncate(time.Second)),
				"auth_time %s must not be before the TOTP code was submitted at %s",
				claims.AuthTime.AsTime(), totpSubmittedAt)

			t.Run("token exchange carries the subject token's amr", func(t *testing.T) {
				ctx.NextTest(t)

				newAccess, _, _, statusCode := doTokenExchangeRequestForClient(ctx, tokens.AccessToken, tokens.IDTokenClaims.ClientID, nil, "", false)
				ctx.Req.Equal(http.StatusOK, statusCode)

				newClaims, err := parseAccessClaims(newAccess)
				ctx.Req.NoError(err)
				ctx.Req.ElementsMatch(claims.AuthenticationMethodsReferences, newClaims.AuthenticationMethodsReferences,
					"a token minted by token exchange must carry the subject token's amr")
				ctx.Req.Equal(claims.AuthTime, newClaims.AuthTime,
					"a token minted by token exchange must carry the subject token's auth_time")
			})

			t.Run("token exchange of a refresh token carries the subject token's amr", func(t *testing.T) {
				ctx.NextTest(t)

				form := map[string][]string{
					"grant_type":         {string(oidc.GrantTypeTokenExchange)},
					"subject_token":      {tokens.RefreshToken},
					"subject_token_type": {string(oidc.RefreshTokenType)},
					"scope":              {"openid offline_access"},
					"client_id":          {tokens.IDTokenClaims.ClientID},
				}
				result := &oidc.TokenExchangeResponse{}

				resp, err := ctx.newAnonymousClientApiRequest().
					SetHeader("content-type", oidc_auth.FormContentType).
					SetMultiValueFormData(form).
					SetResult(result).
					Post("https://" + ctx.ApiHost + "/oidc/oauth/token")

				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "token exchange of a refresh token failed: %s", resp.String())

				newClaims, err := parseAccessClaims(result.AccessToken)
				ctx.Req.NoError(err)
				ctx.Req.ElementsMatch(claims.AuthenticationMethodsReferences, newClaims.AuthenticationMethodsReferences,
					"a token minted by token exchange of a refresh token must carry the subject token's amr")
				ctx.Req.Equal(claims.AuthTime, newClaims.AuthTime,
					"a token minted by token exchange of a refresh token must carry the subject token's auth_time")
			})
		})
	})
}
