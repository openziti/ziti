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
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/identity/certtools"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
)

// generateCsrPem creates a new ECDSA P-384 key pair and a PKCS#10 certificate signing
// request encoded as PEM. It returns the PEM string, the private key, and any error.
func generateCsrPem() (string, crypto.PrivateKey, error) {
	p384 := elliptic.P384()
	privateKey, err := ecdsa.GenerateKey(p384, rand.Reader)
	if err != nil {
		return "", nil, err
	}

	request, err := certtools.NewCertRequest(map[string]string{
		"C": "US", "O": "TEST", "CN": "test-csr",
	}, nil)
	if err != nil {
		return "", nil, err
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, request, privateKey)
	if err != nil {
		return "", nil, err
	}

	csrPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))
	return csrPem, privateKey, nil
}

// certFingerprint computes the SHA-1 fingerprint of a DER-encoded certificate,
// formatted as a lowercase hex string without separators.
func certFingerprint(cert *x509.Certificate) string {
	return fmt.Sprintf("%x", sha1.Sum(cert.Raw))
}

func Test_Authenticate_OIDC_CSR(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	clientHelper := ctx.NewEdgeClientApi(nil)

	managementHelper := ctx.NewEdgeManagementApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)

	t.Run("updb auth with CSR returns session cert", func(t *testing.T) {
		ctx.NextTest(t)

		csrPem, _, err := generateCsrPem()
		ctx.Req.NoError(err)

		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/username?id=" + result.AuthRequestId

		payload := &oidc_auth.OidcUpdbCreds{
			Authenticate: rest_model.Authenticate{
				Password: rest_model.Password(ctx.AdminAuthenticator.Password),
				Username: rest_model.Username(ctx.AdminAuthenticator.Username),
			},
			AuthRequestBody: oidc_auth.AuthRequestBody{
				AuthRequestId: result.AuthRequestId,
			},
			CsrPem: csrPem,
		}

		resp, err := result.Client.R().SetBody(payload).Post(opLoginUri)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusFound, resp.StatusCode())

		locUrl, parseErr := url.Parse(resp.Header().Get("Location"))
		ctx.Req.NoError(parseErr)
		code := locUrl.Query().Get("code")
		ctx.Req.NotEmpty(code)

		tokens, err := result.Exchange(code)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)

		t.Run("access token has CSR cert fingerprint", func(t *testing.T) {
			ctx.NextTest(t)

			parser := jwt.NewParser()
			accessClaims := &common.AccessClaims{}
			_, _, err := parser.ParseUnverified(tokens.AccessToken, accessClaims)
			ctx.Req.NoError(err)

			ctx.Req.Len(accessClaims.CertFingerprints, 1, "updb + CSR should have exactly one cert fingerprint (from the CSR)")
			ctx.Req.Empty(accessClaims.AuthCertFingerprint, "updb auth should have no auth cert fingerprints")
		})
	})

	t.Run("updb auth without CSR has no session cert", func(t *testing.T) {
		ctx.NextTest(t)

		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/username?id=" + result.AuthRequestId

		payload := &oidc_auth.OidcUpdbCreds{
			Authenticate: rest_model.Authenticate{
				Password: rest_model.Password(ctx.AdminAuthenticator.Password),
				Username: rest_model.Username(ctx.AdminAuthenticator.Username),
			},
			AuthRequestBody: oidc_auth.AuthRequestBody{
				AuthRequestId: result.AuthRequestId,
			},
		}

		resp, err := result.Client.R().SetBody(payload).Post(opLoginUri)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusFound, resp.StatusCode())

		locUrl, parseErr := url.Parse(resp.Header().Get("Location"))
		ctx.Req.NoError(parseErr)
		code := locUrl.Query().Get("code")
		ctx.Req.NotEmpty(code)

		tokens, err := result.Exchange(code)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)

		t.Run("access token has no cert fingerprints", func(t *testing.T) {
			ctx.NextTest(t)

			parser := jwt.NewParser()
			accessClaims := &common.AccessClaims{}
			_, _, err := parser.ParseUnverified(tokens.AccessToken, accessClaims)
			ctx.Req.NoError(err)

			ctx.Req.Empty(accessClaims.CertFingerprints, "updb without CSR should have no cert fingerprints")
			ctx.Req.Empty(accessClaims.AuthCertFingerprint, "updb auth should have no auth cert fingerprints")
		})
	})

	t.Run("cert auth with CSR returns session cert", func(t *testing.T) {
		ctx.NextTest(t)

		_, certCreds, err := managementHelper.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		certCreds.CaPool = ctx.ControllerCaPool()

		authCertFp := certFingerprint(certCreds.Certs[0])

		csrPem, _, err := generateCsrPem()
		ctx.Req.NoError(err)

		result, err := clientHelper.OidcAuthorize(certCreds)
		ctx.Req.NoError(err)

		opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/cert"

		payload := &oidc_auth.OidcUpdbCreds{
			AuthRequestBody: oidc_auth.AuthRequestBody{
				AuthRequestId: result.AuthRequestId,
			},
			CsrPem: csrPem,
		}

		resp, err := result.Client.R().SetBody(payload).Post(opLoginUri)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusFound, resp.StatusCode())

		locUrl, parseErr := url.Parse(resp.Header().Get("Location"))
		ctx.Req.NoError(parseErr)
		code := locUrl.Query().Get("code")
		ctx.Req.NotEmpty(code)

		tokens, err := result.Exchange(code)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)

		t.Run("access token has auth cert and CSR cert fingerprints", func(t *testing.T) {
			ctx.NextTest(t)

			parser := jwt.NewParser()
			accessClaims := &common.AccessClaims{}
			_, _, err := parser.ParseUnverified(tokens.AccessToken, accessClaims)
			ctx.Req.NoError(err)

			ctx.Req.Len(accessClaims.CertFingerprints, 2,
				"cert auth + CSR should have two cert fingerprints (auth cert + CSR cert)")
			ctx.Req.NotEmpty(accessClaims.AuthCertFingerprint,
				"cert auth should have an auth cert fingerprint")

			ctx.Req.Equal(authCertFp, accessClaims.AuthCertFingerprint,
				"auth cert fingerprint should match the authenticating certificate")
			ctx.Req.Contains(accessClaims.CertFingerprints, authCertFp,
				"CertFingerprints should contain the auth cert fingerprint")
		})
	})

	t.Run("cert auth without CSR has only client cert", func(t *testing.T) {
		ctx.NextTest(t)

		_, certCreds, err := managementHelper.CreateAndEnrollOttIdentity(false)
		ctx.Req.NoError(err)
		certCreds.CaPool = ctx.ControllerCaPool()

		authCertFp := certFingerprint(certCreds.Certs[0])

		result, err := clientHelper.OidcAuthorize(certCreds)
		ctx.Req.NoError(err)

		opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/cert"

		payload := &oidc_auth.OidcUpdbCreds{
			AuthRequestBody: oidc_auth.AuthRequestBody{
				AuthRequestId: result.AuthRequestId,
			},
		}

		resp, err := result.Client.R().SetBody(payload).Post(opLoginUri)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusFound, resp.StatusCode())

		locUrl, parseErr := url.Parse(resp.Header().Get("Location"))
		ctx.Req.NoError(parseErr)
		code := locUrl.Query().Get("code")
		ctx.Req.NotEmpty(code)

		tokens, err := result.Exchange(code)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)

		t.Run("access token has only auth cert fingerprint", func(t *testing.T) {
			ctx.NextTest(t)

			parser := jwt.NewParser()
			accessClaims := &common.AccessClaims{}
			_, _, err := parser.ParseUnverified(tokens.AccessToken, accessClaims)
			ctx.Req.NoError(err)

			ctx.Req.Len(accessClaims.CertFingerprints, 1,
				"cert auth without CSR should have exactly one cert fingerprint")
			ctx.Req.NotEmpty(accessClaims.AuthCertFingerprint,
				"cert auth should have an auth cert fingerprint")

			ctx.Req.Equal(authCertFp, accessClaims.CertFingerprints[0],
				"CertFingerprints should contain the auth cert fingerprint")
			ctx.Req.Equal(authCertFp, accessClaims.AuthCertFingerprint,
				"AuthCertFingerprint should match the auth cert fingerprint")
		})
	})

	t.Run("invalid CSR returns error during code exchange", func(t *testing.T) {
		ctx.NextTest(t)

		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/username?id=" + result.AuthRequestId

		payload := &oidc_auth.OidcUpdbCreds{
			Authenticate: rest_model.Authenticate{
				Password: rest_model.Password(ctx.AdminAuthenticator.Password),
				Username: rest_model.Username(ctx.AdminAuthenticator.Username),
			},
			AuthRequestBody: oidc_auth.AuthRequestBody{
				AuthRequestId: result.AuthRequestId,
			},
			CsrPem: "not-a-valid-csr",
		}

		resp, err := result.Client.R().SetBody(payload).Post(opLoginUri)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusFound, resp.StatusCode(),
			"login POST should succeed with 302 even with invalid CSR")

		locUrl, parseErr := url.Parse(resp.Header().Get("Location"))
		ctx.Req.NoError(parseErr)
		code := locUrl.Query().Get("code")
		ctx.Req.NotEmpty(code, "authorization code should be present in the redirect")

		_, exchangeErr := result.Exchange(code)
		ctx.Req.Error(exchangeErr, "code exchange should fail when the CSR is invalid")
	})
}
