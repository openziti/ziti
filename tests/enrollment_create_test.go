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
	"encoding/pem"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/common/eid"
	"net/http"
	"testing"
	"time"
)

func Test_EnrollmentCreate(t *testing.T) {
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

			clientAuthenticator := ca.CreateSignedCert(eid.New())

			ctx.completeOttCaEnrollment(clientAuthenticator)
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
