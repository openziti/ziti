//go:build apitests
// +build apitests

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
	"crypto/tls"
	"encoding/pem"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/common/eid"
	"net/http"
	"strings"
	"testing"
)

// Test_EnrollmetnCaAutoSpecific uses the ca specific enrollment endpoint rather than the generic enroll endpoint.
func Test_EnrollmetnCaAutoSpecific(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("ca auto enrollment", func(t *testing.T) {

		t.Run("setup CA", func(t *testing.T) {
			testCa := newTestCa()

			testCa.externalIdClaim = &externalIdClaim{
				location:        rest_model.ExternalIDClaimLocationCOMMONNAME,
				matcher:         rest_model.ExternalIDClaimMatcherALL,
				matcherCriteria: "",
				parser:          rest_model.ExternalIDClaimParserNONE,
				parserCriteria:  "",
				index:           0,
			}

			testCa.identityNameFormat = "[requestedName]"

			ctx.testContextChanged(t)

			testCaId := ctx.AdminManagementSession.requireCreateEntity(testCa)
			ctx.Req.NotEmpty(testCaId)

			caContainer := ctx.AdminManagementSession.requireQuery("cas/" + testCaId)
			ctx.Req.NotEmpty(caContainer)

			token := caContainer.Path("data.verificationToken").Data().(string)
			ctx.Req.NotEmpty(token)

			verifyCert, _, err := generateCaSignedClientCert(testCa.publicCert, testCa.privateKey, token)
			ctx.Req.NoError(err)

			verificationBlock := &pem.Block{
				Type:  "CERTIFICATE",
				Bytes: verifyCert.Raw,
			}
			verifyPem := pem.EncodeToMemory(verificationBlock)

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetHeader("content-type", "text/plain").SetBody(verifyPem).Post("cas/" + testCaId + "/verify")
			ctx.Req.NoError(err)
			standardJsonResponseTests(resp, http.StatusOK, t)

			t.Run("can enroll without a name and a 0 length body with externalId claim", func(t *testing.T) {
				ctx.testContextChanged(t)

				commonName := "test-can-enroll-" + eid.New()
				clientCert, clientKey, err := generateCaSignedClientCert(testCa.publicCert, testCa.privateKey, commonName)
				ctx.Req.NoError(err)

				restClient, _, transport := ctx.NewClientComponents(EdgeClientApiPath)
				transport.TLSClientConfig.Certificates = []tls.Certificate{
					{
						Certificate: [][]byte{clientCert.Raw},
						PrivateKey:  clientKey,
					},
				}

				resp, err := restClient.R().
					SetHeader("content-type", "application/json").
					Post("enroll?method=ca")

				ctx.Req.NoError(err)

				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				contentTypeHeaders := resp.Header().Values("content-type")
				ctx.Req.Equal(1, len(contentTypeHeaders), "expected only 1 content type header")

				contentType := strings.Split(contentTypeHeaders[0], ";")[0]
				ctx.Req.Equal("application/json", contentType)

				t.Run("can authenticate", func(t *testing.T) {
					ctx.testContextChanged(t)

					clientCertAuth := &certAuthenticator{
						cert: clientCert,
						key:  clientKey,
					}

					apiSession, err := clientCertAuth.AuthenticateClientApi(ctx)
					ctx.NoError(err)
					ctx.NotNil(apiSession)

					t.Run("externalId is set properly", func(t *testing.T) {
						ctx.testContextChanged(t)

						identityGet := &rest_model.DetailIdentityEnvelope{}

						resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(identityGet).Get("/identities/" + *apiSession.AuthResponse.IdentityID)
						ctx.NoError(err)
						ctx.Equal(http.StatusOK, resp.StatusCode())
						ctx.NotNil(identityGet)
						ctx.NotNil(identityGet.Data)
						ctx.NotNil(identityGet.Data.ExternalID)
						ctx.Equal(*identityGet.Data.ExternalID, commonName)

					})
				})
			})

			t.Run("can enroll without a name and empty JSON object without externalId claim", func(t *testing.T) {
				ctx.testContextChanged(t)
				cert, key, err := generateCaSignedClientCert(testCa.publicCert, testCa.privateKey, "test-can-enroll-"+eid.New())
				ctx.Req.NoError(err)

				restClient, _, transport := ctx.NewClientComponents(EdgeClientApiPath)
				transport.TLSClientConfig.Certificates = []tls.Certificate{
					{
						Certificate: [][]byte{cert.Raw},
						PrivateKey:  key,
					},
				}

				resp, err := restClient.R().
					SetHeader("content-type", "application/json").
					SetBody("{}").
					Post("enroll?method=ca")

				ctx.Req.NoError(err)

				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				contentTypeHeaders := resp.Header().Values("content-type")
				ctx.Req.Equal(1, len(contentTypeHeaders), "expected only 1 content type header")

				contentType := strings.Split(contentTypeHeaders[0], ";")[0]
				ctx.Req.Equal("application/json", contentType)
			})

			t.Run("can enroll with a name without externalId claim", func(t *testing.T) {
				t.Run("can enroll with", func(t *testing.T) {
					ctx.testContextChanged(t)
					cert, key, err := generateCaSignedClientCert(testCa.publicCert, testCa.privateKey, "test-can-enroll-"+eid.New())
					ctx.Req.NoError(err)

					name := eid.New()
					restClient, _, transport := ctx.NewClientComponents(EdgeClientApiPath)
					transport.TLSClientConfig.Certificates = []tls.Certificate{
						{
							Certificate: [][]byte{cert.Raw},
							PrivateKey:  key,
						},
					}

					resp, err := restClient.R().
						SetHeader("content-type", "application/json").
						SetBody(`{"name": "` + name + `"}`).
						Post("enroll?method=ca")

					ctx.Req.NoError(err)

					ctx.Req.Equal(http.StatusOK, resp.StatusCode())

					t.Run("the name was given to the identity", func(t *testing.T) {
						ctx.testContextChanged(t)

						resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/identities")

						ctx.Req.NoError(err)
						standardJsonResponseTests(resp, http.StatusOK, t)

						envelope := rest_model.ListIdentitiesEnvelope{}

						err = envelope.UnmarshalBinary(resp.Body())
						ctx.Req.NoError(err)
						found := false
						for _, identity := range envelope.Data {
							if *identity.Name == name {
								found = true
								break
							}
						}

						ctx.Req.True(found, "identity with name not found")
					})
				})
			})
		})

	})
}
