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
	"github.com/golang-jwt/jwt"
	"github.com/openziti/edge/rest_model"
	nfpem "github.com/openziti/foundation/util/pem"
	"net/http"
	"testing"
	"time"
)

func Test_Authenticate_External_Jwt(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	// create a bunch of signers to use
	validJwtSignerCommonName := "valid signer"
	validJwtSignerCert, validJwtSignerPrivateKey := newSelfSignedCert(validJwtSignerCommonName)
	validJwtSignerCertPem := nfpem.EncodeToString(validJwtSignerCert)
	validJwtSignerName := "Test JWT Signer - Enabled"
	validJwtSignerEnabled := true
	validJwtSignerFingerprint := nfpem.FingerprintFromCertificate(validJwtSignerCert)

	validJwtSigner := &rest_model.ExternalJWTSignerCreate{
		CertPem: &validJwtSignerCertPem,
		Enabled: &validJwtSignerEnabled,
		Name:    &validJwtSignerName,
	}

	createResponseEnv := &rest_model.CreateEnvelope{}

	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(validJwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

	notEnabledJwtSignerCommonName := "not enabled signer"
	notEnabledJwtSignerCert, notEnabledJwtSignerPrivateKey := newSelfSignedCert(notEnabledJwtSignerCommonName)
	notEnabledJwtSignerCertPem := nfpem.EncodeToString(notEnabledJwtSignerCert)
	notEnabledJwtSignerName := "Test JWT Signer - Not Enabled"
	notEnabledJwtSignerEnabled := false
	notEnabledJwtSignerFingerprint := nfpem.FingerprintFromCertificate(notEnabledJwtSignerCert)

	notEnabledJwtSigner := &rest_model.ExternalJWTSignerCreate{
		CertPem: &notEnabledJwtSignerCertPem,
		Enabled: &notEnabledJwtSignerEnabled,
		Name:    &notEnabledJwtSignerName,
	}

	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(notEnabledJwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

	invalidJwtSignerCommonName := "invalid signer"
	invalidJwtSignerCert, invalidJwtSignerPrivateKey := newSelfSignedCert(invalidJwtSignerCommonName)
	invalidJwtSignerCertFingerprint := nfpem.FingerprintFromCertificate(invalidJwtSignerCert)

	t.Run("authenticating with a valid jwt succeeds", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  "ziti.controller",
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    "fake.issuer",
			NotBefore: time.Now().Unix(),
			Subject:   ctx.AdminManagementSession.identityId,
		}

		jwtToken.Header["kid"] = validJwtSignerFingerprint

		jwtStrSigned, err := jwtToken.SignedString(validJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		ctx.Req.NotNil(result)
		ctx.Req.NotNil(result.Data)
		ctx.Req.NotNil(result.Data.Token)
	})

	t.Run("authenticating with a valid jwt but disabled signer fails", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  "ziti.controller",
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    "fake.issuer",
			NotBefore: time.Now().Unix(),
			Subject:   ctx.AdminManagementSession.identityId,
		}

		jwtToken.Header["kid"] = notEnabledJwtSignerFingerprint

		jwtStrSigned, err := jwtToken.SignedString(notEnabledJwtSignerPrivateKey)
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
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  "ziti.controller",
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    "fake.issuer",
			NotBefore: time.Now().Unix(),
			Subject:   ctx.AdminManagementSession.identityId,
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
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  "ziti.controller",
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    "fake.issuer",
			NotBefore: time.Now().Unix(),
			Subject:   ctx.AdminManagementSession.identityId,
		}

		jwtToken.Header["kid"] = validJwtSignerFingerprint

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
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  "ziti.controller",
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    "fake.issuer",
			NotBefore: time.Now().Unix(),
			Subject:   ctx.AdminManagementSession.identityId,
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
}
