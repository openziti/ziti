//go:build apitests

package tests

import (
	"crypto/x509"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_client_api_client/current_api_session"
	"github.com/openziti/edge-api/rest_client_api_client/current_identity"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	management_identity "github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	nfpem "github.com/openziti/foundation/v2/pem"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"net/url"
	"testing"
	"time"
)

func TestSdkAuth(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	ctx.RequireAdminManagementApiLogin()
	testId := ctx.AdminManagementSession.RequireNewIdentityWithOtt(true)
	testIdCerts := ctx.completeOttEnrollment(testId.Id)

	clientApiUrl, err := url.Parse("https://" + ctx.ApiHost + EdgeClientApiPath)
	ctx.Req.NoError(err)

	managementApiUrl, err := url.Parse("https://" + ctx.ApiHost + EdgeManagementApiPath)
	ctx.Req.NoError(err)

	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminClient := edge_apis.NewManagementApiClient([]*url.URL{managementApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
		strings <- "123"
	})
	apiSession, err := adminClient.Authenticate(adminCreds, nil)

	t.Run("management updb can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminClient)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("client updb, ca pool on client can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
		client := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
			strings <- "123"
		})
		apiSession, err := client.Authenticate(creds, nil)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("oidc client cert + TOTP MFA, ca pool on client can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		testIdMfa := ctx.AdminManagementSession.RequireNewIdentityWithOtt(true)
		testIdMfaCerts := ctx.completeOttEnrollment(testIdMfa.Id)

		certCreds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdMfaCerts.cert}, testIdMfaCerts.key)

		client := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
			strings <- ""
		})
		apiSession, err := client.Authenticate(certCreds, nil)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())

		t.Run("can enroll in MFA TOTP", func(t *testing.T) {
			ctx.testContextChanged(t)

			enrollMfaParams := &current_identity.EnrollMfaParams{}
			enrollMfaResp, err := client.API.CurrentIdentity.EnrollMfa(enrollMfaParams, nil)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(enrollMfaResp)
			ctx.Req.NotNil(enrollMfaResp.Payload)
			ctx.Req.NotNil(enrollMfaResp.Payload.Data)

			detailMfaParams := &current_identity.DetailMfaParams{}
			detailMfaResp, err := client.API.CurrentIdentity.DetailMfa(detailMfaParams, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(detailMfaResp)
			ctx.Req.NotNil(detailMfaResp.Payload)
			ctx.Req.NotNil(detailMfaResp.Payload.Data)
			ctx.Req.NotEmpty(detailMfaResp.Payload.Data.ProvisioningURL)

			parsedUrl, err := url.Parse(detailMfaResp.Payload.Data.ProvisioningURL)
			ctx.Req.NoError(err)

			queryParams, err := url.ParseQuery(parsedUrl.RawQuery)
			ctx.Req.NoError(err)
			secrets := queryParams["secret"]
			ctx.Req.NotNil(secrets)
			ctx.Req.NotEmpty(secrets)

			mfaSecret := secrets[0]

			code := computeMFACode(mfaSecret)

			verifyMfaParams := &current_identity.VerifyMfaParams{
				MfaValidation: &rest_model.MfaCode{
					Code: ToPtr(code),
				},
			}

			verifyMfaResp, err := client.API.CurrentIdentity.VerifyMfa(verifyMfaParams, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(verifyMfaResp)

			t.Run("can authenticate with newly enrolled TOTP MFA", func(t *testing.T) {
				ctx.testContextChanged(t)

				client := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
					parsedUrl, err := url.Parse(detailMfaResp.Payload.Data.ProvisioningURL)
					ctx.Req.NoError(err)

					queryParams, err := url.ParseQuery(parsedUrl.RawQuery)
					ctx.Req.NoError(err)
					secrets := queryParams["secret"]
					ctx.Req.NotNil(secrets)
					ctx.Req.NotEmpty(secrets)

					mfaSecret := secrets[0]

					code := computeMFACode(mfaSecret)

					strings <- code
				})
				client.SetUseOidc(true)
				apiSession, err := client.Authenticate(certCreds, nil)

				ctx.Req.NoError(err)
				ctx.Req.NotNil(client)
				ctx.Req.NotNil(apiSession)
				ctx.Req.NotNil(apiSession.GetToken())
			})
		})
	})

	t.Run("client updb, ca pool on cert can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
		creds.CaPool = ctx.ControllerConfig.Id.CA()

		client := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, nil, func(strings chan string) {
			strings <- "123"
		})
		apiSession, err := client.Authenticate(creds, nil)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("management cert, ca pool on client can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		client := edge_apis.NewManagementApiClient([]*url.URL{managementApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
			strings <- "123"
		})
		apiSession, err := client.Authenticate(creds, nil)

		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("management cert, ca pool on cert can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		creds.CaPool = ctx.ControllerConfig.Id.CA()

		client := edge_apis.NewManagementApiClient([]*url.URL{managementApiUrl}, nil, func(strings chan string) {
			strings <- "123"
		})
		apiSession, err := client.Authenticate(creds, nil)

		err = rest_util.WrapErr(err)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("client cert, ca pool on client can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)

		client := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
			strings <- "123"
		})
		apiSession, err := client.Authenticate(creds, nil)

		err = rest_util.WrapErr(err)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("client cert, ca pool on creds can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		creds.CaPool = ctx.ControllerConfig.Id.CA()

		client := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, nil, func(strings chan string) {
			strings <- "123"
		})
		apiSession, err := client.Authenticate(creds, nil)

		err = rest_util.WrapErr(err)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("ext jwt signer can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		//valid signer with issuer and audience
		validJwtSignerCert, validJwtSignerPrivateKey := newSelfSignedCert("valid signer")
		validJwtSignerCertPem := nfpem.EncodeToString(validJwtSignerCert)

		validJwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:       &validJwtSignerCertPem,
			Enabled:       B(true),
			Name:          S("Test JWT Signer - Enabled"),
			Kid:           S(uuid.NewString()),
			Issuer:        S("the-very-best-iss"),
			Audience:      S("the-very-best-aud"),
			UseExternalID: B(true),
		}

		//valid signed jwt
		subjectId := uuid.NewString()
		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{*validJwtSigner.Audience},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    *validJwtSigner.Issuer,
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   subjectId,
		}

		jwtToken.Header["kid"] = *validJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(validJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		// ext jwt
		createExtJwtParams := external_jwt_signer.NewCreateExternalJWTSignerParams()
		createExtJwtParams.ExternalJWTSigner = validJwtSigner

		createExtJwtSignerResp, err := adminClient.API.ExternalJWTSigner.CreateExternalJWTSigner(createExtJwtParams, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(createExtJwtSignerResp)
		ctx.Req.NotEmpty(createExtJwtSignerResp.Payload.Data.ID)

		//auth policy
		authPolicyCreate := &rest_model.AuthPolicyCreate{
			Name: S(uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert:   &rest_model.AuthPolicyPrimaryCert{Allowed: B(false), AllowExpiredCerts: B(false)},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{Allowed: B(true), AllowedSigners: []string{createExtJwtSignerResp.Payload.Data.ID}},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(false),
					LockoutDurationMinutes: I(0),
					MaxAttempts:            I(0),
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
			Tags: &rest_model.Tags{SubTags: rest_model.SubTags{}},
		}

		authPolicyCreateParams := auth_policy.NewCreateAuthPolicyParams()
		authPolicyCreateParams.AuthPolicy = authPolicyCreate

		createAuthPolicyResp, err := adminClient.API.AuthPolicy.CreateAuthPolicy(authPolicyCreateParams, nil)
		err = rest_util.WrapErr(err)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(createAuthPolicyResp)
		ctx.Req.NotEmpty(createAuthPolicyResp.Payload.Data.ID)

		//identity
		identityType := rest_model.IdentityTypeUser
		idCreate := &rest_model.IdentityCreate{
			AuthPolicyID: S(createAuthPolicyResp.Payload.Data.ID),
			ExternalID:   S(subjectId),
			IsAdmin:      B(false),
			Name:         S(uuid.NewString()),
			Type:         &identityType,
		}

		identityCreateParams := management_identity.NewCreateIdentityParams()
		identityCreateParams.Identity = idCreate

		idCreateResp, err := adminClient.API.Identity.CreateIdentity(identityCreateParams, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(idCreateResp)
		ctx.Req.NotEmpty(idCreateResp.Payload.Data.ID)

		t.Run("client jwt, ca pool on client", func(t *testing.T) {
			ctx.testContextChanged(t)
			creds := edge_apis.NewJwtCredentials(jwtStrSigned)
			client := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
				strings <- "123"
			})
			apiSession, err := client.Authenticate(creds, nil)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(client)
			ctx.Req.NotNil(apiSession)
			ctx.Req.NotNil(apiSession.GetToken())
		})

		t.Run("client jwt, ca pool on creds", func(t *testing.T) {
			ctx.testContextChanged(t)
			creds := edge_apis.NewJwtCredentials(jwtStrSigned)
			creds.CaPool = ctx.ControllerConfig.Id.CA()

			client := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, nil, func(strings chan string) {
				strings <- "123"
			})
			apiSession, err := client.Authenticate(creds, nil)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(client)
			ctx.Req.NotNil(apiSession)
			ctx.Req.NotNil(apiSession.GetToken())
		})

		t.Run("management jwt, ca pool on client", func(t *testing.T) {
			ctx.testContextChanged(t)
			creds := edge_apis.NewJwtCredentials(jwtStrSigned)
			client := edge_apis.NewManagementApiClient([]*url.URL{managementApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
				strings <- "123"
			})
			apiSession, err := client.Authenticate(creds, nil)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(client)
			ctx.Req.NotNil(apiSession)
			ctx.Req.NotNil(apiSession.GetToken())
		})

		t.Run("management jwt, ca pool on creds", func(t *testing.T) {
			ctx.testContextChanged(t)
			creds := edge_apis.NewJwtCredentials(jwtStrSigned)
			creds.CaPool = ctx.ControllerConfig.Id.CA()

			client := edge_apis.NewManagementApiClient([]*url.URL{managementApiUrl}, nil, func(strings chan string) {
				strings <- "123"
			})

			apiSession, err := client.Authenticate(creds, nil)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(client)
			ctx.Req.NotNil(apiSession)
			ctx.Req.NotNil(apiSession.GetToken())
		})
	})

	t.Run("client cert + secondary JWT  can authenticate", func(t *testing.T) {
		ctx.testContextChanged(t)

		certJwtId := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
		certJwtCerts := ctx.completeOttEnrollment(certJwtId.Id)

		//valid signer with issuer and audience
		validJwtSignerCert, validJwtSignerPrivateKey := newSelfSignedCert("valid signer")
		validJwtSignerCertPem := nfpem.EncodeToString(validJwtSignerCert)

		validJwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:       &validJwtSignerCertPem,
			Enabled:       B(true),
			Name:          S("Test Cert+JWT Signer - Enabled"),
			Kid:           S(uuid.NewString()),
			Issuer:        S("the-very-best-iss2"),
			Audience:      S("the-very-best-aud2"),
			UseExternalID: B(false),
		}

		//valid signed jwt
		jwtToken := jwt.New(jwt.SigningMethodES256)
		jwtToken.Claims = jwt.RegisteredClaims{
			Audience:  []string{*validJwtSigner.Audience},
			ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
			ID:        time.Now().String(),
			IssuedAt:  &jwt.NumericDate{Time: time.Now()},
			Issuer:    *validJwtSigner.Issuer,
			NotBefore: &jwt.NumericDate{Time: time.Now()},
			Subject:   certJwtId.Id,
		}

		jwtToken.Header["kid"] = *validJwtSigner.Kid

		jwtStrSigned, err := jwtToken.SignedString(validJwtSignerPrivateKey)
		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(jwtStrSigned)

		// ext jwt
		createExtJwtParams := external_jwt_signer.NewCreateExternalJWTSignerParams()
		createExtJwtParams.ExternalJWTSigner = validJwtSigner

		createExtJwtSignerResp, err := adminClient.API.ExternalJWTSigner.CreateExternalJWTSigner(createExtJwtParams, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(createExtJwtSignerResp)
		ctx.Req.NotEmpty(createExtJwtSignerResp.Payload.Data.ID)

		//auth policy
		authPolicyCreate := &rest_model.AuthPolicyCreate{
			Name: S(uuid.NewString()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert:   &rest_model.AuthPolicyPrimaryCert{Allowed: B(true), AllowExpiredCerts: B(false)},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{Allowed: B(false), AllowedSigners: make([]string, 0)},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                B(false),
					LockoutDurationMinutes: I(0),
					MaxAttempts:            I(0),
					MinPasswordLength:      I(5),
					RequireMixedCase:       B(true),
					RequireNumberChar:      B(true),
					RequireSpecialChar:     B(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: ToPtr(createExtJwtSignerResp.Payload.Data.ID),
				RequireTotp:         B(false),
			},
			Tags: &rest_model.Tags{SubTags: rest_model.SubTags{}},
		}

		authPolicyCreateParams := auth_policy.NewCreateAuthPolicyParams()
		authPolicyCreateParams.AuthPolicy = authPolicyCreate

		createAuthPolicyResp, err := adminClient.API.AuthPolicy.CreateAuthPolicy(authPolicyCreateParams, nil)
		err = rest_util.WrapErr(err)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(createAuthPolicyResp)
		ctx.Req.NotEmpty(createAuthPolicyResp.Payload.Data.ID)

		//assign identity auth policy
		identityPatchParams := &management_identity.PatchIdentityParams{
			ID: certJwtId.Id,
			Identity: &rest_model.IdentityPatch{
				AuthPolicyID: S(createAuthPolicyResp.Payload.Data.ID),
			},
		}

		identityPatchResp, err := adminClient.API.Identity.PatchIdentity(identityPatchParams, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(identityPatchResp)

		t.Run("client cert + secondary JWT, ca pool on client", func(t *testing.T) {
			ctx.testContextChanged(t)
			creds := edge_apis.NewCertCredentials([]*x509.Certificate{certJwtCerts.cert}, certJwtCerts.key)

			creds.AddJWT(jwtStrSigned)

			client := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, ctx.ControllerConfig.Id.CA(), func(strings chan string) {
				strings <- "123"
			})
			apiSession, err := client.Authenticate(creds, nil)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(client)
			ctx.Req.NotNil(apiSession)
			ctx.Req.NotNil(apiSession.GetToken())

			t.Run("can issue requests", func(t *testing.T) {
				ctx.testContextChanged(t)

				getCurrentApiSessionparams := &current_api_session.GetCurrentAPISessionParams{}

				resp, err := client.API.CurrentAPISession.GetCurrentAPISession(getCurrentApiSessionparams, nil)

				ctx.Req.NoError(err)
				ctx.Req.NotNil(resp)
				ctx.Req.NotNil(resp.Payload)
				ctx.Req.NotNil(resp.Payload.Data)
				ctx.Req.NotEmpty(resp.Payload.Data.ID)
			})
		})
	})
}
