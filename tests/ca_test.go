//go:build apitests

package tests

import (
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/Jeffail/gabs"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/model"
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

		identity := ctx.AdminManagementSession.requireQuery("identities/" + *enrolledSession.AuthResponse.IdentityID)
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

		identity := ctx.AdminManagementSession.requireQuery("identities/" + *enrolledSession.AuthResponse.IdentityID)
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

		firstIdentity := ctx.AdminManagementSession.requireQuery("identities/" + *firstEnrolledSession.AuthResponse.IdentityID)
		ctx.Req.Equal(firstExpectedName, firstIdentity.Path("data.name").Data().(string))

		//second firstIdentity that collides, becomes
		secondClientAuthenticator := ca.CreateSignedCert(eid.New())
		ctx.completeOttCaEnrollment(secondClientAuthenticator)

		secondEnrolledSession, err := secondClientAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		secondIdentity := ctx.AdminManagementSession.requireQuery("identities/" + *secondEnrolledSession.AuthResponse.IdentityID)
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

		identity := ctx.AdminManagementSession.requireQuery("identities/" + *enrolledSession.AuthResponse.IdentityID)
		expectedName := fmt.Sprintf("%s - %s - %s - %s - %s", ca.name, ca.id, clientAuthenticator.certs[0].Subject.CommonName, requestedName, *enrolledSession.AuthResponse.IdentityID)

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

		t.Run("auth from CA should not be extendable", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.NotNil(enrolledSession.AuthResponse)
			ctx.Req.NotNil(enrolledSession.AuthResponse.IsCertExtendable)
			ctx.Req.False(*enrolledSession.AuthResponse.IsCertExtendable, "expected isCertExtendable on 3rd party CA certificate authentication to be false")
		})

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
			CertPem: ToPtr(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationCOMMONNAME),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherALL),
				MatcherCriteria: ToPtr(""),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             ToPtr(true),
			IsAutoCaEnrollmentEnabled: ToPtr(true),
			IsOttCaEnrollmentEnabled:  ToPtr(true),
			Name:                      ToPtr(eid.New()),
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
			CertPem: ToPtr(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationSANURI),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSCHEME),
				MatcherCriteria: ToPtr("spiffe"),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             ToPtr(true),
			IsAutoCaEnrollmentEnabled: ToPtr(true),
			IsOttCaEnrollmentEnabled:  ToPtr(true),
			Name:                      ToPtr(eid.New()),
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
			CertPem: ToPtr(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationSANEMAIL),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSUFFIX),
				MatcherCriteria: ToPtr("@example.org"),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             ToPtr(true),
			IsAutoCaEnrollmentEnabled: ToPtr(true),
			IsOttCaEnrollmentEnabled:  ToPtr(true),
			Name:                      ToPtr(eid.New()),
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
			CertPem:                   ToPtr(caPEM.String()),
			ExternalIDClaim:           nil,
			IdentityRoles:             []string{},
			IsAuthEnabled:             ToPtr(true),
			IsAutoCaEnrollmentEnabled: ToPtr(true),
			IsOttCaEnrollmentEnabled:  ToPtr(true),
			Name:                      ToPtr(eid.New()),
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
			CertPem: ToPtr(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationSANEMAIL),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSCHEME),
				MatcherCriteria: ToPtr("@example.org"),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             ToPtr(true),
			IsAutoCaEnrollmentEnabled: ToPtr(true),
			IsOttCaEnrollmentEnabled:  ToPtr(true),
			Name:                      ToPtr(eid.New()),
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
			CertPem: ToPtr(caPEM.String()),
			ExternalIDClaim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](0),
				Location:        nil,
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSCHEME),
				MatcherCriteria: ToPtr("@example.org"),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			},
			IdentityRoles:             []string{},
			IsAuthEnabled:             ToPtr(true),
			IsAutoCaEnrollmentEnabled: ToPtr(true),
			IsOttCaEnrollmentEnabled:  ToPtr(true),
			Name:                      ToPtr(eid.New()),
		}

		caCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
		ctx.NoError(err)
		ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("can update a CA with externalIdClaim with a CN location, no parsing, all matcher", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPEM := newTestCaCert() //x509.Cert, PrivKey, caPem

		caCreate := &rest_model.CaCreate{
			CertPem:                   ToPtr(caPEM.String()),
			IdentityRoles:             []string{},
			IsAuthEnabled:             ToPtr(true),
			IsAutoCaEnrollmentEnabled: ToPtr(true),
			IsOttCaEnrollmentEnabled:  ToPtr(true),
			Name:                      ToPtr(eid.New()),
		}

		caCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

		t.Run("can patch externalIdClaim", func(t *testing.T) {
			ctx.testContextChanged(t)
			caPatch := &rest_model.CaPatch{
				ExternalIDClaim: &rest_model.ExternalIDClaimPatch{
					Index:           ToPtr[int64](0),
					Location:        ToPtr(rest_model.ExternalIDClaimLocationCOMMONNAME),
					Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherALL),
					MatcherCriteria: ToPtr(""),
					Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
					ParserCriteria:  ToPtr(""),
				},
			}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caPatch).Patch("/cas/" + caCreateResult.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

			t.Run("patched ca values are correct", func(t *testing.T) {
				ctx.testContextChanged(t)

				caGetResult := &rest_model.DetailCaEnvelope{}

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(caGetResult).Get("/cas/" + caCreateResult.Data.ID)
				ctx.NoError(err)
				ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
				ctx.NotNil(caGetResult, string(resp.Body()))
				ctx.NotNil(caGetResult.Data, string(resp.Body()))
				ctx.Equal(caCreateResult.Data.ID, *caGetResult.Data.ID, string(resp.Body()))
				ctx.NotNil(caGetResult.Data.ExternalIDClaim, string(resp.Body()))

				ctx.Equal(*caPatch.ExternalIDClaim.Index, *caGetResult.Data.ExternalIDClaim.Index)
				ctx.Equal(*caPatch.ExternalIDClaim.Location, *caGetResult.Data.ExternalIDClaim.Location)
				ctx.Equal(*caPatch.ExternalIDClaim.Matcher, *caGetResult.Data.ExternalIDClaim.Matcher)
				ctx.Equal(caPatch.ExternalIDClaim.MatcherCriteria, caGetResult.Data.ExternalIDClaim.MatcherCriteria)
				ctx.Equal(*caPatch.ExternalIDClaim.Parser, *caGetResult.Data.ExternalIDClaim.Parser)
				ctx.Equal(caPatch.ExternalIDClaim.ParserCriteria, caGetResult.Data.ExternalIDClaim.ParserCriteria)
			})
		})
	})

	t.Run("can update a CA with externalIdClaim with SAN location, SCHEME matcher, spiffe scheme, no parsing", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPEM := newTestCaCert() //x509.Cert, PrivKey, caPem

		caCreate := &rest_model.CaCreate{
			CertPem:                   ToPtr(caPEM.String()),
			IdentityRoles:             []string{},
			IsAuthEnabled:             ToPtr(true),
			IsAutoCaEnrollmentEnabled: ToPtr(true),
			IsOttCaEnrollmentEnabled:  ToPtr(true),
			Name:                      ToPtr(eid.New()),
		}

		caCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

		t.Run("can patch externalIdClaim", func(t *testing.T) {
			ctx.testContextChanged(t)
			caPatch := &rest_model.CaPatch{
				ExternalIDClaim: &rest_model.ExternalIDClaimPatch{
					Index:           ToPtr[int64](0),
					Location:        ToPtr(rest_model.ExternalIDClaimPatchLocationSANURI),
					Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSCHEME),
					MatcherCriteria: ToPtr("spiffe"),
					Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
					ParserCriteria:  ToPtr(""),
				},
			}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caPatch).Patch("/cas/" + caCreateResult.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

			t.Run("patched ca values are correct", func(t *testing.T) {
				ctx.testContextChanged(t)

				caGetResult := &rest_model.DetailCaEnvelope{}

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(caGetResult).Get("/cas/" + caCreateResult.Data.ID)
				ctx.NoError(err)
				ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
				ctx.NotNil(caGetResult, string(resp.Body()))
				ctx.NotNil(caGetResult.Data, string(resp.Body()))
				ctx.Equal(caCreateResult.Data.ID, *caGetResult.Data.ID, string(resp.Body()))
				ctx.NotNil(caGetResult.Data.ExternalIDClaim, string(resp.Body()))

				ctx.Equal(*caPatch.ExternalIDClaim.Index, *caGetResult.Data.ExternalIDClaim.Index)
				ctx.Equal(*caPatch.ExternalIDClaim.Location, *caGetResult.Data.ExternalIDClaim.Location)
				ctx.Equal(*caPatch.ExternalIDClaim.Matcher, *caGetResult.Data.ExternalIDClaim.Matcher)
				ctx.Equal(caPatch.ExternalIDClaim.MatcherCriteria, caGetResult.Data.ExternalIDClaim.MatcherCriteria)
				ctx.Equal(*caPatch.ExternalIDClaim.Parser, *caGetResult.Data.ExternalIDClaim.Parser)
				ctx.Equal(caPatch.ExternalIDClaim.ParserCriteria, caGetResult.Data.ExternalIDClaim.ParserCriteria)
			})
		})
	})
}

