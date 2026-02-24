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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_client_api_client/authentication"
	"github.com/openziti/edge-api/rest_management_api_client/current_api_session"
	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/v2/controller/jwtsigner"
)

// Tests_AuthHeaders tests for www-authenticate headers in various scenarios for authentication and authorization.
// This includes legacy authentication as well as arbitrary requests to endpoints.
func Test_Legacy_AuthHeaders(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	t.Run("request with no tokens", func(t *testing.T) {
		t.Run("to anonymous endpoint", func(t *testing.T) {

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

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

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			resp, err := managementApiClient.GetCurrentApiSessionDetailResponse()

			unauthorizedError := &current_api_session.GetCurrentAPISessionUnauthorized{}
			unauthorizedErrorOk := errors.As(err, &unauthorizedError)

			t.Run("errors", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Error(err)
			})

			t.Run("returns an error of GetCurrentAPISessionUnauthorized with 2 www-authenticate headers", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(unauthorizedError)
				ctx.Req.True(unauthorizedErrorOk)
				ctx.Req.Len(unauthorizedError.WWWAuthenticate, 2)

				t.Run("that have a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					var bearerWa *WWWAuthenticate
					var ztSessionWa *WWWAuthenticate

					for _, header := range unauthorizedError.WWWAuthenticate {
						wa, err := ParseWWWAuthenticate(header)
						ctx.Req.NoError(err)

						if err == nil {
							if wa.Scheme == "Bearer" {
								bearerWa = wa
							} else if wa.Scheme == "zt-session" {
								ztSessionWa = wa
							} else {
								ctx.Req.Failf("unexpected www-authenticate scheme: %s", wa.Scheme)
							}
						}
					}

					t.Run("and contain a bearer missing error", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.NotNil(bearerWa)
						ctx.Req.Equal("Bearer", bearerWa.Scheme)
						ctx.Req.NotEmpty(bearerWa.Params)
						ctx.Req.Equal("missing", bearerWa.Params["error"])
						ctx.Req.NotEmpty(bearerWa.Params["error_description"])
					})

					t.Run("and contain a zt-session missing error", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.NotNil(ztSessionWa)
						ctx.Req.Equal("zt-session", ztSessionWa.Scheme)
						ctx.Req.NotEmpty(ztSessionWa.Params)
						ctx.Req.Equal("missing", ztSessionWa.Params["error"])
					})
				})
			})

			t.Run("returns nil response", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Nil(resp)
			})
		})
	})

	t.Run("request with an invalid zt-session uuid", func(t *testing.T) {
		ctx.testContextChanged(t)

		t.Run("to anonymous endpoint", func(t *testing.T) {

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			var ztApiSession edge_apis.ApiSession = &edge_apis.ApiSessionLegacy{
				Detail: &rest_model.CurrentAPISessionDetail{
					APISessionDetail: rest_model.APISessionDetail{
						Token: ToPtr(uuid.NewString()),
					},
				},
			}
			managementApiClient.ApiSession.Store(&ztApiSession)

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

			var ztApiSession edge_apis.ApiSession = &edge_apis.ApiSessionLegacy{
				Detail: &rest_model.CurrentAPISessionDetail{
					APISessionDetail: rest_model.APISessionDetail{
						Token: ToPtr(uuid.NewString()),
					},
				},
			}
			managementApiClient.ApiSession.Store(&ztApiSession)

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

					ztSessionWa, err := ParseWWWAuthenticate(unauthorizedError.WWWAuthenticate[0])
					ctx.Req.NoError(err)

					t.Run("has a zt-session invalid error", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.NotNil(ztSessionWa)
						ctx.Req.Equal("zt-session", ztSessionWa.Scheme)
						ctx.Req.NotEmpty(ztSessionWa.Params)
						ctx.Req.Equal("invalid", ztSessionWa.Params["error"])
					})
				})
			})

		})
	})

	t.Run("request with a malformed zt-session", func(t *testing.T) {
		ctx.testContextChanged(t)

		t.Run("to anonymous endpoint", func(t *testing.T) {

			managementApiClient := ctx.NewEdgeManagementApi(nil)
			ctx.Req.NotNil(managementApiClient)

			var ztApiSession edge_apis.ApiSession = &edge_apis.ApiSessionLegacy{
				Detail: &rest_model.CurrentAPISessionDetail{
					APISessionDetail: rest_model.APISessionDetail{
						Token: ToPtr("30jy093jh590-hj4509hj5409"),
					},
				},
			}
			managementApiClient.ApiSession.Store(&ztApiSession)

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

			var ztApiSession edge_apis.ApiSession = &edge_apis.ApiSessionLegacy{
				Detail: &rest_model.CurrentAPISessionDetail{
					APISessionDetail: rest_model.APISessionDetail{
						Token: ToPtr(uuid.NewString()),
					},
				},
			}
			managementApiClient.ApiSession.Store(&ztApiSession)

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

					ztSessionWa, err := ParseWWWAuthenticate(unauthorizedError.WWWAuthenticate[0])
					ctx.Req.NoError(err)

					t.Run("has a zt-session invalid error", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.NotNil(ztSessionWa)
						ctx.Req.Equal("zt-session", ztSessionWa.Scheme)
						ctx.Req.NotEmpty(ztSessionWa.Params)
						ctx.Req.Equal("invalid", ztSessionWa.Params["error"])
					})
				})
			})

		})
	})

	t.Run("cert auth with secondary signer", func(t *testing.T) {
		ctx.testContextChanged(t)

		managementApiClient := ctx.NewEdgeManagementApi(nil)
		ctx.Req.NotNil(managementApiClient)

		adminCreds := ctx.NewAdminCredentials()
		apiSession, err := managementApiClient.Authenticate(adminCreds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(apiSession)

		jwtSignerCommonName := "soCommon"
		jwtSignerCert, jwtSignerKey := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey
		jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)
		jwtSignerName := "Test JWT Signer"
		jwtSignerEnabled := true

		jwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:         &jwtSignerCertPem,
			ClaimsProperty:  ToPtr("sub"),
			Enabled:         &jwtSignerEnabled,
			ExternalAuthURL: ToPtr("https://some-auth-url"),
			Name:            &jwtSignerName,
			Tags:            nil,
			UseExternalID:   ToPtr(false),
			Kid:             ToPtr(uuid.New().String()),
			Issuer:          ToPtr("i-am-the-issuer"),
			Audience:        ToPtr("you-are-the-audience"),
			ClientID:        ToPtr("you-are-the-client-id"),
			Scopes:          []string{"scope1", "scope2"},
			TargetToken:     ToPtr(rest_model.TargetTokenID),
		}

		signerDetail, err := managementApiClient.CreateExtJwtSigner(jwtSigner)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(signerDetail)

		authPolicy := &rest_model.AuthPolicyCreate{
			Name: ToPtr("testPolicyCertAndSecondarySigner-" + uuid.NewString()),
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
					MaxAttempts:            ToPtr(int64(3)),
					MinPasswordLength:      ToPtr(int64(7)),
					RequireMixedCase:       ToPtr(true),
					RequireNumberChar:      ToPtr(true),
					RequireSpecialChar:     ToPtr(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: signerDetail.ID,
				RequireTotp:         ToPtr(false),
			},
		}

		authPolicyDetails, err := managementApiClient.CreateAuthPolicy(authPolicy)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(authPolicyDetails)

		identityDetails, identityCreds, err := managementApiClient.CreateAndEnrollOttIdentity(false)
		identityCreds.CaPool = ctx.ControllerCaPool()
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityDetails)
		ctx.Req.NotNil(identityCreds)

		identityPatch := &rest_model.IdentityPatch{
			AuthPolicyID: authPolicyDetails.ID,
		}

		identityDetails, err = managementApiClient.PatchIdentity(*identityDetails.ID, identityPatch)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityDetails)

		t.Run("if a jwt is not supplied", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)
			resp, err := clientApi.RawLegacyAuthRequest(identityCreds)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)

			t.Run("a www-authenticate header is returned", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Len(resp.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader, err := ParseWWWAuthenticate(resp.WWWAuthenticate[0])
					ctx.Req.NoError(err)
					ctx.Req.NotNil(wwwAuthHeader)

					t.Run("is a bearer scheme", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("Bearer", wwwAuthHeader.Scheme)
					})

					t.Run("is an ext jwt realm", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("openziti-secondary-ext-jwt", wwwAuthHeader.Params["realm"])
					})

					t.Run("that has a missing error", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("missing", wwwAuthHeader.Params["error"])
					})

					t.Run("has the correct ext jwt id", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["id"])

						ids := strings.Split(wwwAuthHeader.Params["id"], "|")
						ctx.Req.Contains(ids, *signerDetail.ID)
					})

					t.Run("has the correct issuer", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])

						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])
						issuers := strings.Split(wwwAuthHeader.Params["issuer"], "|")
						ctx.Req.Contains(issuers, *jwtSigner.Issuer)
					})
				})
			})
		})

		t.Run("if an expired jwt is supplied", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			expiredToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": jwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *jwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*jwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			expiredTokenStr, err := expiredToken.SignedString(jwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(expiredTokenStr)

			identityCreds.AddJWT(expiredTokenStr)

			resp, err := clientApi.RawLegacyAuthRequest(identityCreds)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)

			t.Run("a www-authenticate header is returned", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Len(resp.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader, err := ParseWWWAuthenticate(resp.WWWAuthenticate[0])
					ctx.Req.NoError(err)
					ctx.Req.NotNil(wwwAuthHeader)

					t.Run("is a bearer scheme", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("Bearer", wwwAuthHeader.Scheme)
					})

					t.Run("is an ext jwt realm", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("openziti-secondary-ext-jwt", wwwAuthHeader.Params["realm"])
					})

					t.Run("that has a expired error", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("expired", wwwAuthHeader.Params["error"])
					})

					t.Run("has the correct ext jwt id", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["id"])

						ids := strings.Split(wwwAuthHeader.Params["id"], "|")
						ctx.Req.Contains(ids, *signerDetail.ID)
					})

					t.Run("has the correct issuer", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])
						issuers := strings.Split(wwwAuthHeader.Params["issuer"], "|")
						ctx.Req.Contains(issuers, *jwtSigner.Issuer)
					})
				})
			})
		})

		t.Run("if a malformed jwt is supplied", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			malformedTokenStr := "ey23849384932.234238434928.a9e8hg4928h2"

			identityCreds.AddJWT(malformedTokenStr)

			resp, err := clientApi.RawLegacyAuthRequest(identityCreds)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)

			t.Run("a www-authenticate header is returned", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Len(resp.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader, err := ParseWWWAuthenticate(resp.WWWAuthenticate[0])
					ctx.Req.NoError(err)
					ctx.Req.NotNil(wwwAuthHeader)

					t.Run("is a bearer scheme", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("Bearer", wwwAuthHeader.Scheme)
					})

					t.Run("is an ext jwt realm", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("openziti-secondary-ext-jwt", wwwAuthHeader.Params["realm"])
					})

					t.Run("that has a missing error", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("missing", wwwAuthHeader.Params["error"])
					})

					t.Run("has the correct ext jwt id", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["id"])

						ids := strings.Split(wwwAuthHeader.Params["id"], "|")
						ctx.Req.Contains(ids, *signerDetail.ID)
					})

					t.Run("has the correct issuer", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])
						issuers := strings.Split(wwwAuthHeader.Params["issuer"], "|")
						ctx.Req.Contains(issuers, *jwtSigner.Issuer)
					})
				})
			})
		})

		t.Run("if an improperly signed jwt is supplied", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			validClaimsWrongSignerToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": jwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *jwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*jwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			//make a random new priv key and sign with it
			_, invalidJwtSignerKey := newSelfSignedCert(jwtSignerCommonName)

			improperlySignedTokenStr, err := validClaimsWrongSignerToken.SignedString(invalidJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(improperlySignedTokenStr)

			identityCreds.AddJWT(improperlySignedTokenStr)

			resp, err := clientApi.RawLegacyAuthRequest(identityCreds)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)

			t.Run("a www-authenticate header is returned", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Len(resp.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader, err := ParseWWWAuthenticate(resp.WWWAuthenticate[0])
					ctx.Req.NoError(err)
					ctx.Req.NotNil(wwwAuthHeader)

					t.Run("is a bearer scheme", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("Bearer", wwwAuthHeader.Scheme)
					})

					t.Run("is an ext jwt realm", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("openziti-secondary-ext-jwt", wwwAuthHeader.Params["realm"])
					})

					t.Run("that has an invalid error", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("invalid", wwwAuthHeader.Params["error"])
					})

					t.Run("has the correct ext jwt id", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["id"])

						ids := strings.Split(wwwAuthHeader.Params["id"], "|")
						ctx.Req.Contains(ids, *signerDetail.ID)
					})

					t.Run("has the correct issuer", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])
						issuers := strings.Split(wwwAuthHeader.Params["issuer"], "|")
						ctx.Req.Contains(issuers, *jwtSigner.Issuer)
					})
				})
			})
		})

		t.Run("if a valid jwt is supplied", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			goodToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": jwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *jwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*jwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			goodTokeStr, err := goodToken.SignedString(jwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(goodTokeStr)

			identityCreds.AddJWT(goodTokeStr)

			resp, err := clientApi.RawLegacyAuthRequest(identityCreds)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)

			t.Run("a www-authenticate header is returned", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Len(resp.WWWAuthenticate, 0)
			})
		})
	})

	t.Run("jwt auth with same secondary signer", func(t *testing.T) {
		ctx.testContextChanged(t)

		managementApiClient := ctx.NewEdgeManagementApi(nil)
		ctx.Req.NotNil(managementApiClient)

		adminCreds := ctx.NewAdminCredentials()
		apiSession, err := managementApiClient.Authenticate(adminCreds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(apiSession)

		jwtSignerCommonName := "soCommonItHurts"
		jwtSignerCert, jwtSignerKey := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey
		jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)
		jwtSignerName := "Test JWT Signer 2001 For Greatness"
		jwtSignerEnabled := true

		jwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:         &jwtSignerCertPem,
			ClaimsProperty:  ToPtr("sub"),
			Enabled:         &jwtSignerEnabled,
			ExternalAuthURL: ToPtr("https://test.example.com"),
			Name:            &jwtSignerName,
			Tags:            nil,
			UseExternalID:   ToPtr(false),
			Kid:             ToPtr(uuid.New().String()),
			Issuer:          ToPtr("another-issuer"),
			Audience:        ToPtr("another-audience"),
			ClientID:        ToPtr("another-client-id"),
			Scopes:          []string{"scope1", "scope2"},
			TargetToken:     ToPtr(rest_model.TargetTokenID),
		}

		signerDetail, err := managementApiClient.CreateExtJwtSigner(jwtSigner)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(signerDetail)

		authPolicy := &rest_model.AuthPolicyCreate{
			Name: ToPtr("testPolicyCertAndSecondarySigner-" + uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: ToPtr(false),
					Allowed:           ToPtr(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        ToPtr(true),
					AllowedSigners: []string{*signerDetail.ID},
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
				RequireExtJWTSigner: signerDetail.ID,
				RequireTotp:         ToPtr(false),
			},
		}

		authPolicyDetails, err := managementApiClient.CreateAuthPolicy(authPolicy)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(authPolicyDetails)

		identityCreateLoc, err := managementApiClient.CreateIdentity(uuid.NewString(), false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityCreateLoc)

		identityPatch := &rest_model.IdentityPatch{
			AuthPolicyID: authPolicyDetails.ID,
		}

		identityDetails, err := managementApiClient.PatchIdentity(identityCreateLoc.ID, identityPatch)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityDetails)
		ctx.Req.NotNil(identityCreateLoc)

		t.Run("if a jwt is not supplied", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtCreds := edge_apis.NewJwtCredentials("")
			jwtCreds.CaPool = ctx.ControllerCaPool()
			ctx.Req.NotNil(jwtCreds)

			clientApi := ctx.NewEdgeClientApi(nil)
			resp, err := clientApi.RawLegacyAuthRequest(jwtCreds)

			ctx.Req.Nil(resp)
			ctx.Req.Error(err)
			unauthedErr := &authentication.AuthenticateUnauthorized{}

			ctx.Req.ErrorAs(err, &unauthedErr)

			t.Run("a www-authenticate header is returned", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Len(unauthedErr.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader, err := ParseWWWAuthenticate(unauthedErr.WWWAuthenticate[0])
					ctx.Req.NoError(err)
					ctx.Req.NotNil(wwwAuthHeader)

					t.Run("is a bearer scheme", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("Bearer", wwwAuthHeader.Scheme)
					})

					t.Run("is an ext jwt realm", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("openziti-primary-ext-jwt", wwwAuthHeader.Params["realm"])
					})

					t.Run("that has a missing error", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("missing", wwwAuthHeader.Params["error"])
					})

					t.Run("has the correct ext jwt id", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["id"])

						ids := strings.Split(wwwAuthHeader.Params["id"], "|")
						ctx.Req.Contains(ids, *signerDetail.ID)
					})

					t.Run("has the correct issuer", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])
						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])
						issuers := strings.Split(wwwAuthHeader.Params["issuer"], "|")
						ctx.Req.Contains(issuers, *jwtSigner.Issuer)
					})
				})
			})
		})

		t.Run("if an expired jwt is supplied", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			expiredToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": jwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *jwtSigner.Issuer,
					Subject: identityCreateLoc.ID,
					Audience: jwt.ClaimStrings{
						*jwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			expiredTokenStr, err := expiredToken.SignedString(jwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(expiredTokenStr)

			jwtCreds := edge_apis.NewJwtCredentials(expiredTokenStr)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			resp, err := clientApi.RawLegacyAuthRequest(jwtCreds)

			ctx.Req.Nil(resp)
			ctx.Req.Error(err)
			unauthedErr := &authentication.AuthenticateUnauthorized{}

			ctx.Req.ErrorAs(err, &unauthedErr)

			t.Run("a www-authenticate header is returned", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Len(unauthedErr.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader, err := ParseWWWAuthenticate(unauthedErr.WWWAuthenticate[0])
					ctx.Req.NoError(err)
					ctx.Req.NotNil(wwwAuthHeader)

					t.Run("is a bearer scheme", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("Bearer", wwwAuthHeader.Scheme)
					})

					t.Run("is an ext jwt realm", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("openziti-primary-ext-jwt", wwwAuthHeader.Params["realm"])
					})

					t.Run("that has a expired error", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("expired", wwwAuthHeader.Params["error"])
					})

					t.Run("has the correct ext jwt id", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["id"])

						ids := strings.Split(wwwAuthHeader.Params["id"], "|")
						ctx.Req.Contains(ids, *signerDetail.ID)
					})

					t.Run("has the correct issuer", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])

						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])
						issuers := strings.Split(wwwAuthHeader.Params["issuer"], "|")
						ctx.Req.Contains(issuers, *jwtSigner.Issuer)
					})
				})
			})
		})

		t.Run("if a malformed jwt is supplied", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			malformedTokenStr := "ey23849384932.234238434928.a9e8hg4928h2"

			jwtCreds := edge_apis.NewJwtCredentials(malformedTokenStr)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			resp, err := clientApi.RawLegacyAuthRequest(jwtCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(resp)

			unauthorizedError := &authentication.AuthenticateUnauthorized{}
			unauthorizedErrorOk := errors.As(err, &unauthorizedError)
			ctx.Req.True(unauthorizedErrorOk)

			t.Run("a www-authenticate header is returned", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Len(unauthorizedError.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader, err := ParseWWWAuthenticate(unauthorizedError.WWWAuthenticate[0])
					ctx.Req.NoError(err)
					ctx.Req.NotNil(wwwAuthHeader)

					t.Run("is a bearer scheme", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("Bearer", wwwAuthHeader.Scheme)
					})

					t.Run("is an ext jwt realm", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("openziti-primary-ext-jwt", wwwAuthHeader.Params["realm"])
					})

					t.Run("that has a missing error", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("missing", wwwAuthHeader.Params["error"])
					})

					t.Run("has the correct ext jwt id", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["id"])

						ids := strings.Split(wwwAuthHeader.Params["id"], "|")
						ctx.Req.Contains(ids, *signerDetail.ID)
					})

					t.Run("has the correct issuer", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])
						issuers := strings.Split(wwwAuthHeader.Params["issuer"], "|")
						ctx.Req.Contains(issuers, *jwtSigner.Issuer)
					})
				})
			})
		})

		t.Run("if an improperly signed jwt is supplied", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			validClaimsWrongSignerToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": jwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *jwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*jwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			//make a random new priv key and sign with it
			_, invalidJwtSignerKey := newSelfSignedCert(jwtSignerCommonName)

			improperlySignedTokenStr, err := validClaimsWrongSignerToken.SignedString(invalidJwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(improperlySignedTokenStr)

			jwtCreds := edge_apis.NewJwtCredentials(improperlySignedTokenStr)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			resp, err := clientApi.RawLegacyAuthRequest(jwtCreds)

			ctx.Req.Error(err)
			ctx.Req.Nil(resp)

			unauthorizedError := &authentication.AuthenticateUnauthorized{}
			unauthorizedErrorOk := errors.As(err, &unauthorizedError)
			ctx.Req.True(unauthorizedErrorOk)

			t.Run("a www-authenticate header is returned", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Len(unauthorizedError.WWWAuthenticate, 1)

				t.Run("that has a valid format", func(t *testing.T) {
					ctx.testContextChanged(t)

					wwwAuthHeader, err := ParseWWWAuthenticate(unauthorizedError.WWWAuthenticate[0])
					ctx.Req.NoError(err)
					ctx.Req.NotNil(wwwAuthHeader)

					t.Run("is a bearer scheme", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("Bearer", wwwAuthHeader.Scheme)
					})

					t.Run("is an ext jwt realm", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("openziti-primary-ext-jwt", wwwAuthHeader.Params["realm"])
					})

					t.Run("that has an invalid error", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.Equal("invalid", wwwAuthHeader.Params["error"])
					})

					t.Run("has the correct ext jwt id", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["id"])

						ids := strings.Split(wwwAuthHeader.Params["id"], "|")
						ctx.Req.Contains(ids, *signerDetail.ID)
					})

					t.Run("has the correct issuer", func(t *testing.T) {
						ctx.testContextChanged(t)

						ctx.Req.NotEmpty(wwwAuthHeader.Params["issuer"])
						issuers := strings.Split(wwwAuthHeader.Params["issuer"], "|")
						ctx.Req.Contains(issuers, *jwtSigner.Issuer)
					})
				})
			})
		})

		t.Run("if a valid jwt is supplied", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApi := ctx.NewEdgeClientApi(nil)

			goodToken := jwt.Token{
				Header: map[string]any{
					"alg": jwt.SigningMethodES256.Alg(),
					"kid": jwtSigner.Kid,
				},
				Method: jwt.SigningMethodES256,
				Claims: &jwt.RegisteredClaims{
					Issuer:  *jwtSigner.Issuer,
					Subject: *identityDetails.ID,
					Audience: jwt.ClaimStrings{
						*jwtSigner.Audience,
					},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
					NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
					ID:        uuid.NewString(),
				},
			}

			goodTokenStr, err := goodToken.SignedString(jwtSignerKey)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(goodTokenStr)

			jwtCreds := edge_apis.NewJwtCredentials(goodTokenStr)
			jwtCreds.CaPool = ctx.ControllerCaPool()

			resp, err := clientApi.RawLegacyAuthRequest(jwtCreds)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)

			t.Run("a www-authenticate header is returned", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Len(resp.WWWAuthenticate, 0)
			})
		})
	})
}

