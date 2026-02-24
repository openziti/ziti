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
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/current_api_session"
	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/v2/common"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/oauth2"
)

// Tests_AuthHeaders tests for www-authenticate headers in various scenarios for authentication and authorization.
// This includes OIDC authentication as well as arbitrary requests to endpoints.
func Test_OidcAuthHeaders(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	managementApiClient := ctx.NewEdgeManagementApi(nil)
	ctx.Req.NotNil(managementApiClient)

	adminCreds := ctx.NewAdminCredentials()
	apiSession, err := managementApiClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(apiSession)

	firstJwtSignerCommonName := "oidcWwwAuthHeaderTest01CommonName"
	firstJwtSignerCert, firstJwtSignerKey := newSelfSignedCert(firstJwtSignerCommonName)

	firstJwtSignerCertPem := nfpem.EncodeToString(firstJwtSignerCert)
	firstJwtSignerName := "oidcWwwAuthHeaderTest01ExtSignerName"

	ctx.Req.NotNil(firstJwtSignerKey) //todo: remove

	firstJwtSigner := &rest_model.ExternalJWTSignerCreate{
		CertPem:         &firstJwtSignerCertPem,
		ClaimsProperty:  ToPtr("sub"),
		Enabled:         ToPtr(true),
		ExternalAuthURL: ToPtr("https://oidc.wwwauth.header.tests.01.example.com"),
		Name:            &firstJwtSignerName,
		Tags:            nil,
		UseExternalID:   ToPtr(false),
		Kid:             ToPtr(uuid.New().String()),
		Issuer:          ToPtr("oidcWwwAuthHeaderTest01-issuer"),
		Audience:        ToPtr("oidcWwwAuthHeaderTest01-audience"),
		ClientID:        ToPtr("oidcWwwAuthHeaderTest01-client-id"),
		Scopes:          []string{"scope1", "scope2"},
		TargetToken:     ToPtr(rest_model.TargetTokenID),
	}

	firstJwtSignerDetails, err := managementApiClient.CreateExtJwtSigner(firstJwtSigner)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(firstJwtSignerDetails)

	secondJwtSignerCommonName := "oidcWwwAuthHeaderTest02CommonName"
	secondJwtSignerCert, secondJwtSignerKey := newSelfSignedCert(secondJwtSignerCommonName)
	secondJwtSignerCertPem := nfpem.EncodeToString(secondJwtSignerCert)
	secondJwtSignerName := "oidcWwwAuthHeaderTest02ExtSignerName"

	secondJwtSigner := &rest_model.ExternalJWTSignerCreate{
		CertPem:         &secondJwtSignerCertPem,
		ClaimsProperty:  ToPtr("sub"),
		Enabled:         ToPtr(true),
		ExternalAuthURL: ToPtr("https://oidc.wwwauth.header.tests.02.example.com"),
		Name:            &secondJwtSignerName,
		Tags:            nil,
		UseExternalID:   ToPtr(false),
		Kid:             ToPtr(uuid.New().String()),
		Issuer:          ToPtr("oidcWwwAuthHeaderTest02-issuer"),
		Audience:        ToPtr("oidcWwwAuthHeaderTest02-audience"),
		ClientID:        ToPtr("oidcWwwAuthHeaderTest02-client-id"),
		Scopes:          []string{"scope1", "scope2"},
		TargetToken:     ToPtr(rest_model.TargetTokenID),
	}

	secondJwtSignerDetails, err := managementApiClient.CreateExtJwtSigner(secondJwtSigner)

	ctx.Req.NoError(err)
	ctx.Req.NotNil(secondJwtSignerDetails)
	ctx.Req.NotNil(secondJwtSignerKey)

	authPolicyOneSigner := &rest_model.AuthPolicyCreate{
		Name: ToPtr("oidcWwwAuthHeaderTest01-" + uuid.NewString()),
		Primary: &rest_model.AuthPolicyPrimary{
			Cert: &rest_model.AuthPolicyPrimaryCert{
				AllowExpiredCerts: ToPtr(false),
				Allowed:           ToPtr(true),
			},
			ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
				Allowed:        ToPtr(true),
				AllowedSigners: []string{*firstJwtSignerDetails.ID},
			},
			Updb: &rest_model.AuthPolicyPrimaryUpdb{
				Allowed:                ToPtr(false),
				LockoutDurationMinutes: ToPtr(int64(0)),
				MaxAttempts:            ToPtr(int64(3)),
				MinPasswordLength:      ToPtr(int64(7)),
				RequireMixedCase:       ToPtr(true),
				RequireNumberChar:      ToPtr(true),
				RequireSpecialChar:     ToPtr(true),
			},
		},
		Secondary: &rest_model.AuthPolicySecondary{
			RequireExtJWTSigner: firstJwtSignerDetails.ID,
			RequireTotp:         ToPtr(false),
		},
	}

	authPolicyOneSignerDetails, err := managementApiClient.CreateAuthPolicy(authPolicyOneSigner)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(authPolicyOneSignerDetails)

	authPolicyTwoDifferentSigners := &rest_model.AuthPolicyCreate{
		Name: ToPtr("oidcWwwAuthHeaderTest02-" + uuid.NewString()),
		Primary: &rest_model.AuthPolicyPrimary{
			Cert: &rest_model.AuthPolicyPrimaryCert{
				AllowExpiredCerts: ToPtr(false),
				Allowed:           ToPtr(true),
			},
			ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
				Allowed:        ToPtr(true),
				AllowedSigners: []string{*firstJwtSignerDetails.ID},
			},
			Updb: &rest_model.AuthPolicyPrimaryUpdb{
				Allowed:                ToPtr(false),
				LockoutDurationMinutes: ToPtr(int64(0)),
				MaxAttempts:            ToPtr(int64(3)),
				MinPasswordLength:      ToPtr(int64(7)),
				RequireMixedCase:       ToPtr(true),
				RequireNumberChar:      ToPtr(true),
				RequireSpecialChar:     ToPtr(true),
			},
		},
		Secondary: &rest_model.AuthPolicySecondary{
			RequireExtJWTSigner: secondJwtSignerDetails.ID,
			RequireTotp:         ToPtr(false),
		},
	}

	authPolicyTwoDifferentSignersDetails, err := managementApiClient.CreateAuthPolicy(authPolicyTwoDifferentSigners)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(authPolicyTwoDifferentSignersDetails)

	t.Run("request with a mistyped bearer token", func(t *testing.T) {
		ctx.testContextChanged(t)

		now := time.Now()

		defaultAdmin, err := ctx.EdgeController.AppEnv.Managers.Identity.ReadDefaultAdmin()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(defaultAdmin)

		jwtSigner := ctx.EdgeController.AppEnv.GetRootTlsJwtSigner()

		claims := &common.AccessClaims{
			AccessTokenClaims: oidc.AccessTokenClaims{
				TokenClaims: oidc.TokenClaims{
					Issuer:     ctx.EdgeController.AppEnv.RootIssuer(),
					Subject:    defaultAdmin.Id,
					Audience:   []string{common.ClaimAudienceOpenZiti},
					Expiration: oidc.FromTime(now.Add(5 * time.Minute)),
					IssuedAt:   oidc.FromTime(now.Add(-2 * time.Minute)),
					AuthTime:   oidc.FromTime(now.Add(-2 * time.Minute)),
					NotBefore:  oidc.FromTime(now.Add(-2 * time.Minute)),
					Nonce:      uuid.NewString(),
					ClientID:   common.ClaimClientIdOpenZiti,
					JWTID:      uuid.NewString(),
				},
				Scopes: []string{"openid"},
			},
			CustomClaims: common.CustomClaims{
				ApiSessionId: uuid.NewString(),
				Type:         "invalid",
			},
		}

		unexpiredTokenWithWrongType, err := jwtSigner.Generate(claims)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(unexpiredTokenWithWrongType)

		t.Run("to anonymous endpoint", func(t *testing.T) {

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			var oidcToken edge_apis.ApiSession = &edge_apis.ApiSessionOidc{
				OidcTokens: &oidc.Tokens[*oidc.IDTokenClaims]{
					Token: &oauth2.Token{
						AccessToken: unexpiredTokenWithWrongType,
					},
				},
			}
			managementApiClient.ApiSession.Store(&oidcToken)

			resp, err := managementApiClient.GetVersionResponse()

			t.Run("does not error", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NoError(err)
			})

			t.Run("returns non-nil response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(resp)
			})

			t.Run("returns no www authenticate headers", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Empty(resp.WWWAuthenticate)
			})
		})

		t.Run("to an authenticated endpoint", func(t *testing.T) {
			ctx.testContextChanged(t)

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			var oidcToken edge_apis.ApiSession = &edge_apis.ApiSessionOidc{
				OidcTokens: &oidc.Tokens[*oidc.IDTokenClaims]{
					Token: &oauth2.Token{
						AccessToken: unexpiredTokenWithWrongType,
					},
				},
			}
			managementApiClient.ApiSession.Store(&oidcToken)

			resp, err := managementApiClient.GetCurrentApiSessionDetailResponse()

			unauthorizedError := &current_api_session.GetCurrentAPISessionUnauthorized{}
			unauthorizedErrorOk := errors.As(err, &unauthorizedError)

			t.Run("errors", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Error(err)
			})

			t.Run("returns nil response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Nil(resp)
			})

			t.Run("returns an error of GetCurrentAPISessionUnauthorized with 2 www-authenticate headers", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(unauthorizedError)
				ctx.Req.True(unauthorizedErrorOk)
				ctx.Req.Len(unauthorizedError.WWWAuthenticate, 2)

				t.Run("the first has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					ztSessionWa, err := ParseWWWAuthenticate(unauthorizedError.WWWAuthenticate[0])
					ctx.Req.NoError(err)

					t.Run("has a bearer missing error", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.NotNil(ztSessionWa)
						ctx.Req.Equal("zt-session", ztSessionWa.Scheme)
						ctx.Req.NotEmpty(ztSessionWa.Params)
						ctx.Req.Equal("missing", ztSessionWa.Params["error"])
					})
				})

				t.Run("the second has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					ztSessionWa, err := ParseWWWAuthenticate(unauthorizedError.WWWAuthenticate[1])
					ctx.Req.NoError(err)

					t.Run("has a bearer missing error", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.NotNil(ztSessionWa)
						ctx.Req.Equal("Bearer", ztSessionWa.Scheme)
						ctx.Req.NotEmpty(ztSessionWa.Params)
						ctx.Req.Equal("missing", ztSessionWa.Params["error"])
					})
				})
			})

		})
	})

	t.Run("request with a malformed bearer token", func(t *testing.T) {
		ctx.testContextChanged(t)

		const malformedToken = `sadr23f23f23f23t23f23f2f23f23f23f23f23f2f323`

		t.Run("to anonymous endpoint", func(t *testing.T) {

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			var oidcToken edge_apis.ApiSession = &edge_apis.ApiSessionOidc{
				OidcTokens: &oidc.Tokens[*oidc.IDTokenClaims]{
					Token: &oauth2.Token{
						AccessToken: malformedToken,
					},
				},
			}
			managementApiClient.ApiSession.Store(&oidcToken)

			resp, err := managementApiClient.GetVersionResponse()

			t.Run("does not error", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NoError(err)
			})

			t.Run("returns non-nil response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(resp)
			})

			t.Run("returns no www authenticate headers", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Empty(resp.WWWAuthenticate)
			})
		})

		t.Run("to an authenticated endpoint", func(t *testing.T) {
			ctx.testContextChanged(t)

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			var oidcToken edge_apis.ApiSession = &edge_apis.ApiSessionOidc{
				OidcTokens: &oidc.Tokens[*oidc.IDTokenClaims]{
					Token: &oauth2.Token{
						AccessToken: malformedToken,
					},
				},
			}
			managementApiClient.ApiSession.Store(&oidcToken)

			resp, err := managementApiClient.GetCurrentApiSessionDetailResponse()

			unauthorizedError := &current_api_session.GetCurrentAPISessionUnauthorized{}
			unauthorizedErrorOk := errors.As(err, &unauthorizedError)

			t.Run("errors", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Error(err)
			})

			t.Run("returns nil response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Nil(resp)
			})

			t.Run("returns an error of GetCurrentAPISessionUnauthorized with 1 www-authenticate headers", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(unauthorizedError)
				ctx.Req.True(unauthorizedErrorOk)
				ctx.Req.Len(unauthorizedError.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					bearerWa, err := ParseWWWAuthenticate(unauthorizedError.WWWAuthenticate[0])
					ctx.Req.NoError(err)

					t.Run("has a bearer invalid error", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.NotNil(bearerWa)
						ctx.Req.Equal("Bearer", bearerWa.Scheme)
						ctx.Req.NotEmpty(bearerWa.Params)
						ctx.Req.Equal("invalid", bearerWa.Params["error"])
					})
				})
			})

		})
	})

	t.Run("request with correctly issued but expired bearer token", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtSigner := ctx.EdgeController.AppEnv.GetRootTlsJwtSigner()
		ctx.Req.NotNil(jwtSigner)

		defaultAdmin, err := ctx.EdgeController.AppEnv.Managers.Identity.ReadDefaultAdmin()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(defaultAdmin)

		now := time.Now()

		claims := &common.AccessClaims{
			AccessTokenClaims: oidc.AccessTokenClaims{
				TokenClaims: oidc.TokenClaims{
					Issuer:     ctx.EdgeController.AppEnv.RootIssuer(),
					Subject:    defaultAdmin.Id,
					Audience:   []string{common.ClaimAudienceOpenZiti},
					Expiration: oidc.FromTime(now.Add(-1 * time.Minute)),
					IssuedAt:   oidc.FromTime(now.Add(-2 * time.Minute)),
					AuthTime:   oidc.FromTime(now.Add(-2 * time.Minute)),
					NotBefore:  oidc.FromTime(now.Add(-2 * time.Minute)),
					Nonce:      uuid.NewString(),
					ClientID:   common.ClaimClientIdOpenZiti,
					JWTID:      uuid.NewString(),
				},
				Scopes: []string{"openid"},
			},
			CustomClaims: common.CustomClaims{
				ApiSessionId: uuid.NewString(),
				Type:         common.TokenTypeAccess,
			},
		}

		jwtToken, err := jwtSigner.Generate(claims)

		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtToken)

		t.Run("to anonymous endpoint", func(t *testing.T) {

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			var oidcToken edge_apis.ApiSession = &edge_apis.ApiSessionOidc{
				OidcTokens: &oidc.Tokens[*oidc.IDTokenClaims]{
					Token: &oauth2.Token{
						AccessToken: jwtToken,
					},
				},
			}
			managementApiClient.ApiSession.Store(&oidcToken)

			resp, err := managementApiClient.GetVersionResponse()

			t.Run("does not error", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NoError(err)
			})

			t.Run("returns non-nil response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(resp)
			})

			t.Run("returns no www authenticate headers", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Empty(resp.WWWAuthenticate)
			})
		})

		t.Run("to an authenticated endpoint", func(t *testing.T) {
			ctx.testContextChanged(t)

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			var oidcToken edge_apis.ApiSession = &edge_apis.ApiSessionOidc{
				OidcTokens: &oidc.Tokens[*oidc.IDTokenClaims]{
					Token: &oauth2.Token{
						AccessToken: jwtToken,
					},
				},
			}
			managementApiClient.ApiSession.Store(&oidcToken)

			resp, err := managementApiClient.GetCurrentApiSessionDetailResponse()

			unauthorizedError := &current_api_session.GetCurrentAPISessionUnauthorized{}
			unauthorizedErrorOk := errors.As(err, &unauthorizedError)

			t.Run("errors", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Error(err)
			})

			t.Run("returns nil response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Nil(resp)
			})

			t.Run("returns an error of GetCurrentAPISessionUnauthorized with 1 www-authenticate headers", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(unauthorizedError)
				ctx.Req.True(unauthorizedErrorOk)
				ctx.Req.Len(unauthorizedError.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					oidcWa, err := ParseWWWAuthenticate(unauthorizedError.WWWAuthenticate[0])
					ctx.Req.NoError(err)

					t.Run("has a bearer expired error", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.NotNil(oidcWa)
						ctx.Req.Equal("Bearer", oidcWa.Scheme)
						ctx.Req.NotEmpty(oidcWa.Params)
						ctx.Req.Equal("expired", oidcWa.Params["error"])
					})
				})
			})
		})
	})

	t.Run("request with improperly signed token", func(t *testing.T) {
		ctx.testContextChanged(t)

		//intentionally using the key id the controller will look for
		jwtSigner := NewRandomHmacSignerWithKeyId(ctx.EdgeController.AppEnv.GetRootTlsJwtSigner().KeyId())
		ctx.Req.NotNil(jwtSigner)

		defaultAdmin, err := ctx.EdgeController.AppEnv.Managers.Identity.ReadDefaultAdmin()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(defaultAdmin)

		now := time.Now()

		claims := &common.AccessClaims{
			AccessTokenClaims: oidc.AccessTokenClaims{
				TokenClaims: oidc.TokenClaims{
					Issuer:     ctx.EdgeController.AppEnv.RootIssuer(),
					Subject:    defaultAdmin.Id,
					Audience:   []string{common.ClaimAudienceOpenZiti},
					Expiration: oidc.FromTime(now.Add(5 * time.Minute)),
					IssuedAt:   oidc.FromTime(now.Add(-2 * time.Minute)),
					AuthTime:   oidc.FromTime(now.Add(-2 * time.Minute)),
					NotBefore:  oidc.FromTime(now.Add(-2 * time.Minute)),
					Nonce:      uuid.NewString(),
					ClientID:   common.ClaimClientIdOpenZiti,
					JWTID:      uuid.NewString(),
				},
				Scopes: []string{"openid"},
			},
			CustomClaims: common.CustomClaims{
				ApiSessionId: uuid.NewString(),
				Type:         common.TokenTypeAccess,
			},
		}

		improperlySignedToken, err := jwtSigner.Generate(claims)

		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(improperlySignedToken)

		t.Run("to anonymous endpoint", func(t *testing.T) {

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			var oidcToken edge_apis.ApiSession = &edge_apis.ApiSessionOidc{
				OidcTokens: &oidc.Tokens[*oidc.IDTokenClaims]{
					Token: &oauth2.Token{
						AccessToken: improperlySignedToken,
					},
				},
			}
			managementApiClient.ApiSession.Store(&oidcToken)

			resp, err := managementApiClient.GetVersionResponse()

			t.Run("does not error", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NoError(err)
			})

			t.Run("returns non-nil response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(resp)
			})

			t.Run("returns no www authenticate headers", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Empty(resp.WWWAuthenticate)
			})
		})

		t.Run("to an authenticated endpoint", func(t *testing.T) {
			ctx.testContextChanged(t)

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			var oidcToken edge_apis.ApiSession = &edge_apis.ApiSessionOidc{
				OidcTokens: &oidc.Tokens[*oidc.IDTokenClaims]{
					Token: &oauth2.Token{
						AccessToken: improperlySignedToken,
					},
				},
			}
			managementApiClient.ApiSession.Store(&oidcToken)

			resp, err := managementApiClient.GetCurrentApiSessionDetailResponse()

			unauthorizedError := &current_api_session.GetCurrentAPISessionUnauthorized{}
			unauthorizedErrorOk := errors.As(err, &unauthorizedError)

			t.Run("errors", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Error(err)
			})

			t.Run("returns nil response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Nil(resp)
			})

			t.Run("returns an error of GetCurrentAPISessionUnauthorized with 1 www-authenticate headers", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(unauthorizedError)
				ctx.Req.True(unauthorizedErrorOk)
				ctx.Req.Len(unauthorizedError.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					oidcWa, err := ParseWWWAuthenticate(unauthorizedError.WWWAuthenticate[0])
					ctx.Req.NoError(err)

					t.Run("has a bearer invalid error", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.NotNil(oidcWa)
						ctx.Req.Equal("Bearer", oidcWa.Scheme)
						ctx.Req.NotEmpty(oidcWa.Params)
						ctx.Req.Equal("invalid", oidcWa.Params["error"])
					})
				})
			})
		})
	})

	t.Run("OIDC cert auth with a secondary JWT required", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityDetails, identityCreds, err := managementApiClient.CreateAndEnrollOttIdentity(false)
		identityCreds.CaPool = ctx.ControllerCaPool()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityDetails)
		ctx.Req.NotNil(identityCreds)

		identityPatch := &rest_model.IdentityPatch{
			AuthPolicyID: authPolicyOneSignerDetails.ID,
		}

		identityDetails, err = managementApiClient.PatchIdentity(*identityDetails.ID, identityPatch)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityDetails)

		t.Run("with no jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)
			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(identityCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid values", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-secondary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("missing", wwwAuthWa.Params["error"])
							ctx.Req.Equal(*firstJwtSignerDetails.ID, wwwAuthWa.Params["id"])
							ctx.Req.Equal(*firstJwtSignerDetails.Issuer, wwwAuthWa.Params["issuer"])
						})
					})
				})
			})
		})

		t.Run("with an expired jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			expiredToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": firstJwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *firstJwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*firstJwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			expiredTokenStr, err := expiredToken.SignedString(firstJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(expiredTokenStr)

			identityCreds.AddJWT(expiredTokenStr)

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(identityCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid value", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-secondary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("expired", wwwAuthWa.Params["error"])
							ctx.Req.Equal(*firstJwtSignerDetails.ID, wwwAuthWa.Params["id"])
							ctx.Req.Equal(*firstJwtSignerDetails.Issuer, wwwAuthWa.Params["issuer"])
						})
					})
				})
			})
		})

		t.Run("with a malformed jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			malformedToken := "ey238tuwejfwodijvoij.20982j824f20fj420f9j092.292j093393fgjoeif"

			identityCreds.AddJWT(malformedToken)

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(identityCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid value", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-secondary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("missing", wwwAuthWa.Params["error"])
							ctx.Req.Equal(*firstJwtSignerDetails.ID, wwwAuthWa.Params["id"])
							ctx.Req.Equal(*firstJwtSignerDetails.Issuer, wwwAuthWa.Params["issuer"])
						})
					})
				})
			})
		})

		t.Run("with a valid jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			goodToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": firstJwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *firstJwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*firstJwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			goodTokenStr, err := goodToken.SignedString(firstJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(goodTokenStr)

			identityCreds.AddJWT(goodTokenStr)

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(identityCreds)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("returns credential response", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)

				t.Run("with no authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 0)

				})
			})

			t.Run("returns a redirect response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(oidcResponses.RedirectResponse)
			})
		})
	})

	t.Run("OIDC jwt auth with the same primary and secondary jwt required", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityCreateLoc, err := managementApiClient.CreateIdentity(uuid.NewString(), false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityCreateLoc)

		identityPatch := &rest_model.IdentityPatch{
			AuthPolicyID: authPolicyOneSignerDetails.ID,
		}

		identityDetails, err := managementApiClient.PatchIdentity(identityCreateLoc.ID, identityPatch)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityDetails)

		t.Run("with no jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtCreds := edge_apis.NewJwtCredentials("")
			jwtCreds.CaPool = ctx.ControllerCaPool()

			clientApi := ctx.NewEdgeClientApi(nil)
			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(jwtCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid values", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-primary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("missing", wwwAuthWa.Params["error"])

							pipeIds := wwwAuthWa.Params["id"]
							ids := strings.Split(pipeIds, "|")
							ctx.Req.Contains(ids, *firstJwtSignerDetails.ID)

							pipeIssuers := wwwAuthWa.Params["issuer"]
							issuers := strings.Split(pipeIssuers, "|")
							ctx.Req.Contains(issuers, *firstJwtSignerDetails.Issuer)
						})
					})
				})
			})
		})

		t.Run("with an expired jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			expiredToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": firstJwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *firstJwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*firstJwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			expiredTokenStr, err := expiredToken.SignedString(firstJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(expiredTokenStr)

			jwtCreds := edge_apis.NewJwtCredentials(expiredTokenStr)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(jwtCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid value", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-primary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("expired", wwwAuthWa.Params["error"])

							pipeIds := wwwAuthWa.Params["id"]
							ids := strings.Split(pipeIds, "|")
							ctx.Req.Contains(ids, *firstJwtSignerDetails.ID)

							pipeIssuers := wwwAuthWa.Params["issuer"]
							issuers := strings.Split(pipeIssuers, "|")
							ctx.Req.Contains(issuers, *firstJwtSignerDetails.Issuer)
						})
					})
				})
			})
		})

		t.Run("with a malformed jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			malformedToken := "ey238tuwejfwodijvoij.20982j824f20fj420f9j092.292j093393fgjoeif"
			jwtCreds := edge_apis.NewJwtCredentials(malformedToken)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(jwtCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid value", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-primary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("missing", wwwAuthWa.Params["error"])

							pipeIds := wwwAuthWa.Params["id"]
							ids := strings.Split(pipeIds, "|")
							ctx.Req.Contains(ids, *firstJwtSignerDetails.ID)

							pipeIssuers := wwwAuthWa.Params["issuer"]
							issuers := strings.Split(pipeIssuers, "|")
							ctx.Req.Contains(issuers, *firstJwtSignerDetails.Issuer)
						})
					})
				})
			})
		})

		t.Run("with a valid jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			goodToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": firstJwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *firstJwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*firstJwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			goodTokenStr, err := goodToken.SignedString(firstJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(goodTokenStr)

			jwtCreds := edge_apis.NewJwtCredentials(goodTokenStr)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(jwtCreds)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("returns credential response", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)

				t.Run("with no authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 0)

				})
			})

			t.Run("returns a redirect response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(oidcResponses.RedirectResponse)
			})
		})
	})

	t.Run("OIDC jwt auth with different primary and secondary jwt required", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityCreateLoc, err := managementApiClient.CreateIdentity(uuid.NewString(), false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityCreateLoc)

		identityPatch := &rest_model.IdentityPatch{
			AuthPolicyID: authPolicyTwoDifferentSignersDetails.ID,
		}

		identityDetails, err := managementApiClient.PatchIdentity(identityCreateLoc.ID, identityPatch)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityDetails)

		t.Run("with no jwts", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtCreds := edge_apis.NewJwtCredentials("")
			jwtCreds.CaPool = ctx.ControllerCaPool()

			clientApi := ctx.NewEdgeClientApi(nil)
			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(jwtCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid values", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-primary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("missing", wwwAuthWa.Params["error"])

							pipeIds := wwwAuthWa.Params["id"]
							ids := strings.Split(pipeIds, "|")
							ctx.Req.Contains(ids, *firstJwtSignerDetails.ID)

							pipeIssuers := wwwAuthWa.Params["issuer"]
							issuers := strings.Split(pipeIssuers, "|")
							ctx.Req.Contains(issuers, *firstJwtSignerDetails.Issuer)
						})
					})
				})
			})
		})

		t.Run("with an expired primary jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			expiredPrimaryToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": firstJwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *firstJwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*firstJwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			expiredTokenStr, err := expiredPrimaryToken.SignedString(firstJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(expiredTokenStr)

			jwtCreds := edge_apis.NewJwtCredentials(expiredTokenStr)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(jwtCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid value", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-primary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("expired", wwwAuthWa.Params["error"])

							pipeIds := wwwAuthWa.Params["id"]
							ids := strings.Split(pipeIds, "|")
							ctx.Req.Contains(ids, *firstJwtSignerDetails.ID)

							pipeIssuers := wwwAuthWa.Params["issuer"]
							issuers := strings.Split(pipeIssuers, "|")
							ctx.Req.Contains(issuers, *firstJwtSignerDetails.Issuer)
						})
					})
				})
			})
		})

		t.Run("with a malformed primary jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			malformedToken := "ey238tuwejfwodijvoij.20982j824f20fj420f9j092.292j093393fgjoeif"
			jwtCreds := edge_apis.NewJwtCredentials(malformedToken)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(jwtCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid value", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-primary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("missing", wwwAuthWa.Params["error"])

							pipeIds := wwwAuthWa.Params["id"]
							ids := strings.Split(pipeIds, "|")
							ctx.Req.Contains(ids, *firstJwtSignerDetails.ID)

							pipeIssuers := wwwAuthWa.Params["issuer"]
							issuers := strings.Split(pipeIssuers, "|")
							ctx.Req.Contains(issuers, *firstJwtSignerDetails.Issuer)
						})
					})
				})
			})
		})

		t.Run("with a valid primary jwt and no secondary jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			goodPrimaryToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": firstJwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *firstJwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*firstJwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			goodTokenStr, err := goodPrimaryToken.SignedString(firstJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(goodTokenStr)

			jwtCreds := edge_apis.NewJwtCredentials(goodTokenStr)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(jwtCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid value", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-secondary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("missing", wwwAuthWa.Params["error"])
							ctx.Req.Equal(*secondJwtSignerDetails.ID, wwwAuthWa.Params["id"])
							ctx.Req.Equal(*secondJwtSignerDetails.Issuer, wwwAuthWa.Params["issuer"])
						})
					})
				})
			})
		})

		t.Run("with a valid primary jwt and an expired secondary jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			goodPrimaryToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": firstJwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *firstJwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*firstJwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			goodTokenStr, err := goodPrimaryToken.SignedString(firstJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(goodTokenStr)

			expiredSecondaryToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": secondJwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *secondJwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*secondJwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			expiredSecondaryTokenStr, err := expiredSecondaryToken.SignedString(secondJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(expiredSecondaryToken)

			jwtCreds := edge_apis.NewJwtCredentials(goodTokenStr)
			jwtCreds.AddJWT(expiredSecondaryTokenStr)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(jwtCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does not redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.Nil(oidcResponses.RedirectResponse)

				t.Run("with a valid www authenticate header", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader := oidcResponses.PrimaryCredentialResponse.Header().Values("WWW-Authenticate")
					ctx.Req.Len(wwwAuthHeader, 1)

					t.Run("that has a valid format", func(t *testing.T) {
						ctx.testContextChanged(t)

						wwwAuthWa, err := ParseWWWAuthenticate(wwwAuthHeader[0])
						ctx.Req.NoError(err)
						ctx.Req.NotNil(wwwAuthWa)

						t.Run("and has valid value", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.Equal("Bearer", wwwAuthWa.Scheme)
							ctx.Req.Equal("openziti-secondary-ext-jwt", wwwAuthWa.Params["realm"])
							ctx.Req.Equal("expired", wwwAuthWa.Params["error"])
							ctx.Req.Equal(*secondJwtSignerDetails.ID, wwwAuthWa.Params["id"])
							ctx.Req.Equal(*secondJwtSignerDetails.Issuer, wwwAuthWa.Params["issuer"])
						})
					})
				})
			})
		})

		t.Run("with a valid primary jwt and a valid secondary jwt", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			goodPrimaryToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": firstJwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *firstJwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*firstJwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			goodPrimaryTokenStr, err := goodPrimaryToken.SignedString(firstJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(goodPrimaryTokenStr)

			goodSecondaryToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": secondJwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *secondJwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*secondJwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			goodSecondaryTokenStr, err := goodSecondaryToken.SignedString(secondJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(goodSecondaryToken)

			jwtCreds := edge_apis.NewJwtCredentials(goodPrimaryTokenStr)
			jwtCreds.AddJWT(goodSecondaryTokenStr)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			tokens, oidcResponses, err := clientApi.RawOidcAuthRequest(jwtCreds)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(tokens)
			ctx.Req.NotNil(oidcResponses)
			ctx.Req.NotNil(oidcResponses.InitResponse)

			t.Run("does redirect", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotNil(oidcResponses.PrimaryCredentialResponse)
				ctx.Req.NotNil(oidcResponses.RedirectResponse)
				ctx.Req.NotNil(tokens)
				ctx.Req.Len(oidcResponses.PrimaryCredentialResponse.Header().Values("www-authenticate"), 0)
			})
		})
	})
}