// Test_CA_ExternalIdClaim_Validation asserts that unsupported or incomplete externalIdClaim
// configurations are rejected with HTTP 400 at CA create and update time, rather than being
// stored and later crashing enrollment/authentication with an HTTP 500 (see issue matrix).
func Test_CA_ExternalIdClaim_Validation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	invalidClaims := []struct {
		name  string
		claim *rest_model.ExternalIDClaim
	}{
		{
			name: "san uri with prefix matcher",
			claim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationSANURI),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherPREFIX),
				MatcherCriteria: ToPtr("acme:tenant:"),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			},
		},
		{
			name: "san uri with suffix matcher",
			claim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationSANURI),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSUFFIX),
				MatcherCriteria: ToPtr(":042"),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			},
		},
		{
			name: "scheme matcher with empty criteria",
			claim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationSANURI),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSCHEME),
				MatcherCriteria: ToPtr(""),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			},
		},
		{
			name: "prefix matcher with empty criteria",
			claim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationCOMMONNAME),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherPREFIX),
				MatcherCriteria: ToPtr(""),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			},
		},
		{
			name: "split parser with empty criteria",
			claim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationCOMMONNAME),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherALL),
				MatcherCriteria: ToPtr(""),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserSPLIT),
				ParserCriteria:  ToPtr(""),
			},
		},
		{
			name: "negative index",
			claim: &rest_model.ExternalIDClaim{
				Index:           ToPtr[int64](-1),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationCOMMONNAME),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherALL),
				MatcherCriteria: ToPtr(""),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			},
		},
	}

	for _, tc := range invalidClaims {
		t.Run("can not create a CA with "+tc.name, func(t *testing.T) {
			ctx.testContextChanged(t)

			_, _, caPEM := newTestCaCert()

			caCreate := &rest_model.CaCreate{
				CertPem:                   ToPtr(caPEM.String()),
				ExternalIDClaim:           tc.claim,
				IdentityRoles:             []string{},
				IsAuthEnabled:             ToPtr(true),
				IsAutoCaEnrollmentEnabled: ToPtr(true),
				IsOttCaEnrollmentEnabled:  ToPtr(true),
				Name:                      ToPtr(eid.New()),
			}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).Post("/cas")
			ctx.NoError(err)
			ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
		})
	}

	for _, tc := range invalidClaims {
		t.Run("can not patch a CA to "+tc.name, func(t *testing.T) {
			ctx.testContextChanged(t)

			_, _, caPEM := newTestCaCert()

			caCreate := &rest_model.CaCreate{
				CertPem:                   ToPtr(caPEM.String()),
				IdentityRoles:             []string{},
				IsAuthEnabled:             ToPtr(true),
				IsAutoCaEnrollmentEnabled: ToPtr(true),
				IsOttCaEnrollmentEnabled:  ToPtr(true),
				Name:                      ToPtr(eid.New()),
			}

			caCreateResult := &rest_model.CreateEnvelope{}

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
			ctx.NoError(err)
			ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

			caPatch := &rest_model.CaPatch{
				ExternalIDClaim: &rest_model.ExternalIDClaimPatch{
					Index:           tc.claim.Index,
					Location:        tc.claim.Location,
					Matcher:         tc.claim.Matcher,
					MatcherCriteria: tc.claim.MatcherCriteria,
					Parser:          tc.claim.Parser,
					ParserCriteria:  tc.claim.ParserCriteria,
				},
			}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caPatch).Patch("/cas/" + caCreateResult.Data.ID)
			ctx.NoError(err)
			ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
		})
	}
}

