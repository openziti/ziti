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
	"encoding/base64"
	"encoding/json"
	"github.com/go-openapi/strfmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/ziti/controller/model"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
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
	return "http://localhost:" + strconv.Itoa(js.port) + "/jwks"
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
		_ = js.server.Serve(listener)
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
		Enabled:      B(true),
		Name:         S("Test JWT Signer - JWKS - Enabled"),
		Issuer:       S("the-very-best-iss-jwks"),
		Audience:     S("the-very-best-aud-jwks"),
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
		Enabled:         B(true),
		Name:            S("Test JWT Signer - Enabled"),
		Kid:             S(uuid.NewString()),
		Issuer:          S("the-very-best-iss"),
		Audience:        S("the-very-best-aud"),
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
		Enabled:       B(true),
		Name:          S("Test JWT Signer - Enabled - ExternalId"),
		Kid:           S(uuid.NewString()),
		Issuer:        S("the-very-best-iss-ext"),
		Audience:      S("the-very-best-aud-ext"),
		UseExternalID: B(true),
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
		Enabled:        B(true),
		Name:           S("Test JWT Signer - Enabled - ExternalId - Alt"),
		Kid:            S(uuid.NewString()),
		Issuer:         S("the-very-best-iss-ext-alt"),
		Audience:       S("the-very-best-aud-ext-alt"),
		UseExternalID:  B(true),
		ClaimsProperty: S("alt"),
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
		Enabled:  B(false),
		Name:     S("Test JWT Signer - Not Enabled"),
		Kid:      S(uuid.NewString()),
		Issuer:   S("test-issuer"),
		Audience: S("test-audience"),
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
			Name: S(uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					Allowed:           B(false),
					AllowExpiredCerts: B(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        B(true),
					AllowedSigners: []string{validExtIdJwtSignerId},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(false),
					LockoutDurationMinutes: I(5),
					MaxAttempts:            I(3),
					MinPasswordLength:      I(5),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
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
			AuthPolicyID: S(authPolicyCreateResult.Data.ID),
			ExternalID:   S(externalId),
			IsAdmin:      B(false),
			Name:         S(uuid.NewString()),
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
			Name: S(uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					Allowed:           B(false),
					AllowExpiredCerts: B(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        B(true),
					AllowedSigners: []string{validExtIdJwtSignerId},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(false),
					LockoutDurationMinutes: I(5),
					MaxAttempts:            I(3),
					MinPasswordLength:      I(5),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
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
			AuthPolicyID: S(authPolicyCreateResult.Data.ID),
			ExternalID:   S(externalId),
			IsAdmin:      B(false),
			Name:         S(uuid.NewString()),
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
			Name: S(uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					Allowed:           B(false),
					AllowExpiredCerts: B(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        B(true),
					AllowedSigners: []string{validExtIdJwtSignerId},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(false),
					LockoutDurationMinutes: I(5),
					MaxAttempts:            I(3),
					MinPasswordLength:      I(5),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
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
			AuthPolicyID: S(authPolicyCreateResult.Data.ID),
			ExternalID:   S(externalId),
			IsAdmin:      B(false),
			Name:         S(uuid.NewString()),
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
			Name: S(uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					Allowed:           B(false),
					AllowExpiredCerts: B(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        B(true),
					AllowedSigners: []string{validAltExtIdJwtSignerId},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(false),
					LockoutDurationMinutes: I(5),
					MaxAttempts:            I(3),
					MinPasswordLength:      I(5),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         B(false),
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
			AuthPolicyID: S(authPolicyCreateResult.Data.ID),
			ExternalID:   S(externalId),
			IsAdmin:      B(false),
			Name:         S(uuid.NewString()),
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
