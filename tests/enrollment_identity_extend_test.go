//go:build apitests
// +build apitests

/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/foundation/common/constants"
	"github.com/openziti/foundation/identity/certtools"
	nfpem "github.com/openziti/foundation/util/pem"
	"gopkg.in/resty.v1"
	"net/http"
	"testing"
)

func Test_EnrollmentIdentityExtend(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("valid cert and csr extends enrollment", func(t *testing.T) {
		ctx.testContextChanged(t)

		name := eid.New()
		_, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(name, false)
		identityApiSession, err := identityAuth.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		newPrivateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		request, err := certtools.NewCertRequest(map[string]string{
			"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.cert.Subject.CommonName,
		}, nil)

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

		url := fmt.Sprintf("/current-identity/authenticators/%s/extend", *currentAuthenticator.ID)
		extendResp, err := identityApiSession.NewRequest().SetBody(csrRequest).Post(url)
		ctx.Req.NoError(err)
		ctx.Req.Equal(200, extendResp.StatusCode())

		respEnvelope := &rest_model.IdentityExtendEnrollmentEnvelope{}
		ctx.Req.NoError(json.Unmarshal(extendResp.Body(), respEnvelope))
		ctx.Req.NotNil(respEnvelope.Data)

		newClientCerts := nfpem.PemStringToCertificates(respEnvelope.Data.ClientCert)
		ctx.Req.Len(newClientCerts, 1)

		t.Run("old cert works pre verify", func(t *testing.T) {
			ctx.testContextChanged(t)
			_, err := identityAuth.AuthenticateClientApi(ctx)
			ctx.Req.NoError(err)
		})

		t.Run("new cert used for auth fails pre verify", func(t *testing.T) {
			ctx.testContextChanged(t)
			origCert := identityAuth.cert
			origKey := identityAuth.key

			identityAuth.cert = newClientCerts[0]
			identityAuth.key = newPrivateKey
			_, err := identityAuth.AuthenticateClientApi(ctx)
			ctx.Req.Error(err)

			identityAuth.cert = origCert
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

				url := fmt.Sprintf("/current-identity/authenticators/%s/extend-verify", *currentAuthenticator.ID)
				verifyResp, err := identityApiSession.NewRequest().SetBody(verifyRequest).Post(url)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusBadRequest, verifyResp.StatusCode())
			})

			t.Run("with an the old client cert fails", func(t *testing.T) {
				ctx.testContextChanged(t)

				oldCertPem := nfpem.EncodeToString(identityAuth.cert)

				verifyRequest := &rest_model.IdentityExtendValidateEnrollmentRequest{
					ClientCert: &oldCertPem,
				}

				url := fmt.Sprintf("/current-identity/authenticators/%s/extend-verify", *currentAuthenticator.ID)
				verifyResp, err := identityApiSession.NewRequest().SetBody(verifyRequest).Post(url)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusBadRequest, verifyResp.StatusCode())
			})

			t.Run("with the correct client cert succeeds", func(t *testing.T) {
				ctx.testContextChanged(t)
				verifyRequest := &rest_model.IdentityExtendValidateEnrollmentRequest{
					ClientCert: &respEnvelope.Data.ClientCert,
				}

				url := fmt.Sprintf("/current-identity/authenticators/%s/extend-verify", *currentAuthenticator.ID)
				verifyResp, err := identityApiSession.NewRequest().SetBody(verifyRequest).Post(url)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, verifyResp.StatusCode())

				t.Run("old cert used for auth fails post verify", func(t *testing.T) {
					ctx.testContextChanged(t)
					_, err := identityAuth.AuthenticateClientApi(ctx)
					ctx.Req.Error(err)
				})

				t.Run("new cert used for auth succeeds post verify", func(t *testing.T) {
					ctx.testContextChanged(t)
					identityAuth.cert = newClientCerts[0]
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
		request, err := certtools.NewCertRequest(map[string]string{
			"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.cert.Subject.CommonName,
		}, nil)

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

		path := fmt.Sprintf("/current-identity/authenticators/%s/extend", *currentAuthenticator.ID)
		resolvedUrl, err := identityApiSession.resolveApiUrl(ctx.ApiHost, path)
		client := resty.New().SetTLSClientConfig(&tls.Config{
			InsecureSkipVerify: true,
		})
		extendResp, err := client.R().SetHeader(constants.ZitiSession, identityApiSession.token).SetBody(csrRequest).Post(resolvedUrl)
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
		request, err := certtools.NewCertRequest(map[string]string{
			"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.cert.Subject.CommonName,
		}, nil)

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

		url := fmt.Sprintf("/current-identity/authenticators/%s/extend", *currentAuthenticator.ID)
		//use second identity http client w/ first identity's API Session
		extendResp, err := secondIdentityApiSession.NewRequest().SetHeader(constants.ZitiSession, identityApiSession.token).SetBody(csrRequest).Post(url)
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

		url := fmt.Sprintf("/current-identity/authenticators/%s/extend", *authenticatorFromDifferentIdentity.ID)
		extendResp, err := identityApiSession.NewRequest().SetBody(csrRequest).Post(url)
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
		request, err := certtools.NewCertRequest(map[string]string{
			"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.cert.Subject.CommonName,
		}, nil)

		csr, err := x509.CreateCertificateRequest(rand.Reader, request, newPrivateKey)
		ctx.Req.NoError(err)

		csrPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))

		csrRequest := &rest_model.IdentityExtendEnrollmentRequest{
			ClientCertCsr: &csrPem,
		}

		path := fmt.Sprintf("/current-identity/authenticators/%s/extend", "fake")
		resolvedUrl, err := identityApiSession.resolveApiUrl(ctx.ApiHost, path)

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
		request, err := certtools.NewCertRequest(map[string]string{
			"C": "US", "O": "NetFoundry-API-Test", "CN": identityAuth.cert.Subject.CommonName,
		}, nil)

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

		url := fmt.Sprintf("/current-identity/authenticators/%s/extend", *authenticatorFromDifferentIdentity.ID)
		extendResp, err := identityApiSession.NewRequest().SetBody(csrRequest).Post(url)
		ctx.Req.NoError(err)
		ctx.Req.Equal(401, extendResp.StatusCode())
	})

}
