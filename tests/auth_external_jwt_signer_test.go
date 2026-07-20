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
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/ziti/v2/controller/model"
)

type jsonWebKey struct {
	Kid string   `json:"kid"`
	X5C []string `json:"x5c"`
}

type jsonWebKeysResponse struct {
	Keys []jsonWebKey `json:"keys"`
}

type jwksServer struct {
	server       *http.Server
	port         int
	certificates []*x509.Certificate
	mutex        sync.Mutex
	requestCount int
	listener     net.Listener
	tlsCert      *tls.Certificate
}

func newJwksServer(certificates []*x509.Certificate) *jwksServer {
	srv := &jwksServer{
		certificates: certificates,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/jwks", srv.handleJWKS)
	srv.server = &http.Server{
		Handler: mux,
	}
	return srv
}

// newTlsJwksServer returns a jwksServer that serves its JWKS over HTTPS using tlsCert as its
// server certificate. Callers must ensure the controller trusts tlsCert (see the root CA added
// to http.DefaultTransport in the overlapping-kid test) for the controller's JWKS fetch to succeed.
func newTlsJwksServer(tlsCert *tls.Certificate, certificates []*x509.Certificate) *jwksServer {
	srv := newJwksServer(certificates)
	srv.tlsCert = tlsCert
	srv.server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{*tlsCert}}
	return srv
}

func (js *jwksServer) AddCertificate(certificate *x509.Certificate) {
	js.mutex.Lock()
	defer js.mutex.Unlock()

	js.certificates = append(js.certificates, certificate)
}

func (js *jwksServer) RemoveCertificate(certificate *x509.Certificate) {
	js.mutex.Lock()
	defer js.mutex.Unlock()

	for i, cert := range js.certificates {
		if cert.Equal(certificate) {
			js.certificates = append(js.certificates[:i], js.certificates[i+1:]...)
			break
		}
	}
}

func (js *jwksServer) GetJwksUrl() string {
	scheme := "http"
	if js.tlsCert != nil {
		scheme = "https"
	}
	return scheme + "://localhost:" + strconv.Itoa(js.port) + "/jwks"
}

func (js *jwksServer) GetRequestCount() int {
	js.mutex.Lock()
	defer js.mutex.Unlock()

	return js.requestCount
}

func (js *jwksServer) Start() error {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	js.mutex.Lock()
	js.listener = listener
	js.port = listener.Addr().(*net.TCPAddr).Port
	js.mutex.Unlock()
	go func() {
		if js.tlsCert != nil {
			_ = js.server.ServeTLS(listener, "", "")
		} else {
			_ = js.server.Serve(listener)
		}
	}()

	return nil
}

func (js *jwksServer) Stop() error {
	return js.server.Close()
}

