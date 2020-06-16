// +build apitests

package tests

import (
	"fmt"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/eid"
	"net/http"
	"sort"
	"testing"
)

func Test_CA(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	t.Run("identity attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		ca := newTestCa(role1, role2)
		ca.id = ctx.AdminSession.requireCreateEntity(ca)
		ctx.AdminSession.validateEntityWithQuery(ca)
		ctx.AdminSession.validateEntityWithLookup(ca)
	})

	t.Run("identity attributes should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		ca := newTestCa(role1, role2)
		ca.id = ctx.AdminSession.requireCreateEntity(ca)

		role3 := eid.New()
		ca.identityRoles = []string{role2, role3}
		ctx.AdminSession.requireUpdateEntity(ca)
		ctx.AdminSession.validateEntityWithLookup(ca)
	})

	t.Run("identity name format should default if not specified", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		ca := newTestCa(role1, role2)

		ca.identityNameFormat = ""
		ca.id = ctx.AdminSession.requireCreateEntity(ca)

		//set to default for verification
		ca.identityNameFormat = model.DefaultCaIdentityNameFormat

		ctx.AdminSession.validateEntityWithQuery(ca)
		ctx.AdminSession.validateEntityWithLookup(ca)
	})

	t.Run("identities from auto enrollment inherit CA identity roles", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		ca := newTestCa(role1, role2)
		ca.id = ctx.AdminSession.requireCreateEntity(ca)

		caValues := ctx.AdminSession.requireQuery("cas/" + ca.id)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)

		ctx.req.NotEmpty(verificationToken)

		validationAuth := ca.CreateSignedCert(verificationToken)

		clientAuthenticator := ca.CreateSignedCert(eid.New())

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

	t.Run("identities from auto enrollment use identity name format for naming", func(t *testing.T) {
		ctx.testContextChanged(t)
		ca := newTestCa()

		expectedName := "singular.name.not.great"
		ca.identityNameFormat = expectedName
		ca.id = ctx.AdminSession.requireCreateEntity(ca)

		caValues := ctx.AdminSession.requireQuery("cas/" + ca.id)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)

		ctx.req.NotEmpty(verificationToken)

		validationAuth := ca.CreateSignedCert(verificationToken)

		clientAuthenticator := ca.CreateSignedCert(eid.New())

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
		ctx.req.Equal(expectedName, identity.Path("data.name").Data().(string))
	})

	t.Run("identities from auto enrollment identity name collisions add numbers to the end", func(t *testing.T) {
		ctx.testContextChanged(t)

		firstExpectedName := "some.static.name.no.replacements"
		secondExpectedName := "some.static.name.no.replacements000001"

		//create CA
		ca := newTestCa()
		ca.identityNameFormat = firstExpectedName
		ca.id = ctx.AdminSession.requireCreateEntity(ca)

		caValues := ctx.AdminSession.requireQuery("cas/" + ca.id)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)

		ctx.req.NotEmpty(verificationToken)

		//validate CA
		validationAuth := ca.CreateSignedCert(verificationToken)
		resp, err := ctx.AdminSession.newAuthenticatedRequest().
			SetHeader("content-type", "text/plain").
			SetBody(validationAuth.certPem).
			Post("cas/" + ca.id + "/verify")

		ctx.req.NoError(err)
		ctx.logJson(resp.Body())
		ctx.req.Equal(http.StatusOK, resp.StatusCode())

		//first firstIdentity, no issues
		firstClientAuthenticator := ca.CreateSignedCert(eid.New())
		ctx.completeCaAutoEnrollment(firstClientAuthenticator)

		firstEnrolledSession, err := firstClientAuthenticator.Authenticate(ctx)

		ctx.req.NoError(err)

		firstIdentity := ctx.AdminSession.requireQuery("identities/" + firstEnrolledSession.identityId)
		ctx.req.Equal(firstExpectedName, firstIdentity.Path("data.name").Data().(string))

		//second firstIdentity that collides, becomes
		secondClientAuthenticator := ca.CreateSignedCert(eid.New())
		ctx.completeCaAutoEnrollment(secondClientAuthenticator)

		secondEnrolledSession, err := secondClientAuthenticator.Authenticate(ctx)

		ctx.req.NoError(err)

		secondIdentity := ctx.AdminSession.requireQuery("identities/" + secondEnrolledSession.identityId)
		ctx.req.Equal(secondExpectedName, secondIdentity.Path("data.name").Data().(string))
	})

	t.Run("identities from auto enrollment use identity name format for naming with replacements", func(t *testing.T) {
		ctx.testContextChanged(t)
		ca := newTestCa()
		ca.identityNameFormat = "[caName] - [caId] - [commonName] - [requestedName] - [identityId]"

		ca.id = ctx.AdminSession.requireCreateEntity(ca)

		caValues := ctx.AdminSession.requireQuery("cas/" + ca.id)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)

		ctx.req.NotEmpty(verificationToken)

		validationAuth := ca.CreateSignedCert(verificationToken)

		clientAuthenticator := ca.CreateSignedCert(eid.New())

		resp, err := ctx.AdminSession.newAuthenticatedRequest().
			SetHeader("content-type", "text/plain").
			SetBody(validationAuth.certPem).
			Post("cas/" + ca.id + "/verify")

		ctx.req.NoError(err)
		ctx.logJson(resp.Body())
		ctx.req.Equal(http.StatusOK, resp.StatusCode())

		requestedName := "bobby"
		ctx.completeCaAutoEnrollmentWithName(clientAuthenticator, requestedName)

		enrolledSession, err := clientAuthenticator.Authenticate(ctx)

		ctx.req.NoError(err)

		identity := ctx.AdminSession.requireQuery("identities/" + enrolledSession.identityId)
		expectedName := fmt.Sprintf("%s - %s - %s - %s - %s", ca.name, ca.id, clientAuthenticator.cert.Subject.CommonName, requestedName, enrolledSession.identityId)

		ctx.req.Equal(expectedName, identity.Path("data.name").Data().(string))
	})
}
