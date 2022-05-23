//go:build apitests
// +build apitests

package tests

import (
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/rest_model"
	"net/http"
	"sort"
	"testing"
)

func Test_CA(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("identity attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		ca := newTestCa(role1, role2)
		ca.id = ctx.AdminManagementSession.requireCreateEntity(ca)
		ctx.AdminManagementSession.validateEntityWithQuery(ca)
		ctx.AdminManagementSession.validateEntityWithLookup(ca)
	})

	t.Run("identity attributes should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		ca := newTestCa(role1, role2)
		ca.id = ctx.AdminManagementSession.requireCreateEntity(ca)

		role3 := eid.New()
		ca.identityRoles = []string{role2, role3}
		ctx.AdminManagementSession.requireUpdateEntity(ca)
		ctx.AdminManagementSession.validateEntityWithLookup(ca)
	})

	t.Run("identityNameFormat should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		ca := newTestCa(role1, role2)
		ca.id = ctx.AdminManagementSession.requireCreateEntity(ca)

		ca.identityNameFormat = "123"
		ctx.AdminManagementSession.requireUpdateEntity(ca)
		ctx.AdminManagementSession.validateEntityWithLookup(ca)
	})

	t.Run("identity name format should default if not specified", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		ca := newTestCa(role1, role2)

		ca.identityNameFormat = ""
		ca.id = ctx.AdminManagementSession.requireCreateEntity(ca)

		//set to default for verification
		ca.identityNameFormat = model.DefaultCaIdentityNameFormat

		ctx.AdminManagementSession.validateEntityWithQuery(ca)
		ctx.AdminManagementSession.validateEntityWithLookup(ca)
	})

	t.Run("identities from auto enrollment inherit CA identity roles", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		ca := newTestCa(role1, role2)
		ca.id = ctx.AdminManagementSession.requireCreateEntity(ca)

		caValues := ctx.AdminManagementSession.requireQuery("cas/" + ca.id)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)

		ctx.Req.NotEmpty(verificationToken)

		validationAuth := ca.CreateSignedCert(verificationToken)

		clientAuthenticator := ca.CreateSignedCert(eid.New())

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
			SetHeader("content-type", "text/plain").
			SetBody(validationAuth.certPem).
			Post("cas/" + ca.id + "/verify")

		ctx.Req.NoError(err)
		ctx.logJson(resp.Body())
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		ctx.completeOttCaEnrollment(clientAuthenticator)

		enrolledSession, err := clientAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		identity := ctx.AdminManagementSession.requireQuery("identities/" + enrolledSession.identityId)
		sort.Strings(ca.identityRoles)
		ctx.pathEqualsStringSlice(identity, ca.identityRoles, path("data", "roleAttributes"))
	})

	t.Run("identities from auto enrollment use identity name format for naming", func(t *testing.T) {
		ctx.testContextChanged(t)
		ca := newTestCa()

		expectedName := "singular.name.not.great"
		ca.identityNameFormat = expectedName
		ca.id = ctx.AdminManagementSession.requireCreateEntity(ca)

		caValues := ctx.AdminManagementSession.requireQuery("cas/" + ca.id)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)

		ctx.Req.NotEmpty(verificationToken)

		validationAuth := ca.CreateSignedCert(verificationToken)

		clientAuthenticator := ca.CreateSignedCert(eid.New())

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
			SetHeader("content-type", "text/plain").
			SetBody(validationAuth.certPem).
			Post("cas/" + ca.id + "/verify")

		ctx.Req.NoError(err)
		ctx.logJson(resp.Body())
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		ctx.completeOttCaEnrollment(clientAuthenticator)

		enrolledSession, err := clientAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		identity := ctx.AdminManagementSession.requireQuery("identities/" + enrolledSession.identityId)
		ctx.Req.Equal(expectedName, identity.Path("data.name").Data().(string))
	})

	t.Run("identities from auto enrollment identity name collisions add numbers to the end", func(t *testing.T) {
		ctx.testContextChanged(t)

		firstExpectedName := "some.static.name.no.replacements"
		secondExpectedName := "some.static.name.no.replacements000001"

		//create CA
		ca := newTestCa()
		ca.identityNameFormat = firstExpectedName
		ca.id = ctx.AdminManagementSession.requireCreateEntity(ca)

		caValues := ctx.AdminManagementSession.requireQuery("cas/" + ca.id)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)

		ctx.Req.NotEmpty(verificationToken)

		//validate CA
		validationAuth := ca.CreateSignedCert(verificationToken)
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
			SetHeader("content-type", "text/plain").
			SetBody(validationAuth.certPem).
			Post("cas/" + ca.id + "/verify")

		ctx.Req.NoError(err)
		ctx.logJson(resp.Body())
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		//first firstIdentity, no issues
		firstClientAuthenticator := ca.CreateSignedCert(eid.New())
		ctx.completeOttCaEnrollment(firstClientAuthenticator)

		firstEnrolledSession, err := firstClientAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		firstIdentity := ctx.AdminManagementSession.requireQuery("identities/" + firstEnrolledSession.identityId)
		ctx.Req.Equal(firstExpectedName, firstIdentity.Path("data.name").Data().(string))

		//second firstIdentity that collides, becomes
		secondClientAuthenticator := ca.CreateSignedCert(eid.New())
		ctx.completeOttCaEnrollment(secondClientAuthenticator)

		secondEnrolledSession, err := secondClientAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		secondIdentity := ctx.AdminManagementSession.requireQuery("identities/" + secondEnrolledSession.identityId)
		ctx.Req.Equal(secondExpectedName, secondIdentity.Path("data.name").Data().(string))
	})

	t.Run("identities from auto enrollment use identity name format for naming with replacements", func(t *testing.T) {
		ctx.testContextChanged(t)
		ca := newTestCa()
		ca.identityNameFormat = "[caName] - [caId] - [commonName] - [requestedName] - [identityId]"

		ca.id = ctx.AdminManagementSession.requireCreateEntity(ca)

		caValues := ctx.AdminManagementSession.requireQuery("cas/" + ca.id)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)

		ctx.Req.NotEmpty(verificationToken)

		validationAuth := ca.CreateSignedCert(verificationToken)

		clientAuthenticator := ca.CreateSignedCert(eid.New())

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
			SetHeader("content-type", "text/plain").
			SetBody(validationAuth.certPem).
			Post("cas/" + ca.id + "/verify")

		ctx.Req.NoError(err)
		ctx.logJson(resp.Body())
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		requestedName := "bobby"
		ctx.completeCaAutoEnrollmentWithName(clientAuthenticator, requestedName)

		enrolledSession, err := clientAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		identity := ctx.AdminManagementSession.requireQuery("identities/" + enrolledSession.identityId)
		expectedName := fmt.Sprintf("%s - %s - %s - %s - %s", ca.name, ca.id, clientAuthenticator.cert.Subject.CommonName, requestedName, enrolledSession.identityId)

		ctx.Req.Equal(expectedName, identity.Path("data.name").Data().(string))
	})

	t.Run("CAs with auth enabled can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)
		ca := newTestCa()

		ca.id = ctx.AdminManagementSession.requireCreateEntity(ca)

		caValues := ctx.AdminManagementSession.requireQuery("cas/" + ca.id)
		verificationToken := caValues.Path("data.verificationToken").Data().(string)
		ctx.Req.NotEmpty(verificationToken)

		validationAuth := ca.CreateSignedCert(verificationToken)
		clientAuthenticator := ca.CreateSignedCert(eid.New())

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
			SetHeader("content-type", "text/plain").
			SetBody(validationAuth.certPem).
			Post("cas/" + ca.id + "/verify")

		ctx.Req.NoError(err)
		ctx.logJson(resp.Body())
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		ctx.completeOttCaEnrollment(clientAuthenticator)

		enrolledSession, err := clientAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(enrolledSession)

		t.Run("CAs with auth disabled can no longer authenticate", func(t *testing.T) {
			ctx.testContextChanged(t)
			ca.isAuthEnabled = false
			resp := ctx.AdminManagementSession.patchEntity(ca, "isAuthEnabled")
			ctx.Req.NotEmpty(resp)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			enrolledSession, err := clientAuthenticator.AuthenticateClientApi(ctx)

			ctx.Req.Error(err)
			ctx.Req.Empty(enrolledSession)
		})

		t.Run("CAs with auth re-enabled an authenticate", func(t *testing.T) {
			ctx.testContextChanged(t)
			ca.isAuthEnabled = true
			resp := ctx.AdminManagementSession.patchEntity(ca, "isAuthEnabled")
			ctx.Req.NotEmpty(resp)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			enrolledSession, err := clientAuthenticator.AuthenticateClientApi(ctx)

			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrolledSession)
		})

		t.Run("deleting a CA no longer allows authentication", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.AdminManagementSession.requireDeleteEntity(ca)

			enrolledSession, err := clientAuthenticator.AuthenticateClientApi(ctx)

			ctx.Req.Error(err)
			ctx.Req.Empty(enrolledSession)
		})
	})

	t.Run("deleting a CA should", func(t *testing.T) {

		t.Run("clean up outstanding enrollments", func(t *testing.T) {
			ctx.testContextChanged(t)

			//shared across tests
			enrollmentId := ""
			var unenrolledOttCaIdentity *identity
			var unenrolledOttCaIdentityContainer *gabs.Container

			ca := newTestCa()

			ca.id = ctx.AdminManagementSession.requireCreateEntity(ca)

			caValues := ctx.AdminManagementSession.requireQuery("cas/" + ca.id)
			verificationToken := caValues.Path("data.verificationToken").Data().(string)
			ctx.Req.NotEmpty(verificationToken)

			validationAuth := ca.CreateSignedCert(verificationToken)

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
				SetHeader("content-type", "text/plain").
				SetBody(validationAuth.certPem).
				Post("cas/" + ca.id + "/verify")

			ctx.Req.NoError(err)
			ctx.logJson(resp.Body())
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			unenrolledOttCaIdentity = ctx.AdminManagementSession.RequireNewIdentityWithCaOtt(false, ca.id)

			unenrolledOttCaIdentityContainer = ctx.AdminManagementSession.requireQuery("/identities/" + unenrolledOttCaIdentity.Id)

			ctx.Req.True(unenrolledOttCaIdentityContainer.ExistsP("data.enrollment.ottca.id"), "expected ottca to have an enrollment id")

			enrollmentId = unenrolledOttCaIdentityContainer.Path("data.enrollment.ottca.id").Data().(string)
			ctx.Req.NotEmpty(enrollmentId, "enrollment id should not be empty string")

			ctx.AdminManagementSession.requireDeleteEntity(ca)

			t.Run("enrollment should have been removed", func(t *testing.T) {
				ctx.testContextChanged(t)

				status, _ := ctx.AdminManagementSession.query("enrollments/" + enrollmentId)

				ctx.Req.Equal(http.StatusNotFound, status, "expected enrollment to not be found")

			})

			t.Run("identities with previous enrollments tied to deleted CAs should not error on list", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotEmpty(unenrolledOttCaIdentity.Id)
				_ = ctx.AdminManagementSession.requireQuery(fmt.Sprintf(`identities?filter=id="%s"`, unenrolledOttCaIdentity.Id))
			})

			t.Run("identities with previous enrollments tied to deleted CAs should not error on detail", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.NotEmpty(unenrolledOttCaIdentity.Id)
				_ = ctx.AdminManagementSession.requireQuery("identities/" + unenrolledOttCaIdentity.Id)
			})
		})
	})

	t.Run("can create a CA with externalIdClaim in common name, all, no parsing", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPEM := newTestCaCert() //x509.Cert, PrivKey, caPem

		caCreate := &rest_model.CaCreate{
			CertPem: S(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           I(0),
				Location:        S(rest_model.ExternalIDClaimLocationCOMMONNAME),
				Matcher:         S(rest_model.ExternalIDClaimMatcherALL),
				MatcherCriteria: S(""),
				Parser:          S(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  S(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             B(true),
			IsAutoCaEnrollmentEnabled: B(true),
			IsOttCaEnrollmentEnabled:  B(true),
			Name:                      S(eid.New()),
		}

		caCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(caCreateResult)
		ctx.NotNil(caCreateResult.Data)
		ctx.NotEmpty(caCreateResult.Data.ID)

		t.Run("created ca values are correct", func(t *testing.T) {
			ctx.testContextChanged(t)

			caGetResult := &rest_model.DetailCaEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(caGetResult).Get("/cas/" + caCreateResult.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.NotNil(caGetResult, string(resp.Body()))
			ctx.NotNil(caGetResult.Data, string(resp.Body()))
			ctx.NotNil(caGetResult.Data.ID, string(resp.Body()))
			ctx.NotEmpty(*caGetResult.Data.ID, string(resp.Body()))
			ctx.NotNil(caGetResult.Data.ExternalIDClaim, string(resp.Body()))

			ctx.Equal(*caCreate.ExternalIDClaim.Index, *caGetResult.Data.ExternalIDClaim.Index)
			ctx.Equal(*caCreate.ExternalIDClaim.Location, *caGetResult.Data.ExternalIDClaim.Location)
			ctx.Equal(*caCreate.ExternalIDClaim.Matcher, *caGetResult.Data.ExternalIDClaim.Matcher)
			ctx.Equal(caCreate.ExternalIDClaim.MatcherCriteria, caGetResult.Data.ExternalIDClaim.MatcherCriteria)
			ctx.Equal(*caCreate.ExternalIDClaim.Parser, *caGetResult.Data.ExternalIDClaim.Parser)
			ctx.Equal(caCreate.ExternalIDClaim.ParserCriteria, caGetResult.Data.ExternalIDClaim.ParserCriteria)

		})
	})

	t.Run("can create a CA with externalIdClaim in san uri, scheme, no parsing", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPEM := newTestCaCert() //x509.Cert, PrivKey, caPem

		caCreate := &rest_model.CaCreate{
			CertPem: S(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           I(0),
				Location:        S(rest_model.ExternalIDClaimLocationSANURI),
				Matcher:         S(rest_model.ExternalIDClaimMatcherSCHEME),
				MatcherCriteria: S("spiffe"),
				Parser:          S(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  S(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             B(true),
			IsAutoCaEnrollmentEnabled: B(true),
			IsOttCaEnrollmentEnabled:  B(true),
			Name:                      S(eid.New()),
		}

		caCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(caCreateResult)
		ctx.NotNil(caCreateResult.Data)
		ctx.NotEmpty(caCreateResult.Data.ID)

		t.Run("created ca values are correct", func(t *testing.T) {
			ctx.testContextChanged(t)

			caGetResult := &rest_model.DetailCaEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(caGetResult).Get("/cas/" + caCreateResult.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.NotNil(caGetResult, string(resp.Body()))
			ctx.NotNil(caGetResult.Data, string(resp.Body()))
			ctx.NotNil(caGetResult.Data.ID, string(resp.Body()))
			ctx.NotEmpty(*caGetResult.Data.ID, string(resp.Body()))
			ctx.NotNil(caGetResult.Data.ExternalIDClaim, string(resp.Body()))

			ctx.Equal(*caCreate.ExternalIDClaim.Index, *caGetResult.Data.ExternalIDClaim.Index)
			ctx.Equal(*caCreate.ExternalIDClaim.Location, *caGetResult.Data.ExternalIDClaim.Location)
			ctx.Equal(*caCreate.ExternalIDClaim.Matcher, *caGetResult.Data.ExternalIDClaim.Matcher)
			ctx.Equal(caCreate.ExternalIDClaim.MatcherCriteria, caGetResult.Data.ExternalIDClaim.MatcherCriteria)
			ctx.Equal(*caCreate.ExternalIDClaim.Parser, *caGetResult.Data.ExternalIDClaim.Parser)
			ctx.Equal(caCreate.ExternalIDClaim.ParserCriteria, caGetResult.Data.ExternalIDClaim.ParserCriteria)

		})
	})

	t.Run("can create a CA with externalIdClaim in email, suffix, no parsing", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPEM := newTestCaCert() //x509.Cert, PrivKey, caPem

		caCreate := &rest_model.CaCreate{
			CertPem: S(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           I(0),
				Location:        S(rest_model.ExternalIDClaimLocationSANEMAIL),
				Matcher:         S(rest_model.ExternalIDClaimMatcherSUFFIX),
				MatcherCriteria: S("@example.org"),
				Parser:          S(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  S(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             B(true),
			IsAutoCaEnrollmentEnabled: B(true),
			IsOttCaEnrollmentEnabled:  B(true),
			Name:                      S(eid.New()),
		}

		caCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(caCreateResult)
		ctx.NotNil(caCreateResult.Data)
		ctx.NotEmpty(caCreateResult.Data.ID)

		t.Run("created ca values are correct", func(t *testing.T) {
			ctx.testContextChanged(t)

			caGetResult := &rest_model.DetailCaEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(caGetResult).Get("/cas/" + caCreateResult.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.NotNil(caGetResult, string(resp.Body()))
			ctx.NotNil(caGetResult.Data, string(resp.Body()))
			ctx.NotNil(caGetResult.Data.ID, string(resp.Body()))
			ctx.NotEmpty(*caGetResult.Data.ID, string(resp.Body()))
			ctx.NotNil(caGetResult.Data.ExternalIDClaim, string(resp.Body()))

			ctx.Equal(*caCreate.ExternalIDClaim.Index, *caGetResult.Data.ExternalIDClaim.Index)
			ctx.Equal(*caCreate.ExternalIDClaim.Location, *caGetResult.Data.ExternalIDClaim.Location)
			ctx.Equal(*caCreate.ExternalIDClaim.Matcher, *caGetResult.Data.ExternalIDClaim.Matcher)
			ctx.Equal(caCreate.ExternalIDClaim.MatcherCriteria, caGetResult.Data.ExternalIDClaim.MatcherCriteria)
			ctx.Equal(*caCreate.ExternalIDClaim.Parser, *caGetResult.Data.ExternalIDClaim.Parser)
			ctx.Equal(caCreate.ExternalIDClaim.ParserCriteria, caGetResult.Data.ExternalIDClaim.ParserCriteria)

		})
	})

	t.Run("can create a CA with no externalIdClaim", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPEM := newTestCaCert() //x509.Cert, privKey, caPem

		caCreate := &rest_model.CaCreate{
			CertPem:                   S(caPEM.String()),
			ExternalIDClaim:           nil,
			IdentityRoles:             []string{},
			IsAuthEnabled:             B(true),
			IsAutoCaEnrollmentEnabled: B(true),
			IsOttCaEnrollmentEnabled:  B(true),
			Name:                      S(eid.New()),
		}

		caCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

		t.Run("created ca values are correct", func(t *testing.T) {
			ctx.testContextChanged(t)

			caGetResult := &rest_model.DetailCaEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(caGetResult).Get("/cas/" + caCreateResult.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.NotNil(caGetResult, string(resp.Body()))
			ctx.NotNil(caGetResult.Data, string(resp.Body()))
			ctx.NotNil(caGetResult.Data.ID, string(resp.Body()))
			ctx.NotEmpty(*caGetResult.Data.ID, string(resp.Body()))
			ctx.Nil(caGetResult.Data.ExternalIDClaim, string(resp.Body()))
			ctx.Equal(caCreate.ExternalIDClaim, caGetResult.Data.ExternalIDClaim)

		})
	})

	t.Run("can not create a CA with externalIdClaim in email, scheme, no parsing", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPEM := newTestCaCert() //x509.Cert, PrivKey, caPem

		caCreate := &rest_model.CaCreate{
			CertPem: S(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           I(0),
				Location:        S(rest_model.ExternalIDClaimLocationSANEMAIL),
				Matcher:         S(rest_model.ExternalIDClaimMatcherSCHEME),
				MatcherCriteria: S("@example.org"),
				Parser:          S(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  S(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             B(true),
			IsAutoCaEnrollmentEnabled: B(true),
			IsOttCaEnrollmentEnabled:  B(true),
			Name:                      S(eid.New()),
		}

		caCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
		ctx.NoError(err)
		ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("can not create a CA with externalIdClaim with missing location", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPEM := newTestCaCert() //x509.Cert, PrivKey, caPem

		caCreate := &rest_model.CaCreate{
			CertPem: S(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           I(0),
				Location:        nil,
				Matcher:         S(rest_model.ExternalIDClaimMatcherSCHEME),
				MatcherCriteria: S("@example.org"),
				Parser:          S(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  S(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             B(true),
			IsAutoCaEnrollmentEnabled: B(true),
			IsOttCaEnrollmentEnabled:  B(true),
			Name:                      S(eid.New()),
		}

		caCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
		ctx.NoError(err)
		ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
	})
}
