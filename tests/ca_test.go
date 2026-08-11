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
	"crypto"
	"crypto/x509"
	"fmt"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/foundation/v2/errorz"
	edgeApis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
)

func Test_CA(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	t.Run("identity attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)

		role1 := eid.New()
		role2 := eid.New()

		_, _, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		caCreate.IdentityRoles = []string{role1, role2}

		created, err := mgmtClient.CreateCa(caCreate)
		ctx.Req.NoError(err)

		detail, err := mgmtClient.GetCa(created.ID)
		ctx.Req.NoError(err)

		ctx.Req.Equal(*caCreate.Name, *detail.Name)
		ctx.Req.ElementsMatch([]string{role1, role2}, detail.IdentityRoles)
		ctx.Req.Equal(caPem, *detail.CertPem)
		ctx.Req.Equal(caCreate.IdentityNameFormat, *detail.IdentityNameFormat)
		ctx.Req.Equal(*caCreate.IsAuthEnabled, *detail.IsAuthEnabled)
		ctx.Req.Equal(*caCreate.IsAutoCaEnrollmentEnabled, *detail.IsAutoCaEnrollmentEnabled)
		ctx.Req.Equal(*caCreate.IsOttCaEnrollmentEnabled, *detail.IsOttCaEnrollmentEnabled)
	})

	t.Run("identity attributes should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)

		role1 := eid.New()
		role2 := eid.New()
		role3 := eid.New()

		_, _, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		caCreate.IdentityRoles = []string{role1, role2}

		created, err := mgmtClient.CreateCa(caCreate)
		ctx.Req.NoError(err)

		update := newCaUpdate(caCreate)
		update.IdentityRoles = []string{role2, role3}

		ctx.Req.NoError(mgmtClient.UpdateCa(created.ID, update))

		detail, err := mgmtClient.GetCa(created.ID)
		ctx.Req.NoError(err)
		ctx.Req.ElementsMatch([]string{role2, role3}, detail.IdentityRoles)
	})

	t.Run("create with a # prefixed identity role should fail", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		caCreate.IdentityRoles = []string{"#badrole"}

		_, err = mgmtClient.CreateCa(caCreate)

		requireIdentityRolesFieldError(ctx, err, "#badrole")
	})

	t.Run("create with an @ prefixed identity role should fail", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		caCreate.IdentityRoles = []string{"@badrole"}

		_, err = mgmtClient.CreateCa(caCreate)

		requireIdentityRolesFieldError(ctx, err, "@badrole")
	})

	t.Run("update with a # prefixed identity role should fail", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		caCreate.IdentityRoles = []string{eid.New()}

		created, err := mgmtClient.CreateCa(caCreate)
		ctx.Req.NoError(err)

		update := newCaUpdate(caCreate)
		update.IdentityRoles = []string{"#badrole"}

		err = mgmtClient.UpdateCa(created.ID, update)

		requireIdentityRolesFieldError(ctx, err, "#badrole")
	})

	t.Run("patch with a # prefixed identity role should fail", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		created, err := mgmtClient.CreateCa(NewCaCreate(caPem))
		ctx.Req.NoError(err)

		err = mgmtClient.PatchCa(created.ID, &rest_model.CaPatch{IdentityRoles: []string{"#badrole"}})

		requireIdentityRolesFieldError(ctx, err, "#badrole")
	})

	t.Run("patch of an unrelated field should not validate existing identity roles", func(t *testing.T) {
		ctx.testContextChanged(t)
		// Role validation is gated on the roles field actually being patched, so a CA carrying a
		// role from before validation existed can still be renamed.
		_, _, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		created, err := mgmtClient.CreateCa(NewCaCreate(caPem))
		ctx.Req.NoError(err)

		newName := eid.New()
		ctx.Req.NoError(mgmtClient.PatchCa(created.ID, &rest_model.CaPatch{Name: ToPtr(newName)}))

		detail, err := mgmtClient.GetCa(created.ID)
		ctx.Req.NoError(err)
		ctx.Req.Equal(newName, *detail.Name)
	})

	t.Run("a ca holding prefixed identity roles can still be verified", func(t *testing.T) {
		ctx.testContextChanged(t)
		// Role validation must respect the field checker. Verification writes only isVerified, so a
		// stored role it never touches must not block it.
		caCert, caKey, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		created, err := mgmtClient.CreateCa(NewCaCreate(caPem))
		ctx.Req.NoError(err)

		seedCaIdentityRoles(ctx, created.ID, []string{"#badrole"})

		detail, err := mgmtClient.GetCa(created.ID)
		ctx.Req.NoError(err)

		ctx.Req.NoError(mgmtClient.VerifyCa(created.ID, detail.VerificationToken.String(), caCert, caKey))

		verified, err := mgmtClient.GetCa(created.ID)
		ctx.Req.NoError(err)
		ctx.Req.True(*verified.IsVerified)
	})

	t.Run("enrollment through a ca holding prefixed identity roles fails without panicking", func(t *testing.T) {
		ctx.testContextChanged(t)

		caCert, caKey, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caId, err := mgmtClient.CreateAndVerifyCa(NewCaCreate(caPem), caCert, caKey)
		ctx.Req.NoError(err)

		seedCaIdentityRoles(ctx, caId, []string{"#badrole"})

		clientCert, clientKey, err := generateCaSignedClientCert(caCert, caKey, eid.New())
		ctx.Req.NoError(err)

		clientApi := ctx.NewEdgeClientApi(nil)
		err = clientApi.CompleteCaAutoEnrollment([]*x509.Certificate{clientCert}, clientKey, "")

		ctx.Req.Error(err, "enrollment through a ca with an invalid identity role must fail")

		var apiErr *rest_util.APIFormattedError
		ctx.Req.ErrorAs(err, &apiErr, "enrollment must fail with a well formed api error, not a truncated response")
		ctx.Req.NotEmpty(apiErr.Code)
	})

	t.Run("identityNameFormat should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		created, err := mgmtClient.CreateCa(caCreate)
		ctx.Req.NoError(err)

		update := newCaUpdate(caCreate)
		update.IdentityNameFormat = ToPtr("123")

		ctx.Req.NoError(mgmtClient.UpdateCa(created.ID, update))

		detail, err := mgmtClient.GetCa(created.ID)
		ctx.Req.NoError(err)
		ctx.Req.Equal("123", *detail.IdentityNameFormat)
	})

	t.Run("identity name format should default if not specified", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, _, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		caCreate.IdentityNameFormat = ""

		created, err := mgmtClient.CreateCa(caCreate)
		ctx.Req.NoError(err)

		detail, err := mgmtClient.GetCa(created.ID)
		ctx.Req.NoError(err)
		ctx.Req.Equal(model.DefaultCaIdentityNameFormat, *detail.IdentityNameFormat)
	})

	t.Run("identities from auto enrollment inherit CA identity roles", func(t *testing.T) {
		ctx.testContextChanged(t)

		role1 := eid.New()
		role2 := eid.New()

		caCert, caKey, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		caCreate.IdentityRoles = []string{role1, role2}

		_, err = mgmtClient.CreateAndVerifyCa(caCreate, caCert, caKey)
		ctx.Req.NoError(err)

		apiSession := requireCaAutoEnrollment(ctx, caCert, caKey, "")

		identity, err := mgmtClient.GetIdentity(apiSession.GetIdentityId())
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identity.RoleAttributes)
		ctx.Req.ElementsMatch([]string{role1, role2}, *identity.RoleAttributes)
	})

	t.Run("identities from auto enrollment use identity name format for naming", func(t *testing.T) {
		ctx.testContextChanged(t)

		expectedName := "singular.name.not.great"

		caCert, caKey, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		caCreate.IdentityNameFormat = expectedName

		_, err = mgmtClient.CreateAndVerifyCa(caCreate, caCert, caKey)
		ctx.Req.NoError(err)

		apiSession := requireCaAutoEnrollment(ctx, caCert, caKey, "")

		identity, err := mgmtClient.GetIdentity(apiSession.GetIdentityId())
		ctx.Req.NoError(err)
		ctx.Req.Equal(expectedName, *identity.Name)
	})

	t.Run("identities from auto enrollment identity name collisions add numbers to the end", func(t *testing.T) {
		ctx.testContextChanged(t)

		firstExpectedName := "some.static.name.no.replacements"
		secondExpectedName := "some.static.name.no.replacements000001"

		caCert, caKey, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		caCreate.IdentityNameFormat = firstExpectedName

		_, err = mgmtClient.CreateAndVerifyCa(caCreate, caCert, caKey)
		ctx.Req.NoError(err)

		firstSession := requireCaAutoEnrollment(ctx, caCert, caKey, "")

		firstIdentity, err := mgmtClient.GetIdentity(firstSession.GetIdentityId())
		ctx.Req.NoError(err)
		ctx.Req.Equal(firstExpectedName, *firstIdentity.Name)

		secondSession := requireCaAutoEnrollment(ctx, caCert, caKey, "")

		secondIdentity, err := mgmtClient.GetIdentity(secondSession.GetIdentityId())
		ctx.Req.NoError(err)
		ctx.Req.Equal(secondExpectedName, *secondIdentity.Name)
	})

	t.Run("identities from auto enrollment use identity name format for naming with replacements", func(t *testing.T) {
		ctx.testContextChanged(t)

		caCert, caKey, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caCreate := NewCaCreate(caPem)
		caCreate.IdentityNameFormat = "[caName] - [caId] - [commonName] - [requestedName] - [identityId]"

		caId, err := mgmtClient.CreateAndVerifyCa(caCreate, caCert, caKey)
		ctx.Req.NoError(err)

		commonName := eid.New()
		requestedName := "bobby"

		clientCert, clientKey, err := generateCaSignedClientCert(caCert, caKey, commonName)
		ctx.Req.NoError(err)

		clientApi := ctx.NewEdgeClientApi(nil)
		ctx.Req.NoError(clientApi.CompleteCaAutoEnrollment([]*x509.Certificate{clientCert}, clientKey, requestedName))

		apiSession, err := clientApi.Authenticate(edgeApis.NewCertCredentials([]*x509.Certificate{clientCert}, clientKey), nil)
		ctx.Req.NoError(err)

		identity, err := mgmtClient.GetIdentity(apiSession.GetIdentityId())
		ctx.Req.NoError(err)

		expectedName := fmt.Sprintf("%s - %s - %s - %s - %s",
			*caCreate.Name, caId, commonName, requestedName, apiSession.GetIdentityId())
		ctx.Req.Equal(expectedName, *identity.Name)
	})

	t.Run("CAs with auth enabled can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		caCert, caKey, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caId, err := mgmtClient.CreateAndVerifyCa(NewCaCreate(caPem), caCert, caKey)
		ctx.Req.NoError(err)

		clientCert, clientKey, err := generateCaSignedClientCert(caCert, caKey, eid.New())
		ctx.Req.NoError(err)

		enrollClientApi := ctx.NewEdgeClientApi(nil)
		ctx.Req.NoError(enrollClientApi.CompleteCaAutoEnrollment([]*x509.Certificate{clientCert}, clientKey, ""))

		certCreds := edgeApis.NewCertCredentials([]*x509.Certificate{clientCert}, clientKey)

		clientApi := ctx.NewEdgeClientApi(nil)
		apiSession, err := clientApi.Authenticate(certCreds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(apiSession)

		t.Run("auth from CA should not be extendable", func(t *testing.T) {
			ctx.testContextChanged(t)

			sessionDetail, err := clientApi.GetCurrentApiSessionDetail()
			ctx.Req.NoError(err)
			ctx.Req.NotNil(sessionDetail.IsCertExtendable)
			ctx.Req.False(*sessionDetail.IsCertExtendable, "expected isCertExtendable on 3rd party CA certificate authentication to be false")
		})

		t.Run("CAs with auth disabled can no longer authenticate", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.Req.NoError(mgmtClient.PatchCa(caId, &rest_model.CaPatch{IsAuthEnabled: ToPtr(false)}))

			deniedApi := ctx.NewEdgeClientApi(nil)
			deniedSession, err := deniedApi.Authenticate(certCreds, nil)

			ctx.Req.Error(err)
			ctx.Req.Nil(deniedSession)
		})

		t.Run("CAs with auth re-enabled an authenticate", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.Req.NoError(mgmtClient.PatchCa(caId, &rest_model.CaPatch{IsAuthEnabled: ToPtr(true)}))

			allowedApi := ctx.NewEdgeClientApi(nil)
			allowedSession, err := allowedApi.Authenticate(certCreds, nil)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(allowedSession)
		})

		t.Run("deleting a CA no longer allows authentication", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.Req.NoError(mgmtClient.DeleteCa(caId))

			deletedApi := ctx.NewEdgeClientApi(nil)
			deletedSession, err := deletedApi.Authenticate(certCreds, nil)

			ctx.Req.Error(err)
			ctx.Req.Nil(deletedSession)
		})
	})

	t.Run("deleting a CA should clean up outstanding enrollments", func(t *testing.T) {
		ctx.testContextChanged(t)

		caCert, caKey, caPem, err := newCaKeyPair()
		ctx.Req.NoError(err)

		caId, err := mgmtClient.CreateAndVerifyCa(NewCaCreate(caPem), caCert, caKey)
		ctx.Req.NoError(err)

		createdIdentity, err := mgmtClient.CreateIdentity(eid.New(), false)
		ctx.Req.NoError(err)

		createdEnrollment, err := mgmtClient.CreateEnrollmentOttCa(ToPtr(createdIdentity.ID), ToPtr(caId), ToPtr(time.Now().Add(time.Hour)))
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(createdEnrollment.ID)

		ctx.Req.NoError(mgmtClient.DeleteCa(caId))

		t.Run("enrollment should have been removed", func(t *testing.T) {
			ctx.testContextChanged(t)

			_, err := mgmtClient.GetEnrollment(createdEnrollment.ID)
			ctx.Req.Error(err, "expected enrollment to not be found")
		})

		t.Run("identities with previous enrollments tied to deleted CAs should not error on list", func(t *testing.T) {
			ctx.testContextChanged(t)

			identities, err := mgmtClient.ListIdentitiesByFilter(fmt.Sprintf(`id="%s"`, createdIdentity.ID))
			ctx.Req.NoError(err)
			ctx.Req.Len(identities, 1)
		})

		t.Run("identities with previous enrollments tied to deleted CAs should not error on detail", func(t *testing.T) {
			ctx.testContextChanged(t)

			identity, err := mgmtClient.GetIdentity(createdIdentity.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(createdIdentity.ID, *identity.ID)
		})
	})

	t.Run("can create a CA with externalIdClaim in common name, all, no parsing", func(t *testing.T) {
		ctx.testContextChanged(t)

		claim := &rest_model.ExternalIDClaim{
			Index:           ToPtr[int64](0),
			Location:        ToPtr(rest_model.ExternalIDClaimLocationCOMMONNAME),
			Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherALL),
			MatcherCriteria: ToPtr(""),
			Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
			ParserCriteria:  ToPtr(""),
		}

		created, err := createCaWithClaim(ctx, mgmtClient, claim)
		ctx.Req.NoError(err)

		t.Run("created ca values are correct", func(t *testing.T) {
			ctx.testContextChanged(t)

			detail, err := mgmtClient.GetCa(created.ID)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(detail.ExternalIDClaim)

			requireClaimEquals(ctx, claim, detail.ExternalIDClaim)
		})
	})

	t.Run("can create a CA with externalIdClaim in san uri, scheme, no parsing", func(t *testing.T) {
		ctx.testContextChanged(t)

		claim := &rest_model.ExternalIDClaim{
			Index:           ToPtr[int64](0),
			Location:        ToPtr(rest_model.ExternalIDClaimLocationSANURI),
			Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSCHEME),
			MatcherCriteria: ToPtr("spiffe"),
			Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
			ParserCriteria:  ToPtr(""),
		}

		created, err := createCaWithClaim(ctx, mgmtClient, claim)
		ctx.Req.NoError(err)

		t.Run("created ca values are correct", func(t *testing.T) {
			ctx.testContextChanged(t)

			detail, err := mgmtClient.GetCa(created.ID)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(detail.ExternalIDClaim)

			requireClaimEquals(ctx, claim, detail.ExternalIDClaim)
		})
	})

	t.Run("can create a CA with externalIdClaim in email, suffix, no parsing", func(t *testing.T) {
		ctx.testContextChanged(t)

		claim := &rest_model.ExternalIDClaim{
			Index:           ToPtr[int64](0),
			Location:        ToPtr(rest_model.ExternalIDClaimLocationSANEMAIL),
			Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSUFFIX),
			MatcherCriteria: ToPtr("@example.org"),
			Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
			ParserCriteria:  ToPtr(""),
		}

		created, err := createCaWithClaim(ctx, mgmtClient, claim)
		ctx.Req.NoError(err)

		t.Run("created ca values are correct", func(t *testing.T) {
			ctx.testContextChanged(t)

			detail, err := mgmtClient.GetCa(created.ID)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(detail.ExternalIDClaim)

			requireClaimEquals(ctx, claim, detail.ExternalIDClaim)
		})
	})

	t.Run("can create a CA with no externalIdClaim", func(t *testing.T) {
		ctx.testContextChanged(t)

		created, err := createCaWithClaim(ctx, mgmtClient, nil)
		ctx.Req.NoError(err)

		t.Run("created ca values are correct", func(t *testing.T) {
			ctx.testContextChanged(t)

			detail, err := mgmtClient.GetCa(created.ID)
			ctx.Req.NoError(err)
			ctx.Req.Nil(detail.ExternalIDClaim)
		})
	})

	t.Run("can not create a CA with externalIdClaim in email, scheme, no parsing", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, err := createCaWithClaim(ctx, mgmtClient, &rest_model.ExternalIDClaim{
			Index:           ToPtr[int64](0),
			Location:        ToPtr(rest_model.ExternalIDClaimLocationSANEMAIL),
			Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSCHEME),
			MatcherCriteria: ToPtr("@example.org"),
			Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
			ParserCriteria:  ToPtr(""),
		})

		requireBadRequest(ctx, err)
	})

	t.Run("can not create a CA with externalIdClaim with missing location", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, err := createCaWithClaim(ctx, mgmtClient, &rest_model.ExternalIDClaim{
			Index:           ToPtr[int64](0),
			Location:        nil,
			Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSCHEME),
			MatcherCriteria: ToPtr("@example.org"),
			Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
			ParserCriteria:  ToPtr(""),
		})

		requireBadRequest(ctx, err)
	})

	t.Run("can update a CA with externalIdClaim with a CN location, no parsing, all matcher", func(t *testing.T) {
		ctx.testContextChanged(t)

		created, err := createCaWithClaim(ctx, mgmtClient, nil)
		ctx.Req.NoError(err)

		t.Run("can patch externalIdClaim", func(t *testing.T) {
			ctx.testContextChanged(t)

			claimPatch := &rest_model.ExternalIDClaimPatch{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimLocationCOMMONNAME),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherALL),
				MatcherCriteria: ToPtr(""),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			}

			ctx.Req.NoError(mgmtClient.PatchCa(created.ID, &rest_model.CaPatch{ExternalIDClaim: claimPatch}))

			t.Run("patched ca values are correct", func(t *testing.T) {
				ctx.testContextChanged(t)

				detail, err := mgmtClient.GetCa(created.ID)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(detail.ExternalIDClaim)

				ctx.Req.Equal(*claimPatch.Index, *detail.ExternalIDClaim.Index)
				ctx.Req.Equal(*claimPatch.Location, *detail.ExternalIDClaim.Location)
				ctx.Req.Equal(*claimPatch.Matcher, *detail.ExternalIDClaim.Matcher)
				ctx.Req.Equal(claimPatch.MatcherCriteria, detail.ExternalIDClaim.MatcherCriteria)
				ctx.Req.Equal(*claimPatch.Parser, *detail.ExternalIDClaim.Parser)
				ctx.Req.Equal(claimPatch.ParserCriteria, detail.ExternalIDClaim.ParserCriteria)
			})
		})
	})

	t.Run("can update a CA with externalIdClaim with SAN location, SCHEME matcher, spiffe scheme, no parsing", func(t *testing.T) {
		ctx.testContextChanged(t)

		created, err := createCaWithClaim(ctx, mgmtClient, nil)
		ctx.Req.NoError(err)

		t.Run("can patch externalIdClaim", func(t *testing.T) {
			ctx.testContextChanged(t)

			claimPatch := &rest_model.ExternalIDClaimPatch{
				Index:           ToPtr[int64](0),
				Location:        ToPtr(rest_model.ExternalIDClaimPatchLocationSANURI),
				Matcher:         ToPtr(rest_model.ExternalIDClaimMatcherSCHEME),
				MatcherCriteria: ToPtr("spiffe"),
				Parser:          ToPtr(rest_model.ExternalIDClaimParserNONE),
				ParserCriteria:  ToPtr(""),
			}

			ctx.Req.NoError(mgmtClient.PatchCa(created.ID, &rest_model.CaPatch{ExternalIDClaim: claimPatch}))

			t.Run("patched ca values are correct", func(t *testing.T) {
				ctx.testContextChanged(t)

				detail, err := mgmtClient.GetCa(created.ID)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(detail.ExternalIDClaim)

				ctx.Req.Equal(*claimPatch.Index, *detail.ExternalIDClaim.Index)
				ctx.Req.Equal(*claimPatch.Location, *detail.ExternalIDClaim.Location)
				ctx.Req.Equal(*claimPatch.Matcher, *detail.ExternalIDClaim.Matcher)
				ctx.Req.Equal(claimPatch.MatcherCriteria, detail.ExternalIDClaim.MatcherCriteria)
				ctx.Req.Equal(*claimPatch.Parser, *detail.ExternalIDClaim.Parser)
				ctx.Req.Equal(claimPatch.ParserCriteria, detail.ExternalIDClaim.ParserCriteria)
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

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

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

			_, err := createCaWithClaim(ctx, mgmtClient, tc.claim)

			requireBadRequest(ctx, err)
		})
	}

	for _, tc := range invalidClaims {
		t.Run("can not patch a CA to "+tc.name, func(t *testing.T) {
			ctx.testContextChanged(t)

			created, err := createCaWithClaim(ctx, mgmtClient, nil)
			ctx.Req.NoError(err)

			err = mgmtClient.PatchCa(created.ID, &rest_model.CaPatch{
				ExternalIDClaim: &rest_model.ExternalIDClaimPatch{
					Index:           tc.claim.Index,
					Location:        tc.claim.Location,
					Matcher:         tc.claim.Matcher,
					MatcherCriteria: tc.claim.MatcherCriteria,
					Parser:          tc.claim.Parser,
					ParserCriteria:  tc.claim.ParserCriteria,
				},
			})

			requireBadRequest(ctx, err)
		})
	}
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

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

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
		created, err := createCaWithClaim(ctx, mgmtClient, baseClaim())
		ctx.Req.NoError(err)

		ctx.Req.NoError(mgmtClient.PatchCa(created.ID, &rest_model.CaPatch{
			Name:            ToPtr(eid.New()),
			ExternalIDClaim: &rest_model.ExternalIDClaimPatch{},
		}))

		detail, err := mgmtClient.GetCa(created.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(detail.ExternalIDClaim)
		ctx.Req.Equal("acme:", *detail.ExternalIDClaim.MatcherCriteria, "stored claim should be unchanged")
	})

	t.Run("a supplied subfield merges with stored values", func(t *testing.T) {
		ctx.testContextChanged(t)
		// Patching only matcherCriteria leaves location/matcher/parser as stored.
		created, err := createCaWithClaim(ctx, mgmtClient, baseClaim())
		ctx.Req.NoError(err)

		ctx.Req.NoError(mgmtClient.PatchCa(created.ID, &rest_model.CaPatch{
			ExternalIDClaim: &rest_model.ExternalIDClaimPatch{MatcherCriteria: ToPtr("widget:")},
		}))

		detail, err := mgmtClient.GetCa(created.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(detail.ExternalIDClaim)
		ctx.Req.Equal("widget:", *detail.ExternalIDClaim.MatcherCriteria)
		ctx.Req.Equal(rest_model.ExternalIDClaimLocationCOMMONNAME, *detail.ExternalIDClaim.Location, "location should be retained")
		ctx.Req.Equal(rest_model.ExternalIDClaimMatcherPREFIX, *detail.ExternalIDClaim.Matcher, "matcher should be retained")
	})

	t.Run("a subfield patch producing an invalid merged claim is rejected", func(t *testing.T) {
		ctx.testContextChanged(t)
		// SCHEME is invalid for COMMON_NAME; the merged result must be validated and rejected.
		created, err := createCaWithClaim(ctx, mgmtClient, baseClaim())
		ctx.Req.NoError(err)

		err = mgmtClient.PatchCa(created.ID, &rest_model.CaPatch{
			ExternalIDClaim: &rest_model.ExternalIDClaimPatch{Matcher: ToPtr(rest_model.ExternalIDClaimMatcherSCHEME)},
		})

		requireBadRequest(ctx, err)
	})

	t.Run("empty claim object on a CA with no claim is a no-op", func(t *testing.T) {
		ctx.testContextChanged(t)
		// Same old-CLI path against a CA that never had a claim: must not 400.
		created, err := createCaWithClaim(ctx, mgmtClient, nil)
		ctx.Req.NoError(err)

		ctx.Req.NoError(mgmtClient.PatchCa(created.ID, &rest_model.CaPatch{
			Name:            ToPtr(eid.New()),
			ExternalIDClaim: &rest_model.ExternalIDClaimPatch{},
		}))
	})

	t.Run("omitting the claim clears it", func(t *testing.T) {
		ctx.testContextChanged(t)
		// A patch with no externalIdClaim key removes the stored claim (delete-on-nil). This is the
		// signal the new --clear-external-id-claim CLI flag sends.
		created, err := createCaWithClaim(ctx, mgmtClient, baseClaim())
		ctx.Req.NoError(err)

		ctx.Req.NoError(mgmtClient.PatchCa(created.ID, &rest_model.CaPatch{Name: ToPtr(eid.New())}))

		detail, err := mgmtClient.GetCa(created.ID)
		ctx.Req.NoError(err)
		ctx.Req.Nil(detail.ExternalIDClaim, "claim should be cleared")
	})
}

// newCaUpdate builds a full CA update body carrying the create body's values. Update replaces every
// field, so a test that varies one has to send the rest unchanged.
func newCaUpdate(caCreate *rest_model.CaCreate) *rest_model.CaUpdate {
	return &rest_model.CaUpdate{
		IdentityNameFormat:        ToPtr(caCreate.IdentityNameFormat),
		IdentityRoles:             caCreate.IdentityRoles,
		IsAuthEnabled:             caCreate.IsAuthEnabled,
		IsAutoCaEnrollmentEnabled: caCreate.IsAutoCaEnrollmentEnabled,
		IsOttCaEnrollmentEnabled:  caCreate.IsOttCaEnrollmentEnabled,
		Name:                      caCreate.Name,
	}
}

// createCaWithClaim creates a CA carrying the supplied externalIdClaim, nil for none, returning the
// create result and any API error so the caller can assert on either.
func createCaWithClaim(ctx *TestContext, mgmtClient *ManagementHelperClient, claim *rest_model.ExternalIDClaim) (*rest_model.CreateLocation, error) {
	ctx.T().Helper()

	_, _, caPem, err := newCaKeyPair()
	ctx.Req.NoError(err)

	caCreate := NewCaCreate(caPem)
	caCreate.ExternalIDClaim = claim

	return mgmtClient.CreateCa(caCreate)
}

// requireCaAutoEnrollment mints a client certificate from the supplied CA, enrolls it, and returns
// the authenticated api session for the identity enrollment created.
func requireCaAutoEnrollment(ctx *TestContext, caCert *x509.Certificate, caKey crypto.Signer, requestedName string) edgeApis.ApiSession {
	ctx.T().Helper()

	clientCert, clientKey, err := generateCaSignedClientCert(caCert, caKey, eid.New())
	ctx.Req.NoError(err)

	clientApi := ctx.NewEdgeClientApi(nil)
	ctx.Req.NoError(clientApi.CompleteCaAutoEnrollment([]*x509.Certificate{clientCert}, clientKey, requestedName))

	apiSession, err := clientApi.Authenticate(edgeApis.NewCertCredentials([]*x509.Certificate{clientCert}, clientKey), nil)
	ctx.Req.NoError(err)

	return apiSession
}

// seedCaIdentityRoles writes identity roles straight into a CA's entity bucket, bypassing
// PersistEntity. It stands in for a CA stored before the prefixes were validated, which the API can
// no longer create.
func seedCaIdentityRoles(ctx *TestContext, caId string, roles []string) {
	ctx.T().Helper()

	appEnv := ctx.EdgeController.AppEnv
	err := appEnv.GetDb().Update(nil, func(mc boltz.MutateContext) error {
		bucket := appEnv.GetStores().Ca.GetEntityBucket(mc.Tx(), []byte(caId))
		if bucket == nil {
			return fmt.Errorf("no ca entity bucket for %v", caId)
		}
		bucket.SetStringList(db.FieldIdentityRoles, roles, nil)
		return bucket.GetError()
	})
	ctx.Req.NoError(err)
}

// requireIdentityRolesFieldError asserts err is the 400 field error the API returns for an identity
// role carrying a reserved policy prefix.
func requireIdentityRolesFieldError(ctx *TestContext, err error, expectedValue string) {
	ctx.T().Helper()

	ctx.Req.Error(err)

	var apiErr *rest_util.APIFormattedError
	ctx.Req.ErrorAs(err, &apiErr)
	ctx.Req.Equal(errorz.CouldNotValidateCode, apiErr.Code)
	ctx.Req.NotNil(apiErr.Cause)
	ctx.Req.Equal("identityRoles", apiErr.Cause.Field)
	ctx.Req.Equal(expectedValue, apiErr.Cause.Value)
}

// requireBadRequest asserts err carries an API error with a 400 status.
func requireBadRequest(ctx *TestContext, err error) {
	ctx.T().Helper()

	ctx.Req.Error(err)

	var apiErr *rest_util.APIFormattedError
	ctx.Req.ErrorAs(err, &apiErr)
	ctx.Req.NotEmpty(apiErr.Code)
}

// requireClaimEquals asserts a stored externalIdClaim matches the one supplied at create time.
func requireClaimEquals(ctx *TestContext, expected *rest_model.ExternalIDClaim, actual *rest_model.ExternalIDClaim) {
	ctx.T().Helper()

	ctx.Req.Equal(*expected.Index, *actual.Index)
	ctx.Req.Equal(*expected.Location, *actual.Location)
	ctx.Req.Equal(*expected.Matcher, *actual.Matcher)
	ctx.Req.Equal(expected.MatcherCriteria, actual.MatcherCriteria)
	ctx.Req.Equal(*expected.Parser, *actual.Parser)
	ctx.Req.Equal(expected.ParserCriteria, actual.ParserCriteria)
}
