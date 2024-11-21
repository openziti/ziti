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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	cryptoTls "crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/identity/certtools"
	"github.com/openziti/ziti/common/eid"
	"gopkg.in/resty.v1"
	"net/http"
	"testing"
	"time"
)

func Test_EnrollmentOtt(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("can create an OTT enrollment", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity := ctx.AdminManagementSession.requireNewIdentity(false)

		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: &identity.Id,
			Method:     S(rest_model.EnrollmentCreateMethodOtt),
			ExpiresAt:  ST(time.Now().Add(1 * time.Hour).UTC()),
		}

		enrollmentCreateResp := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(enrollmentCreateResp)
		ctx.NotNil(enrollmentCreateResp.Data)
		ctx.NotEmpty(enrollmentCreateResp.Data.ID)

		t.Run("enrollment has the proper values", func(t *testing.T) {
			ctx.testContextChanged(t)
			enrollmentGetResp := &rest_model.DetailEnrollmentEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(enrollmentGetResp).Get("/enrollments/" + enrollmentCreateResp.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.NotNil(enrollmentGetResp)
			ctx.NotNil(enrollmentGetResp.Data)
			ctx.NotNil(enrollmentGetResp.Data.ID)
			ctx.NotNil(enrollmentGetResp.Data.Method)
			ctx.NotNil(enrollmentGetResp.Data.ExpiresAt)
			ctx.NotNil(enrollmentGetResp.Data.Identity)
			ctx.NotNil(enrollmentGetResp.Data.Token)
			ctx.NotEmpty(*enrollmentGetResp.Data.Token)
			ctx.NotEmpty(enrollmentGetResp.Data.IdentityID)
			ctx.NotEmpty(enrollmentGetResp.Data.JWT)

			ctx.Equal(*enrollmentCreate.Method, *enrollmentGetResp.Data.Method)
			ctx.Equal(enrollmentCreate.ExpiresAt.String(), enrollmentGetResp.Data.ExpiresAt.String())
			ctx.Equal(*enrollmentCreate.IdentityID, enrollmentGetResp.Data.IdentityID)
		})

		t.Run("can enroll", func(t *testing.T) {
			ctx.testContextChanged(t)
			authenticator := ctx.completeOttEnrollment(identity.Id)
			ctx.Req.NotNil(authenticator)
		})
	})

	t.Run("can not enroll with an expired enrollment", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity := ctx.AdminManagementSession.requireNewIdentity(false)

		expiresAt := time.Now().Add(5 * time.Second).UTC()
		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: &identity.Id,
			Method:     S(rest_model.EnrollmentCreateMethodOtt),
			ExpiresAt:  ST(expiresAt),
		}

		enrollmentCreateResp := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(enrollmentCreateResp)
		ctx.NotNil(enrollmentCreateResp.Data)
		ctx.NotEmpty(enrollmentCreateResp.Data.ID)

		t.Run("enrollment has the proper values", func(t *testing.T) {
			ctx.testContextChanged(t)
			enrollmentGetResp := &rest_model.DetailEnrollmentEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(enrollmentGetResp).Get("/enrollments/" + enrollmentCreateResp.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.NotNil(enrollmentGetResp)
			ctx.NotNil(enrollmentGetResp.Data)
			ctx.NotNil(enrollmentGetResp.Data.ID)
			ctx.NotNil(enrollmentGetResp.Data.Method)
			ctx.NotNil(enrollmentGetResp.Data.ExpiresAt)
			ctx.NotNil(enrollmentGetResp.Data.Identity)
			ctx.NotNil(enrollmentGetResp.Data.Token)
			ctx.NotEmpty(*enrollmentGetResp.Data.Token)
			ctx.NotEmpty(enrollmentGetResp.Data.IdentityID)
			ctx.NotEmpty(enrollmentGetResp.Data.JWT)

			ctx.Equal(*enrollmentCreate.Method, *enrollmentGetResp.Data.Method)
			ctx.Equal(enrollmentCreate.ExpiresAt.String(), enrollmentGetResp.Data.ExpiresAt.String())
			ctx.Equal(*enrollmentCreate.IdentityID, enrollmentGetResp.Data.IdentityID)
		})

		t.Run("can not enroll", func(t *testing.T) {
			ctx.testContextChanged(t)
			duration := time.Until(expiresAt)

			if duration > 0 {
				time.Sleep(duration)
			}

			result := ctx.AdminManagementSession.requireQuery(fmt.Sprintf("identities/%v", identity.Id))

			tokenValue := result.Path("data.enrollment.ott.token")

			ctx.Req.NotNil(tokenValue)
			token, ok := tokenValue.Data().(string)
			ctx.Req.True(ok)

			privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
			ctx.Req.NoError(err)

			request, err := certtools.NewCertRequest(map[string]string{
				"C": "US", "O": "NetFoundry-API-Test", "CN": identity.Id,
			}, nil)
			ctx.Req.NoError(err)

			csr, err := x509.CreateCertificateRequest(rand.Reader, request, privateKey)
			ctx.Req.NoError(err)

			csrPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})

			enrollResp, err := ctx.newAnonymousClientApiRequest().
				SetBody(csrPem).
				SetHeader("content-type", "application/x-pem-file").
				SetHeader("accept", "application/json").
				Post("enroll?token=" + token)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusBadRequest, enrollResp.StatusCode())
		})
	})

	t.Run("can not create two OTT enrollments", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity := ctx.AdminManagementSession.requireNewIdentity(false)

		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: &identity.Id,
			Method:     S(rest_model.EnrollmentCreateMethodOtt),
			ExpiresAt:  ST(time.Now().Add(1 * time.Hour).UTC()),
		}

		enrollmentCreateResp := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(enrollmentCreateResp)
		ctx.NotNil(enrollmentCreateResp.Data)
		ctx.NotEmpty(enrollmentCreateResp.Data.ID)

		t.Run("creating second OTT enrollment fails", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
			ctx.NoError(err)
			ctx.Equal(http.StatusConflict, resp.StatusCode(), string(resp.Body()))
		})
	})

	t.Run("can not create an OTT enrollment with invalid identity id", func(t *testing.T) {
		ctx.testContextChanged(t)

		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: S("made up"),
			Method:     S(rest_model.EnrollmentCreateMethodOtt),
			ExpiresAt:  ST(time.Now().Add(1 * time.Hour).UTC()),
		}

		enrollmentCreateResp := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("can not create an OTT enrollment with nil identity id", func(t *testing.T) {
		ctx.testContextChanged(t)

		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: nil,
			Method:     S(rest_model.EnrollmentCreateMethodOtt),
			ExpiresAt:  ST(time.Now().Add(1 * time.Hour).UTC()),
		}

		enrollmentCreateResp := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("can not create an OTT enrollment with an expiresAt in the past", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity := ctx.AdminManagementSession.requireNewIdentity(false)

		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: &identity.Id,
			Method:     S(rest_model.EnrollmentCreateMethodOtt),
			ExpiresAt:  ST(time.Now().Add(-1 * time.Second).UTC()),
		}

		enrollmentCreateResp := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("can not create an OTT enrollment with a nil expiresAt", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity := ctx.AdminManagementSession.requireNewIdentity(false)

		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: &identity.Id,
			Method:     S(rest_model.EnrollmentCreateMethodOtt),
			ExpiresAt:  nil,
		}

		enrollmentCreateResp := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("can create an OTTCA enrollment", func(t *testing.T) {
		ctx.testContextChanged(t)

		testIdentity := ctx.AdminManagementSession.requireNewIdentity(false)
		testCa := newTestCa()

		caCreate := &rest_model.CaCreate{
			CertPem:                   &testCa.certPem,
			IdentityNameFormat:        testCa.identityNameFormat,
			IdentityRoles:             testCa.identityRoles,
			IsAuthEnabled:             &testCa.isAuthEnabled,
			IsAutoCaEnrollmentEnabled: &testCa.isAutoCaEnrollmentEnabled,
			IsOttCaEnrollmentEnabled:  &testCa.isOttCaEnrollmentEnabled,
			Name:                      &testCa.name,
		}

		caCreateResp := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResp).Post("cas/")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(caCreateResp)
		ctx.NotNil(caCreateResp.Data)
		ctx.NotEmpty(caCreateResp.Data.ID)

		caGetResp := &rest_model.DetailCaEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(caGetResp).Get("/cas/" + caCreateResp.Data.ID)
		ctx.NoError(err)
		ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(caGetResp)
		ctx.NotNil(caGetResp.Data)
		ctx.NotNil(caGetResp.Data.ID)
		ctx.NotEmpty(caGetResp.Data.VerificationToken)

		verifyCert, _, err := generateCaSignedClientCert(testCa.publicCert, testCa.privateKey, caGetResp.Data.VerificationToken.String())
		ctx.Req.NoError(err)

		verificationBlock := &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: verifyCert.Raw,
		}
		verifyPem := pem.EncodeToMemory(verificationBlock)

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetHeader("content-type", "text/plain").SetBody(verifyPem).Post("cas/" + caCreateResp.Data.ID + "/verify")
		ctx.Req.NoError(err)
		standardJsonResponseTests(resp, http.StatusOK, t)

		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: &testIdentity.Id,
			CaID:       S(caCreateResp.Data.ID),
			Method:     S(rest_model.EnrollmentCreateMethodOttca),
			ExpiresAt:  ST(time.Now().Add(1 * time.Hour).UTC()),
		}

		enrollmentCreateResp := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(enrollmentCreateResp)
		ctx.NotNil(enrollmentCreateResp.Data)
		ctx.NotEmpty(enrollmentCreateResp.Data.ID)

		t.Run("enrollment has the proper values", func(t *testing.T) {
			ctx.testContextChanged(t)
			enrollmentGetResp := &rest_model.DetailEnrollmentEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(enrollmentGetResp).Get("/enrollments/" + enrollmentCreateResp.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.NotNil(enrollmentGetResp)
			ctx.NotNil(enrollmentGetResp.Data)
			ctx.NotNil(enrollmentGetResp.Data.ID)
			ctx.NotNil(enrollmentGetResp.Data.Method)
			ctx.NotNil(enrollmentGetResp.Data.ExpiresAt)
			ctx.NotNil(enrollmentGetResp.Data.Identity)
			ctx.NotNil(enrollmentGetResp.Data.Token)
			ctx.NotEmpty(*enrollmentGetResp.Data.Token)
			ctx.NotEmpty(enrollmentGetResp.Data.IdentityID)
			ctx.NotEmpty(enrollmentGetResp.Data.JWT)

			ctx.Equal(*enrollmentCreate.Method, *enrollmentGetResp.Data.Method)
			ctx.Equal(enrollmentCreate.ExpiresAt.String(), enrollmentGetResp.Data.ExpiresAt.String())
			ctx.Equal(*enrollmentCreate.IdentityID, enrollmentGetResp.Data.IdentityID)
			ctx.Equal(*enrollmentCreate.CaID, *enrollmentGetResp.Data.CaID)
		})

		t.Run("can enroll", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientAuthenticator := testCa.CreateSignedCert(eid.New())

			ctx.completeOttCaEnrollment(clientAuthenticator)
		})
	})

	t.Run("can not enroll with an expired OTTCA enrollment", func(t *testing.T) {
		ctx.testContextChanged(t)

		testId := ctx.AdminManagementSession.requireNewIdentity(false)
		testCa := newTestCa()

		caCreate := &rest_model.CaCreate{
			CertPem:                   &testCa.certPem,
			IdentityNameFormat:        testCa.identityNameFormat,
			IdentityRoles:             testCa.identityRoles,
			IsAuthEnabled:             &testCa.isAuthEnabled,
			IsAutoCaEnrollmentEnabled: &testCa.isAutoCaEnrollmentEnabled,
			IsOttCaEnrollmentEnabled:  &testCa.isOttCaEnrollmentEnabled,
			Name:                      &testCa.name,
		}

		caCreateResp := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResp).Post("cas/")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(caCreateResp)
		ctx.NotNil(caCreateResp.Data)
		ctx.NotEmpty(caCreateResp.Data.ID)

		caGetResp := &rest_model.DetailCaEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(caGetResp).Get("/cas/" + caCreateResp.Data.ID)
		ctx.NoError(err)
		ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(caGetResp)
		ctx.NotNil(caGetResp.Data)
		ctx.NotNil(caGetResp.Data.ID)
		ctx.NotEmpty(caGetResp.Data.VerificationToken)

		verifyCert, _, err := generateCaSignedClientCert(testCa.publicCert, testCa.privateKey, caGetResp.Data.VerificationToken.String())
		ctx.Req.NoError(err)

		verificationBlock := &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: verifyCert.Raw,
		}
		verifyPem := pem.EncodeToMemory(verificationBlock)

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetHeader("content-type", "text/plain").SetBody(verifyPem).Post("cas/" + caCreateResp.Data.ID + "/verify")
		ctx.Req.NoError(err)
		standardJsonResponseTests(resp, http.StatusOK, t)

		expiresAt := time.Now().Add(5 * time.Second).UTC()
		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: &testId.Id,
			CaID:       S(caCreateResp.Data.ID),
			Method:     S(rest_model.EnrollmentCreateMethodOttca),
			ExpiresAt:  ST(expiresAt),
		}

		enrollmentCreateResp := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(enrollmentCreateResp)
		ctx.NotNil(enrollmentCreateResp.Data)
		ctx.NotEmpty(enrollmentCreateResp.Data.ID)

		t.Run("enrollment has the proper values", func(t *testing.T) {
			ctx.testContextChanged(t)
			enrollmentGetResp := &rest_model.DetailEnrollmentEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(enrollmentGetResp).Get("/enrollments/" + enrollmentCreateResp.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.NotNil(enrollmentGetResp)
			ctx.NotNil(enrollmentGetResp.Data)
			ctx.NotNil(enrollmentGetResp.Data.ID)
			ctx.NotNil(enrollmentGetResp.Data.Method)
			ctx.NotNil(enrollmentGetResp.Data.ExpiresAt)
			ctx.NotNil(enrollmentGetResp.Data.Identity)
			ctx.NotNil(enrollmentGetResp.Data.Token)
			ctx.NotEmpty(*enrollmentGetResp.Data.Token)
			ctx.NotEmpty(enrollmentGetResp.Data.IdentityID)
			ctx.NotEmpty(enrollmentGetResp.Data.JWT)

			ctx.Equal(*enrollmentCreate.Method, *enrollmentGetResp.Data.Method)
			ctx.Equal(enrollmentCreate.ExpiresAt.String(), enrollmentGetResp.Data.ExpiresAt.String())
			ctx.Equal(*enrollmentCreate.IdentityID, enrollmentGetResp.Data.IdentityID)
			ctx.Equal(*enrollmentCreate.CaID, *enrollmentGetResp.Data.CaID)

			t.Run("can not enroll", func(t *testing.T) {
				ctx.testContextChanged(t)

				clientAuthenticator := testCa.CreateSignedCert(eid.New())

				ctx.completeOttCaEnrollment(clientAuthenticator)

				trans := ctx.NewTransport()
				trans.TLSClientConfig.Certificates = []cryptoTls.Certificate{
					{
						Certificate: [][]byte{clientAuthenticator.cert.Raw},
						PrivateKey:  clientAuthenticator.key,
					},
				}
				client := resty.NewWithClient(ctx.NewHttpClient(trans))
				client.SetHostURL("https://" + ctx.ApiHost + EdgeClientApiPath)

				duration := time.Until(expiresAt)

				if duration > 0 {
					time.Sleep(duration)
				}

				enrollResp, err := client.NewRequest().
					SetBody("{}").
					SetHeader("content-type", "application/x-pem-file").
					Post("enroll?method=ottca&token=" + *enrollmentGetResp.Data.Token)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusBadRequest, enrollResp.StatusCode())
			})
		})

	})

	t.Run("can not create two OTTCA enrollments", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity := ctx.AdminManagementSession.requireNewIdentity(false)
		ca := newTestCa()

		caCreate := &rest_model.CaCreate{
			CertPem:                   &ca.certPem,
			IdentityNameFormat:        ca.identityNameFormat,
			IdentityRoles:             ca.identityRoles,
			IsAuthEnabled:             &ca.isAuthEnabled,
			IsAutoCaEnrollmentEnabled: &ca.isAutoCaEnrollmentEnabled,
			IsOttCaEnrollmentEnabled:  &ca.isOttCaEnrollmentEnabled,
			Name:                      &ca.name,
		}

		caCreateResp := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResp).Post("cas/")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(caCreateResp)
		ctx.NotNil(caCreateResp.Data)
		ctx.NotEmpty(caCreateResp.Data.ID)

		caGetResp := &rest_model.DetailCaEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(caGetResp).Get("/cas/" + caCreateResp.Data.ID)
		ctx.NoError(err)
		ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(caGetResp)
		ctx.NotNil(caGetResp.Data)
		ctx.NotNil(caGetResp.Data.ID)
		ctx.NotEmpty(caGetResp.Data.VerificationToken)

		verifyCert, _, err := generateCaSignedClientCert(ca.publicCert, ca.privateKey, caGetResp.Data.VerificationToken.String())
		ctx.Req.NoError(err)

		verificationBlock := &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: verifyCert.Raw,
		}
		verifyPem := pem.EncodeToMemory(verificationBlock)

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetHeader("content-type", "text/plain").SetBody(verifyPem).Post("cas/" + caCreateResp.Data.ID + "/verify")
		ctx.Req.NoError(err)
		standardJsonResponseTests(resp, http.StatusOK, t)

		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: &identity.Id,
			CaID:       S(caCreateResp.Data.ID),
			Method:     S(rest_model.EnrollmentCreateMethodOttca),
			ExpiresAt:  ST(time.Now().Add(1 * time.Hour).UTC()),
		}

		enrollmentCreateResp := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).SetResult(enrollmentCreateResp).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(enrollmentCreateResp)
		ctx.NotNil(enrollmentCreateResp.Data)
		ctx.NotEmpty(enrollmentCreateResp.Data.ID)

		t.Run("creating second OTTCA enrollment fails", func(t *testing.T) {
			enrollmentCreate := &rest_model.EnrollmentCreate{
				IdentityID: &identity.Id,
				CaID:       S(caCreateResp.Data.ID),
				Method:     S(rest_model.EnrollmentCreateMethodOttca),
				ExpiresAt:  ST(time.Now().Add(2 * time.Hour).UTC()),
			}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).Post("/enrollments")
			ctx.NoError(err)
			ctx.Equal(http.StatusConflict, resp.StatusCode(), string(resp.Body()))
		})
	})

	t.Run("can not create an OTTCA enrollment with an invalid caId", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity := ctx.AdminManagementSession.requireNewIdentity(false)

		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: &identity.Id,
			CaID:       S("invalid"),
			Method:     S(rest_model.EnrollmentCreateMethodOttca),
			ExpiresAt:  ST(time.Now().Add(1 * time.Hour).UTC()),
		}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("can not create an OTTCA enrollment with a nil caId", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity := ctx.AdminManagementSession.requireNewIdentity(false)

		enrollmentCreate := &rest_model.EnrollmentCreate{
			IdentityID: &identity.Id,
			CaID:       nil,
			Method:     S(rest_model.EnrollmentCreateMethodOttca),
			ExpiresAt:  ST(time.Now().Add(1 * time.Hour).UTC()),
		}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(enrollmentCreate).Post("/enrollments")
		ctx.NoError(err)
		ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
	})
}
