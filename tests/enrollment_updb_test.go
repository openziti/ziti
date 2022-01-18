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
	"github.com/Jeffail/gabs"
	"github.com/google/uuid"
	"net/http"
	"testing"
)

func Test_UpdbEnrollment(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("a updb enrolling identity can be created", func(t *testing.T) {
		ctx.testContextChanged(t)

		updbCreate := gabs.New()

		updbName := uuid.New().String()
		updbUsername := uuid.New().String()
		updbPassword := uuid.New().String()
		updbType := "User"

		updbCreate.Set(updbName, "name")
		updbCreate.Set(updbType, "type")
		updbCreate.Set(map[string]string{
			"updb": updbUsername,
		}, "enrollment")
		updbCreate.Set(false, "isAdmin")

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(updbCreate.String()).Post("identities")
		ctx.Req.NoError(err)

		standardJsonResponseTests(resp, http.StatusCreated, t)

		createdResp, err := gabs.ParseJSON(resp.Body())
		ctx.Req.NoError(err)

		ctx.Req.True(createdResp.ExistsP("data.id"), "created response should have an data.id field")

		updbId, ok := createdResp.Path("data.id").Data().(string)
		ctx.Req.True(ok, "created response should have a string data.id")
		ctx.Req.NotEmpty(updbId)

		t.Run("created updb identity has a UPDB JWT", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("identities/" + updbId)
			ctx.Req.NoError(err)

			standardJsonResponseTests(resp, http.StatusOK, t)

			updbIdentity, err := gabs.ParseJSON(resp.Body())
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(updbIdentity)

			ctx.Req.True(updbIdentity.ExistsP("data.enrollment"), "expected data.enrollment to exist")
			ctx.Req.True(updbIdentity.ExistsP("data.enrollment.updb"), "expected data.enrollment.updb to exist")
			ctx.Req.True(updbIdentity.ExistsP("data.enrollment.updb.jwt"), "expected data.enrollment.updb.jwt to exist")
			ctx.Req.True(updbIdentity.ExistsP("data.enrollment.updb.token"), "expected data.enrollment.updb.token to exist")

			updbJwt, ok := updbIdentity.Path("data.enrollment.updb.jwt").Data().(string)
			ctx.Req.True(ok, "expected data.enrollment.updb.jwt to be a string")
			ctx.Req.NotEmpty(updbJwt, "expected data.enrollment.updb.jwt to be a non-empty string")

			updbEnrollmentToken, ok := updbIdentity.Path("data.enrollment.updb.token").Data().(string)
			ctx.Req.True(ok, "expected data.enrollment.updb.token to be a string")
			ctx.Req.NotEmpty(updbEnrollmentToken, "expected data.enrollment.updb.token to be a non-empty string")

			t.Run("updb JWT can enroll", func(t *testing.T) {
				ctx.testContextChanged(t)

				enrollmentBody := gabs.New()

				enrollmentBody.Set(updbPassword, "password")

				resp, err := ctx.newAnonymousClientApiRequest().SetBody(enrollmentBody.String()).Post("enroll?method=updb&token=" + updbEnrollmentToken)
				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				t.Run("enrolled updb identity can authenticate", func(t *testing.T) {
					ctx.testContextChanged(t)

					updbAuth := updbAuthenticator{
						Username:    updbUsername,
						Password:    updbPassword,
						ConfigTypes: nil,
					}

					updbApiSession, err := updbAuth.AuthenticateClientApi(ctx)
					ctx.Req.NoError(err)
					ctx.Req.NotEmpty(updbApiSession)
					ctx.Req.NotEmpty(updbApiSession.token)

					t.Run("authenticated updb api session can query current api session", func(t *testing.T) {
						ctx.testContextChanged(t)

						resp, err := updbApiSession.newAuthenticatedRequest().Get("current-api-session")
						ctx.Req.NoError(err)

						standardJsonResponseTests(resp, http.StatusOK, t)
					})
				})
			})
		})
	})
}
