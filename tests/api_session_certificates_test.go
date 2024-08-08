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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_client_api_client/current_api_session"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/identity/certtools"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/common/spiffehlp"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

func Test_Api_Session_Certs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	t.Run("as the default admin, session certs, using legacy authentication", func(t *testing.T) {
		var createdResponseBody *gabs.Container //used across multiple subtests, set in first
		var createdId string                    //used across multiple subtests, set in first

		t.Run("can be created", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.RequireAdminManagementApiLogin()
			ctx.RequireAdminClientApiLogin()
			csr, _, err := generateCsr()
			ctx.Req.NoError(err)

			csrPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})
			ctx.Req.NotEmpty(csrPem)

			request := ctx.AdminClientSession.newAuthenticatedRequest()

			body := gabs.New()
			_, err = body.Set(string(csrPem), "csr")
			ctx.Req.NoError(err)
			bodyStr := body.String()
			request.SetBody(bodyStr)

			resp, err := request.Post("current-api-session/certificates")

			ctx.Req.NoError(err)
			standardJsonResponseTests(resp, http.StatusCreated, t)

			createdResponseBody, err = gabs.ParseJSON(resp.Body())
			ctx.Req.NoError(err)

			var ok bool
			createdId, ok = createdResponseBody.Path("data.id").Data().(string)
			ctx.Req.True(ok)
			ctx.Req.NotEmpty(createdId)

			certificate := createdResponseBody.Path("data.certificate").Data().(string)
			ctx.Req.NotEmpty(certificate)

			x509Cert, err := certtools.LoadCert([]byte(certificate))
			ctx.Req.NotEmpty(x509Cert)
			ctx.Req.NoError(err)

		})

		t.Run("can be listed", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminClientSession.newAuthenticatedRequest().Get("current-api-session/certificates")
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			t.Run("list contains the API session cert", func(t *testing.T) {
				ctx.testContextChanged(t)

				bodyStr := resp.Body()
				body, err := gabs.ParseJSON(bodyStr)
				ctx.Req.NoError(err)

				data := body.Path("data")
				ctx.Req.NotEmpty(data)

				children, err := data.Children()
				ctx.Req.NoError(err)

				ctx.Req.Len(children, 1)

				subject := children[0].Path("subject").Data().(string)
				ctx.Req.NotEmpty(subject)

				fingerprint := children[0].Path("fingerprint").Data().(string)
				ctx.Req.NotEmpty(fingerprint)

				validFrom := children[0].Path("validFrom").Data().(string)
				ctx.Req.NotEmpty(validFrom)

				validTo := children[0].Path("validTo").Data().(string)
				ctx.Req.NotEmpty(validTo)

				certificate := children[0].Path("certificate").Data().(string)
				ctx.Req.NotEmpty(certificate)

				x509Cert, err := certtools.LoadCert([]byte(certificate))
				ctx.Req.NotEmpty(x509Cert)
				ctx.Req.NoError(err)
			})
		})

		t.Run("can be detailed", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminClientSession.newAuthenticatedRequest().Get("current-api-session/certificates/" + createdId)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			t.Run("contains proper values", func(t *testing.T) {
				ctx.testContextChanged(t)

				bodyStr := resp.Body()
				body, err := gabs.ParseJSON(bodyStr)
				ctx.Req.NoError(err)

				data := body.Path("data")
				ctx.Req.NotEmpty(data)

				subject := data.Path("subject").Data().(string)
				ctx.Req.NotEmpty(subject)

				fingerprint := data.Path("fingerprint").Data().(string)
				ctx.Req.NotEmpty(fingerprint)

				validFrom := data.Path("validFrom").Data().(string)
				ctx.Req.NotEmpty(validFrom)

				validTo := data.Path("validTo").Data().(string)
				ctx.Req.NotEmpty(validTo)

				certificate := data.Path("certificate").Data().(string)
				ctx.Req.NotEmpty(certificate)

				x509Cert, err := certtools.LoadCert([]byte(certificate))
				ctx.Req.NotEmpty(x509Cert)
				ctx.Req.NoError(err)
			})
		})

		t.Run("can be deleted", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminClientSession.newAuthenticatedRequest().Delete("current-api-session/certificates/" + createdId)
			ctx.Req.NoError(err)

			standardJsonResponseTests(resp, http.StatusOK, t)
		})
	})

	t.Run("as admin using oidc authentication", func(t *testing.T) {
		ctx.testContextChanged(t)

		adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)

		clientApiUrl, err := url.Parse("https://" + ctx.ApiHost + EdgeClientApiPath)
		ctx.Req.NoError(err)

		adminClientClient := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
			strings <- "123"
		})

		adminClientClient.SetUseOidc(true)
		adminClientApiSession, err := adminClientClient.Authenticate(adminCreds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminClientApiSession)

		managementApiUrl, err := url.Parse("https://" + ctx.ApiHost + EdgeManagementApiPath)
		ctx.Req.NoError(err)

		adminManagementClient := edge_apis.NewManagementApiClient([]*url.URL{managementApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
			strings <- "123"
		})

		adminManagementClient.SetUseOidc(true)
		adminManagementApiSession, err := adminManagementClient.Authenticate(adminCreds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminManagementApiSession)

		t.Run("can create a new session certificate", func(t *testing.T) {
			ctx.testContextChanged(t)

			csrBytes, privateKey, err := generateCsr()

			ctx.Req.NoError(err)
			ctx.Req.NotNil(privateKey)
			ctx.Req.NotEmpty(csrBytes)

			csrPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})
			ctx.Req.NotEmpty(csrPem)

			csr := string(csrPem)

			params := &current_api_session.CreateCurrentAPISessionCertificateParams{
				SessionCertificate: &rest_model.CurrentAPISessionCertificateCreate{
					Csr: &csr,
				},
			}

			resp, err := adminClientClient.API.CurrentAPISession.CreateCurrentAPISessionCertificate(params, nil)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.NotNil(resp.Payload)
			ctx.Req.NotNil(resp.Payload.Data)
			ctx.Req.NotNil(resp.Payload.Data.Certificate)

			t.Run("a parsable certificate chain is returned", func(t *testing.T) {
				ctx.testContextChanged(t)
				certs, err := parsePEMBundle([]byte(*resp.Payload.Data.Certificate))

				ctx.Req.NoError(err)
				ctx.Req.True(len(certs) > 0, "no certificates found")

				t.Run("the first certificate is a leaf", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.False(certs[0].IsCA, "expected IsCA to be false and indicate a leaf certificate")
				})

				t.Run("the first certificate has the client authentication usage", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.Req.True(certs[0].KeyUsage&x509.KeyUsageDigitalSignature > 0, "expected key usage digital signature")
					ctx.Req.True(certs[0].KeyUsage&x509.KeyUsageKeyEncipherment > 0, "expected key usage key encipherment")
					ctx.Req.Contains(certs[0].ExtKeyUsage, x509.ExtKeyUsageClientAuth)
				})

				t.Run("the response includes a certificate chain", func(t *testing.T) {
					ctx.testContextChanged(t)

					ctx.Req.True(len(certs) > 1, "no certificate chain found, potentially a leaf only result")

					t.Run("the chain is in signer order", func(t *testing.T) {
						ctx.testContextChanged(t)
						var prevCert *x509.Certificate
						for i, cert := range certs {
							if prevCert != nil {
								ctx.Req.True(cert.IsCA, "expected certificate at index %d to be a CA as it is not the first (leaf) certificate", i)
								ctx.Req.Equal(cert.Subject.String(), prevCert.Subject.String(), "expected certificate subject to equal previous certificates issuer")
								ctx.Req.NoError(prevCert.CheckSignatureFrom(cert), "expected the previous cert to be signed by the current cert, but got error: %v", err)
							}
						}
					})

					t.Run("the last certificate is not a root certificate", func(t *testing.T) {
						ctx.testContextChanged(t)
						lastCert := certs[len(certs)-1]
						ctx.Req.True(lastCert.IsCA, "expected the last certificate in the chain to be a CA")
						ctx.Req.True(lastCert.Subject.String() != lastCert.Issuer.String(), "expected the last certificate subject to not equal the last certificate (indicates a root is present)")
					})

				})

				t.Run("the certificate contains the proper SPIFFE id", func(t *testing.T) {
					ctx.testContextChanged(t)

					spiffeId, err := spiffehlp.GetSpiffeIdFromCert(certs[0])

					ctx.Req.NoError(err)
					ctx.Req.NotEmpty(spiffeId)

					apiSession := *adminClientClient.ApiSession.Load()
					apiSessionId := apiSession.GetId()
					identityId := apiSession.GetIdentityId()
					expectedId := fmt.Sprintf("%s/identity/%s/apiSession/%s/apiSessionCertificate/", ctx.ControllerConfig.SpiffeIdTrustDomain, identityId, apiSessionId)
					ctx.Req.True(strings.HasPrefix(spiffeId.String(), expectedId), "expected the spiffe id to have the prefix %s, but got %s", expectedId, spiffeId)

				})
			})

		})
	})
}

func generateCsr() ([]byte, crypto.PrivateKey, error) {
	p384 := elliptic.P384()
	pfxlog.Logger().Infof("generating %s EC key", p384.Params().Name)
	privateKey, err := ecdsa.GenerateKey(p384, rand.Reader)

	if err != nil {
		return nil, nil, err
	}

	hostname, err := os.Hostname()

	if err != nil {
		return nil, nil, err
	}

	request, err := certtools.NewCertRequest(map[string]string{
		"C": "US", "O": "TEST", "CN": hostname,
	}, nil)

	if err != nil {
		return nil, nil, err
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, request, privateKey)

	if err != nil {
		return nil, nil, err
	}

	return csr, privateKey, nil
}
