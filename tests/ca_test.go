// +build apitests

package tests

import (
	"net/http"
	"sort"
	"testing"

	"github.com/google/uuid"
)

func Test_CA(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	t.Run("identity attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		ca := newTestCa(role1, role2)
		ca.id = ctx.AdminSession.requireCreateEntity(ca)
		ctx.AdminSession.validateEntityWithQuery(ca)
		ctx.AdminSession.validateEntityWithLookup(ca)
	})

	t.Run("identity attributes should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		ca := newTestCa(role1, role2)
		ca.id = ctx.AdminSession.requireCreateEntity(ca)

		role3 := uuid.New().String()
		ca.identityRoles = []string{role2, role3}
		ctx.AdminSession.requireUpdateEntity(ca)
		ctx.AdminSession.validateEntityWithLookup(ca)
	})

	t.Run("identities from auto enrollment inherit CA identity roles", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		ca := newTestCa(role1, role2)
		ca.id = ctx.AdminSession.requireCreateEntity(ca)

		caValues := ctx.AdminSession.requireQuery("cas/" + ca.id)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)

		ctx.req.NotEmpty(verificationToken)

		validationAuth := ca.CreateSignedCert(verificationToken)
		

		clientAuthenticator := ca.CreateSignedCert(uuid.New().String())

		resp, err := ctx.AdminSession.newAuthenticatedRequest().
			SetHeader("content-type", "text/plain").
			SetBody(validationAuth.certPem).
			Post("cas/" + ca.id + "/verify")

		ctx.req.NoError(err)
		ctx.logJson(resp.Body())
		ctx.req.Equal(http.StatusOK, resp.StatusCode())

		ctx.completeCaAutoEnrollment(clientAuthenticator)

		enrolledSession, err := clientAuthenticator.Authenticate(ctx)

		ctx.req.NoError(err)

		identity := ctx.AdminSession.requireQuery("identities/" + enrolledSession.identityId)
		sort.Strings(ca.identityRoles)
		ctx.pathEqualsStringSlice(identity, ca.identityRoles, path("data", "roleAttributes"))

	})
}
