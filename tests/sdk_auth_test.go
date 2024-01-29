package tests

import (
	"crypto/x509"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	identity2 "github.com/openziti/edge-api/rest_management_api_client/identity"
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
	adminClient := edge_apis.NewManagementApiClient(managementApiUrl, ctx.ControllerConfig.Id.CA(), nil)
	apiSession, err := adminClient.Authenticate(adminCreds, nil)

	t.Run("management updb", func(t *testing.T) {
		ctx.testContextChanged(t)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(adminClient)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("client updb, ca pool on client", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
		client := edge_apis.NewClientApiClient(clientApiUrl, ctx.ControllerConfig.Id.CA(), nil)
		apiSession, err := client.Authenticate(creds, nil)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("client updb, ca pool on cert", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
		creds.CaPool = ctx.ControllerConfig.Id.CA()

		client := edge_apis.NewClientApiClient(clientApiUrl, nil, nil)
		apiSession, err := client.Authenticate(creds, nil)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("management cert, ca pool on  client", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		client := edge_apis.NewManagementApiClient(managementApiUrl, ctx.ControllerConfig.Id.CA(), nil)
		apiSession, err := client.Authenticate(creds, nil)

		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("management cert, ca pool on cert", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		creds.CaPool = ctx.ControllerConfig.Id.CA()

		client := edge_apis.NewManagementApiClient(managementApiUrl, nil, nil)
		apiSession, err := client.Authenticate(creds, nil)

		err = rest_util.WrapErr(err)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("client cert, ca pool on client", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)

		client := edge_apis.NewClientApiClient(clientApiUrl, ctx.ControllerConfig.Id.CA(), nil)
		apiSession, err := client.Authenticate(creds, nil)

		err = rest_util.WrapErr(err)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("client cert, ca pool on creds", func(t *testing.T) {
		ctx.testContextChanged(t)

		creds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		creds.CaPool = ctx.ControllerConfig.Id.CA()

		client := edge_apis.NewClientApiClient(clientApiUrl, nil, nil)
		apiSession, err := client.Authenticate(creds, nil)

		err = rest_util.WrapErr(err)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(client)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotNil(apiSession.GetToken())
	})

	t.Run("ext jwt signer", func(t *testing.T) {
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

		identityCreateParams := identity2.NewCreateIdentityParams()
		identityCreateParams.Identity = idCreate

		idCreateResp, err := adminClient.API.Identity.CreateIdentity(identityCreateParams, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(idCreateResp)
		ctx.Req.NotEmpty(idCreateResp.Payload.Data.ID)

		t.Run("client jwt, ca pool on client", func(t *testing.T) {
			ctx.testContextChanged(t)
			creds := edge_apis.NewJwtCredentials(jwtStrSigned)
			client := edge_apis.NewClientApiClient(clientApiUrl, ctx.ControllerConfig.Id.CA(), nil)
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

			client := edge_apis.NewClientApiClient(clientApiUrl, nil, nil)
			apiSession, err := client.Authenticate(creds, nil)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(client)
			ctx.Req.NotNil(apiSession)
			ctx.Req.NotNil(apiSession.GetToken())
		})

		t.Run("management jwt, ca pool on client", func(t *testing.T) {
			ctx.testContextChanged(t)
			creds := edge_apis.NewJwtCredentials(jwtStrSigned)
			client := edge_apis.NewManagementApiClient(managementApiUrl, ctx.ControllerConfig.Id.CA(), nil)
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

			client := edge_apis.NewManagementApiClient(managementApiUrl, nil, nil)
			apiSession, err := client.Authenticate(creds, nil)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(client)
			ctx.Req.NotNil(apiSession)
			ctx.Req.NotNil(apiSession.GetToken())
		})
	})

}
