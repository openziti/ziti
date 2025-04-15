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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/edge-api/rest_client_api_client/current_api_session"
	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/identity/certtools"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/ziti/util"
	"gopkg.in/resty.v1"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func Test_EnrollmentIdentityExtend(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	clientApiUrl := ctx.ClientApiUrl()

	t.Run("using oidc auth", func(t *testing.T) {

		t.Run("can call extend multiple times", func(t *testing.T) {
			ctx.testContextChanged(t)

			name := eid.New()
			_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(name, false)
			ctx.Req.NotNil(identityAuth)

			newPrivateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
			ctx.NoError(err)

			request, err := certtools.NewCertRequest(map[string]string{
				"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.certs[0].Subject.CommonName,
			}, nil)
			ctx.NoError(err)

			csr, err := x509.CreateCertificateRequest(rand.Reader, request, newPrivateKey)
			ctx.Req.NoError(err)

			csrPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))

			clientApi := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
				strings <- "123"
			})

			clientApi.SetUseOidc(true)

			origCertCreds := edge_apis.NewCertCredentials(identityAuth.certs, identityAuth.key)

			apiSession, err := clientApi.Authenticate(origCertCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(apiSession)
			apiSessionToken := apiSession.GetToken()
			ctx.Req.True(strings.HasPrefix(string(apiSession.GetToken()), "ey"), "expected OIDC auth, which results in a JWT")

			jwtParser := jwt.NewParser()
			accessClaims := &common.AccessClaims{}

			_, _, err = jwtParser.ParseUnverified(string(apiSessionToken), accessClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(accessClaims.AuthenticatorId)
			ctx.Req.True(accessClaims.IsCertExtendable)

			t.Run("the first extend succeeds", func(t *testing.T) {
				ctx.testContextChanged(t)

				authenticatorExtendParams := current_api_session.NewExtendCurrentIdentityAuthenticatorParams()
				authenticatorExtendParams.ID = accessClaims.AuthenticatorId
				authenticatorExtendParams.Extend = &rest_model.IdentityExtendEnrollmentRequest{
					ClientCertCsr: &csrPem,
				}

				extendResp, err := clientApi.API.CurrentAPISession.ExtendCurrentIdentityAuthenticator(authenticatorExtendParams, nil)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(extendResp)

				t.Run("the second extend succeeds", func(t *testing.T) {
					ctx.testContextChanged(t)

					extendResp2, err := clientApi.API.CurrentAPISession.ExtendCurrentIdentityAuthenticator(authenticatorExtendParams, nil)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(extendResp2)
				})
			})

		})

		t.Run("valid cert and csr extends enrollment", func(t *testing.T) {
			ctx.testContextChanged(t)

			name := eid.New()
			_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(name, false)

			ctx.Req.NotNil(identityAuth)

			newPrivateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
			ctx.NoError(err)

			request, err := certtools.NewCertRequest(map[string]string{
				"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.certs[0].Subject.CommonName,
			}, nil)
			ctx.NoError(err)

			csr, err := x509.CreateCertificateRequest(rand.Reader, request, newPrivateKey)
			ctx.Req.NoError(err)

			csrPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))

			clientApi := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
				strings <- "123"
			})

			clientApi.SetUseOidc(true)

			origCertCreds := edge_apis.NewCertCredentials(identityAuth.certs, identityAuth.key)

			apiSession, err := clientApi.Authenticate(origCertCreds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(apiSession)
			apiSessionToken := apiSession.GetToken()
			ctx.Req.True(strings.HasPrefix(string(apiSession.GetToken()), "ey"), "expected OIDC auth, which results in a JWT")

			jwtParser := jwt.NewParser()
			accessClaims := &common.AccessClaims{}

			_, _, err = jwtParser.ParseUnverified(string(apiSessionToken), accessClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(accessClaims.AuthenticatorId)
			ctx.Req.True(accessClaims.IsCertExtendable)

			listAuthenticatorsParams := current_api_session.NewListCurrentIdentityAuthenticatorsParams()
			authenticatorListResp, err := clientApi.API.CurrentAPISession.ListCurrentIdentityAuthenticators(listAuthenticatorsParams, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(authenticatorListResp)
			ctx.Req.NotNil(authenticatorListResp.Payload)
			ctx.Req.Len(authenticatorListResp.Payload.Data, 1)

			authenticatorExtendParams := current_api_session.NewExtendCurrentIdentityAuthenticatorParams()
			authenticatorExtendParams.ID = accessClaims.AuthenticatorId
			authenticatorExtendParams.Extend = &rest_model.IdentityExtendEnrollmentRequest{
				ClientCertCsr: &csrPem,
			}

			extendResp, err := clientApi.API.CurrentAPISession.ExtendCurrentIdentityAuthenticator(authenticatorExtendParams, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extendResp)

			newCerts := nfpem.PemStringToCertificates(extendResp.Payload.Data.ClientCert)
			ctx.Req.Len(newCerts, 2)

			newCertCreds := edge_apis.NewCertCredentials(newCerts, newPrivateKey)

			t.Run("new cert used for auth fails pre verify", func(t *testing.T) {
				ctx.testContextChanged(t)

				newApiSession, err := clientApi.Authenticate(newCertCreds, nil)

				ctx.Req.Error(err)
				ctx.Req.Nil(newApiSession)

				t.Run("can verify using original credentials", func(t *testing.T) {
					ctx.testContextChanged(t)

					newApiSession, err := clientApi.Authenticate(origCertCreds, nil)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(newApiSession)

					authenticatorExtendVerifyParams := current_api_session.NewExtendVerifyCurrentIdentityAuthenticatorParams()

					authenticatorExtendVerifyParams.ID = accessClaims.AuthenticatorId
					authenticatorExtendVerifyParams.Extend = &rest_model.IdentityExtendValidateEnrollmentRequest{
						ClientCert: &extendResp.Payload.Data.ClientCert,
					}

					verifyResp, err := clientApi.API.CurrentAPISession.ExtendVerifyCurrentIdentityAuthenticator(authenticatorExtendVerifyParams, nil)
					ctx.Req.NoError(util.WrapIfApiError(err))
					ctx.Req.NotNil(verifyResp)

					t.Run("after verification old cert fails", func(t *testing.T) {
						newApiSession, err := clientApi.Authenticate(origCertCreds, nil)
						ctx.Req.Error(err)
						ctx.Req.Nil(newApiSession)

						t.Run("after verification new cert succeeds", func(t *testing.T) {
							ctx.testContextChanged(t)

							newApiSession, err := clientApi.Authenticate(newCertCreds, nil)

							ctx.Req.NoError(err)
							ctx.Req.NotNil(newApiSession)
						})

					})
				})
			})
		})
	})

	t.Run("using legacy auth valid cert and csr extends enrollment", func(t *testing.T) {
		ctx.testContextChanged(t)

		name := eid.New()
		_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(name, false)
		identityApiSession, err := identityAuth.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		newPrivateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		ctx.NoError(err)

		request, err := certtools.NewCertRequest(map[string]string{
			"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.certs[0].Subject.CommonName,
		}, nil)
		ctx.NoError(err)

		csr, err := x509.CreateCertificateRequest(rand.Reader, request, newPrivateKey)
		ctx.Req.NoError(err)

		csrPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))

		csrRequest := &rest_model.IdentityExtendEnrollmentRequest{
			ClientCertCsr: &csrPem,
		}

		listAuthResp, err := identityApiSession.NewRequest().Get("current-identity/authenticators")
		ctx.Req.NoError(err)
		ctx.Req.Equal(200, listAuthResp.StatusCode())

		authenticatorsEnvelope := rest_model.ListAuthenticatorsEnvelope{}
		ctx.Req.NoError(json.Unmarshal(listAuthResp.Body(), &authenticatorsEnvelope))
		ctx.Req.Len(authenticatorsEnvelope.Data, 1)
		currentAuthenticator := authenticatorsEnvelope.Data[0]

		extendUrl := fmt.Sprintf("/current-identity/authenticators/%s/extend", *currentAuthenticator.ID)
		extendResp, err := identityApiSession.NewRequest().SetBody(csrRequest).Post(extendUrl)
		ctx.Req.NoError(err)
		ctx.Req.Equal(200, extendResp.StatusCode())

		respEnvelope := &rest_model.IdentityExtendEnrollmentEnvelope{}
		ctx.Req.NoError(json.Unmarshal(extendResp.Body(), respEnvelope))
		ctx.Req.NotNil(respEnvelope.Data)

		newClientCerts := nfpem.PemStringToCertificates(respEnvelope.Data.ClientCert)
		ctx.Req.Len(newClientCerts, 2)

		t.Run("old cert works pre verify", func(t *testing.T) {
			ctx.testContextChanged(t)
			_, err := identityAuth.AuthenticateClientApi(ctx)
			ctx.Req.NoError(err)
		})

		t.Run("new cert used for auth fails pre verify", func(t *testing.T) {
			ctx.testContextChanged(t)
			origCert := identityAuth.certs
			origKey := identityAuth.key

			identityAuth.certs = newClientCerts
			identityAuth.key = newPrivateKey
			_, err := identityAuth.AuthenticateClientApi(ctx)
			ctx.Req.Error(err)

			identityAuth.certs = origCert
			identityAuth.key = origKey
		})

		t.Run("verifying", func(t *testing.T) {
			ctx.testContextChanged(t)

			t.Run("with an mangled client cert fails", func(t *testing.T) {
				ctx.testContextChanged(t)

				mangledPem := "I am very truly not PEM data"

				verifyRequest := &rest_model.IdentityExtendValidateEnrollmentRequest{
					ClientCert: &mangledPem,
				}

				extendVerifyUrl := fmt.Sprintf("/current-identity/authenticators/%s/extend-verify", *currentAuthenticator.ID)
				verifyResp, err := identityApiSession.NewRequest().SetBody(verifyRequest).Post(extendVerifyUrl)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusBadRequest, verifyResp.StatusCode())
			})

			t.Run("with an the old client cert fails", func(t *testing.T) {
				ctx.testContextChanged(t)

				oldCertPem := nfpem.EncodeToString(identityAuth.certs[0])

				verifyRequest := &rest_model.IdentityExtendValidateEnrollmentRequest{
					ClientCert: &oldCertPem,
				}

				extendVerifyUrl := fmt.Sprintf("/current-identity/authenticators/%s/extend-verify", *currentAuthenticator.ID)
				verifyResp, err := identityApiSession.NewRequest().SetBody(verifyRequest).Post(extendVerifyUrl)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusBadRequest, verifyResp.StatusCode())
			})

			t.Run("with the correct client cert succeeds", func(t *testing.T) {
				ctx.testContextChanged(t)
				verifyRequest := &rest_model.IdentityExtendValidateEnrollmentRequest{
					ClientCert: &respEnvelope.Data.ClientCert,
				}

				extendVerifyUrl := fmt.Sprintf("/current-identity/authenticators/%s/extend-verify", *currentAuthenticator.ID)
				verifyResp, err := identityApiSession.NewRequest().SetBody(verifyRequest).Post(extendVerifyUrl)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, verifyResp.StatusCode(), "expected %d, got %d, body: %s", http.StatusOK, verifyResp.Status(), verifyResp.Body())

				t.Run("old cert used for auth fails post verify", func(t *testing.T) {
					ctx.testContextChanged(t)
					_, err := identityAuth.AuthenticateClientApi(ctx)
					ctx.Req.Error(err)
				})

				t.Run("new cert used for auth succeeds post verify", func(t *testing.T) {
					ctx.testContextChanged(t)
					identityAuth.certs = newClientCerts
					identityAuth.key = newPrivateKey
					_, err := identityAuth.AuthenticateClientApi(ctx)
					ctx.Req.NoError(err)
				})
			})

		})

	})

	t.Run("no client cert used on request errors", func(t *testing.T) {
		ctx.testContextChanged(t)

		name := eid.New()
		_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(name, false)
		identityApiSession, err := identityAuth.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		newPrivateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		ctx.Req.NoError(err)

		request, err := certtools.NewCertRequest(map[string]string{
			"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.certs[0].Subject.CommonName,
		}, nil)
		ctx.Req.NoError(err)

		csr, err := x509.CreateCertificateRequest(rand.Reader, request, newPrivateKey)
		ctx.Req.NoError(err)

		csrPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))

		csrRequest := &rest_model.IdentityExtendEnrollmentRequest{
			ClientCertCsr: &csrPem,
		}

		listAuthResp, err := identityApiSession.NewRequest().Get("current-identity/authenticators")
		ctx.Req.NoError(err)
		ctx.Req.Equal(200, listAuthResp.StatusCode())

		authenticatorsEnvelope := rest_model.ListAuthenticatorsEnvelope{}
		ctx.Req.NoError(json.Unmarshal(listAuthResp.Body(), &authenticatorsEnvelope))
		ctx.Req.Len(authenticatorsEnvelope.Data, 1)
		currentAuthenticator := authenticatorsEnvelope.Data[0]

		path := fmt.Sprintf("/edge/client/v1/current-identity/authenticators/%s/extend", *currentAuthenticator.ID)
		resolvedUrl, err := identityApiSession.resolveApiUrl(ctx.ApiHost, path)
		ctx.Req.NoError(err)

		client := resty.New().SetTLSClientConfig(&tls.Config{
			InsecureSkipVerify: true,
		})
		extendResp, err := client.R().SetHeader(env.ZitiSession, *identityApiSession.AuthResponse.Token).SetBody(csrRequest).Post(resolvedUrl)
		ctx.Req.NoError(err)
		ctx.Req.Equal(401, extendResp.StatusCode())
	})

	t.Run("wrong client cert used on request errors", func(t *testing.T) {
		ctx.testContextChanged(t)

		name := eid.New()
		_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(name, false)
		identityApiSession, err := identityAuth.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)

		_, secondIdentity := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false)
		secondIdentityApiSession, err := secondIdentity.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)

		newPrivateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		ctx.Req.NoError(err)

		request, err := certtools.NewCertRequest(map[string]string{
			"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.certs[0].Subject.CommonName,
		}, nil)
		ctx.Req.NoError(err)

		csr, err := x509.CreateCertificateRequest(rand.Reader, request, newPrivateKey)
		ctx.Req.NoError(err)

		csrPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))

		csrRequest := &rest_model.IdentityExtendEnrollmentRequest{
			ClientCertCsr: &csrPem,
		}

		listAuthResp, err := identityApiSession.NewRequest().Get("current-identity/authenticators")
		ctx.Req.NoError(err)
		ctx.Req.Equal(200, listAuthResp.StatusCode())

		authenticatorsEnvelope := rest_model.ListAuthenticatorsEnvelope{}
		ctx.Req.NoError(json.Unmarshal(listAuthResp.Body(), &authenticatorsEnvelope))
		ctx.Req.Len(authenticatorsEnvelope.Data, 1)
		currentAuthenticator := authenticatorsEnvelope.Data[0]

		extendUrl := fmt.Sprintf("/current-identity/authenticators/%s/extend", *currentAuthenticator.ID)
		//use second identity http client w/ first identity's API Session
		extendResp, err := secondIdentityApiSession.NewRequest().SetHeader(env.ZitiSession, *identityApiSession.AuthResponse.Token).SetBody(csrRequest).Post(extendUrl)
		ctx.Req.NoError(err)
		ctx.Req.Equal(401, extendResp.StatusCode())
	})

	t.Run("mangled CSR errors", func(t *testing.T) {
		ctx.testContextChanged(t)

		name := eid.New()
		_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(name, false)
		identityApiSession, err := identityAuth.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)

		_, secondIdentityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false)
		secondIdentityApiSession, err := secondIdentityAuth.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)

		csrPem := "-----BEGIN NEW CERTIFICATE REQUEST-----\nOhHelloThere\n-----END NEW CERTIFICATE REQUEST-----"
		csrRequest := &rest_model.IdentityExtendEnrollmentRequest{
			ClientCertCsr: &csrPem,
		}

		//get authenticators from a different identity
		listAuthResp, err := secondIdentityApiSession.NewRequest().Get("current-identity/authenticators")
		ctx.Req.NoError(err)
		ctx.Req.Equal(200, listAuthResp.StatusCode())

		authenticatorsEnvelope := rest_model.ListAuthenticatorsEnvelope{}
		ctx.Req.NoError(json.Unmarshal(listAuthResp.Body(), &authenticatorsEnvelope))
		ctx.Req.Len(authenticatorsEnvelope.Data, 1)
		authenticatorFromDifferentIdentity := authenticatorsEnvelope.Data[0]

		extendUrl := fmt.Sprintf("/current-identity/authenticators/%s/extend", *authenticatorFromDifferentIdentity.ID)
		extendResp, err := identityApiSession.NewRequest().SetBody(csrRequest).Post(extendUrl)
		ctx.Req.NoError(err)
		ctx.Req.Equal(401, extendResp.StatusCode())
	})

	t.Run("invalid authenticator id errors", func(t *testing.T) {
		ctx.testContextChanged(t)

		name := eid.New()
		_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(name, false)
		identityApiSession, err := identityAuth.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)

		newPrivateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		ctx.Req.NoError(err)

		request, err := certtools.NewCertRequest(map[string]string{
			"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.certs[0].Subject.CommonName,
		}, nil)
		ctx.Req.NoError(err)

		csr, err := x509.CreateCertificateRequest(rand.Reader, request, newPrivateKey)
		ctx.Req.NoError(err)

		csrPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))

		csrRequest := &rest_model.IdentityExtendEnrollmentRequest{
			ClientCertCsr: &csrPem,
		}

		path := fmt.Sprintf("/current-identity/authenticators/%s/extend", "fake")
		resolvedUrl, err := identityApiSession.resolveApiUrl(ctx.ApiHost, path)
		ctx.Req.NoError(err)

		extendResp, err := identityApiSession.NewRequest().SetBody(csrRequest).Post(resolvedUrl)
		ctx.Req.NoError(err)
		ctx.Req.Equal(404, extendResp.StatusCode())
	})

	t.Run("authenticator owned by another identity errors", func(t *testing.T) {
		ctx.testContextChanged(t)

		name := eid.New()
		_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(name, false)
		identityApiSession, err := identityAuth.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)

		_, secondIdentityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false)
		secondIdentityApiSession, err := secondIdentityAuth.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)

		newPrivateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		ctx.Req.NoError(err)

		request, err := certtools.NewCertRequest(map[string]string{
			"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.certs[0].Subject.CommonName,
		}, nil)
		ctx.Req.NoError(err)

		csr, err := x509.CreateCertificateRequest(rand.Reader, request, newPrivateKey)
		ctx.Req.NoError(err)

		csrPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))

		csrRequest := &rest_model.IdentityExtendEnrollmentRequest{
			ClientCertCsr: &csrPem,
		}

		//get authenticators from a different identity
		listAuthResp, err := secondIdentityApiSession.NewRequest().Get("current-identity/authenticators")
		ctx.Req.NoError(err)
		ctx.Req.Equal(200, listAuthResp.StatusCode())

		authenticatorsEnvelope := rest_model.ListAuthenticatorsEnvelope{}
		ctx.Req.NoError(json.Unmarshal(listAuthResp.Body(), &authenticatorsEnvelope))
		ctx.Req.Len(authenticatorsEnvelope.Data, 1)
		authenticatorFromDifferentIdentity := authenticatorsEnvelope.Data[0]

		extendUrl := fmt.Sprintf("/current-identity/authenticators/%s/extend", *authenticatorFromDifferentIdentity.ID)
		extendResp, err := identityApiSession.NewRequest().SetBody(csrRequest).Post(extendUrl)
		ctx.Req.NoError(err)
		ctx.Req.Equal(401, extendResp.StatusCode())
	})
}
