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
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
		signerIds = append(signerIds, createResponseEnv.Data.ID)

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
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  *validExtIdJwtSigner.Audience,
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    *validExtIdJwtSigner.Issuer,
			NotBefore: time.Now().Unix(),
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
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  *validExtIdJwtSigner.Audience,
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    *validExtIdJwtSigner.Issuer,
			NotBefore: time.Now().Unix(),
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
		jwtToken.Claims = jwt.StandardClaims{
			Audience:  *validExtIdJwtSigner.Audience,
			ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
			Id:        time.Now().String(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    *validExtIdJwtSigner.Issuer,
			NotBefore: time.Now().Unix(),
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
			jwt.StandardClaims
			Alt string `json:"alt,omitempty"`
		}

		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = altClaims{
			StandardClaims: jwt.StandardClaims{
				Audience:  *validAltExtIdJwtSigner.Audience,
				ExpiresAt: time.Now().Add(2 * time.Hour).Unix(),
				Id:        time.Now().String(),
				IssuedAt:  time.Now().Unix(),
				Issuer:    *validAltExtIdJwtSigner.Issuer,
				NotBefore: time.Now().Unix(),
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
}
