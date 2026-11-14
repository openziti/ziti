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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/ziti/v2/common/eid"
)

// Test_Authenticate_Cert_ExternalId_Admin verifies that a certificate from a third party CA can log in as
// the identity its externalId claim names, unless that identity is an admin.
func Test_Authenticate_Cert_ExternalId_Admin(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	caCert, caSigner, caPEM := newTestCaCert()

	// issues a client certificate from the CA with the given common name
	issue := func(commonName string) *certAuthenticator {
		clientSigner, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		ctx.Req.NoError(err)

		template := &x509.Certificate{
			NotBefore:             time.Now().Add(-time.Minute),
			NotAfter:              time.Now().Add(time.Hour),
			SerialNumber:          big.NewInt(time.Now().UnixNano()),
			Subject:               pkix.Name{CommonName: commonName},
			KeyUsage:              x509.KeyUsageDigitalSignature,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			BasicConstraintsValid: true,
		}

		rawCert, err := x509.CreateCertificate(rand.Reader, template, caCert, &clientSigner.PublicKey, caSigner)
		ctx.Req.NoError(err)

		clientCert, err := x509.ParseCertificate(rawCert)
		ctx.Req.NoError(err)

		return &certAuthenticator{
			certs: []*x509.Certificate{clientCert},
			key:   clientSigner,
		}
	}

	createIdentity := func(externalId string, isAdmin bool) {
		identityCreate := &rest_model.IdentityCreate{
			Name:       ToPtr(eid.New()),
			IsAdmin:    ToPtr(isAdmin),
			Type:       ToPtr(rest_model.IdentityTypeUser),
			ExternalID: ToPtr(externalId),
		}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityCreate).Post("/identities")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
	}

	// a verified CA with auth enabled that takes the externalId from the common name
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
		IsAutoCaEnrollmentEnabled: ToPtr(false),
		IsOttCaEnrollmentEnabled:  ToPtr(false),
		Name:                      ToPtr(eid.New()),
	}

	caCreateResult := &rest_model.CreateEnvelope{}
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(caCreate).SetResult(caCreateResult).Post("/cas")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

	caValues := ctx.AdminManagementSession.requireQuery("cas/" + caCreateResult.Data.ID)
	verificationToken := caValues.Path("data.verificationToken").Data().(string)
	ctx.Req.NotEmpty(verificationToken)

	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().
		SetHeader("content-type", "text/plain").
		SetBody(nfpem.EncodeToBytes(issue(verificationToken).certs[0])).
		Post("cas/" + caCreateResult.Data.ID + "/verify")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

	t.Run("a non-admin identity can authenticate by externalId", func(t *testing.T) {
		ctx.testContextChanged(t)

		externalId := eid.New()
		createIdentity(externalId, false)

		session, err := issue(externalId).AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(session)
	})

	t.Run("an admin identity cannot authenticate by externalId", func(t *testing.T) {
		ctx.testContextChanged(t)

		externalId := eid.New()
		createIdentity(externalId, true)

		for _, authenticate := range []func(*TestContext) (*session, error){
			issue(externalId).AuthenticateClientApi,
			issue(externalId).AuthenticateManagementApi,
		} {
			_, err := authenticate(ctx)
			ctx.Req.Error(err)
		}
	})
}
