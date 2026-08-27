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
	"fmt"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/eid"
)

// Test_Identity_Admin_Enrollment_Secrets locks in that the identity routes never hand a non-admin
// caller the live enrollment secrets of an admin identity. An enrollment's token and jwt are the
// credentials used to enroll as its target identity: the public enrollment endpoints accept either
// and mint a certificate or password authenticator on that identity, so a caller holding only
// identity or enrollment read access would escalate to admin if it could read them.
//
// The top level /enrollments routes scope this already. These are the identity routes that render
// the same secrets: the identity detail and list, which embed the enrollment, and the identity
// enrollments subresource. Every enrollment method is covered, since each is rendered separately.
func Test_Identity_Admin_Enrollment_Secrets(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminHelper := ctx.NewEdgeManagementApi(nil)
	_, err := adminHelper.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	namePrefix := "aes-" + eid.New() + "-"
	expiresAt := time.Now().Add(time.Hour)

	caLoc, err := adminHelper.CreateOttCaEnabledCa(eid.New())
	ctx.Req.NoError(err)

	adminTarget, err := adminHelper.CreateIdentity(namePrefix+"admin", true)
	ctx.Req.NoError(err)

	userTarget, err := adminHelper.CreateIdentity(namePrefix+"user", false)
	ctx.Req.NoError(err)

	for _, targetId := range []string{adminTarget.ID, userTarget.ID} {
		_, err = adminHelper.CreateEnrollmentOtt(&targetId, &expiresAt)
		ctx.Req.NoError(err)

		_, err = adminHelper.CreateEnrollmentUpdb(&targetId, ToPtr(eid.New()), &expiresAt)
		ctx.Req.NoError(err)

		_, err = adminHelper.CreateEnrollmentOttCa(&targetId, &caLoc.ID, &expiresAt)
		ctx.Req.NoError(err)
	}

	_, identityReadCreds, err := adminHelper.CreateAndEnrollOttIdentityWithPermissions(false, []string{"identity.read"})
	ctx.Req.NoError(err)
	identityReadCreds.CaPool = ctx.ControllerCaPool()

	identityReadHelper := ctx.NewEdgeManagementApi(nil)
	_, err = identityReadHelper.Authenticate(identityReadCreds, nil)
	ctx.Req.NoError(err)

	_, enrollmentReadCreds, err := adminHelper.CreateAndEnrollOttIdentityWithPermissions(false, []string{"enrollment.read"})
	ctx.Req.NoError(err)
	enrollmentReadCreds.CaPool = ctx.ControllerCaPool()

	enrollmentReadHelper := ctx.NewEdgeManagementApi(nil)
	_, err = enrollmentReadHelper.Authenticate(enrollmentReadCreds, nil)
	ctx.Req.NoError(err)

	listFilter := fmt.Sprintf(`name contains "%s" sort by name`, namePrefix)

	t.Run("identity detail withholds an admin identity's enrollment secrets from a non-admin", func(t *testing.T) {
		ctx.testContextChanged(t)

		detail, err := identityReadHelper.GetIdentity(adminTarget.ID)

		ctx.Req.NoError(err)
		ctx.Req.True(*detail.IsAdmin, "the target must be an admin identity for this test to mean anything")

		ctx.Req.NotNil(detail.Enrollment.Ott, "the ott enrollment should still be reported")
		ctx.Req.Empty(detail.Enrollment.Ott.Token, "the ott enrollment token must be withheld")
		ctx.Req.Empty(detail.Enrollment.Ott.JWT, "the ott enrollment jwt must be withheld")

		ctx.Req.NotNil(detail.Enrollment.Updb, "the updb enrollment should still be reported")
		ctx.Req.Empty(detail.Enrollment.Updb.Token, "the updb enrollment token must be withheld")
		ctx.Req.Empty(detail.Enrollment.Updb.JWT, "the updb enrollment jwt must be withheld")

		ctx.Req.NotNil(detail.Enrollment.Ottca, "the ottca enrollment should still be reported")
		ctx.Req.Empty(detail.Enrollment.Ottca.Token, "the ottca enrollment token must be withheld")
		ctx.Req.Empty(detail.Enrollment.Ottca.JWT, "the ottca enrollment jwt must be withheld")
	})

	t.Run("identity detail still returns a non-admin identity's enrollment secrets", func(t *testing.T) {
		ctx.testContextChanged(t)

		detail, err := identityReadHelper.GetIdentity(userTarget.ID)

		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(detail.Enrollment.Ott.Token, "the ott enrollment token should be visible")
		ctx.Req.NotEmpty(detail.Enrollment.Ott.JWT, "the ott enrollment jwt should be visible")
		ctx.Req.NotEmpty(detail.Enrollment.Updb.Token, "the updb enrollment token should be visible")
		ctx.Req.NotEmpty(detail.Enrollment.Updb.JWT, "the updb enrollment jwt should be visible")
		ctx.Req.NotEmpty(detail.Enrollment.Ottca.Token, "the ottca enrollment token should be visible")
		ctx.Req.NotEmpty(detail.Enrollment.Ottca.JWT, "the ottca enrollment jwt should be visible")
	})

	t.Run("identity detail returns an admin identity's enrollment secrets to an admin", func(t *testing.T) {
		ctx.testContextChanged(t)

		detail, err := adminHelper.GetIdentity(adminTarget.ID)

		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(detail.Enrollment.Ott.Token, "the ott enrollment token should be visible")
		ctx.Req.NotEmpty(detail.Enrollment.Ott.JWT, "the ott enrollment jwt should be visible")
		ctx.Req.NotEmpty(detail.Enrollment.Updb.Token, "the updb enrollment token should be visible")
		ctx.Req.NotEmpty(detail.Enrollment.Updb.JWT, "the updb enrollment jwt should be visible")
		ctx.Req.NotEmpty(detail.Enrollment.Ottca.Token, "the ottca enrollment token should be visible")
		ctx.Req.NotEmpty(detail.Enrollment.Ottca.JWT, "the ottca enrollment jwt should be visible")
	})

	t.Run("identity list withholds an admin identity's enrollment secrets from a non-admin", func(t *testing.T) {
		ctx.testContextChanged(t)

		identities, err := identityReadHelper.ListIdentitiesByFilter(listFilter)

		ctx.Req.NoError(err)
		ctx.Req.Len(identities, 2, "the filter should match exactly the admin and non-admin targets")
		ctx.Req.Equal(adminTarget.ID, *identities[0].ID, "sorted by name, the admin target comes first")

		ctx.Req.Empty(identities[0].Enrollment.Ott.Token, "the ott enrollment token must be withheld")
		ctx.Req.Empty(identities[0].Enrollment.Ott.JWT, "the ott enrollment jwt must be withheld")
		ctx.Req.Empty(identities[0].Enrollment.Updb.Token, "the updb enrollment token must be withheld")
		ctx.Req.Empty(identities[0].Enrollment.Updb.JWT, "the updb enrollment jwt must be withheld")
		ctx.Req.Empty(identities[0].Enrollment.Ottca.Token, "the ottca enrollment token must be withheld")
		ctx.Req.Empty(identities[0].Enrollment.Ottca.JWT, "the ottca enrollment jwt must be withheld")

		ctx.Req.NotEmpty(identities[1].Enrollment.Ott.Token, "the non-admin target's secrets stay visible")
	})

	t.Run("identity list returns an admin identity's enrollment secrets to an admin", func(t *testing.T) {
		ctx.testContextChanged(t)

		identities, err := adminHelper.ListIdentitiesByFilter(listFilter)

		ctx.Req.NoError(err)
		ctx.Req.Len(identities, 2, "the filter should match exactly the admin and non-admin targets")
		ctx.Req.Equal(adminTarget.ID, *identities[0].ID, "sorted by name, the admin target comes first")

		ctx.Req.NotEmpty(identities[0].Enrollment.Ott.Token, "the ott enrollment token should be visible")
		ctx.Req.NotEmpty(identities[0].Enrollment.Ott.JWT, "the ott enrollment jwt should be visible")
		ctx.Req.NotEmpty(identities[0].Enrollment.Updb.Token, "the updb enrollment token should be visible")
		ctx.Req.NotEmpty(identities[0].Enrollment.Updb.JWT, "the updb enrollment jwt should be visible")
		ctx.Req.NotEmpty(identities[0].Enrollment.Ottca.Token, "the ottca enrollment token should be visible")
		ctx.Req.NotEmpty(identities[0].Enrollment.Ottca.JWT, "the ottca enrollment jwt should be visible")
	})

	t.Run("identity enrollments subresource excludes an admin identity's enrollments", func(t *testing.T) {
		ctx.testContextChanged(t)

		enrollments, err := enrollmentReadHelper.ListIdentityEnrollments(adminTarget.ID)

		ctx.Req.NoError(err)
		ctx.Req.Empty(enrollments, "a non-admin must not see any enrollment of an admin identity")
	})

	t.Run("identity enrollments subresource still returns a non-admin identity's enrollments", func(t *testing.T) {
		ctx.testContextChanged(t)

		enrollments, err := enrollmentReadHelper.ListIdentityEnrollments(userTarget.ID)

		ctx.Req.NoError(err)
		ctx.Req.Len(enrollments, 3, "all three enrollment methods should be listed")
		for _, enrollment := range enrollments {
			ctx.Req.NotEmpty(*enrollment.Token, "a non-admin identity's enrollment token stays visible")
			ctx.Req.NotEmpty(enrollment.JWT, "a non-admin identity's enrollment jwt stays visible")
		}
	})

	t.Run("identity enrollments subresource returns an admin identity's enrollments to an admin", func(t *testing.T) {
		ctx.testContextChanged(t)

		enrollments, err := adminHelper.ListIdentityEnrollments(adminTarget.ID)

		ctx.Req.NoError(err)
		ctx.Req.Len(enrollments, 3, "all three enrollment methods should be listed")
		for _, enrollment := range enrollments {
			ctx.Req.NotEmpty(*enrollment.Token, "an admin caller sees the enrollment token")
			ctx.Req.NotEmpty(enrollment.JWT, "an admin caller sees the enrollment jwt")
		}
	})
}
