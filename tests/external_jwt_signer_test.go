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
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"github.com/google/uuid"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/rest_model"
	nfpem "github.com/openziti/foundation/util/pem"
	"math/big"
	"net/http"
	"testing"
	"time"
)

func Test_ExternalJWTSigner(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("create with valid values returns 200 Ok", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtSignerCommonName := "soCommon"
		jwtSignerCert, _ := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey
		jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)
		jwtSignerName := "Test JWT Signer"
		jwtSignerEnabled := true

		jwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:         &jwtSignerCertPem,
			ClaimsProperty:  S("someMadeUpClaim"),
			Enabled:         &jwtSignerEnabled,
			ExternalAuthURL: S("https://some-auth-url"),
			Name:            &jwtSignerName,
			Tags:            nil,
			UseExternalID:   B(true),
			Kid:             S(uuid.New().String()),
			Issuer:          S("i-am-the-issuer"),
			Audience:        S("you-are-the-audience"),
		}

		createResponseEnv := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

		t.Run("get after create returns 200 Ok", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSignerDetailEnv := &rest_model.DetailExternalJWTSignerEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(jwtSignerDetailEnv).Get("/external-jwt-signers/" + createResponseEnv.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			jwtSignerDetail := jwtSignerDetailEnv.Data

			t.Run("has the correct value", func(t *testing.T) {
				ctx.testContextChanged(t)

				fingerprint := nfpem.FingerprintFromCertificate(jwtSignerCert)

				ctx.Req.Equal(jwtSignerName, *jwtSignerDetail.Name)
				ctx.Req.Equal(jwtSignerCommonName, *jwtSignerDetail.CommonName)
				ctx.Req.Equal(jwtSignerCertPem, *jwtSignerDetail.CertPem)
				ctx.Req.Equal(jwtSignerEnabled, *jwtSignerDetail.Enabled)
				ctx.Req.Equal(jwtSignerCert.NotBefore, time.Time(*jwtSignerDetail.NotBefore))
				ctx.Req.Equal(jwtSignerCert.NotAfter, time.Time(*jwtSignerDetail.NotAfter))
				ctx.Req.Equal(fingerprint, *jwtSignerDetail.Fingerprint)
				ctx.Req.Equal(*jwtSigner.UseExternalID, *jwtSignerDetail.UseExternalID)
				ctx.Req.Equal(*jwtSigner.ClaimsProperty, *jwtSignerDetail.ClaimsProperty)
				ctx.Req.Equal(*jwtSigner.ExternalAuthURL, *jwtSignerDetail.ExternalAuthURL)
				ctx.Req.Equal(*jwtSigner.Kid, *jwtSignerDetail.Kid)
				ctx.Req.Equal(*jwtSigner.Issuer, *jwtSignerDetail.Issuer)
				ctx.Req.Equal(*jwtSigner.Audience, *jwtSignerDetail.Audience)
			})
		})

		t.Run("delete after create returns 200 ok", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/external-jwt-signers/" + createResponseEnv.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(resp.StatusCode(), http.StatusOK)

			t.Run("delete after delete returns 404 not found", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/external-jwt-signers/" + createResponseEnv.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(resp.StatusCode(), http.StatusNotFound)
			})

			t.Run("get after delete returns 404 not found", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/external-jwt-signers/" + createResponseEnv.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(resp.StatusCode(), http.StatusNotFound)
			})

			t.Run("patch after delete returns 404 not found", func(t *testing.T) {
				ctx.testContextChanged(t)

				patchBody := &rest_model.ExternalJWTSignerPatch{
					CertPem: &jwtSignerCertPem,
					Enabled: &jwtSignerEnabled,
					Name:    &jwtSignerName,
				}

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(patchBody).Patch("/external-jwt-signers/" + createResponseEnv.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(resp.StatusCode(), http.StatusNotFound)
			})

			t.Run("put after delete returns 404 not found", func(t *testing.T) {
				ctx.testContextChanged(t)

				putBody := &rest_model.ExternalJWTSignerUpdate{
					CertPem:  &jwtSignerCertPem,
					Enabled:  &jwtSignerEnabled,
					Name:     &jwtSignerName,
					Kid:      S(uuid.NewString()),
					Issuer:   S(uuid.NewString()),
					Audience: S(uuid.NewString()),
				}

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(putBody).Put("/external-jwt-signers/" + createResponseEnv.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusNotFound, resp.StatusCode(), string(resp.Body()))
			})
		})
	})

	t.Run("create with only required values returns 200 Ok", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtSignerCommonName := "soCommon"
		jwtSignerCert, _ := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey
		jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)
		jwtSignerName := "Test JWT Signer"
		jwtSignerEnabled := true

		jwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:  &jwtSignerCertPem,
			Enabled:  &jwtSignerEnabled,
			Name:     &jwtSignerName,
			Kid:      S(uuid.New().String()),
			Issuer:   S(uuid.NewString()),
			Audience: S(uuid.NewString()),
		}

		createResponseEnv := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

		t.Run("get after create returns 200 Ok", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSignerDetailEnv := &rest_model.DetailExternalJWTSignerEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(jwtSignerDetailEnv).Get("/external-jwt-signers/" + createResponseEnv.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			jwtSignerDetail := jwtSignerDetailEnv.Data

			t.Run("has the correct values and default values", func(t *testing.T) {
				ctx.testContextChanged(t)

				fingerprint := nfpem.FingerprintFromCertificate(jwtSignerCert)

				ctx.Req.Equal(jwtSignerName, *jwtSignerDetail.Name)
				ctx.Req.Equal(jwtSignerCommonName, *jwtSignerDetail.CommonName)
				ctx.Req.Equal(jwtSignerCertPem, *jwtSignerDetail.CertPem)
				ctx.Req.Equal(jwtSignerEnabled, *jwtSignerDetail.Enabled)
				ctx.Req.Equal(jwtSignerCert.NotBefore, time.Time(*jwtSignerDetail.NotBefore))
				ctx.Req.Equal(jwtSignerCert.NotAfter, time.Time(*jwtSignerDetail.NotAfter))
				ctx.Req.Equal(fingerprint, *jwtSignerDetail.Fingerprint)
				ctx.Req.False(*jwtSignerDetail.UseExternalID)
				ctx.Req.Equal(persistence.DefaultClaimsProperty, *jwtSignerDetail.ClaimsProperty)
				ctx.Req.Nil(jwtSignerDetail.ExternalAuthURL)
				ctx.Req.Equal(*jwtSigner.Issuer, *jwtSignerDetail.Issuer)
				ctx.Req.Equal(*jwtSigner.Audience, *jwtSignerDetail.Audience)
			})
		})
	})

	t.Run("create with missing values returns 400 bad request", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtSignerCommonName := "soCommon"
		jwtSignerCert, _ := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey
		jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)
		jwtSignerName := "Test JWT Signer"
		jwtSignerEnabled := true

		t.Run("missing cert pem", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSigner := &rest_model.ExternalJWTSignerCreate{
				Enabled: &jwtSignerEnabled,
				Name:    &jwtSignerName,
				Kid:     S(uuid.New().String()),
			}

			createResponseEnv := &rest_model.CreateEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
		})

		t.Run("missing enabled", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSigner := &rest_model.ExternalJWTSignerCreate{
				CertPem: &jwtSignerCertPem,
				Name:    &jwtSignerName,
				Kid:     S(uuid.New().String()),
			}

			createResponseEnv := &rest_model.CreateEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
		})

		t.Run("missing name", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSigner := &rest_model.ExternalJWTSignerCreate{
				CertPem: &jwtSignerCertPem,
				Enabled: &jwtSignerEnabled,
				Kid:     S(uuid.New().String()),
			}

			createResponseEnv := &rest_model.CreateEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
		})

		t.Run("missing kid", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSigner := &rest_model.ExternalJWTSignerCreate{
				CertPem: &jwtSignerCertPem,
				Enabled: &jwtSignerEnabled,
				Name:    &jwtSignerName,
			}

			createResponseEnv := &rest_model.CreateEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
		})
	})

	t.Run("create with an invalid cert pem returns 400 bad request", func(t *testing.T) {
		ctx.testContextChanged(t)

		invalidCertPem := "probably won't parse right?"
		jwtSignerName := "Test JWT Signer"
		jwtSignerEnabled := true

		t.Run("missing cert pem", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSigner := &rest_model.ExternalJWTSignerCreate{
				CertPem: &invalidCertPem,
				Enabled: &jwtSignerEnabled,
				Name:    &jwtSignerName,
				Kid:     S(uuid.New().String()),
			}

			errorResponse := &rest_model.APIErrorEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(errorResponse).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
		})
	})

	t.Run("create with re-used signing certificate/kid fails", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtSignerCommonName := "soCommon-dupe1"
		jwtSignerCert, _ := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey
		jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)
		jwtSignerName := "Test JWT Signer 05"
		jwtSignerEnabled := true

		jwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:         &jwtSignerCertPem,
			ClaimsProperty:  S("someMadeUpClaim"),
			Enabled:         &jwtSignerEnabled,
			ExternalAuthURL: S("https://some-auth-url"),
			Name:            &jwtSignerName,
			Tags:            nil,
			UseExternalID:   B(true),
			Kid:             S(uuid.New().String()),
			Issuer:          S(uuid.NewString()),
			Audience:        S(uuid.NewString()),
		}

		createResponseEnv := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

		t.Run("re-used cert fails with 400 bad request", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSignerReusedCert := &rest_model.ExternalJWTSignerCreate{
				CertPem:         &jwtSignerCertPem,
				ClaimsProperty:  S("whatever"),
				Enabled:         &jwtSignerEnabled,
				ExternalAuthURL: S("https://some-other-auth-url"),
				Name:            S("dupe-should fail"),
				Tags:            nil,
				UseExternalID:   B(true),
				Kid:             S(uuid.New().String()),
			}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSignerReusedCert).SetResult(createResponseEnv).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
		})

		t.Run("re-used kid create fails with 400 bad request", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSignerReusedCert := &rest_model.ExternalJWTSignerCreate{
				CertPem:         &jwtSignerCertPem,
				ClaimsProperty:  S("whatever"),
				Enabled:         &jwtSignerEnabled,
				ExternalAuthURL: S("https://some-other-auth-url"),
				Name:            S("dupe-should fail"),
				Tags:            nil,
				UseExternalID:   B(true),
				Kid:             jwtSigner.Kid,
			}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSignerReusedCert).SetResult(createResponseEnv).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
		})
	})

	t.Run("update with all values succeeds", func(t *testing.T) {
		ctx.testContextChanged(t)
		jwtSignerCommonName := "soCommon2"
		jwtSignerCommonNameUpdated := "soCommon2Updated"

		jwtSignerCert, _ := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey
		jwtSignerCertUpdated, _ := newSelfSignedCert(jwtSignerCommonNameUpdated)

		jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)
		jwtSignerCertPemUpdated := nfpem.EncodeToString(jwtSignerCertUpdated)

		jwtSignerName := "Test JWT Signer 06"
		jwtSignerNameUpdated := "Test JWT Signer 06 Updated"

		jwtSignerEnabled := false
		jwtSignerEnabledUpdated := true

		jwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:  &jwtSignerCertPem,
			Enabled:  &jwtSignerEnabled,
			Name:     &jwtSignerName,
			Kid:      S(uuid.New().String()),
			Issuer:   S("origIssues"),
			Audience: S("origAudience"),
		}

		createResponseEnv := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

		jwtSignerUpdate := &rest_model.ExternalJWTSignerUpdate{
			CertPem:  &jwtSignerCertPemUpdated,
			Enabled:  &jwtSignerEnabledUpdated,
			Name:     &jwtSignerNameUpdated,
			Kid:      S(uuid.NewString()),
			Issuer:   S(uuid.NewString()),
			Audience: S(uuid.NewString()),
		}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSignerUpdate).SetResult(createResponseEnv).Put("/external-jwt-signers/" + createResponseEnv.Data.ID)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

		t.Run("get after update returns 200 Ok", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtSignerDetailEnv := &rest_model.DetailExternalJWTSignerEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(jwtSignerDetailEnv).Get("/external-jwt-signers/" + createResponseEnv.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			jwtSignerDetail := jwtSignerDetailEnv.Data

			t.Run("has the correct value", func(t *testing.T) {
				ctx.testContextChanged(t)

				fingerprint := nfpem.FingerprintFromCertificate(jwtSignerCertUpdated)

				ctx.Req.Equal(jwtSignerNameUpdated, *jwtSignerDetail.Name)
				ctx.Req.Equal(jwtSignerCommonNameUpdated, *jwtSignerDetail.CommonName)
				ctx.Req.Equal(jwtSignerCertPemUpdated, *jwtSignerDetail.CertPem)
				ctx.Req.Equal(jwtSignerEnabledUpdated, *jwtSignerDetail.Enabled)
				ctx.Req.Equal(jwtSignerCertUpdated.NotBefore, time.Time(*jwtSignerDetail.NotBefore))
				ctx.Req.Equal(jwtSignerCertUpdated.NotAfter, time.Time(*jwtSignerDetail.NotAfter))
				ctx.Req.Equal(fingerprint, *jwtSignerDetail.Fingerprint)
				ctx.Req.Equal(*jwtSignerUpdate.Kid, *jwtSignerDetail.Kid)
				ctx.Req.Equal(*jwtSignerUpdate.Issuer, *jwtSignerDetail.Issuer)
				ctx.Req.Equal(*jwtSignerUpdate.Audience, *jwtSignerDetail.Audience)
			})
		})
	})

	t.Run("patch", func(t *testing.T) {
		ctx.testContextChanged(t)

		t.Run("name only succeeds", func(t *testing.T) {
			ctx.testContextChanged(t)
			jwtSignerCommonName := "soCommon patch name"

			jwtSignerCert, _ := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey

			jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)

			jwtSignerName := "Test JWT Signer Pre-Patch Name"
			jwtSignerNamePatched := "Test JWT Signer Post-Patched Name"

			jwtSignerEnabled := true

			jwtSigner := &rest_model.ExternalJWTSignerCreate{
				CertPem:  &jwtSignerCertPem,
				Enabled:  &jwtSignerEnabled,
				Name:     &jwtSignerName,
				Kid:      S(uuid.New().String()),
				Issuer:   S(uuid.NewString()),
				Audience: S(uuid.NewString()),
			}

			createResponseEnv := &rest_model.CreateEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

			jwtSignerPatch := &rest_model.ExternalJWTSignerPatch{
				Name: &jwtSignerNamePatched,
			}

			patchResponseEnv := &rest_model.Empty{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSignerPatch).SetResult(patchResponseEnv).Patch("/external-jwt-signers/" + createResponseEnv.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			t.Run("get after patch returns 200 Ok", func(t *testing.T) {
				ctx.testContextChanged(t)

				jwtSignerDetailEnv := &rest_model.DetailExternalJWTSignerEnvelope{}

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(jwtSignerDetailEnv).Get("/external-jwt-signers/" + createResponseEnv.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				jwtSignerDetail := jwtSignerDetailEnv.Data

				t.Run("has the correct value", func(t *testing.T) {
					ctx.testContextChanged(t)

					fingerprint := nfpem.FingerprintFromCertificate(jwtSignerCert)

					ctx.Req.Equal(jwtSignerNamePatched, *jwtSignerDetail.Name)
					ctx.Req.Equal(jwtSignerCommonName, *jwtSignerDetail.CommonName)
					ctx.Req.Equal(jwtSignerCertPem, *jwtSignerDetail.CertPem)
					ctx.Req.Equal(jwtSignerEnabled, *jwtSignerDetail.Enabled)
					ctx.Req.Equal(jwtSignerCert.NotBefore, time.Time(*jwtSignerDetail.NotBefore))
					ctx.Req.Equal(jwtSignerCert.NotAfter, time.Time(*jwtSignerDetail.NotAfter))
					ctx.Req.Equal(fingerprint, *jwtSignerDetail.Fingerprint)
					ctx.Req.Equal(*jwtSigner.Kid, *jwtSignerDetail.Kid)
				})
			})
		})

		t.Run("cert only succeeds", func(t *testing.T) {
			ctx.testContextChanged(t)
			jwtSignerCommonName := "soCommon patch cert"
			jwtSignerCommonNamePatched := "soCommon patch cert post patched"

			jwtSignerCert, _ := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey
			jwtSignerCertPatched, _ := newSelfSignedCert(jwtSignerCommonNamePatched)

			jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)
			jwtSignerCertPemPatched := nfpem.EncodeToString(jwtSignerCertPatched)

			jwtSignerName := "Test JWT Signer Pre-Patch Cert"

			jwtSignerEnabled := true

			jwtSigner := &rest_model.ExternalJWTSignerCreate{
				CertPem:  &jwtSignerCertPem,
				Enabled:  &jwtSignerEnabled,
				Name:     &jwtSignerName,
				Kid:      S(uuid.New().String()),
				Issuer:   S(uuid.NewString()),
				Audience: S(uuid.NewString()),
			}

			createResponseEnv := &rest_model.CreateEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

			jwtSignerPatch := &rest_model.ExternalJWTSignerPatch{
				CertPem: &jwtSignerCertPemPatched,
			}

			patchResponseEnv := &rest_model.Empty{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSignerPatch).SetResult(patchResponseEnv).Patch("/external-jwt-signers/" + createResponseEnv.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			t.Run("get after patch returns 200 Ok", func(t *testing.T) {
				ctx.testContextChanged(t)

				jwtSignerDetailEnv := &rest_model.DetailExternalJWTSignerEnvelope{}

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(jwtSignerDetailEnv).Get("/external-jwt-signers/" + createResponseEnv.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				jwtSignerDetail := jwtSignerDetailEnv.Data

				t.Run("has the correct value", func(t *testing.T) {
					ctx.testContextChanged(t)

					fingerprint := nfpem.FingerprintFromCertificate(jwtSignerCertPatched)

					ctx.Req.Equal(jwtSignerName, *jwtSignerDetail.Name)
					ctx.Req.Equal(jwtSignerCommonNamePatched, *jwtSignerDetail.CommonName)
					ctx.Req.Equal(jwtSignerCertPemPatched, *jwtSignerDetail.CertPem)
					ctx.Req.Equal(jwtSignerEnabled, *jwtSignerDetail.Enabled)
					ctx.Req.Equal(jwtSignerCertPatched.NotBefore, time.Time(*jwtSignerDetail.NotBefore))
					ctx.Req.Equal(jwtSignerCertPatched.NotAfter, time.Time(*jwtSignerDetail.NotAfter))
					ctx.Req.Equal(fingerprint, *jwtSignerDetail.Fingerprint)
					ctx.Req.Equal(*jwtSigner.Kid, *jwtSignerDetail.Kid)
				})
			})
		})

		t.Run("enable only succeeds", func(t *testing.T) {
			ctx.testContextChanged(t)
			jwtSignerCommonName := "soCommon patch enable"

			jwtSignerCert, _ := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey

			jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)

			jwtSignerName := "Test JWT Signer Pre-Patch Enable"

			jwtSignerEnabled := true
			jwtSignerEnabledPatched := false

			jwtSigner := &rest_model.ExternalJWTSignerCreate{
				CertPem:  &jwtSignerCertPem,
				Enabled:  &jwtSignerEnabled,
				Name:     &jwtSignerName,
				Kid:      S(uuid.NewString()),
				Issuer:   S(uuid.NewString()),
				Audience: S(uuid.NewString()),
			}

			createResponseEnv := &rest_model.CreateEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

			jwtSignerPatch := &rest_model.ExternalJWTSignerPatch{
				Enabled: &jwtSignerEnabledPatched,
			}

			patchResponseEnv := &rest_model.Empty{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSignerPatch).SetResult(patchResponseEnv).Patch("/external-jwt-signers/" + createResponseEnv.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			t.Run("get after patch returns 200 Ok", func(t *testing.T) {
				ctx.testContextChanged(t)

				jwtSignerDetailEnv := &rest_model.DetailExternalJWTSignerEnvelope{}

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(jwtSignerDetailEnv).Get("/external-jwt-signers/" + createResponseEnv.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				jwtSignerDetail := jwtSignerDetailEnv.Data

				t.Run("has the correct value", func(t *testing.T) {
					ctx.testContextChanged(t)

					fingerprint := nfpem.FingerprintFromCertificate(jwtSignerCert)

					ctx.Req.Equal(jwtSignerName, *jwtSignerDetail.Name)
					ctx.Req.Equal(jwtSignerCommonName, *jwtSignerDetail.CommonName)
					ctx.Req.Equal(jwtSignerCertPem, *jwtSignerDetail.CertPem)
					ctx.Req.Equal(jwtSignerEnabledPatched, *jwtSignerDetail.Enabled)
					ctx.Req.Equal(jwtSignerCert.NotBefore, time.Time(*jwtSignerDetail.NotBefore))
					ctx.Req.Equal(jwtSignerCert.NotAfter, time.Time(*jwtSignerDetail.NotAfter))
					ctx.Req.Equal(fingerprint, *jwtSignerDetail.Fingerprint)
					ctx.Req.Equal(*jwtSigner.Kid, *jwtSignerDetail.Kid)
				})
			})
		})

		t.Run("kid only succeeds", func(t *testing.T) {
			ctx.testContextChanged(t)
			jwtSignerCommonName := "soCommon patch kid"

			jwtSignerCert, _ := newSelfSignedCert(jwtSignerCommonName) // jwtSignerPrivKey

			jwtSignerCertPem := nfpem.EncodeToString(jwtSignerCert)

			jwtSignerName := "Test JWT Signer Pre-Patch Kid"

			jwtSignerEnabled := true

			jwtSigner := &rest_model.ExternalJWTSignerCreate{
				CertPem:  &jwtSignerCertPem,
				Enabled:  &jwtSignerEnabled,
				Name:     &jwtSignerName,
				Kid:      S(uuid.New().String()),
				Issuer:   S(uuid.NewString()),
				Audience: S(uuid.NewString()),
			}

			createResponseEnv := &rest_model.CreateEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

			jwtSignerPatch := &rest_model.ExternalJWTSignerPatch{
				Kid: S(uuid.New().String()),
			}

			patchResponseEnv := &rest_model.Empty{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(jwtSignerPatch).SetResult(patchResponseEnv).Patch("/external-jwt-signers/" + createResponseEnv.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			t.Run("get after patch returns 200 Ok", func(t *testing.T) {
				ctx.testContextChanged(t)

				jwtSignerDetailEnv := &rest_model.DetailExternalJWTSignerEnvelope{}

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(jwtSignerDetailEnv).Get("/external-jwt-signers/" + createResponseEnv.Data.ID)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				jwtSignerDetail := jwtSignerDetailEnv.Data

				t.Run("has the correct value", func(t *testing.T) {
					ctx.testContextChanged(t)

					fingerprint := nfpem.FingerprintFromCertificate(jwtSignerCert)

					ctx.Req.Equal(jwtSignerName, *jwtSignerDetail.Name)
					ctx.Req.Equal(jwtSignerCommonName, *jwtSignerDetail.CommonName)
					ctx.Req.Equal(jwtSignerCertPem, *jwtSignerDetail.CertPem)
					ctx.Req.Equal(*jwtSigner.Enabled, *jwtSignerDetail.Enabled)
					ctx.Req.Equal(jwtSignerCert.NotBefore, time.Time(*jwtSignerDetail.NotBefore))
					ctx.Req.Equal(jwtSignerCert.NotAfter, time.Time(*jwtSignerDetail.NotAfter))
					ctx.Req.Equal(fingerprint, *jwtSignerDetail.Fingerprint)
					ctx.Req.Equal(*jwtSignerPatch.Kid, *jwtSignerDetail.Kid)
				})
			})
		})
	})
}

func newSelfSignedCert(commonName string) (*x509.Certificate, crypto.PrivateKey) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"API Test Co"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		panic(err)
	}
	cert, err := x509.ParseCertificate(der)

	if err != nil {
		panic(err)
	}

	return cert, priv
}
