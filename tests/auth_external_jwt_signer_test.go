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
	"github.com/google/uuid"
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

	var signerIds []string

	// create a bunch of signers to use

	//valid signer with issuer and audience
	validJwtSignerCert, validJwtSignerPrivateKey := newSelfSignedCert("valid signer")
	validJwtSignerCertPem := nfpem.EncodeToString(validJwtSignerCert)

	validJwtSigner := &rest_model.ExternalJWTSignerCreate{
		CertPem:  &validJwtSignerCertPem,
		Enabled:  B(true),
		Name:     S("Test JWT Signer - Enabled"),
		Kid:      S(uuid.NewString()),
		Issuer:   S("the-very-best-iss"),
		Audience: S("the-very-best-aud"),
	}

	createResponseEnv := &rest_model.CreateEnvelope{}

	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(validJwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
	signerIds = append(signerIds, createResponseEnv.Data.ID)

	//valid signer no issuer no audienceS
	validJwtSignerNoIssNoAudCert, validJwtSignerNoIssNoAudPrivateKey := newSelfSignedCert("valid signer")
	validJwtSignerCertPemNoIssNoAud := nfpem.EncodeToString(validJwtSignerNoIssNoAudCert)

	validJwtSignerNoIssNoAud := &rest_model.ExternalJWTSignerCreate{
		CertPem: &validJwtSignerCertPemNoIssNoAud,
		Enabled: B(true),
		Name:    S("Test JWT Signer - Enabled No Iss No Aud"),
		Kid:     S(uuid.NewString()),
	}

	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(validJwtSignerNoIssNoAud).SetResult(createResponseEnv).Post("/external-jwt-signers")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
	signerIds = append(signerIds, createResponseEnv.Data.ID)

	createResponseEnv = &rest_model.CreateEnvelope{}

	notEnabledJwtSignerCert, notEnabledJwtSignerPrivateKey := newSelfSignedCert("not enabled signer")
	notEnabledJwtSignerCertPem := nfpem.EncodeToString(notEnabledJwtSignerCert)

	notEnabledJwtSigner := &rest_model.ExternalJWTSignerCreate{
		CertPem: &notEnabledJwtSignerCertPem,
		Enabled: B(false),
		Name:    S("Test JWT Signer - Not Enabled"),
		Kid:     S(uuid.NewString()),
	}

	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(notEnabledJwtSigner).SetResult(createResponseEnv).Post("/external-jwt-signers")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
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

	t.Run("authenticating with a valid jwt succeeds", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  *validJwtSigner.Audience,
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    *validJwtSigner.Issuer,
			NotBefore: time.Now().Unix(),
			Subject:   ctx.AdminManagementSession.identityId,
		}

		jwtToken.Header["kid"] = *validJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(validJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		result := &rest_model.CurrentAPISessionDetailEnvelope{}

		resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())
		signerIds = append(signerIds, createResponseEnv.Data.ID)

		ctx.Req.NotNil(result)
		ctx.Req.NotNil(result.Data)
		ctx.Req.NotNil(result.Data.Token)
	})

	t.Run("authenticating with a valid jwt succeeds and no iss no aud succeeds", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  "i do not matter",
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    "i do not matter",
			NotBefore: time.Now().Unix(),
			Subject:   ctx.AdminManagementSession.identityId,
		}

		jwtToken.Header["kid"] = *validJwtSignerNoIssNoAud.Kid

		jwtStrSigned, err := jwtToken.SignedString(validJwtSignerNoIssNoAudPrivateKey)
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
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  *validJwtSigner.Audience,
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    "i will cause this to fail",
			NotBefore: time.Now().Unix(),
			Subject:   ctx.AdminManagementSession.identityId,
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
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  "this test shall not succeed",
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    *validJwtSigner.Issuer,
			NotBefore: time.Now().Unix(),
			Subject:   ctx.AdminManagementSession.identityId,
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
