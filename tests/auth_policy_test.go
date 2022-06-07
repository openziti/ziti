//go:build apitests
// +build apitests

/*
	Copyright NetFoundry, Inc.

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
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/rest_model"
	nfpem "github.com/openziti/foundation/util/pem"
	"net/http"
	"testing"
	"time"
)

func Test_AuthPolicies(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("create with valid values returns 200 Ok", func(t *testing.T) {
		ctx.testContextChanged(t)
		jwtSignerCert, _ := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy")

		extJwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:  S(nfpem.EncodeToString(jwtSignerCert)),
			Enabled:  B(true),
			Name:     S("Test JWT Signer - Auth Policy"),
			Kid:      S(uuid.NewString()),
			Issuer:   S("test-issuer-99"),
			Audience: S("test-audience-99"),
		}

		extJwtSignerCreated := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSigner).SetResult(extJwtSignerCreated).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSigner, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreated.Data.ID)

		tag1Name := "originalTag1Name"
		tag1Value := "originalTag1Value"
		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 1"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed: B(true),
					AllowedSigners: []string{
						extJwtSignerCreated.Data.ID,
					},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(true),
					MaxAttempts:            I(5),
					MinPasswordLength:      I(5),
					LockoutDurationMinutes: I(0),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
			},
			Tags: &rest_model.Tags{
				SubTags: map[string]interface{}{
					tag1Name: tag1Value,
				},
			},
		}

		authPolicyCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).SetResult(authPolicyCreated).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", authPolicy, resp.Body())
		ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

		t.Run("get returns 200 and same input values", func(t *testing.T) {
			ctx.testContextChanged(t)

			authPolicyEnvelope := &rest_model.DetailAuthPolicyEnvelope{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(authPolicyEnvelope).Get("/auth-policies/" + authPolicyCreated.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for GET %s: %s", resp.Request.URL, resp.Body())
			authPolicyDetail := authPolicyEnvelope.Data

			ctx.Req.Equal(authPolicyCreated.Data.ID, *authPolicyDetail.ID)
			ctx.Req.Equal(len(authPolicy.Tags.SubTags), len(authPolicyDetail.Tags.SubTags))
			ctx.Req.Equal(authPolicy.Tags.SubTags[tag1Name], authPolicyDetail.Tags.SubTags[tag1Name])
			ctx.Req.Equal(*authPolicy.Name, *authPolicyDetail.Name)

			ctx.Req.Equal(len(authPolicy.Primary.ExtJWT.AllowedSigners), len(authPolicyDetail.Primary.ExtJWT.AllowedSigners))
			ctx.Req.Equal(authPolicy.Primary.ExtJWT.AllowedSigners[0], authPolicyDetail.Primary.ExtJWT.AllowedSigners[0])
			ctx.Req.Equal(*authPolicy.Primary.ExtJWT.Allowed, *authPolicyDetail.Primary.ExtJWT.Allowed)

			ctx.Req.Equal(*authPolicy.Primary.Updb.Allowed, *authPolicyDetail.Primary.Updb.Allowed)
			ctx.Req.Equal(*authPolicy.Primary.Updb.MaxAttempts, *authPolicyDetail.Primary.Updb.MaxAttempts)
			ctx.Req.Equal(*authPolicy.Primary.Updb.MinPasswordLength, *authPolicyDetail.Primary.Updb.MinPasswordLength)
			ctx.Req.Equal(*authPolicy.Primary.Updb.LockoutDurationMinutes, *authPolicyDetail.Primary.Updb.LockoutDurationMinutes)
			ctx.Req.Equal(*authPolicy.Primary.Updb.RequireMixedCase, *authPolicyDetail.Primary.Updb.RequireMixedCase)
			ctx.Req.Equal(*authPolicy.Primary.Updb.RequireNumberChar, *authPolicyDetail.Primary.Updb.RequireNumberChar)
			ctx.Req.Equal(*authPolicy.Primary.Updb.RequireSpecialChar, *authPolicyDetail.Primary.Updb.RequireSpecialChar)

			ctx.Req.Equal(*authPolicy.Primary.Cert.Allowed, *authPolicyDetail.Primary.Cert.Allowed)
			ctx.Req.Equal(*authPolicy.Primary.Cert.AllowExpiredCerts, *authPolicyDetail.Primary.Cert.AllowExpiredCerts)

			ctx.Req.Equal(*authPolicy.Secondary.RequireTotp, *authPolicyDetail.Secondary.RequireTotp)
			ctx.Req.Equal(authPolicy.Secondary.RequireExtJWTSigner, authPolicyDetail.Secondary.RequireExtJWTSigner)
		})

		t.Run("update returns 200", func(t *testing.T) {
			ctx.testContextChanged(t)

			tag2Name := "updatedTag2Name"
			tag2Value := "updatedTag2Value"

			authPolicyUpdate := &rest_model.AuthPolicyUpdate{
				AuthPolicyCreate: rest_model.AuthPolicyCreate{
					Name: S("Updated Name 1"),
					Primary: &rest_model.AuthPolicyPrimary{
						Cert: &rest_model.AuthPolicyPrimaryCert{
							AllowExpiredCerts: B(false),
							Allowed:           B(false),
						},
						ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
							Allowed: B(true),
							AllowedSigners: []string{
								extJwtSignerCreated.Data.ID,
							},
						},
						Updb: &rest_model.AuthPolicyPrimaryUpdb{
							Allowed:                B(false),
							MaxAttempts:            I(1),
							MinPasswordLength:      I(12),
							LockoutDurationMinutes: I(1),
							RequireMixedCase:       B(false),
							RequireNumberChar:      B(false),
							RequireSpecialChar:     B(false),
						},
					},
					Secondary: &rest_model.AuthPolicySecondary{
						RequireExtJWTSigner: &extJwtSignerCreated.Data.ID,
						RequireTotp:         B(true),
					},
					Tags: &rest_model.Tags{
						SubTags: map[string]interface{}{
							tag1Name: tag1Value,
						},
					},
				},
			}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyUpdate).Put("/auth-policies/" + authPolicyCreated.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for PUT %T: %s", extJwtSigner, resp.Body())

			t.Run("get returns 200 and updated values", func(t *testing.T) {
				ctx.testContextChanged(t)

				authPolicyUpdatedEnvelope := &rest_model.DetailAuthPolicyEnvelope{}

				resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(authPolicyUpdatedEnvelope).Get("/auth-policies/" + authPolicyCreated.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for GET %s: %s", resp.Request.URL, resp.Body())
				authPolicyUpdatedDetail := authPolicyUpdatedEnvelope.Data

				ctx.Req.Equal(len(authPolicyUpdate.Tags.SubTags), len(authPolicyUpdatedDetail.Tags.SubTags))
				ctx.Req.Equal(authPolicyUpdate.Tags.SubTags[tag1Name], authPolicyUpdatedDetail.Tags.SubTags[tag1Name])
				ctx.Req.Equal(authPolicyUpdate.Tags.SubTags[tag1Name], authPolicyUpdatedDetail.Tags.SubTags[tag1Name])
				ctx.Req.Equal(authPolicyUpdate.Tags.SubTags[tag2Name], authPolicyUpdatedDetail.Tags.SubTags[tag2Value])
				ctx.Req.Equal(*authPolicyUpdate.Name, *authPolicyUpdatedDetail.Name)

				ctx.Req.Equal(len(authPolicyUpdate.Primary.ExtJWT.AllowedSigners), len(authPolicyUpdatedDetail.Primary.ExtJWT.AllowedSigners))
				ctx.Req.Equal(*authPolicyUpdate.Primary.ExtJWT.Allowed, *authPolicyUpdatedDetail.Primary.ExtJWT.Allowed)

				ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.Allowed, *authPolicyUpdatedDetail.Primary.Updb.Allowed)
				ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.MaxAttempts, *authPolicyUpdatedDetail.Primary.Updb.MaxAttempts)
				ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.MinPasswordLength, *authPolicyUpdatedDetail.Primary.Updb.MinPasswordLength)
				ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.LockoutDurationMinutes, *authPolicyUpdatedDetail.Primary.Updb.LockoutDurationMinutes)
				ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.RequireMixedCase, *authPolicyUpdatedDetail.Primary.Updb.RequireMixedCase)
				ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.RequireNumberChar, *authPolicyUpdatedDetail.Primary.Updb.RequireNumberChar)
				ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.RequireSpecialChar, *authPolicyUpdatedDetail.Primary.Updb.RequireSpecialChar)

				ctx.Req.Equal(*authPolicyUpdate.Primary.Cert.Allowed, *authPolicyUpdatedDetail.Primary.Cert.Allowed)
				ctx.Req.Equal(*authPolicyUpdate.Primary.Cert.AllowExpiredCerts, *authPolicyUpdatedDetail.Primary.Cert.AllowExpiredCerts)

				ctx.Req.Equal(*authPolicyUpdate.Secondary.RequireTotp, *authPolicyUpdatedDetail.Secondary.RequireTotp)
				ctx.Req.Equal(*authPolicyUpdate.Secondary.RequireExtJWTSigner, *authPolicyUpdatedDetail.Secondary.RequireExtJWTSigner)
			})
		})

		t.Run("delete returns 200", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/auth-policies/" + authPolicyCreated.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for DELETE %s: %s", resp.Request.URL, resp.Body())

			t.Run("after delete get returns 404", func(t *testing.T) {
				resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().Get("/auth-policies/" + authPolicyCreated.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusNotFound, resp.StatusCode(), "expected 404 for GET %s: %s", resp.Request.URL, resp.Body())
			})
		})
	})

	t.Run("patching names returns 200 and updates only the proper field", func(t *testing.T) {
		ctx.testContextChanged(t)
		jwtSignerCert, _ := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy Patch")

		extJwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:  S(nfpem.EncodeToString(jwtSignerCert)),
			Enabled:  B(true),
			Name:     S("Test JWT Signer - Auth Policy Patch"),
			Kid:      S(uuid.NewString()),
			Issuer:   S("test-issuer-100"),
			Audience: S("test-audience-100"),
		}

		extJwtSignerCreated := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSigner).SetResult(extJwtSignerCreated).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSigner, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreated.Data.ID)

		tag1Name := "originalTag1Name"
		tag1Value := "originalTag1Value"
		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 1"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed: B(true),
					AllowedSigners: []string{
						extJwtSignerCreated.Data.ID,
					},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(true),
					MaxAttempts:            I(5),
					MinPasswordLength:      I(5),
					LockoutDurationMinutes: I(0),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
			},
			Tags: &rest_model.Tags{
				SubTags: map[string]interface{}{
					tag1Name: tag1Value,
				},
			},
		}

		authPolicyCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).SetResult(authPolicyCreated).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", authPolicy, resp.Body())
		ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

		t.Run("name returns 200", func(t *testing.T) {
			ctx.testContextChanged(t)
			authPolicyPatch := &rest_model.AuthPolicyPatch{
				Name: S("PatchedName"),
			}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyPatch).Patch("/auth-policies/" + authPolicyCreated.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for PATCH %T: %s", authPolicy, resp.Body())

			t.Run("get returns 200 and updated values", func(t *testing.T) {
				ctx.testContextChanged(t)

				authPolicyPatchEnvelope := &rest_model.DetailAuthPolicyEnvelope{}

				resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(authPolicyPatchEnvelope).Get("/auth-policies/" + authPolicyCreated.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for GET %s: %s", resp.Request.URL, resp.Body())
				authPolicyPatchedDetail := authPolicyPatchEnvelope.Data

				ctx.Req.Equal(len(authPolicy.Tags.SubTags), len(authPolicyPatchedDetail.Tags.SubTags))
				ctx.Req.Equal(authPolicy.Tags.SubTags[tag1Name], authPolicyPatchedDetail.Tags.SubTags[tag1Name])
				ctx.Req.Equal(authPolicy.Tags.SubTags[tag1Name], authPolicyPatchedDetail.Tags.SubTags[tag1Name])
				ctx.Req.Equal(*authPolicyPatch.Name, *authPolicyPatchedDetail.Name)

				ctx.Req.Equal(len(authPolicy.Primary.ExtJWT.AllowedSigners), len(authPolicyPatchedDetail.Primary.ExtJWT.AllowedSigners))
				ctx.Req.Equal(*authPolicy.Primary.ExtJWT.Allowed, *authPolicyPatchedDetail.Primary.ExtJWT.Allowed)

				ctx.Req.Equal(*authPolicy.Primary.Updb.Allowed, *authPolicyPatchedDetail.Primary.Updb.Allowed)
				ctx.Req.Equal(*authPolicy.Primary.Updb.MaxAttempts, *authPolicyPatchedDetail.Primary.Updb.MaxAttempts)
				ctx.Req.Equal(*authPolicy.Primary.Updb.MinPasswordLength, *authPolicyPatchedDetail.Primary.Updb.MinPasswordLength)
				ctx.Req.Equal(*authPolicy.Primary.Updb.LockoutDurationMinutes, *authPolicyPatchedDetail.Primary.Updb.LockoutDurationMinutes)
				ctx.Req.Equal(*authPolicy.Primary.Updb.RequireMixedCase, *authPolicyPatchedDetail.Primary.Updb.RequireMixedCase)
				ctx.Req.Equal(*authPolicy.Primary.Updb.RequireNumberChar, *authPolicyPatchedDetail.Primary.Updb.RequireNumberChar)
				ctx.Req.Equal(*authPolicy.Primary.Updb.RequireSpecialChar, *authPolicyPatchedDetail.Primary.Updb.RequireSpecialChar)

				ctx.Req.Equal(*authPolicy.Primary.Cert.Allowed, *authPolicyPatchedDetail.Primary.Cert.Allowed)
				ctx.Req.Equal(*authPolicy.Primary.Cert.AllowExpiredCerts, *authPolicyPatchedDetail.Primary.Cert.AllowExpiredCerts)

				ctx.Req.Equal(*authPolicy.Secondary.RequireTotp, *authPolicyPatchedDetail.Secondary.RequireTotp)
				ctx.Req.Equal(authPolicy.Secondary.RequireExtJWTSigner, authPolicyPatchedDetail.Secondary.RequireExtJWTSigner)
			})
		})
	})

	t.Run("patching primary updb allowed returns 200 and updates only the proper field", func(t *testing.T) {
		ctx.testContextChanged(t)
		jwtSignerCert, _ := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy Patch")

		extJwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:  S(nfpem.EncodeToString(jwtSignerCert)),
			Enabled:  B(true),
			Name:     S("Test JWT Signer - Auth Policy Patch1"),
			Kid:      S(uuid.NewString()),
			Issuer:   S("test-issuer-101"),
			Audience: S("test-audience-101"),
		}

		extJwtSignerCreated := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSigner).SetResult(extJwtSignerCreated).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSigner, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreated.Data.ID)

		tag1Name := "originalTag1Name"
		tag1Value := "originalTag1Value"
		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 1 - Patch Updb Allowed"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed: B(true),
					AllowedSigners: []string{
						extJwtSignerCreated.Data.ID,
					},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(true),
					MaxAttempts:            I(5),
					MinPasswordLength:      I(5),
					LockoutDurationMinutes: I(0),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
			},
			Tags: &rest_model.Tags{
				SubTags: map[string]interface{}{
					tag1Name: tag1Value,
				},
			},
		}

		authPolicyCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).SetResult(authPolicyCreated).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for  %T: %s", authPolicy, resp.Body())
		ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

		t.Run("returns 200", func(t *testing.T) {
			ctx.testContextChanged(t)
			authPolicyPatch := &rest_model.AuthPolicyPatch{
				Primary: &rest_model.AuthPolicyPrimaryPatch{
					Cert:   nil,
					ExtJWT: nil,
					Updb: &rest_model.AuthPolicyPrimaryUpdbPatch{
						Allowed: B(false),
					},
				},
			}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyPatch).Patch("/auth-policies/" + authPolicyCreated.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for PATCH %T: %s", authPolicy, resp.Body())

			t.Run("get returns 200 and updated values", func(t *testing.T) {
				ctx.testContextChanged(t)

				authPolicyPatchEnvelope := &rest_model.DetailAuthPolicyEnvelope{}

				resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(authPolicyPatchEnvelope).Get("/auth-policies/" + authPolicyCreated.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for GET %s: %s", resp.Request.URL, resp.Body())
				authPolicyPatchedDetail := authPolicyPatchEnvelope.Data

				ctx.Req.Equal(len(authPolicy.Tags.SubTags), len(authPolicyPatchedDetail.Tags.SubTags))
				ctx.Req.Equal(authPolicy.Tags.SubTags[tag1Name], authPolicyPatchedDetail.Tags.SubTags[tag1Name])
				ctx.Req.Equal(authPolicy.Tags.SubTags[tag1Name], authPolicyPatchedDetail.Tags.SubTags[tag1Name])
				ctx.Req.Equal(*authPolicy.Name, *authPolicyPatchedDetail.Name)

				ctx.Req.Equal(len(authPolicy.Primary.ExtJWT.AllowedSigners), len(authPolicyPatchedDetail.Primary.ExtJWT.AllowedSigners))
				ctx.Req.Equal(*authPolicy.Primary.ExtJWT.Allowed, *authPolicyPatchedDetail.Primary.ExtJWT.Allowed)

				ctx.Req.Equal(*authPolicyPatch.Primary.Updb.Allowed, *authPolicyPatchedDetail.Primary.Updb.Allowed)
				ctx.Req.Equal(*authPolicy.Primary.Updb.MaxAttempts, *authPolicyPatchedDetail.Primary.Updb.MaxAttempts)
				ctx.Req.Equal(*authPolicy.Primary.Updb.MinPasswordLength, *authPolicyPatchedDetail.Primary.Updb.MinPasswordLength)
				ctx.Req.Equal(*authPolicy.Primary.Updb.LockoutDurationMinutes, *authPolicyPatchedDetail.Primary.Updb.LockoutDurationMinutes)
				ctx.Req.Equal(*authPolicy.Primary.Updb.RequireMixedCase, *authPolicyPatchedDetail.Primary.Updb.RequireMixedCase)
				ctx.Req.Equal(*authPolicy.Primary.Updb.RequireNumberChar, *authPolicyPatchedDetail.Primary.Updb.RequireNumberChar)
				ctx.Req.Equal(*authPolicy.Primary.Updb.RequireSpecialChar, *authPolicyPatchedDetail.Primary.Updb.RequireSpecialChar)

				ctx.Req.Equal(*authPolicy.Primary.Cert.Allowed, *authPolicyPatchedDetail.Primary.Cert.Allowed)
				ctx.Req.Equal(*authPolicy.Primary.Cert.AllowExpiredCerts, *authPolicyPatchedDetail.Primary.Cert.AllowExpiredCerts)

				ctx.Req.Equal(*authPolicy.Secondary.RequireTotp, *authPolicyPatchedDetail.Secondary.RequireTotp)
				ctx.Req.Equal(authPolicy.Secondary.RequireExtJWTSigner, authPolicyPatchedDetail.Secondary.RequireExtJWTSigner)
			})
		})
	})

	t.Run("create with invalid primary external jwt signers returns 404", func(t *testing.T) {
		ctx.testContextChanged(t)

		tag1Name := "originalTag1Name"
		tag1Value := "originalTag1Value"
		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 1"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed: B(true),
					AllowedSigners: []string{
						"badId",
					},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(true),
					MaxAttempts:            I(5),
					MinPasswordLength:      I(5),
					LockoutDurationMinutes: I(0),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: S(""),
				RequireTotp:         B(false),
			},
			Tags: &rest_model.Tags{
				SubTags: map[string]interface{}{
					tag1Name: tag1Value,
				},
			},
		}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusNotFound, resp.StatusCode(), "expected 404 for POST %T: %s", authPolicy, resp.Body())
	})

	t.Run("create with invalid secondary external jwt signers returns 404", func(t *testing.T) {
		ctx.testContextChanged(t)

		tag1Name := "originalTag1Name"
		tag1Value := "originalTag1Value"
		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 1"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        B(true),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(true),
					MaxAttempts:            I(5),
					MinPasswordLength:      I(5),
					LockoutDurationMinutes: I(0),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: S("badId"),
				RequireTotp:         B(false),
			},
			Tags: &rest_model.Tags{
				SubTags: map[string]interface{}{
					tag1Name: tag1Value,
				},
			},
		}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusNotFound, resp.StatusCode(), "expected 404 for POST %T: %s", authPolicy, resp.Body())
	})

	t.Run("create auth policy with external jwt signer references", func(t *testing.T) {
		ctx.testContextChanged(t)
		jwtSignerCert1, _ := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy - Delete Scenarios 1")

		extJwtSigner1 := &rest_model.ExternalJWTSignerCreate{
			CertPem:  S(nfpem.EncodeToString(jwtSignerCert1)),
			Enabled:  B(true),
			Name:     S("Test JWT Signer - Auth Policy - Delete Scenarios 1"),
			Kid:      S(uuid.NewString()),
			Issuer:   S("test-issuer-102"),
			Audience: S("test-audience-102"),
		}

		extJwtSignerCreated1 := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSigner1).SetResult(extJwtSignerCreated1).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSigner1, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreated1.Data.ID)

		jwtSignerCert2, _ := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy - Delete Scenarios 2")
		extJwtSigner2 := &rest_model.ExternalJWTSignerCreate{
			CertPem:  S(nfpem.EncodeToString(jwtSignerCert2)),
			Enabled:  B(true),
			Name:     S("Test JWT Signer - Auth Policy - Delete Scenarios 2"),
			Kid:      S(uuid.NewString()),
			Issuer:   S("test-issuer-200"),
			Audience: S("test-audience-200"),
		}

		extJwtSignerCreated2 := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSigner2).SetResult(extJwtSignerCreated2).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSigner2, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreated2.Data.ID)

		tag1Name := "originalTag1Name"
		tag1Value := "originalTag1Value"
		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 1"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed: B(true),
					AllowedSigners: []string{
						extJwtSignerCreated1.Data.ID,
					},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(true),
					MaxAttempts:            I(5),
					MinPasswordLength:      I(5),
					LockoutDurationMinutes: I(0),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: &extJwtSignerCreated2.Data.ID,
				RequireTotp:         B(false),
			},
			Tags: &rest_model.Tags{
				SubTags: map[string]interface{}{
					tag1Name: tag1Value,
				},
			},
		}

		authPolicyCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).SetResult(authPolicyCreated).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", authPolicy, resp.Body())
		ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

		t.Run("can not delete external jwt signer referenced by auth policy", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/external-jwt-signers/" + extJwtSignerCreated1.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusConflict, resp.StatusCode(), "expected 409 for DELETE %T[%s]: %s", extJwtSigner1, extJwtSignerCreated1.Data.ID, resp.Body())

			t.Run("can remove reference via patch", func(t *testing.T) {
				ctx.testContextChanged(t)
				authPolicyPatch := &rest_model.AuthPolicyPatch{
					Primary: &rest_model.AuthPolicyPrimaryPatch{
						ExtJWT: &rest_model.AuthPolicyPrimaryExtJWTPatch{
							AllowedSigners: []string{},
						},
					},
				}

				resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyPatch).Patch("/auth-policies/" + authPolicyCreated.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for PATCH %T: %s", authPolicy, resp.Body())

				t.Run("delete ext jwt signer returns 200", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/external-jwt-signers/" + extJwtSignerCreated1.Data.ID)
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for DELETE %T[%s]: %s", extJwtSigner1, extJwtSignerCreated1.Data.ID, resp.Body())

					t.Run("get deleted ext jwt signer returns 404", func(t *testing.T) {
						ctx.testContextChanged(t)
						resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/external-jwt-signers/" + extJwtSignerCreated1.Data.ID)
						ctx.Req.NoError(err)
						ctx.Req.Equal(http.StatusNotFound, resp.StatusCode(), "expected 404 for GET %T[%s]: %s", extJwtSigner1, extJwtSignerCreated1.Data.ID, resp.Body())
					})
				})
			})

			t.Run("auth policy primary ext jwt signers is updated", func(t *testing.T) {
				ctx.testContextChanged(t)

				authPolicyEnvelope := &rest_model.DetailAuthPolicyEnvelope{}

				resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(authPolicyEnvelope).Get("/auth-policies/" + authPolicyCreated.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for GET %s: %s", resp.Request.URL, resp.Body())
				authPolicyDetail := authPolicyEnvelope.Data

				ctx.Req.Equal(0, len(authPolicyDetail.Primary.ExtJWT.AllowedSigners))
			})
		})

		t.Run("can not delete referenced secondary jwt signer", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/external-jwt-signers/" + extJwtSignerCreated2.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusConflict, resp.StatusCode(), "expected 409 on DELETE %T[%s]: %s", extJwtSigner2, extJwtSignerCreated2.Data.ID, resp.Body())

			t.Run("can remove reference via patch", func(t *testing.T) {
				ctx.testContextChanged(t)
				authPolicyPatch := &rest_model.AuthPolicyPatch{
					Secondary: &rest_model.AuthPolicySecondaryPatch{
						RequireExtJWTSigner: S(""),
					},
				}

				resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyPatch).Patch("/auth-policies/" + authPolicyCreated.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for PATCH %T: %s", authPolicy, resp.Body())

				t.Run("delete secondary ext jwt signer returns 200", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/external-jwt-signers/" + extJwtSignerCreated2.Data.ID)
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for POST %T[%s]: %s", extJwtSigner2, extJwtSignerCreated2.Data.ID, resp.Body())

					t.Run("get deleted ext jwt signer returns 404", func(t *testing.T) {
						ctx.testContextChanged(t)
						resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/external-jwt-signers/" + extJwtSignerCreated2.Data.ID)
						ctx.Req.NoError(err)
						ctx.Req.Equal(http.StatusNotFound, resp.StatusCode(), "expected 404 for GET %T[%s]: %s", extJwtSigner2, extJwtSignerCreated2.Data.ID, resp.Body())
					})
				})
			})
		})
	})

	t.Run("can create an identity referencing auth policies", func(t *testing.T) {
		ctx.testContextChanged(t)

		tag1Name := "originalTag1Name"
		tag1Value := "originalTag1Value"
		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 1 - Identity Refs"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        B(true),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(true),
					MaxAttempts:            I(5),
					MinPasswordLength:      I(5),
					LockoutDurationMinutes: I(0),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
			},
			Tags: &rest_model.Tags{
				SubTags: map[string]interface{}{
					tag1Name: tag1Value,
				},
			},
		}

		authPolicyCreated := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).SetResult(authPolicyCreated).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", authPolicy, resp.Body())
		ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

		identityType := rest_model.IdentityTypeDevice
		identity := &rest_model.IdentityCreate{
			AuthPolicyID: &authPolicyCreated.Data.ID,
			IsAdmin:      B(false),
			Name:         S("test-identity-auth-policy-ref"),
			Type:         &identityType,
		}

		identityCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identity).SetResult(identityCreated).Post("/identities")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", authPolicy, resp.Body())
		ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

		t.Run("identity has auth policy set", func(t *testing.T) {
			ctx.testContextChanged(t)
			getResponse := &rest_model.DetailIdentityEnvelope{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(getResponse).Get("/identities/" + identityCreated.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())
			ctx.Req.Equal(*identity.AuthPolicyID, *getResponse.Data.AuthPolicyID)
		})

		t.Run("cannot delete auth policy assigned to identity", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/auth-policies/" + authPolicyCreated.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusConflict, resp.StatusCode(), "expected 409 for DELETE %T: %s", authPolicy, resp.Body())
		})

		t.Run("can delete auth policy after identity assignment removed", func(t *testing.T) {
			ctx.testContextChanged(t)

			identityPatch := &rest_model.IdentityPatch{
				AuthPolicyID: S(persistence.DefaultAuthPolicyId),
			}
			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityPatch).Patch("/identities/" + identityCreated.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for PATCH %T: %s", authPolicy, resp.Body())

			t.Run("delete returns 200", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/auth-policies/" + authPolicyCreated.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for DELETE %T: %s", authPolicy, resp.Body())
			})
		})
	})

	t.Run("cannot delete the default auth policy", func(t *testing.T) {
		ctx.testContextChanged(t)
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/auth-policies/" + persistence.DefaultAuthPolicyId)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusConflict, resp.StatusCode(), "expected 409 for DELETE: %s", resp.Body())
	})

	t.Run("can update the default auth policy", func(t *testing.T) {
		ctx.testContextChanged(t)
		jwtSignerCert, _ := newSelfSignedCert("Test Jwt Signer Cert - Update Default")

		extJwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:  S(nfpem.EncodeToString(jwtSignerCert)),
			Enabled:  B(true),
			Name:     S("Test JWT Signer - Auth Policy - Update Default"),
			Kid:      S(uuid.NewString()),
			Issuer:   S("test-issuer-104"),
			Audience: S("test-audience-104"),
		}

		extJwtSignerCreated := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSigner).SetResult(extJwtSignerCreated).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSigner, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreated.Data.ID)

		tag1Name := "updatedTag1Name"
		tag1Value := "updatedTag1Value"

		authPolicyUpdate := &rest_model.AuthPolicyUpdate{
			AuthPolicyCreate: rest_model.AuthPolicyCreate{
				Name: S("Updated Name 1"),
				Primary: &rest_model.AuthPolicyPrimary{
					Cert: &rest_model.AuthPolicyPrimaryCert{
						AllowExpiredCerts: B(false),
						Allowed:           B(false),
					},
					ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
						Allowed: B(true),
						AllowedSigners: []string{
							extJwtSignerCreated.Data.ID,
						},
					},
					Updb: &rest_model.AuthPolicyPrimaryUpdb{
						Allowed:                B(false),
						MaxAttempts:            I(1),
						MinPasswordLength:      I(5),
						LockoutDurationMinutes: I(1),
						RequireMixedCase:       B(false),
						RequireNumberChar:      B(false),
						RequireSpecialChar:     B(false),
					},
				},
				Secondary: &rest_model.AuthPolicySecondary{
					RequireExtJWTSigner: &extJwtSignerCreated.Data.ID,
					RequireTotp:         B(true),
				},
				Tags: &rest_model.Tags{
					SubTags: map[string]interface{}{
						tag1Name: tag1Value,
					},
				},
			},
		}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyUpdate).Put("/auth-policies/" + persistence.DefaultAuthPolicyId)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for PUT %T: %s", extJwtSigner, resp.Body())

		t.Run("get returns 200 and updated values", func(t *testing.T) {
			ctx.testContextChanged(t)

			authPolicyUpdatedEnvelope := &rest_model.DetailAuthPolicyEnvelope{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(authPolicyUpdatedEnvelope).Get("/auth-policies/" + persistence.DefaultAuthPolicyId)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for GET %s: %s", resp.Request.URL, resp.Body())
			authPolicyUpdatedDetail := authPolicyUpdatedEnvelope.Data

			ctx.Req.Equal(len(authPolicyUpdate.Tags.SubTags), len(authPolicyUpdatedDetail.Tags.SubTags))
			ctx.Req.Equal(authPolicyUpdate.Tags.SubTags[tag1Name], authPolicyUpdatedDetail.Tags.SubTags[tag1Name])
			ctx.Req.Equal(authPolicyUpdate.Tags.SubTags[tag1Name], authPolicyUpdatedDetail.Tags.SubTags[tag1Name])
			ctx.Req.Equal(*authPolicyUpdate.Name, *authPolicyUpdatedDetail.Name)

			ctx.Req.Equal(len(authPolicyUpdate.Primary.ExtJWT.AllowedSigners), len(authPolicyUpdatedDetail.Primary.ExtJWT.AllowedSigners))
			ctx.Req.Equal(*authPolicyUpdate.Primary.ExtJWT.Allowed, *authPolicyUpdatedDetail.Primary.ExtJWT.Allowed)

			ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.Allowed, *authPolicyUpdatedDetail.Primary.Updb.Allowed)
			ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.MaxAttempts, *authPolicyUpdatedDetail.Primary.Updb.MaxAttempts)
			ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.MinPasswordLength, *authPolicyUpdatedDetail.Primary.Updb.MinPasswordLength)
			ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.LockoutDurationMinutes, *authPolicyUpdatedDetail.Primary.Updb.LockoutDurationMinutes)
			ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.RequireMixedCase, *authPolicyUpdatedDetail.Primary.Updb.RequireMixedCase)
			ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.RequireNumberChar, *authPolicyUpdatedDetail.Primary.Updb.RequireNumberChar)
			ctx.Req.Equal(*authPolicyUpdate.Primary.Updb.RequireSpecialChar, *authPolicyUpdatedDetail.Primary.Updb.RequireSpecialChar)

			ctx.Req.Equal(*authPolicyUpdate.Primary.Cert.Allowed, *authPolicyUpdatedDetail.Primary.Cert.Allowed)
			ctx.Req.Equal(*authPolicyUpdate.Primary.Cert.AllowExpiredCerts, *authPolicyUpdatedDetail.Primary.Cert.AllowExpiredCerts)

			ctx.Req.Equal(*authPolicyUpdate.Secondary.RequireTotp, *authPolicyUpdatedDetail.Secondary.RequireTotp)
			ctx.Req.Equal(*authPolicyUpdate.Secondary.RequireExtJWTSigner, *authPolicyUpdatedDetail.Secondary.RequireExtJWTSigner)
		})
	})

	t.Run("can patch default policy returns 200 and updates only the proper field", func(t *testing.T) {
		ctx.testContextChanged(t)
		authPolicyPatch := &rest_model.AuthPolicyPatch{
			Name: S("PatchedName On Default"),
		}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyPatch).Patch("/auth-policies/" + persistence.DefaultAuthPolicyId)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for PATCH %T: %s", "default auth policy", resp.Body())

		t.Run("get returns 200 and updated values", func(t *testing.T) {
			ctx.testContextChanged(t)

			authPolicyPatchEnvelope := &rest_model.DetailAuthPolicyEnvelope{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(authPolicyPatchEnvelope).Get("/auth-policies/" + persistence.DefaultAuthPolicyId)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for GET %s: %s", resp.Request.URL, resp.Body())
			authPolicyPatchedDetail := authPolicyPatchEnvelope.Data

			ctx.Req.Equal(*authPolicyPatch.Name, *authPolicyPatchedDetail.Name)
		})
	})

	t.Run("create with below minimum values default with 200 Ok", func(t *testing.T) {
		ctx.testContextChanged(t)
		jwtSignerCert, _ := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy 08")

		extJwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:  S(nfpem.EncodeToString(jwtSignerCert)),
			Enabled:  B(true),
			Name:     S("Test JWT Signer - Auth Policy 08"),
			Kid:      S(uuid.NewString()),
			Issuer:   S("test-issuer-105"),
			Audience: S("test-audience-105"),
		}

		extJwtSignerCreated := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSigner).SetResult(extJwtSignerCreated).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSigner, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreated.Data.ID)

		tag1Name := "originalTag1Name"
		tag1Value := "originalTag1Value"
		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 6"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed: B(true),
					AllowedSigners: []string{
						extJwtSignerCreated.Data.ID,
					},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(true),
					MaxAttempts:            I(-1),
					MinPasswordLength:      I(-1),
					LockoutDurationMinutes: I(-1),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
			},
			Tags: &rest_model.Tags{
				SubTags: map[string]interface{}{
					tag1Name: tag1Value,
				},
			},
		}

		authPolicyCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).SetResult(authPolicyCreated).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", authPolicy, resp.Body())
		ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

		t.Run("get returns 200 and same input values", func(t *testing.T) {
			ctx.testContextChanged(t)

			authPolicyEnvelope := &rest_model.DetailAuthPolicyEnvelope{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(authPolicyEnvelope).Get("/auth-policies/" + authPolicyCreated.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "expected 200 for GET %s: %s", resp.Request.URL, resp.Body())
			authPolicyDetail := authPolicyEnvelope.Data

			ctx.Req.Equal(authPolicyCreated.Data.ID, *authPolicyDetail.ID)
			ctx.Req.Equal(len(authPolicy.Tags.SubTags), len(authPolicyDetail.Tags.SubTags))
			ctx.Req.Equal(authPolicy.Tags.SubTags[tag1Name], authPolicyDetail.Tags.SubTags[tag1Name])
			ctx.Req.Equal(*authPolicy.Name, *authPolicyDetail.Name)

			ctx.Req.Equal(len(authPolicy.Primary.ExtJWT.AllowedSigners), len(authPolicyDetail.Primary.ExtJWT.AllowedSigners))
			ctx.Req.Equal(authPolicy.Primary.ExtJWT.AllowedSigners[0], authPolicyDetail.Primary.ExtJWT.AllowedSigners[0])
			ctx.Req.Equal(*authPolicy.Primary.ExtJWT.Allowed, *authPolicyDetail.Primary.ExtJWT.Allowed)

			ctx.Req.Equal(*authPolicy.Primary.Updb.Allowed, *authPolicyDetail.Primary.Updb.Allowed)
			ctx.Req.Equal(persistence.UpdbUnlimitedAttemptsLimit, *authPolicyDetail.Primary.Updb.MaxAttempts)
			ctx.Req.Equal(persistence.DefaultUpdbMinPasswordLength, *authPolicyDetail.Primary.Updb.MinPasswordLength)
			ctx.Req.Equal(persistence.UpdbIndefiniteLockout, *authPolicyDetail.Primary.Updb.LockoutDurationMinutes)
			ctx.Req.Equal(*authPolicy.Primary.Updb.RequireMixedCase, *authPolicyDetail.Primary.Updb.RequireMixedCase)
			ctx.Req.Equal(*authPolicy.Primary.Updb.RequireNumberChar, *authPolicyDetail.Primary.Updb.RequireNumberChar)
			ctx.Req.Equal(*authPolicy.Primary.Updb.RequireSpecialChar, *authPolicyDetail.Primary.Updb.RequireSpecialChar)

			ctx.Req.Equal(*authPolicy.Primary.Cert.Allowed, *authPolicyDetail.Primary.Cert.Allowed)
			ctx.Req.Equal(*authPolicy.Primary.Cert.AllowExpiredCerts, *authPolicyDetail.Primary.Cert.AllowExpiredCerts)

			ctx.Req.Equal(*authPolicy.Secondary.RequireTotp, *authPolicyDetail.Secondary.RequireTotp)
			ctx.Req.Equal(authPolicy.Secondary.RequireExtJWTSigner, authPolicyDetail.Secondary.RequireExtJWTSigner)
		})
	})

	t.Run("disabled auth modules stop authentication", func(t *testing.T) {
		ctx.testContextChanged(t)

		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 1 - Stops Auth"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        B(false),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(false),
					MaxAttempts:            I(5),
					MinPasswordLength:      I(5),
					LockoutDurationMinutes: I(0),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
			},
		}

		authPolicyCreated := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).SetResult(authPolicyCreated).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", authPolicy, resp.Body())
		ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

		identityType := rest_model.IdentityTypeDevice

		t.Run("cert authentication fails", func(t *testing.T) {
			ctx.testContextChanged(t)
			identityOtt := &rest_model.IdentityCreate{
				AuthPolicyID: &authPolicyCreated.Data.ID,
				IsAdmin:      B(false),
				Name:         S("test-identity-auth-policy-cert-not-allowed"),
				Type:         &identityType,
				Enrollment: &rest_model.IdentityCreateEnrollment{
					Ott: true,
				},
			}

			identityOttCreated := &rest_model.CreateEnvelope{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityOtt).SetResult(identityOttCreated).Post("/identities")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", identityOtt, resp.Body())
			ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

			certAuthenticator := ctx.completeOttEnrollment(identityOttCreated.Data.ID)

			apiSession, err := certAuthenticator.AuthenticateClientApi(ctx)
			ctx.Req.Error(err)
			ctx.Req.Nil(apiSession)
		})

		t.Run("updb authentication fails", func(t *testing.T) {
			ctx.testContextChanged(t)
			username := "test-identity-auth-policy-updb"
			password := "123abcABC#%!#"

			identityUpdb := &rest_model.IdentityCreate{
				AuthPolicyID: &authPolicyCreated.Data.ID,
				IsAdmin:      B(false),
				Name:         S("test-identity-auth-policy-updb-not-allowed"),
				Type:         &identityType,
				Enrollment: &rest_model.IdentityCreateEnrollment{
					Updb: username,
				},
			}

			identityUpdbCreated := &rest_model.CreateEnvelope{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityUpdb).SetResult(identityUpdbCreated).Post("/identities")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", identityUpdb, resp.Body())
			ctx.Req.NotEmpty(identityUpdbCreated.Data.ID)

			ctx.completeUpdbEnrollment(identityUpdbCreated.Data.ID, password)

			passwordAuthenticator := &updbAuthenticator{
				Username: username,
				Password: password,
			}

			apiSession, err := passwordAuthenticator.AuthenticateClientApi(ctx)
			ctx.Req.Error(err)
			ctx.Req.Nil(apiSession)
		})

		t.Run("ext jwt authentication fails", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSignerCert, jwtSignerPrivate := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy Ext JWT Not Allowed 01")

			extJwtSigner := &rest_model.ExternalJWTSignerCreate{
				CertPem:  S(nfpem.EncodeToString(jwtSignerCert)),
				Enabled:  B(true),
				Name:     S("Test JWT Signer - Auth Policy - Auth Policy Ext JWT Not Allowed 01"),
				Kid:      S(uuid.NewString()),
				Issuer:   S("test-issuer-106"),
				Audience: S("test-audience-106"),
			}

			extJwtSignerCreated := &rest_model.CreateEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSigner).SetResult(extJwtSignerCreated).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSigner, resp.Body())
			ctx.Req.NotEmpty(extJwtSignerCreated.Data.ID)

			identityExtJwt := &rest_model.IdentityCreate{
				AuthPolicyID: &authPolicyCreated.Data.ID,
				IsAdmin:      B(false),
				Name:         S("test-identity-auth-policy-ext-jwt-not-allowed"),
				Type:         &identityType,
			}

			identityExtJwtCreated := &rest_model.CreateEnvelope{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityExtJwt).SetResult(identityExtJwtCreated).Post("/identities")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", identityExtJwt, resp.Body())
			ctx.Req.NotEmpty(identityExtJwtCreated.Data.ID)

			jwtToken := jwt.New(jwt.SigningMethodES256)
			jwtToken.Claims = jwt.StandardClaims{
				Audience:  "ziti.controller",
				ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
				Id:        time.Now().String(),
				IssuedAt:  time.Now().Unix(),
				Issuer:    "fake.issuer",
				NotBefore: time.Now().Unix(),
				Subject:   identityExtJwtCreated.Data.ID,
			}

			jwtToken.Header["kid"] = *extJwtSigner.Kid

			jwtStrSigned, err := jwtToken.SignedString(jwtSignerPrivate)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(jwtStrSigned)

			result := &rest_model.CurrentAPISessionDetailEnvelope{}

			resp, err = ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())

		})

	})

	t.Run("auth module limits valid ext-jwt signers", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtSignerCertAllowed, jwtSignerPrivateAllowed := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy Ext JWT Allowed 01")

		extJwtSignerAllowed := &rest_model.ExternalJWTSignerCreate{
			CertPem:  S(nfpem.EncodeToString(jwtSignerCertAllowed)),
			Enabled:  B(true),
			Name:     S("Test JWT Signer - Auth Policy - Auth Policy Ext JWT Limited 01"),
			Kid:      S(uuid.NewString()),
			Issuer:   S("test-issuer-107"),
			Audience: S("test-audience-107"),
		}

		extJwtSignerCreatedAllowed := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSignerAllowed).SetResult(extJwtSignerCreatedAllowed).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSignerAllowed, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreatedAllowed.Data.ID)

		jwtSignerCertNotAllowed, jwtSignerPrivateNotAllowed := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy Ext JWT Not Allowed 02")

		extJwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:  S(nfpem.EncodeToString(jwtSignerCertNotAllowed)),
			Enabled:  B(true),
			Name:     S("Test JWT Signer - Auth Policy - Auth Policy Ext JWT Limited 02"),
			Kid:      S(uuid.NewString()),
			Issuer:   S("test-issuer-108"),
			Audience: S("test-audience-108"),
		}

		extJwtSignerCreatedNotAllowed := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSigner).SetResult(extJwtSignerCreatedNotAllowed).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSigner, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreatedNotAllowed.Data.ID)

		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 1 - Limits ext-jwt signers"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed: B(true),
					AllowedSigners: []string{
						extJwtSignerCreatedAllowed.Data.ID,
					},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(false),
					MaxAttempts:            I(5),
					MinPasswordLength:      I(5),
					LockoutDurationMinutes: I(0),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
			},
		}

		authPolicyCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).SetResult(authPolicyCreated).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", authPolicy, resp.Body())
		ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

		identityType := rest_model.IdentityTypeDevice

		identityExtJwt := &rest_model.IdentityCreate{
			AuthPolicyID: &authPolicyCreated.Data.ID,
			IsAdmin:      B(false),
			Name:         S("test-identity-auth-policy-ext-jwt-not-allowed 02"),
			Type:         &identityType,
		}

		identityExtJwtCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityExtJwt).SetResult(identityExtJwtCreated).Post("/identities")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", identityExtJwt, resp.Body())
		ctx.Req.NotEmpty(identityExtJwtCreated.Data.ID)

		t.Run("can authenticate with approved external jwt signer", func(t *testing.T) {
			jwtToken := jwt.New(jwt.SigningMethodES256)
			jwtToken.Claims = jwt.StandardClaims{
				ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
				Id:        time.Now().String(),
				IssuedAt:  time.Now().Unix(),
				Issuer:    "test-issuer-107",
				Audience:  "test-audience-107",
				NotBefore: time.Now().Unix(),
				Subject:   identityExtJwtCreated.Data.ID,
			}

			jwtToken.Header["kid"] = *extJwtSignerAllowed.Kid

			jwtStrSigned, err := jwtToken.SignedString(jwtSignerPrivateAllowed)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(jwtStrSigned)

			result := &rest_model.CurrentAPISessionDetailEnvelope{}

			resp, err = ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())
		})

		t.Run("cannot authenticate with unapproved external jwt signer", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtToken := jwt.New(jwt.SigningMethodES256)
			jwtToken.Claims = jwt.StandardClaims{
				Audience:  "ziti.controller",
				ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
				Id:        time.Now().String(),
				IssuedAt:  time.Now().Unix(),
				Issuer:    "fake.issuer",
				NotBefore: time.Now().Unix(),
				Subject:   identityExtJwtCreated.Data.ID,
			}

			jwtToken.Header["kid"] = *extJwtSigner.Kid

			jwtStrSigned, err := jwtToken.SignedString(jwtSignerPrivateNotAllowed)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(jwtStrSigned)

			result := &rest_model.CurrentAPISessionDetailEnvelope{}

			resp, err = ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
		})
	})

	t.Run("auth policy with secondary ext jwt signer", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtSignerCertAllowed, validJwtSignerPrivateKey := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy Ext JWT FA 01")

		extJwtSignerAllowed := &rest_model.ExternalJWTSignerCreate{
			CertPem:         S(nfpem.EncodeToString(jwtSignerCertAllowed)),
			Enabled:         B(true),
			Name:            S("Test JWT Signer - Auth Policy - Auth Policy Ext JWT FA 01"),
			ExternalAuthURL: S("https://get.some.jwt.here"),
			Kid:             S(uuid.NewString()),
			Issuer:          S("test-issuer-109"),
			Audience:        S("test-audience-109"),
		}

		extJwtSignerCreatedAllowed := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSignerAllowed).SetResult(extJwtSignerCreatedAllowed).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSignerAllowed, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreatedAllowed.Data.ID)

		authPolicy := &rest_model.AuthPolicyCreate{
			Name: S("Original Name 1 - ext jwt FA 01"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: B(true),
					Allowed:           B(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        B(true),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(false),
					MaxAttempts:            I(5),
					MinPasswordLength:      I(5),
					LockoutDurationMinutes: I(0),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: &extJwtSignerCreatedAllowed.Data.ID,
				RequireTotp:         B(false),
			},
		}

		authPolicyCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicy).SetResult(authPolicyCreated).Post("/auth-policies")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", authPolicy, resp.Body())
		ctx.Req.NotEmpty(authPolicyCreated.Data.ID)

		identityType := rest_model.IdentityTypeDevice

		identityExtJwt := &rest_model.IdentityCreate{
			AuthPolicyID: &authPolicyCreated.Data.ID,
			IsAdmin:      B(false),
			Name:         S("test-identity-auth-policy-ext-jwt-FA 01"),
			Type:         &identityType,
			Enrollment: &rest_model.IdentityCreateEnrollment{
				Ott: true,
			},
		}

		identityExtJwtCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityExtJwt).SetResult(identityExtJwtCreated).Post("/identities")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", identityExtJwt, resp.Body())
		ctx.Req.NotEmpty(identityExtJwtCreated.Data.ID)

		certAuthenticator := ctx.completeOttEnrollment(identityExtJwtCreated.Data.ID)
		ctx.Req.NotNil(certAuthenticator)

		t.Run("authenticating returns jwt auth query", func(t *testing.T) {
			ctx.testContextChanged(t)

			apiSession, err := certAuthenticator.AuthenticateClientApi(ctx)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(apiSession)

			currentApiSessionEnv := &rest_model.CurrentAPISessionDetailEnvelope{}

			resp, err = apiSession.newAuthenticatedRequest().SetResult(currentApiSessionEnv).Get("current-api-session")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())
			ctx.Req.NotNil(currentApiSessionEnv.Data)

			ctx.Req.NotEmpty(currentApiSessionEnv.Data.AuthQueries)

			ctx.Req.Equal("EXT-JWT", currentApiSessionEnv.Data.AuthQueries[0].TypeID)
			ctx.Req.Equal(*extJwtSignerAllowed.ExternalAuthURL, currentApiSessionEnv.Data.AuthQueries[0].HTTPURL)

			t.Run("without bearer token partially authenticated", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := apiSession.newAuthenticatedRequest().Get("services")
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
			})

			t.Run("with bearer token full access is granted", func(t *testing.T) {
				ctx.testContextChanged(t)

				jwtToken := jwt.New(jwt.SigningMethodES256)
				jwtToken.Claims = jwt.StandardClaims{
					ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
					Id:        time.Now().String(),
					IssuedAt:  time.Now().Unix(),
					Issuer:    "test-issuer-109",
					Audience:  "test-audience-109",
					NotBefore: time.Now().Unix(),
					Subject:   identityExtJwtCreated.Data.ID,
				}

				jwtToken.Header["kid"] = *extJwtSignerAllowed.Kid

				jwtStrSigned, err := jwtToken.SignedString(validJwtSignerPrivateKey)
				ctx.Req.NoError(err)
				ctx.Req.NotEmpty(jwtStrSigned)

				resp, err := apiSession.newAuthenticatedRequest().SetHeader("Authorization", "Bearer "+jwtStrSigned).Get("services")
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			})
		})

	})
}