func (js *jwksServer) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	js.mutex.Lock()
	defer js.mutex.Unlock()

	js.requestCount = js.requestCount + 1

	var keys []jsonWebKey
	for _, cert := range js.certificates {

		certBase64 := base64.StdEncoding.EncodeToString(cert.Raw)
		key := jsonWebKey{
			Kid: cert.Subject.CommonName,
			X5C: []string{certBase64},
		}
		keys = append(keys, key)
	}

	response := jsonWebKeysResponse{Keys: keys}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func Test_Authenticate_External_Jwt(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	var signerIds []string

	jwksServer := newJwksServer(nil)
	err := jwksServer.Start()
	ctx.Req.NoError(err)
	defer func() {
		_ = jwksServer.Stop()
	}()

	// create a bunch of signers to use

	// valid signer using a jwks endpoint
	validJwksSignerCert1, validJwksSignerPrivateKey1 := newSelfSignedCert("valid jwks signer 1")
	validJwksSignerCert2, validJwksSignerPrivateKey2 := newSelfSignedCert("valid jwks signer 2")
	jwksEndpoint := strfmt.URI(jwksServer.GetJwksUrl())

	jwksServer.AddCertificate(validJwksSignerCert1)

	validJwksSigner := &rest_model.ExternalJWTSignerCreate{
		JwksEndpoint: &jwksEndpoint,
		Enabled:      ToPtr(true),
		Name:         ToPtr("Test JWT Signer - JWKS - Enabled"),
		Issuer:       ToPtr("the-very-best-iss-jwks"),
		Audience:     ToPtr("the-very-best-aud-jwks"),
	}

	createResponseEnv := &rest_model.CreateEnvelope{}

	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(validJwksSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

	signerIds = append(signerIds, createResponseEnv.Data.ID)

	//valid signer with issuer and audience
	validJwtSignerCert, validJwtSignerPrivateKey := newSelfSignedCert("valid signer")
	validJwtSignerCertPem := nfpem.EncodeToString(validJwtSignerCert)

	validJwtSignerAuthUrl := "https://valid.jwt.signer.url.example.com"
	validJwtSignerClientId := "valid-client-id"
	validJwtSignerScopes := []string{"valid-scope1", "valid-scope2"}

	validJwtSigner := &rest_model.ExternalJWTSignerCreate{
		CertPem:         &validJwtSignerCertPem,
		Enabled:         ToPtr(true),
		Name:            ToPtr("Test JWT Signer - Enabled"),
		Kid:             ToPtr(uuid.NewString()),
		Issuer:          ToPtr("the-very-best-iss"),
		Audience:        ToPtr("the-very-best-aud"),
		ClientID:        &validJwtSignerClientId,
		Scopes:          validJwtSignerScopes,
		ExternalAuthURL: &validJwtSignerAuthUrl,
	}

	createResponseEnv = &rest_model.CreateEnvelope{}

	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(validJwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

	validSignerUsingInternalId := createResponseEnv.Data.ID
	signerIds = append(signerIds, createResponseEnv.Data.ID)

	//valid signer w/ external id
	validExtIdJwtSignerCert, validExtIdJwtSignerPrivateKey := newSelfSignedCert("valid signer")
	validExtIdJwtSignerCertPem := nfpem.EncodeToString(validExtIdJwtSignerCert)

	validExtIdJwtSigner := &rest_model.ExternalJWTSignerCreate{
		CertPem:       &validExtIdJwtSignerCertPem,
		Enabled:       ToPtr(true),
		Name:          ToPtr("Test JWT Signer - Enabled - ExternalId"),
		Kid:           ToPtr(uuid.NewString()),
		Issuer:        ToPtr("the-very-best-iss-ext"),
		Audience:      ToPtr("the-very-best-aud-ext"),
		UseExternalID: ToPtr(true),
	}

	createResponseEnv = &rest_model.CreateEnvelope{}

	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(validExtIdJwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

	validExtIdJwtSignerId := createResponseEnv.Data.ID

	//valid signer w/ external id in "alt" field
	validAltExtIdJwtSignerCert, validAltExtIdJwtSignerPrivateKey := newSelfSignedCert("valid signer")
	validAltExtIdJwtSignerCertPem := nfpem.EncodeToString(validAltExtIdJwtSignerCert)

	validAltExtIdJwtSigner := &rest_model.ExternalJWTSignerCreate{
		CertPem:        &validAltExtIdJwtSignerCertPem,
		Enabled:        ToPtr(true),
		Name:           ToPtr("Test JWT Signer - Enabled - ExternalId - Alt"),
		Kid:            ToPtr(uuid.NewString()),
		Issuer:         ToPtr("the-very-best-iss-ext-alt"),
		Audience:       ToPtr("the-very-best-aud-ext-alt"),
		UseExternalID:  ToPtr(true),
		ClaimsProperty: ToPtr("alt"),
	}

	createResponseEnv = &rest_model.CreateEnvelope{}

	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(validAltExtIdJwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

	validAltExtIdJwtSignerId := createResponseEnv.Data.ID

	//not enabled signer
	createResponseEnv = &rest_model.CreateEnvelope{}

	notEnabledJwtSignerCert, notEnabledJwtSignerPrivateKey := newSelfSignedCert("not enabled signer")
	notEnabledJwtSignerCertPem := nfpem.EncodeToString(notEnabledJwtSignerCert)

	notEnabledJwtSigner := &rest_model.ExternalJWTSignerCreate{
		CertPem:  &notEnabledJwtSignerCertPem,
		Enabled:  ToPtr(false),
		Name:     ToPtr("Test JWT Signer - Not Enabled"),
		Kid:      ToPtr(uuid.NewString()),
		Issuer:   ToPtr("test-issuer"),
		Audience: ToPtr("test-audience"),
	}

	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(notEnabledJwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
	signerIds = append(signerIds, createResponseEnv.Data.ID)

	invalidJwtSignerCommonName := "invalid signer"
	invalidJwtSignerCert, invalidJwtSignerPrivateKey := newSelfSignedCert(invalidJwtSignerCommonName)
	invalidJwtSignerCertFingerprint := nfpem.FingerprintFromCertificate(invalidJwtSignerCert)

	authPolicyPatch := &rest_model.AuthPolicyPatch{
		Primary: &rest_model.AuthPolicyPrimaryPatch{
			ExtJWT: &rest_model.AuthPolicyPrimaryExtJWTPatch{
				AllowedSigners: signerIds,
			},
		},
	}
	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyPatch).Patch("/auth-policies/default")
	ctx.NoError(err)
	ctx.Equal(http.StatusOK, resp.StatusCode())

	t.Run("authenticating with a signer with keys from JWKS", func(t *testing.T) {
		ctx.testContextChanged(t)

		t.Run("succeeds with a known key", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtToken := jwt.New(jwt.SigningMethodES256)
			jwtToken.Claims = jwt.RegisteredClaims{
				Audience:  []string{*validJwksSigner.Audience},
				ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
				ID:        time.Now().String(),
				IssuedAt:  &jwt.NumericDate{Time: time.Now()},
				Issuer:    *validJwksSigner.Issuer,
				NotBefore: &jwt.NumericDate{Time: time.Now()},
				Subject:   *ctx.AdminManagementSession.AuthResponse.IdentityID,
			}

			jwtToken.Header["kid"] = validJwksSignerCert1.Subject.CommonName

			jwtStrSigned, err := jwtToken.SignedString(validJwksSignerPrivateKey1)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(jwtStrSigned)

			result := &rest_model.CurrentAPISessionDetailEnvelope{}

			resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.Req.NotNil(result)
			ctx.Req.NotNil(result.Data)
			ctx.Req.NotNil(result.Data.Token)
		})

		t.Run("fails with an unknown key", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtToken := jwt.New(jwt.SigningMethodES256)
			jwtToken.Claims = jwt.RegisteredClaims{
				Audience:  []string{*validJwksSigner.Audience},
				ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
				ID:        time.Now().String(),
				IssuedAt:  &jwt.NumericDate{Time: time.Now()},
				Issuer:    *validJwksSigner.Issuer,
				NotBefore: &jwt.NumericDate{Time: time.Now()},
				Subject:   *ctx.AdminManagementSession.AuthResponse.IdentityID,
			}

			jwtToken.Header["kid"] = validJwksSignerCert2.Subject.CommonName

			jwtStrSigned, err := jwtToken.SignedString(validJwksSignerPrivateKey2)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(jwtStrSigned)

			result := &rest_model.CurrentAPISessionDetailEnvelope{}

			resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
		})

		t.Run("succeeds with a newly added key", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwksServer.AddCertificate(validJwksSignerCert2)
			defer jwksServer.RemoveCertificate(validJwksSignerCert2)

			// allow jwks query timeout to pass (1 request / second)
			time.Sleep(model.JwksQueryTimeout + 500*time.Millisecond)

			jwtToken := jwt.New(jwt.SigningMethodES256)
			jwtToken.Claims = jwt.RegisteredClaims{
				Audience:  []string{*validJwksSigner.Audience},
				ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
				ID:        time.Now().String(),
				IssuedAt:  &jwt.NumericDate{Time: time.Now()},
				Issuer:    *validJwksSigner.Issuer,
				NotBefore: &jwt.NumericDate{Time: time.Now()},
				Subject:   *ctx.AdminManagementSession.AuthResponse.IdentityID,
			}

			jwtToken.Header["kid"] = validJwksSignerCert2.Subject.CommonName

			jwtStrSigned, err := jwtToken.SignedString(validJwksSignerPrivateKey2)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(jwtStrSigned)

			result := &rest_model.CurrentAPISessionDetailEnvelope{}

			resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.Req.NotNil(result)
			ctx.Req.NotNil(result.Data)
			ctx.Req.NotNil(result.Data.Token)
		})
	})

	t.Run("authenticating with a valid jwt succeeds", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{*validJwtSigner.Audience},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    *validJwtSigner.Issuer,
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   *ctx.AdminManagementSession.AuthResponse.IdentityID,
		}

		jwtToken.Header["kid"] = *validJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(validJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
		signerIds = append(signerIds, createResponseEnv.Data.ID)

		ctx.Req.NotNil(result)
		ctx.Req.NotNil(result.Data)
		ctx.Req.NotNil(result.Data.Token)
	})

	t.Run("authenticating with a valid jwt but disabled signer fails", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{"ziti.controller"},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    "fake.issuer",
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   *ctx.AdminManagementSession.AuthResponse.IdentityID,
		}

		jwtToken.Header["kid"] = *notEnabledJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(notEnabledJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("authenticating with an invalid issuer jwt fails", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{*validJwtSigner.Audience},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    "i will cause this to fail",
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   *ctx.AdminManagementSession.AuthResponse.IdentityID,
		}

		jwtToken.Header["kid"] = *validJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(validJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("authenticating with an invalid audience jwt fails", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{"this test shall not succeed"},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    *validJwtSigner.Issuer,
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   *ctx.AdminManagementSession.AuthResponse.IdentityID,
		}

		jwtToken.Header["kid"] = *validJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(validJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("authenticating with a valid jwt but no kid fails", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{"ziti.controller"},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    "fake.issuer",
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   *ctx.AdminManagementSession.AuthResponse.IdentityID,
		}

		jwtStrSigned, err := jwtToken.SignedString(validJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("authenticating with an jwt with unknown signer valid kid fails", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{"ziti.controller"},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    "fake.issuer",
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   *ctx.AdminManagementSession.AuthResponse.IdentityID,
		}

		jwtToken.Header["kid"] = *validJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(invalidJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("authenticating with an jwt with unknown signer invalid kid fails", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{"ziti.controller"},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    "fake.issuer",
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   *ctx.AdminManagementSession.AuthResponse.IdentityID,
		}

		jwtToken.Header["kid"] = invalidJwtSignerCertFingerprint

		jwtStrSigned, err := jwtToken.SignedString(invalidJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("authenticating with a malformed jwt fails", func(t *testing.T) {
		ctx.testContextChanged(t)
		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer 03qwjg03rjg90qr3hngq4390rng93q0rnghq430rng34r90ng4309vn439043").Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("can authenticate with an external id", func(t *testing.T) {
		ctx.testContextChanged(t)

		externalId := uuid.NewString()

		authPolicyCreate := &rest_model.AuthPolicyCreate{
			Name: ToPtr(uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					Allowed:           ToPtr(false),
					AllowExpiredCerts: ToPtr(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        ToPtr(true),
					AllowedSigners: []string{validExtIdJwtSignerId},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                ToPtr(false),
					LockoutDurationMinutes: ToPtr(int64(5)),
					MaxAttempts:            ToPtr(int64(3)),
					MinPasswordLength:      ToPtr(int64(5)),
					RequireMixedCase:       ToPtr(true),
					RequireNumberChar:      ToPtr(true),
					RequireSpecialChar:     ToPtr(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         ToPtr(false),
			},
		}
		authPolicyCreateResult := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyCreate).SetResult(authPolicyCreateResult).Post("/auth-policies")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(authPolicyCreateResult)
		ctx.NotNil(authPolicyCreateResult.Data)
		ctx.NotEmpty(authPolicyCreateResult.Data.ID)

		identityType := rest_model.IdentityTypeUser

		identityCreate := rest_model.IdentityCreate{
			AuthPolicyID: ToPtr(authPolicyCreateResult.Data.ID),
			ExternalID:   ToPtr(externalId),
			IsAdmin:      ToPtr(false),
			Name:         ToPtr(uuid.NewString()),
			Type:         &identityType,
		}

		identityCreateResult := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityCreate).SetResult(identityCreateResult).Post("/identities")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode())
		ctx.NotNil(identityCreateResult)
		ctx.NotNil(identityCreateResult.Data)
		ctx.NotEmpty(identityCreateResult.Data.ID)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{*validExtIdJwtSigner.Audience},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    *validExtIdJwtSigner.Issuer,
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   externalId,
		}

		jwtToken.Header["kid"] = *validExtIdJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(validExtIdJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

		ctx.Req.NotNil(result)
		ctx.Req.NotNil(result.Data)
		ctx.Req.NotNil(result.Data.Token)
	})

	t.Run("can not authenticate with an invalid external id", func(t *testing.T) {
		ctx.testContextChanged(t)

		externalId := uuid.NewString()

		authPolicyCreate := &rest_model.AuthPolicyCreate{
			Name: ToPtr(uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					Allowed:           ToPtr(false),
					AllowExpiredCerts: ToPtr(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        ToPtr(true),
					AllowedSigners: []string{validExtIdJwtSignerId},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                ToPtr(false),
					LockoutDurationMinutes: ToPtr(int64(5)),
					MaxAttempts:            ToPtr(int64(3)),
					MinPasswordLength:      ToPtr(int64(5)),
					RequireMixedCase:       ToPtr(true),
					RequireNumberChar:      ToPtr(true),
					RequireSpecialChar:     ToPtr(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         ToPtr(false),
			},
		}
		authPolicyCreateResult := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyCreate).SetResult(authPolicyCreateResult).Post("/auth-policies")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(authPolicyCreateResult)
		ctx.NotNil(authPolicyCreateResult.Data)
		ctx.NotEmpty(authPolicyCreateResult.Data.ID)

		identityType := rest_model.IdentityTypeUser

		identityCreate := rest_model.IdentityCreate{
			AuthPolicyID: ToPtr(authPolicyCreateResult.Data.ID),
			ExternalID:   ToPtr(externalId),
			IsAdmin:      ToPtr(false),
			Name:         ToPtr(uuid.NewString()),
			Type:         &identityType,
		}

		identityCreateResult := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityCreate).SetResult(identityCreateResult).Post("/identities")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode())
		ctx.NotNil(identityCreateResult)
		ctx.NotNil(identityCreateResult.Data)
		ctx.NotEmpty(identityCreateResult.Data.ID)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{*validExtIdJwtSigner.Audience},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    *validExtIdJwtSigner.Issuer,
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   uuid.NewString(),
		}

		jwtToken.Header["kid"] = *validExtIdJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(validExtIdJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		resp, err := ctx.newAnonymousClientApiRequest().SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("can not authenticate with an external id signed by the wrong signer", func(t *testing.T) {
		ctx.testContextChanged(t)

		externalId := uuid.NewString()

		authPolicyCreate := &rest_model.AuthPolicyCreate{
			Name: ToPtr(uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					Allowed:           ToPtr(false),
					AllowExpiredCerts: ToPtr(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        ToPtr(true),
					AllowedSigners: []string{validExtIdJwtSignerId},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                ToPtr(false),
					LockoutDurationMinutes: ToPtr(int64(5)),
					MaxAttempts:            ToPtr(int64(3)),
					MinPasswordLength:      ToPtr(int64(5)),
					RequireMixedCase:       ToPtr(true),
					RequireNumberChar:      ToPtr(true),
					RequireSpecialChar:     ToPtr(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         ToPtr(false),
			},
		}
		authPolicyCreateResult := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyCreate).SetResult(authPolicyCreateResult).Post("/auth-policies")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(authPolicyCreateResult)
		ctx.NotNil(authPolicyCreateResult.Data)
		ctx.NotEmpty(authPolicyCreateResult.Data.ID)

		identityType := rest_model.IdentityTypeUser

		identityCreate := rest_model.IdentityCreate{
			AuthPolicyID: ToPtr(authPolicyCreateResult.Data.ID),
			ExternalID:   ToPtr(externalId),
			IsAdmin:      ToPtr(false),
			Name:         ToPtr(uuid.NewString()),
			Type:         &identityType,
		}

		identityCreateResult := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityCreate).SetResult(identityCreateResult).Post("/identities")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode())
		ctx.NotNil(identityCreateResult)
		ctx.NotNil(identityCreateResult.Data)
		ctx.NotEmpty(identityCreateResult.Data.ID)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{*validExtIdJwtSigner.Audience},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    *validExtIdJwtSigner.Issuer,
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   externalId,
		}

		jwtToken.Header["kid"] = *validExtIdJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(notEnabledJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		resp, err := ctx.newAnonymousClientApiRequest().SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), string(resp.Body()))
	})

	t.Run("can authenticate with an external id in a non-sub (alt) field", func(t *testing.T) {
		ctx.testContextChanged(t)

		externalId := uuid.NewString()

		authPolicyCreate := &rest_model.AuthPolicyCreate{
			Name: ToPtr(uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					Allowed:           ToPtr(false),
					AllowExpiredCerts: ToPtr(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        ToPtr(true),
					AllowedSigners: []string{validAltExtIdJwtSignerId},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                ToPtr(false),
					LockoutDurationMinutes: ToPtr(int64(5)),
					MaxAttempts:            ToPtr(int64(3)),
					MinPasswordLength:      ToPtr(int64(5)),
					RequireMixedCase:       ToPtr(true),
					RequireNumberChar:      ToPtr(true),
					RequireSpecialChar:     ToPtr(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         ToPtr(false),
			},
		}
		authPolicyCreateResult := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyCreate).SetResult(authPolicyCreateResult).Post("/auth-policies")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(authPolicyCreateResult)
		ctx.NotNil(authPolicyCreateResult.Data)
		ctx.NotEmpty(authPolicyCreateResult.Data.ID)

		identityType := rest_model.IdentityTypeUser

		identityCreate := rest_model.IdentityCreate{
			AuthPolicyID: ToPtr(authPolicyCreateResult.Data.ID),
			ExternalID:   ToPtr(externalId),
			IsAdmin:      ToPtr(false),
			Name:         ToPtr(uuid.NewString()),
			Type:         &identityType,
		}

		identityCreateResult := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityCreate).SetResult(identityCreateResult).Post("/identities")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode())
		ctx.NotNil(identityCreateResult)
		ctx.NotNil(identityCreateResult.Data)
		ctx.NotEmpty(identityCreateResult.Data.ID)

		type altClaims struct {
			jwt.RegisteredClaims
			Alt string `json:"alt,omitempty"`
		}

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = altClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Audience:  []string{*validAltExtIdJwtSigner.Audience},
				ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
				ID:        time.Now().String(),
				IssuedAt:  &jwt.NumericDate{Time: time.Now()},
				Issuer:    *validAltExtIdJwtSigner.Issuer,
				NotBefore: &jwt.NumericDate{Time: time.Now()},
				Subject:   uuid.NewString(),
			},
			Alt: externalId,
		}

		jwtToken.Header["kid"] = *validAltExtIdJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(validAltExtIdJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

		ctx.Req.NotNil(result)
		ctx.Req.NotNil(result.Data)
		ctx.Req.NotNil(result.Data.Token)
	})

	t.Run("can authenticate with cert auth and secondary ext-jwt", func(t *testing.T) {
		ctx.testContextChanged(t)

		//create auth policy, new identity w/ policy, create token for authentication
		authPolicyCertExtJwt := &rest_model.AuthPolicyCreate{
			Name: ToPtr("createCertJwtAuthPolicy"),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: ToPtr(true),
					Allowed:           ToPtr(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					AllowedSigners: []string{},
					Allowed:        ToPtr(false),
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                ToPtr(false),
					LockoutDurationMinutes: ToPtr(int64(0)),
					MaxAttempts:            ToPtr(int64(3)),
					MinPasswordLength:      ToPtr(int64(7)),
					RequireMixedCase:       ToPtr(true),
					RequireNumberChar:      ToPtr(true),
					RequireSpecialChar:     ToPtr(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: ToPtr(validSignerUsingInternalId),
				RequireTotp:         ToPtr(false),
			},
		}

		createCertJwtAuthPolicy := &rest_model.CreateEnvelope{}
		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(createCertJwtAuthPolicy).SetBody(authPolicyCertExtJwt).Post("/auth-policies")
		ctx.NoError(err)
		ctx.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))
		ctx.NotNil(createCertJwtAuthPolicy)
		ctx.NotNil(createCertJwtAuthPolicy.Data)
		ctx.NotEmpty(createCertJwtAuthPolicy.Data.ID)

		newId, certAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(uuid.NewString(), false)

		identityPatch := &rest_model.IdentityPatch{
			AuthPolicyID: ToPtr(createCertJwtAuthPolicy.Data.ID),
		}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityPatch).Patch("/identities/" + newId)
		ctx.NoError(err)
		ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{*validJwtSigner.Audience},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    *validJwtSigner.Issuer,
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   newId,
		}

		jwtToken.Header["kid"] = *validJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(validJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		t.Run("authenticating with cert and jwt yields 0 auth queries", func(t *testing.T) {
			ctx.testContextChanged(t)

			result := &rest_model.CurrentAPISessionDetailEnvelope{}

			testClient, _, transport := ctx.NewClientComponents(EdgeClientApiPath)

			transport.TLSClientConfig.Certificates = certAuth.TLSCertificates()

			resp, err := testClient.NewRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=cert")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.Req.Empty(result.Data.AuthQueries)
		})

		t.Run("authenticating with cert and jwt yields 1 ext-jwt auth query", func(t *testing.T) {
			ctx.testContextChanged(t)

			result := &rest_model.CurrentAPISessionDetailEnvelope{}

			testClient, _, transport := ctx.NewClientComponents(EdgeClientApiPath)

			transport.TLSClientConfig.Certificates = certAuth.TLSCertificates()

			resp, err := testClient.NewRequest().SetResult(result).Post("/authenticate?method=cert")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
			ctx.Req.Len(result.Data.AuthQueries, 1)

			ctx.Req.Equal(rest_model.AuthQueryTypeEXTDashJWT, result.Data.AuthQueries[0].TypeID)
			ctx.Req.Equal(validJwtSignerClientId, result.Data.AuthQueries[0].ClientID)
			ctx.Req.Equal(validJwtSignerScopes[0], result.Data.AuthQueries[0].Scopes[0])
			ctx.Req.Equal(validJwtSignerScopes[1], result.Data.AuthQueries[0].Scopes[1])
			ctx.Req.Equal(validJwtSignerAuthUrl, result.Data.AuthQueries[0].HTTPURL)
			ctx.Req.Equal(validSignerUsingInternalId, result.Data.AuthQueries[0].ID)

		})
	})
}

