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
	"crypto/x509"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	managementAuthenticator "github.com/openziti/edge-api/rest_management_api_client/authenticator"
	edgeRouterApi "github.com/openziti/edge-api/rest_management_api_client/edge_router"
	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
)

// Test_Identity_Certificate_ExtKeyUsage locks in the extended key usages the controller puts on the
// certificates it issues. Every identity client certificate carries both clientAuth and serverAuth,
// because one end of a DirectE2EE connection presents its certificate as a TLS server certificate
// and the Apple and Windows platform verifiers reject a clientAuth-only leaf in that role. Router
// certificates are not used that way and keep the usages they have always had.
func Test_Identity_Certificate_ExtKeyUsage(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminManagementClient := ctx.NewEdgeManagementApi(nil)
	adminCredentials := ctx.NewAdminCredentials()
	adminApiSession, err := adminManagementClient.Authenticate(adminCredentials, nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(adminApiSession)

	identityUsages := []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}

	t.Run("an ott enrolled identity certificate carries clientAuth and serverAuth", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity, certCredentials, err := adminManagementClient.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identity)
		ctx.Req.NotEmpty(certCredentials.Certs)

		ctx.Req.ElementsMatch(identityUsages, certCredentials.Certs[0].ExtKeyUsage,
			"an ott enrolled identity certificate must carry clientAuth and serverAuth")
	})

	t.Run("a token enrolled identity certificate carries clientAuth and serverAuth", func(t *testing.T) {
		ctx.testContextChanged(t)

		authPolicy := createAuthPolicyComponents("eku-token-enrollment")
		authPolicy.Create.Primary.Cert.Allowed = ToPtr(true)
		authPolicy.Detail, err = adminManagementClient.CreateAuthPolicy(authPolicy.Create)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(authPolicy.Detail)

		extJwtSigner := createExtJwtComponents("eku-token-enrollment")
		extJwtSigner.Create.EnrollToCertEnabled = true
		extJwtSigner.Create.EnrollAuthPolicyID = *authPolicy.Detail.ID
		extJwtSigner.Create.EnrollAttributeClaimsSelector = ""
		extJwtSigner.Create.EnrollNameClaimsSelector = ""
		extJwtSigner.Create.ClaimsProperty = nil
		extJwtSigner.Detail, err = adminManagementClient.CreateExtJwtSigner(extJwtSigner.Create)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(extJwtSigner.Detail)

		enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSigner, &claimsWithAttributes{})
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(enrollmentJwt)

		clientApi := ctx.NewEdgeClientApi(nil)
		certCredentials, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(certCredentials.Certs)
		ctx.Req.ElementsMatch(identityUsages, certCredentials.Certs[0].ExtKeyUsage,
			"a token enrolled identity certificate must carry clientAuth and serverAuth")
	})

	t.Run("an extended identity certificate carries clientAuth and serverAuth", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity, certCredentials, err := adminManagementClient.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identity)

		authenticators, err := adminManagementClient.GetIdentityAuthenticators(*identity.ID)
		ctx.Req.NoError(err)
		ctx.Req.Len(authenticators, 1)

		extendParams := managementAuthenticator.NewRequestExtendAuthenticatorParams()
		extendParams.ID = *authenticators[0].ID
		extendParams.RequestExtendAuthenticator = &rest_model.RequestExtendAuthenticator{
			RollKeys: false,
		}
		_, err = adminManagementClient.API.Authenticator.RequestExtendAuthenticator(extendParams, nil)
		ctx.Req.NoError(err)

		identityClient := ctx.NewEdgeClientApi(nil)
		identityApiSession, err := identityClient.Authenticate(certCredentials, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityApiSession)

		extendedCredentials, err := identityClient.ExtendCertsWithAuthenticatorId(*authenticators[0].ID)

		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(extendedCredentials.Certs)
		ctx.Req.ElementsMatch(identityUsages, extendedCredentials.Certs[0].ExtKeyUsage,
			"an extended identity certificate must carry clientAuth and serverAuth")
	})

	t.Run("an api session certificate carries clientAuth and serverAuth", func(t *testing.T) {
		ctx.testContextChanged(t)

		identity, certCredentials, err := adminManagementClient.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identity)

		identityClient := ctx.NewEdgeClientApi(nil)
		identityApiSession, err := identityClient.Authenticate(certCredentials, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityApiSession)

		sessionCert, err := identityClient.CreateCurrentApiSessionCertificate()

		ctx.Req.NoError(err)
		ctx.Req.NotNil(sessionCert.Certificate)

		sessionCerts := nfpem.PemStringToCertificates(*sessionCert.Certificate)
		ctx.Req.NotEmpty(sessionCerts)
		ctx.Req.ElementsMatch(identityUsages, sessionCerts[0].ExtKeyUsage,
			"an api session certificate must carry clientAuth and serverAuth")
	})

	t.Run("an enrolled router client certificate carries clientAuth only", func(t *testing.T) {
		ctx.testContextChanged(t)

		createdRouter, err := adminManagementClient.API.EdgeRouter.CreateEdgeRouter(&edgeRouterApi.CreateEdgeRouterParams{
			EdgeRouter: &rest_model.EdgeRouterCreate{
				Name: ToPtr(uuid.NewString()),
			},
		}, nil)
		ctx.Req.NoError(err)

		routerDetail, err := adminManagementClient.GetEdgeRouter(createdRouter.Payload.Data.ID)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(routerDetail.EnrollmentToken)

		privateKey := generateEcKey()

		clientCsrPem, err := createEnrollmentClientCsrPem(*routerDetail.ID, privateKey)
		ctx.Req.NoError(err)

		serverCsrPem, err := createEnrollmentServerCsrPem(*routerDetail.ID, privateKey)
		ctx.Req.NoError(err)

		// the generic enroll endpoint is the only one that accepts both router CSRs; the generated
		// EnrollErOtt operation carries no server CSR field
		enrollmentResp, err := ctx.newAnonymousClientApiRequest().SetBody(map[string]string{
			"certCsr":       clientCsrPem,
			"serverCertCsr": serverCsrPem,
		}).Post("/enroll?method=erott&token=" + *routerDetail.EnrollmentToken)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, enrollmentResp.StatusCode())

		enrollmentCerts := &routerEnrollmentCertsResponse{}
		ctx.Req.NoError(json.Unmarshal(enrollmentResp.Body(), enrollmentCerts))

		routerCerts := nfpem.PemStringToCertificates(enrollmentCerts.Data.Cert)
		ctx.Req.NotEmpty(routerCerts)

		ctx.Req.Equal([]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, routerCerts[0].ExtKeyUsage,
			"a router client certificate is never presented as a TLS server certificate and must keep clientAuth alone")
	})
}

// routerEnrollmentCertsResponse matches the certificate bundle the generic enroll endpoint returns
// for an erott router enrollment.
type routerEnrollmentCertsResponse struct {
	Data struct {
		Cert       string `json:"cert"`
		ServerCert string `json:"serverCert"`
	} `json:"data"`
}
