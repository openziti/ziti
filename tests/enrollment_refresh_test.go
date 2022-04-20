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
	"github.com/google/uuid"
	"github.com/openziti/edge/rest_model"
	"net/http"
	"testing"
	"time"
)

func Test_EnrollmentRefresh(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("an existing enrollment", func(t *testing.T) {
		ctx.testContextChanged(t)

		idType := rest_model.IdentityTypeUser
		identityPost := &rest_model.IdentityCreate{
			Enrollment: &rest_model.IdentityCreateEnrollment{
				Ott: true,
			},
			Name:    S(uuid.NewString()),
			IsAdmin: B(false),
			Type:    &idType,
		}

		identityCreateEnv := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(identityCreateEnv).SetBody(identityPost).Post("/identities")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(identityCreateEnv)
		ctx.NotNil(identityCreateEnv.Data)
		ctx.NotEmpty(identityCreateEnv.Data.ID)

		origIdentityGetEnv := &rest_model.DetailIdentityEnvelope{}
		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(origIdentityGetEnv).Get("/identities/" + identityCreateEnv.Data.ID)
		ctx.NoError(err)
		ctx.Equal(http.StatusOK, resp.StatusCode())
		ctx.NotNil(origIdentityGetEnv)
		ctx.NotNil(origIdentityGetEnv.Data)
		ctx.NotEmpty(origIdentityGetEnv.Data.ID)
		ctx.NotEmpty(origIdentityGetEnv.Data.Enrollment)
		ctx.NotEmpty(origIdentityGetEnv.Data.Enrollment.Ott)
		ctx.NotEmpty(origIdentityGetEnv.Data.Enrollment.Ott.ID)
		ctx.NotEmpty(origIdentityGetEnv.Data.Enrollment.Ott.JWT)

		t.Run("can not be refreshed with an invalid expiresAt", func(t *testing.T) {
			ctx.testContextChanged(t)

			refreshPost := &rest_model.EnrollmentRefresh{
				ExpiresAt: ST(time.Now().Add(-1 * time.Hour).UTC()),
			}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(refreshPost).Post("/enrollments/" + origIdentityGetEnv.Data.Enrollment.Ott.ID + "/refresh")
			ctx.NoError(err)
			ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))

			verifydIdentityGetEnv := &rest_model.DetailIdentityEnvelope{}
			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(verifydIdentityGetEnv).Get("/identities/" + identityCreateEnv.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode())
			ctx.NotNil(verifydIdentityGetEnv)
			ctx.NotNil(verifydIdentityGetEnv.Data)
			ctx.NotEmpty(verifydIdentityGetEnv.Data.ID)
			ctx.NotEmpty(verifydIdentityGetEnv.Data.Enrollment)
			ctx.NotEmpty(verifydIdentityGetEnv.Data.Enrollment.Ott)
			ctx.NotEmpty(verifydIdentityGetEnv.Data.Enrollment.Ott.ID)
			ctx.NotEmpty(verifydIdentityGetEnv.Data.Enrollment.Ott.JWT)

			ctx.Req.Equal(origIdentityGetEnv.Data.Enrollment.Ott.JWT, verifydIdentityGetEnv.Data.Enrollment.Ott.JWT)
			ctx.Req.Equal(origIdentityGetEnv.Data.Enrollment.Ott.ExpiresAt.String(), verifydIdentityGetEnv.Data.Enrollment.Ott.ExpiresAt.String())
		})

		t.Run("can not be refreshed with a missing expiresAt", func(t *testing.T) {
			ctx.testContextChanged(t)

			refreshPost := &rest_model.EnrollmentRefresh{
				ExpiresAt: nil,
			}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(refreshPost).Post("/enrollments/" + origIdentityGetEnv.Data.Enrollment.Ott.ID + "/refresh")
			ctx.NoError(err)
			ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))

			verifydIdentityGetEnv := &rest_model.DetailIdentityEnvelope{}
			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(verifydIdentityGetEnv).Get("/identities/" + identityCreateEnv.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode())
			ctx.NotNil(verifydIdentityGetEnv)
			ctx.NotNil(verifydIdentityGetEnv.Data)
			ctx.NotEmpty(verifydIdentityGetEnv.Data.ID)
			ctx.NotEmpty(verifydIdentityGetEnv.Data.Enrollment)
			ctx.NotEmpty(verifydIdentityGetEnv.Data.Enrollment.Ott)
			ctx.NotEmpty(verifydIdentityGetEnv.Data.Enrollment.Ott.ID)
			ctx.NotEmpty(verifydIdentityGetEnv.Data.Enrollment.Ott.JWT)

			ctx.Req.Equal(origIdentityGetEnv.Data.Enrollment.Ott.JWT, verifydIdentityGetEnv.Data.Enrollment.Ott.JWT)
			ctx.Req.Equal(origIdentityGetEnv.Data.Enrollment.Ott.ExpiresAt.String(), verifydIdentityGetEnv.Data.Enrollment.Ott.ExpiresAt.String())
		})

		t.Run("can be refreshed with a valid expiresAt", func(t *testing.T) {
			ctx.testContextChanged(t)

			refreshPost := &rest_model.EnrollmentRefresh{
				ExpiresAt: ST(time.Now().Add(1 * time.Hour).UTC()),
			}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(refreshPost).Post("/enrollments/" + origIdentityGetEnv.Data.Enrollment.Ott.ID + "/refresh")
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

			updatedIdentityGetEnv := &rest_model.DetailIdentityEnvelope{}
			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(updatedIdentityGetEnv).Get("/identities/" + identityCreateEnv.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode())
			ctx.NotNil(updatedIdentityGetEnv)
			ctx.NotNil(updatedIdentityGetEnv.Data)
			ctx.NotEmpty(updatedIdentityGetEnv.Data.ID)
			ctx.NotEmpty(updatedIdentityGetEnv.Data.Enrollment)
			ctx.NotEmpty(updatedIdentityGetEnv.Data.Enrollment.Ott)
			ctx.NotEmpty(updatedIdentityGetEnv.Data.Enrollment.Ott.ID)
			ctx.NotEmpty(updatedIdentityGetEnv.Data.Enrollment.Ott.JWT)

			ctx.Req.NotEqual(origIdentityGetEnv.Data.Enrollment.Ott.JWT, updatedIdentityGetEnv.Data.Enrollment.Ott.JWT)
			ctx.Req.NotEqual(origIdentityGetEnv.Data.Enrollment.Ott.ExpiresAt.String(), updatedIdentityGetEnv.Data.Enrollment.Ott.ExpiresAt.String())
			ctx.Req.Equal(refreshPost.ExpiresAt.String(), updatedIdentityGetEnv.Data.Enrollment.Ott.ExpiresAt.String())
		})
	})

	t.Run("can not refresh with invalid enrollment id", func(t *testing.T) {
		refreshPost := &rest_model.EnrollmentRefresh{
			ExpiresAt: ST(time.Now().Add(1 * time.Hour).UTC()),
		}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(refreshPost).Post("/enrollments/WE_DO_NOT_EXIST/refresh")
		ctx.NoError(err)
		ctx.Equal(http.StatusNotFound, resp.StatusCode(), string(resp.Body()))
	})

}