// Test_Authenticate_External_Jwt_Overlapping_Kids reproduces intermittent primary ext-jwt
// authentication failures that occur when multiple external JWT signers expose the same key ID
// (kid), as happens when several signers draw from a shared signing-key pool (e.g. multiple
// Entra tenants using Microsoft's keys). Token-to-issuer binding must be disambiguated by the
// token's iss claim so a shared kid resolves deterministically to the correct signer, and a
// disabled signer that happens to share a kid must not poison resolution for an enabled one.
func Test_Authenticate_External_Jwt_Overlapping_Kids(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	// The controller fetches JWKS over the default HTTP transport. Trust the test PKI root so the
	// controller accepts the HTTPS JWKS providers below, which serve using the controller's own
	// server certificate. Restore the prior transport config when the test completes.
	rootPem, err := os.ReadFile("testdata/pki/root/certs/root.cert")
	ctx.Req.NoError(err)

	rootPool, err := x509.SystemCertPool()
	if err != nil || rootPool == nil {
		rootPool = x509.NewCertPool()
	}
	ctx.Req.True(rootPool.AppendCertsFromPEM(rootPem), "expected the test root CA to be added to the trust pool")

	httpTransport := http.DefaultTransport.(*http.Transport)
	priorTlsConfig := httpTransport.TLSClientConfig
	httpTransport.TLSClientConfig = &tls.Config{RootCAs: rootPool}
	defer func() { httpTransport.TLSClientConfig = priorTlsConfig }()

	serverTlsCert, err := tls.LoadX509KeyPair("testdata/pki/ctrl1/certs/server.chain.pem", "testdata/pki/ctrl1/keys/server.key")
	ctx.Req.NoError(err)

	adminIdentityId := *ctx.AdminManagementSession.AuthResponse.IdentityID

	t.Run("two enabled signers sharing a kid authenticate deterministically by issuer", func(t *testing.T) {
		ctx.testContextChanged(t)

		// One signing key and kid, served by two independent HTTPS JWKS providers - the shared
		// signing-key pool scenario.
		sharedCert, sharedKey := newSelfSignedCert("shared-jwks-pool-" + uuid.NewString())
		sharedKid := sharedCert.Subject.CommonName

		jwksServer1 := newTlsJwksServer(&serverTlsCert, []*x509.Certificate{sharedCert})
		ctx.Req.NoError(jwksServer1.Start())
		defer func() { _ = jwksServer1.Stop() }()

		jwksServer2 := newTlsJwksServer(&serverTlsCert, []*x509.Certificate{sharedCert})
		ctx.Req.NoError(jwksServer2.Start())
		defer func() { _ = jwksServer2.Stop() }()

		signer1Iss := "iss-shared-kid-1-" + uuid.NewString()
		signer1Aud := "aud-shared-kid-1-" + uuid.NewString()
		signer1Endpoint := strfmt.URI(jwksServer1.GetJwksUrl())
		signer1Env := &rest_model.CreateEnvelope{}
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(&rest_model.ExternalJWTSignerCreate{
			JwksEndpoint: &signer1Endpoint,
			Enabled:      ToPtr(true),
			Name:         ToPtr("Overlapping Kid Signer 1 - " + uuid.NewString()),
			Issuer:       ToPtr(signer1Iss),
			Audience:     ToPtr(signer1Aud),
		}).SetResult(signer1Env).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

		signer2Iss := "iss-shared-kid-2-" + uuid.NewString()
		signer2Aud := "aud-shared-kid-2-" + uuid.NewString()
		signer2Endpoint := strfmt.URI(jwksServer2.GetJwksUrl())
		signer2Env := &rest_model.CreateEnvelope{}
		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(&rest_model.ExternalJWTSignerCreate{
			JwksEndpoint: &signer2Endpoint,
			Enabled:      ToPtr(true),
			Name:         ToPtr("Overlapping Kid Signer 2 - " + uuid.NewString()),
			Issuer:       ToPtr(signer2Iss),
			Audience:     ToPtr(signer2Aud),
		}).SetResult(signer2Env).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(&rest_model.AuthPolicyPatch{
			Primary: &rest_model.AuthPolicyPrimaryPatch{
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWTPatch{
					Allowed:        ToPtr(true),
					AllowedSigners: []string{signer1Env.Data.ID, signer2Env.Data.ID},
				},
			},
		}).Patch("/auth-policies/default")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

		// External JWT signer creation events are processed asynchronously, so wait until signer 1's
		// JWKS has resolved into the issuer cache before asserting deterministic behavior.
		ctx.Req.Eventually(func() bool {
			code, err := authenticateWithSignedExtJwt(ctx, signer1Iss, signer1Aud, adminIdentityId, sharedKid, sharedKey)
			return err == nil && code == http.StatusOK
		}, 10*time.Second, 100*time.Millisecond, "signer 1 should become usable for primary authentication")

		// Tokens are issued only by signer 1 (its iss/aud), but the kid is shared with signer 2.
		// Kid-first binding over a non-deterministically ordered map would bind some requests to
		// signer 2 and reject them on the issuer mismatch. Repeat enough times that a single wrong
		// binding is overwhelmingly likely under the pre-fix behavior.
		const attempts = 25
		for i := 0; i < attempts; i++ {
			code, err := authenticateWithSignedExtJwt(ctx, signer1Iss, signer1Aud, adminIdentityId, sharedKid, sharedKey)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, code, "attempt %d: a token issued by signer 1 must authenticate regardless of a kid shared with signer 2", i)
		}
	})

	t.Run("a disabled signer sharing a kid does not poison an enabled signer", func(t *testing.T) {
		ctx.testContextChanged(t)

		// One signing key and kid served by two HTTPS JWKS providers - one signer enabled, one
		// disabled. A disabled signer that shares a kid must not capture the binding and suppress
		// issuer-string resolution for the enabled signer.
		sharedCert, sharedKey := newSelfSignedCert("shared-disabled-poison-" + uuid.NewString())
		sharedKid := sharedCert.Subject.CommonName

		enabledJwksServer := newTlsJwksServer(&serverTlsCert, []*x509.Certificate{sharedCert})
		ctx.Req.NoError(enabledJwksServer.Start())
		defer func() { _ = enabledJwksServer.Stop() }()

		disabledJwksServer := newTlsJwksServer(&serverTlsCert, []*x509.Certificate{sharedCert})
		ctx.Req.NoError(disabledJwksServer.Start())
		defer func() { _ = disabledJwksServer.Stop() }()

		enabledIss := "iss-poison-enabled-" + uuid.NewString()
		enabledAud := "aud-poison-enabled-" + uuid.NewString()
		enabledEndpoint := strfmt.URI(enabledJwksServer.GetJwksUrl())
		enabledEnv := &rest_model.CreateEnvelope{}
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(&rest_model.ExternalJWTSignerCreate{
			JwksEndpoint: &enabledEndpoint,
			Enabled:      ToPtr(true),
			Name:         ToPtr("Poison - Enabled - " + uuid.NewString()),
			Issuer:       ToPtr(enabledIss),
			Audience:     ToPtr(enabledAud),
		}).SetResult(enabledEnv).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

		disabledIss := "iss-poison-disabled-" + uuid.NewString()
		disabledAud := "aud-poison-disabled-" + uuid.NewString()
		disabledEndpoint := strfmt.URI(disabledJwksServer.GetJwksUrl())
		disabledEnv := &rest_model.CreateEnvelope{}
		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(&rest_model.ExternalJWTSignerCreate{
			JwksEndpoint: &disabledEndpoint,
			Enabled:      ToPtr(false),
			Name:         ToPtr("Poison - Disabled - " + uuid.NewString()),
			Issuer:       ToPtr(disabledIss),
			Audience:     ToPtr(disabledAud),
		}).SetResult(disabledEnv).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), string(resp.Body()))

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(&rest_model.AuthPolicyPatch{
			Primary: &rest_model.AuthPolicyPrimaryPatch{
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWTPatch{
					Allowed:        ToPtr(true),
					AllowedSigners: []string{enabledEnv.Data.ID},
				},
			},
		}).Patch("/auth-policies/default")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

		// Wait until the enabled signer's JWKS has resolved into the issuer cache (creation events
		// are processed asynchronously) before asserting deterministic behavior.
		ctx.Req.Eventually(func() bool {
			code, err := authenticateWithSignedExtJwt(ctx, enabledIss, enabledAud, adminIdentityId, sharedKid, sharedKey)
			return err == nil && code == http.StatusOK
		}, 10*time.Second, 100*time.Millisecond, "the enabled signer should become usable for primary authentication")

		// Tokens are issued by the enabled signer. Kid-first binding without an enabled filter
		// would bind some requests to the disabled signer, drop the token without falling back to
		// issuer-string resolution, and fail authentication.
		const attempts = 25
		for i := 0; i < attempts; i++ {
			code, err := authenticateWithSignedExtJwt(ctx, enabledIss, enabledAud, adminIdentityId, sharedKid, sharedKey)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, code, "attempt %d: a token issued by the enabled signer must authenticate even though a disabled signer shares its kid", i)
		}
	})
}

// authenticateWithSignedExtJwt signs an ES256 JWT with the given issuer, audience, subject, and kid
// using key, presents it as a primary ext-jwt bearer token, and returns the resulting HTTP status
// code. It does not assert, leaving the caller to decide what a given status means.
func authenticateWithSignedExtJwt(ctx *TestContext, issuer, audience, subject, kid string, key crypto.PrivateKey) (int, error) {
	jwtToken := jwt.New(jwt.SigningMethodES256)
	jwtToken.Claims = jwt.RegisteredClaims{
		Audience:  []string{audience},
		ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
		ID:        uuid.NewString(),
		IssuedAt:  &jwt.NumericDate{Time: time.Now()},
		Issuer:    issuer,
		NotBefore: &jwt.NumericDate{Time: time.Now()},
		Subject:   subject,
	}
	jwtToken.Header["kid"] = kid

	signed, err := jwtToken.SignedString(key)
	if err != nil {
		return 0, err
	}

	result := &rest_model.CurrentAPISessionDetailEnvelope{}
	resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+signed).Post("/authenticate?method=ext-jwt")
	if err != nil {
		return 0, err
	}

	return resp.StatusCode(), nil
}