type RandomHmacSigner struct {
	Kid string
	Key []byte
}

func (r RandomHmacSigner) Generate(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(r.SigningMethod(), claims)
	token.Header["kid"] = r.KeyId()
	return token.SignedString(r.Key)
}

func (r RandomHmacSigner) SigningMethod() jwt.SigningMethod {
	return jwt.SigningMethodHS256
}

func (r RandomHmacSigner) KeyId() string {
	return r.Kid
}

func NewRandomHmacSignerWithKeyId(keyId string) jwtsigner.Signer {
	result := &RandomHmacSigner{
		Kid: keyId,
		Key: []byte(uuid.NewString()),
	}

	return result
}

type WWWAuthenticate struct {
	Scheme string
	Params map[string]string
}

func ParseWWWAuthenticate(header string) (*WWWAuthenticate, error) {
	header = strings.TrimSpace(header)

	if header == "" {
		return nil, errors.New("empty header")
	}

	scheme, rest, hasRest := strings.Cut(header, " ")

	result := &WWWAuthenticate{
		Scheme: scheme,
		Params: map[string]string{},
	}

	if !hasRest {
		return result, nil
	}

	rest = strings.TrimSpace(rest)
	var err error
	result.Params, err = ParseWWWAuthenticateParams(rest)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func ParseWWWAuthenticateParams(params string) (map[string]string, error) {
	out := make(map[string]string)

	for len(params) > 0 {
		params = strings.TrimSpace(params)
		if params == "" {
			break
		}

		// key
		eq := strings.IndexByte(params, '=')
		if eq < 0 {
			return nil, fmt.Errorf("missing '=' in %q", params)
		}
		key := strings.TrimSpace(params[:eq])
		params = strings.TrimSpace(params[eq+1:])

		// value (must be quoted)
		if !strings.HasPrefix(params, `"`) {
			return nil, fmt.Errorf("expected quoted value for %q", key)
		}
		params = params[1:] // consume opening quote

		end := strings.IndexByte(params, '"')
		if end < 0 {
			return nil, fmt.Errorf("unterminated quoted value for %q", key)
		}
		value := params[:end]
		params = params[end+1:] // consume closing quote

		out[key] = value
	}

	return out, nil
}