// createTestCaWithClaim creates a CA carrying the given externalIdClaim (nil for none) and returns its id.
func createTestCaWithClaim(ctx *TestContext, claim *rest_model.ExternalIDClaim) string {
	_, _, caPEM := newTestCaCert()
	caCreate := &rest_model.CaCreate{
		CertPem:                   ToPtr(caPEM.String()),
		ExternalIDClaim:           claim,
		IdentityRoles:             []string{},
		IsAuthEnabled:             ToPtr(true),
		IsAutoCaEnrollmentEnabled: ToPtr(true),
		IsOttCaEnrollmentEnabled:  ToPtr(true),
		Name:                      ToPtr(eid.New()),
	}
	result := &rest_model.CreateEnvelope{}
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(result).Post("/cas")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
	return result.Data.ID
}

// getTestCaClaim returns the stored externalIdClaim for a CA, or nil if it has none.
func getTestCaClaim(ctx *TestContext, id string) *rest_model.ExternalIDClaim {
	result := &rest_model.DetailCaEnvelope{}
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(result).Get("/cas/" + id)
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
	return result.Data.ExternalIDClaim
}

// Test_CA_ExternalIdClaim_PatchMerge covers PATCH merge semantics for externalIdClaim. A PATCH
// carries only the subfields it names and the server merges them onto the stored claim. The nuance
// this guards: the empty {} object older CLIs send on every update (even a rename) must be a no-op,
// not a validation 400; a supplied subfield must merge with stored values; only the resulting merged
// claim is validated; and omitting the claim clears it.
func Test_CA_ExternalIdClaim_PatchMerge(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	// A valid stored starting point: extract the common name prefixed with "acme:".
	baseClaim := func() *rest_model.ExternalIDClaim {
		return &rest_model.ExternalIDClaim{
			Index:           ToPtr[int64](0),
			Location:        ToPtr(rest_model.ExternalIDClaimLocationCOMMONNAME),
			Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherPREFIX),
			MatcherCriteria: ToPtr("acme:"),
			Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
			ParserCriteria:  ToPtr(""),
		}
	}

	t.Run("empty claim object is a no-op (old CLI rename path)", func(t *testing.T) {
		ctx.testContextChanged(t)
		// Old CLIs always send "externalIdClaim": {} even for a rename. It must preserve the
		// stored claim, not be validated as an incomplete (location-less) claim.
		id := createTestCaWithClaim(ctx, baseClaim())

		caPatch := &rest_model.CaPatch{
			Name:            ToPtr(eid.New()),
			ExternalIDClaim: &rest_model.ExternalIDClaimPatch{},
		}
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caPatch).Patch("/cas/" + id)
		ctx.NoError(err)
		ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

		claim := getTestCaClaim(ctx, id)
		ctx.Req.NotNil(claim)
		ctx.Equal("acme:", *claim.MatcherCriteria, "stored claim should be unchanged")
	})

	t.Run("a supplied subfield merges with stored values", func(t *testing.T) {
		ctx.testContextChanged(t)
		// Patching only matcherCriteria leaves location/matcher/parser as stored.
		id := createTestCaWithClaim(ctx, baseClaim())

		caPatch := &rest_model.CaPatch{
			ExternalIDClaim: &rest_model.ExternalIDClaimPatch{MatcherCriteria: ToPtr("widget:")},
		}
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caPatch).Patch("/cas/" + id)
		ctx.NoError(err)
		ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

		claim := getTestCaClaim(ctx, id)
		ctx.Req.NotNil(claim)
		ctx.Equal("widget:", *claim.MatcherCriteria)
		ctx.Equal(rest_model.ExternalIDClaimLocationCOMMONNAME, *claim.Location, "location should be retained")
		ctx.Equal(rest_model.ExternalIDClaimMatcherPREFIX, *claim.Matcher, "matcher should be retained")
	})

	t.Run("a subfield patch producing an invalid merged claim is rejected", func(t *testing.T) {
		ctx.testContextChanged(t)
		// SCHEME is invalid for COMMON_NAME; the merged result must be validated and rejected.
		id := createTestCaWithClaim(ctx, baseClaim())

		caPatch := &rest_model.CaPatch{
			ExternalIDClaim: &rest_model.ExternalIDClaimPatch{Matcher: ToPtr(rest_model.ExternalIDClaimMatcherSCHEME)},
		}
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caPatch).Patch("/cas/" + id)
		ctx.NoError(err)
		ctx.Equal(http.StatusBadRequest, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("empty claim object on a CA with no claim is a no-op", func(t *testing.T) {
		ctx.testContextChanged(t)
		// Same old-CLI path against a CA that never had a claim: must not 400.
		id := createTestCaWithClaim(ctx, nil)

		caPatch := &rest_model.CaPatch{
			Name:            ToPtr(eid.New()),
			ExternalIDClaim: &rest_model.ExternalIDClaimPatch{},
		}
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caPatch).Patch("/cas/" + id)
		ctx.NoError(err)
		ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("omitting the claim clears it", func(t *testing.T) {
		ctx.testContextChanged(t)
		// A patch with no externalIdClaim key removes the stored claim (delete-on-nil). This is the
		// signal the new --clear-external-id-claim CLI flag sends.
		id := createTestCaWithClaim(ctx, baseClaim())

		caPatch := &rest_model.CaPatch{Name: ToPtr(eid.New())}
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caPatch).Patch("/cas/" + id)
		ctx.NoError(err)
		ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

		ctx.Req.Nil(getTestCaClaim(ctx, id), "claim should be cleared")
	})
}
