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
	"github.com/openziti/edge-api/rest_model"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/common/eid"
	"testing"
	"time"
)

// Test_EnrollmentOttSpecific uses the /enroll/ott specific endpoint.
func Test_EnrollmentOttSpecific(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("can create an OTT enrollment", func(t *testing.T) {
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

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		newEnrollmentExpiresAt := time.Now().Add(10 * time.Minute).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOtt(newIdentity.ID, &newEnrollmentExpiresAt)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newEnrollmentLoc)

		newEnrollment, err := managementApiClient.GetEnrollment(newEnrollmentLoc.ID)
		ctx.NoError(err)
		ctx.NotNil(newEnrollment)

		t.Run("enrollment has the proper values", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.NotNil(newEnrollment.ID)
			ctx.NotNil(newEnrollment.Method)
			ctx.NotNil(newEnrollment.ExpiresAt)
			ctx.NotNil(newEnrollment.Identity)
			ctx.NotNil(newEnrollment.Token)
			ctx.NotEmpty(*newEnrollment.Token)
			ctx.NotEmpty(newEnrollment.IdentityID)
			ctx.NotEmpty(newEnrollment.JWT)

			ctx.Equal(rest_model.EnrollmentCreateMethodOtt, *newEnrollment.Method)

			//API time date format conversions reduce fidelity, truncate to MS comparison
			ctx.Equal(newEnrollmentExpiresAt.Truncate(time.Millisecond), time.Time(*newEnrollment.ExpiresAt).Truncate(time.Millisecond))
			ctx.Equal(newIdentityLoc.ID, newEnrollment.IdentityID)
		})

		t.Run("can enroll", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApiClient := ctx.NewEdgeClientApi(nil)

			newIdentityCertAuth, err := clientApiClient.CompleteOttEnrollment(*newEnrollment.Token)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(newIdentityCertAuth)
		})
	})

	t.Run("can not enroll with an expired enrollment", func(t *testing.T) {
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

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		newEnrollmentExpiresAt := time.Now().Add(2 * time.Second).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOtt(newIdentity.ID, &newEnrollmentExpiresAt)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newEnrollmentLoc)

		newEnrollment, err := managementApiClient.GetEnrollment(newEnrollmentLoc.ID)
		ctx.NoError(err)
		ctx.NotNil(newEnrollment)

		t.Run("enrollment has the proper values", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.NotNil(newEnrollment.ID)
			ctx.NotNil(newEnrollment.Method)
			ctx.NotNil(newEnrollment.ExpiresAt)
			ctx.NotNil(newEnrollment.Identity)
			ctx.NotNil(newEnrollment.Token)
			ctx.NotEmpty(*newEnrollment.Token)
			ctx.NotEmpty(newEnrollment.IdentityID)
			ctx.NotEmpty(newEnrollment.JWT)

			ctx.Equal(rest_model.EnrollmentCreateMethodOtt, *newEnrollment.Method)

			//API time date format conversions reduce fidelity, truncate to MS comparison
			ctx.Equal(newEnrollmentExpiresAt.Truncate(time.Millisecond), time.Time(*newEnrollment.ExpiresAt).Truncate(time.Millisecond))
			ctx.Equal(newIdentityLoc.ID, newEnrollment.IdentityID)
		})

		t.Run("can not enroll", func(t *testing.T) {
			ctx.testContextChanged(t)

			waitDuration := time.Until(newEnrollmentExpiresAt)

			if waitDuration > 0 {
				time.Sleep(waitDuration)
			}

			clientApiClient := ctx.NewEdgeClientApi(nil)

			newIdentityCertAuth, err := clientApiClient.CompleteOttEnrollment(*newEnrollment.Token)
			ctx.Req.Error(err)
			ctx.Req.Nil(newIdentityCertAuth)
		})
	})

	t.Run("can not create two OTT enrollments", func(t *testing.T) {
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

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		newEnrollmentExpiresAt := time.Now().Add(5 * time.Second).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOtt(newIdentity.ID, &newEnrollmentExpiresAt)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newEnrollmentLoc)

		newEnrollment, err := managementApiClient.GetEnrollment(newEnrollmentLoc.ID)
		ctx.NoError(err)
		ctx.NotNil(newEnrollment)

		t.Run("creating second OTT enrollment fails", func(t *testing.T) {
			ctx.testContextChanged(t)
			secondEnrollmentExpiresAt := time.Now().Add(5 * time.Second).UTC()
			secondEnrollmentLoc, err := managementApiClient.CreateEnrollmentOtt(newIdentity.ID, &secondEnrollmentExpiresAt)
			ctx.Req.Error(err)
			ctx.Req.Nil(secondEnrollmentLoc)
		})
	})

	t.Run("can not create an OTT enrollment with invalid identity id", func(t *testing.T) {
		ctx.testContextChanged(t)

		managementApiClient := ctx.NewEdgeManagementApi(nil)
		ctx.Req.NotNil(managementApiClient)

		adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
		adminManApiSession, err := managementApiClient.Authenticate(adminCreds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminManApiSession)

		newEnrollmentExpiresAt := time.Now().Add(5 * time.Second).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOtt(ToPtr("i-do-not-exist"), &newEnrollmentExpiresAt)
		ctx.Req.Error(err)
		ctx.Req.Nil(newEnrollmentLoc)
	})

	t.Run("can not create an OTT enrollment with nil identity id", func(t *testing.T) {
		ctx.testContextChanged(t)

		managementApiClient := ctx.NewEdgeManagementApi(nil)
		ctx.Req.NotNil(managementApiClient)

		adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
		adminManApiSession, err := managementApiClient.Authenticate(adminCreds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminManApiSession)

		newEnrollmentExpiresAt := time.Now().Add(5 * time.Second).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOtt(nil, &newEnrollmentExpiresAt)
		ctx.Req.Error(err)
		ctx.Req.Nil(newEnrollmentLoc)
	})

	t.Run("can not create an OTT enrollment with an expiresAt in the past", func(t *testing.T) {
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

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		newEnrollmentExpiresAt := time.Now().Add(-5 * time.Hour).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOtt(newIdentity.ID, &newEnrollmentExpiresAt)
		ctx.Req.Error(err)
		ctx.Req.Nil(newEnrollmentLoc)
	})

	t.Run("can not create an OTT enrollment with a nil expiresAt", func(t *testing.T) {
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

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOtt(newIdentity.ID, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(newEnrollmentLoc)
	})

	t.Run("can create an OTTCA enrollment", func(t *testing.T) {
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

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		testCa := newTestCa()

		newCaLoc, err := managementApiClient.CreateCa(testCa)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newCaLoc)
		ctx.Req.NotEmpty(newCaLoc.ID)

		newCa, err := managementApiClient.GetCa(newCaLoc.ID)
		ctx.NoError(err)
		ctx.NotNil(newCa)
		ctx.NotEmpty(newCa.VerificationToken)

		err = managementApiClient.VerifyCa(*newCa.ID, newCa.VerificationToken.String(), testCa.publicCert, testCa.privateKey)
		ctx.Req.NoError(err)

		newEnrollmentExpiresAt := time.Now().Add(10 * time.Minute).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOttCa(newIdentity.ID, newCa.ID, &newEnrollmentExpiresAt)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newEnrollmentLoc)

		newEnrollment, err := managementApiClient.GetEnrollment(newEnrollmentLoc.ID)
		ctx.NoError(err)
		ctx.NotNil(newEnrollment)

		t.Run("enrollment has the proper values", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.NotNil(newEnrollment.ID)
			ctx.NotNil(newEnrollment.Method)
			ctx.NotNil(newEnrollment.ExpiresAt)
			ctx.NotNil(newEnrollment.Identity)
			ctx.NotNil(newEnrollment.Token)
			ctx.NotNil(newEnrollment.CaID)
			ctx.NotEmpty(*newEnrollment.Token)
			ctx.NotEmpty(newEnrollment.IdentityID)
			ctx.NotEmpty(newEnrollment.JWT)

			ctx.Equal(rest_model.EnrollmentCreateMethodOttca, *newEnrollment.Method)

			//API time date format conversions reduce fidelity, truncate to MS comparison
			ctx.Equal(newEnrollmentExpiresAt.Truncate(time.Millisecond), time.Time(*newEnrollment.ExpiresAt).Truncate(time.Millisecond))
			ctx.Equal(newIdentityLoc.ID, newEnrollment.IdentityID)
			ctx.Equal(newCaLoc.ID, *newEnrollment.CaID)
		})

		t.Run("can enroll", func(t *testing.T) {
			ctx.testContextChanged(t)

			clientApiClient := ctx.NewEdgeClientApi(nil)

			certAuth := testCa.CreateSignedCert(eid.New())

			_, err := clientApiClient.CompleteOttCaEnrollment(*newEnrollment.Token, certAuth.certs, certAuth.key)
			ctx.Req.NoError(err)

		})
	})

	t.Run("can not enroll with an expired OTTCA enrollment", func(t *testing.T) {
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

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		testCa := newTestCa()

		newCaLoc, err := managementApiClient.CreateCa(testCa)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newCaLoc)
		ctx.Req.NotEmpty(newCaLoc.ID)

		newCa, err := managementApiClient.GetCa(newCaLoc.ID)
		ctx.NoError(err)
		ctx.NotNil(newCa)
		ctx.NotEmpty(newCa.VerificationToken)

		err = managementApiClient.VerifyCa(*newCa.ID, newCa.VerificationToken.String(), testCa.publicCert, testCa.privateKey)
		ctx.Req.NoError(err)

		newEnrollmentExpiresAt := time.Now().Add(5 * time.Second).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOttCa(newIdentity.ID, newCa.ID, &newEnrollmentExpiresAt)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newEnrollmentLoc)

		newEnrollment, err := managementApiClient.GetEnrollment(newEnrollmentLoc.ID)
		ctx.NoError(err)
		ctx.NotNil(newEnrollment)

		t.Run("can not enroll", func(t *testing.T) {
			ctx.testContextChanged(t)

			duration := time.Until(newEnrollmentExpiresAt)

			if duration > 0 {
				time.Sleep(duration)
			}

			ctx.testContextChanged(t)

			clientApiClient := ctx.NewEdgeClientApi(nil)

			certAuth := testCa.CreateSignedCert(eid.New())

			_, err := clientApiClient.CompleteOttCaEnrollment(*newEnrollment.Token, certAuth.certs, certAuth.key)
			ctx.Req.Error(err)
		})
	})

	t.Run("can not create two OTTCA enrollments", func(t *testing.T) {
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

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		testCa := newTestCa()

		newCaLoc, err := managementApiClient.CreateCa(testCa)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newCaLoc)
		ctx.Req.NotEmpty(newCaLoc.ID)

		newCa, err := managementApiClient.GetCa(newCaLoc.ID)
		ctx.NoError(err)
		ctx.NotNil(newCa)
		ctx.NotEmpty(newCa.VerificationToken)

		err = managementApiClient.VerifyCa(*newCa.ID, newCa.VerificationToken.String(), testCa.publicCert, testCa.privateKey)
		ctx.Req.NoError(err)

		newEnrollmentExpiresAt := time.Now().Add(5 * time.Hour).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOttCa(newIdentity.ID, newCa.ID, &newEnrollmentExpiresAt)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newEnrollmentLoc)

		t.Run("creating second OTTCA enrollment fails", func(t *testing.T) {
			ctx.testContextChanged(t)

			secondEnrollmentExpiresAt := time.Now().Add(5 * time.Hour).UTC()
			secondEnrollmentLoc, err := managementApiClient.CreateEnrollmentOttCa(newIdentity.ID, newCa.ID, &secondEnrollmentExpiresAt)
			ctx.Req.Error(err)
			ctx.Req.Nil(secondEnrollmentLoc)
		})
	})

	t.Run("can not create an OTTCA enrollment with an invalid caId", func(t *testing.T) {
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

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		newEnrollmentExpiresAt := time.Now().Add(5 * time.Hour).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOttCa(newIdentity.ID, ToPtr("i-do-not-exist"), &newEnrollmentExpiresAt)
		ctx.Req.Error(err)
		ctx.Req.Nil(newEnrollmentLoc)
	})

	t.Run("can not create an OTTCA enrollment with a nil caId", func(t *testing.T) {
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

		newIdentity, err := managementApiClient.GetIdentity(newIdentityLoc.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(newIdentity)

		newEnrollmentExpiresAt := time.Now().Add(5 * time.Hour).UTC()
		newEnrollmentLoc, err := managementApiClient.CreateEnrollmentOttCa(newIdentity.ID, nil, &newEnrollmentExpiresAt)
		ctx.Req.Error(err)
		ctx.Req.Nil(newEnrollmentLoc)
	})
}
