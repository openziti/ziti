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
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/common/eid"
	"testing"
	"time"
)

// Test_EnrollmentOttSpecific uses the /enroll/updb specific endpoint.
func Test_EnrollmentUpdbSpecific(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("a updb enrolling identity can be created", func(t *testing.T) {
		ctx.testContextChanged(t)

		managementApiClient := ctx.NewEdgeManagementApi(nil)
		ctx.Req.NotNil(managementApiClient)

		adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
		adminManApiSession, err := managementApiClient.Authenticate(adminCreds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminManApiSession)

		newIdentityLoc, err := managementApiClient.CreateIdentity(eid.New(), false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentityLoc)
		ctx.Req.NotEmpty(newIdentityLoc.ID)

		newEnrollmentExpiresAt := time.Now().Add(10 * time.Minute).UTC()
		username := eid.New()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentUpdb(&newIdentityLoc.ID, &username, &newEnrollmentExpiresAt)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newEnrollmentLoc)

		newEnrollment, err := managementApiClient.GetEnrollment(newEnrollmentLoc.ID)
		ctx.NoError(err)
		ctx.NotNil(newEnrollment)

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		t.Run("created updb identity has a UPDB JWT", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.Req.NotNil(newIdentity.Enrollment)
			ctx.Req.NotNil(newIdentity.Enrollment.Updb)
			ctx.Req.NotEmpty(newIdentity.Enrollment.Updb.JWT)
			ctx.Req.NotEmpty(newIdentity.Enrollment.Updb.Token)

			t.Run("updb JWT can enroll", func(t *testing.T) {
				ctx.testContextChanged(t)

				clientApiClient := ctx.NewEdgeClientApi(nil)

				password := eid.New()

				newIdentityUpdbAuth, err := clientApiClient.CompleteUpdbEnrollment(*newEnrollment.Token, username, password)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(newIdentityUpdbAuth)

				t.Run("can authenticate after updb enrollment", func(t *testing.T) {
					ctx.testContextChanged(t)

					apiSesion, err := clientApiClient.Authenticate(newIdentityUpdbAuth, nil)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(apiSesion)

				})
			})
		})
	})
}
