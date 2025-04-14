package tests

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	authenticator2 "github.com/openziti/edge-api/rest_management_api_client/authenticator"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	identity2 "github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	nfpem "github.com/openziti/foundation/v2/pem"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/oidc_auth"
	"github.com/zitadel/oidc/v2/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v2/pkg/http"
	"github.com/zitadel/oidc/v2/pkg/oidc"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"
)

type testRpServer struct {
	Server       *http.Server
	Port         string
	Listener     net.Listener
	TokenChan    <-chan *oidc.Tokens[*oidc.IDTokenClaims]
	CallbackPath string
	CallbackUri  string
	LoginUri     string
}

func (t *testRpServer) Stop() {
	_ = t.Server.Shutdown(context.Background())
}

func (t *testRpServer) Start() {
	go func() {
		_ = t.Server.Serve(t.Listener)
	}()

	//allow the server to actually start so connections aren't simply closed by fast followup requests
	time.Sleep(100 * time.Millisecond)
}

func newOidcTestRp(apiHost string) (*testRpServer, error) {
	tokenOutChan := make(chan *oidc.Tokens[*oidc.IDTokenClaims], 1)
	result := &testRpServer{
		CallbackPath: "/auth/callback",
		TokenChan:    tokenOutChan,
	}
	var err error

	// random port on localhost for our auth callback server, glob pattern mattching in controller must be on
	result.Listener, err = net.Listen("tcp", ":0")

	if err != nil {
		return nil, fmt.Errorf("could not listen on a random port: %w", err)
	}

	_, result.Port, _ = net.SplitHostPort(result.Listener.Addr().String())

	result.LoginUri = "http://127.0.0.1:" + result.Port + "/login"

	key := []byte("test1234test1234")
	urlBase := "https://" + apiHost
	issuer := urlBase + "/oidc"
	clientID := common.ClaimClientIdOpenZiti
	clientSecret := ""
	scopes := []string{"openid", "offline_access"}
	result.CallbackUri = "http://127.0.0.1:" + result.Port + result.CallbackPath

	cookieHandler := httphelper.NewCookieHandler(key, key, httphelper.WithUnsecure())
	jar, _ := cookiejar.New(&cookiejar.Options{})
	httpClient := &http.Client{

		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		CheckRedirect: nil,
		Jar:           jar,
		Timeout:       10 * time.Second,
	}

	options := []rp.Option{
		rp.WithHTTPClient(httpClient),
		rp.WithPKCE(cookieHandler),
	}

	provider, err := rp.NewRelyingPartyOIDC(issuer, clientID, clientSecret, result.CallbackUri, scopes, options...)

	if err != nil {
		return nil, fmt.Errorf("could not create rp OIDC: %w", err)
	}

	state := func() string {
		return uuid.New().String()
	}
	serverMux := http.NewServeMux()

	authhandler := rp.AuthURLHandler(state, provider, rp.WithPromptURLParam("Welcome back!"))
	loginHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authhandler.ServeHTTP(writer, request)
	})

	serverMux.Handle("/login", loginHandler)

	marshalToken := func(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], state string, relyingParty rp.RelyingParty) {
		tokenOutChan <- tokens
		_, _ = w.Write([]byte("done!"))
	}

	serverMux.Handle(result.CallbackPath, rp.CodeExchangeHandler(marshalToken, provider))

	result.Server = &http.Server{Handler: serverMux}

	return result, nil
}

func Test_Authenticate_OIDC_Auth(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	rpServer, err := newOidcTestRp(ctx.ApiHost)
	ctx.Req.NoError(err)

	rpServer.Start()
	defer rpServer.Stop()

	t.Run("attempt to auth with multipart form data, expect unsupported media type", func(t *testing.T) {
		ctx.testContextChanged(t)

		client := resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport()))
		client.SetRedirectPolicy(resty.DomainCheckRedirectPolicy("127.0.0.1", "localhost"))

		loginPath := "https://" + ctx.ApiHost + "/oidc/login/password?authRequestID=12345"

		ctx.Req.NoError(err)
		ctx.Req.NotEmpty(loginPath)

		resp, err := client.R().SetMultipartFormData(map[string]string{
			"username": "admin",
			"password": "admin",
		}).Post(loginPath)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnsupportedMediaType, resp.StatusCode())
	})

	t.Run("updb with auth request id in body", func(t *testing.T) {
		ctx.testContextChanged(t)

		client := resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport()))
		client.SetRedirectPolicy(resty.DomainCheckRedirectPolicy("127.0.0.1", "localhost"))
		resp, err := client.R().Get(rpServer.LoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		authRequestId := resp.Header().Get(oidc_auth.AuthRequestIdHeader)
		ctx.Req.NotEmpty(authRequestId)

		opLoginUri := "https://" + resp.RawResponse.Request.URL.Host + "/oidc/login/username"

		resp, err = client.R().SetFormData(map[string]string{"id": authRequestId, "username": ctx.AdminAuthenticator.Username, "password": ctx.AdminAuthenticator.Password}).Post(opLoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		var outTokens *oidc.Tokens[*oidc.IDTokenClaims]

		select {
		case tokens := <-rpServer.TokenChan:
			outTokens = tokens
		case <-time.After(5 * time.Second):
			ctx.Fail("no tokens received, hit timeout")
		}

		ctx.Req.NotNil(outTokens)
		ctx.Req.NotEmpty(outTokens.IDToken)
		ctx.Req.NotEmpty(outTokens.IDTokenClaims)
		ctx.Req.NotEmpty(outTokens.AccessToken)
		ctx.Req.NotEmpty(outTokens.RefreshToken)

		t.Run("access token has expected values", func(t *testing.T) {
			ctx.testContextChanged(t)
			parser := jwt.NewParser()

			accessClaims := &common.AccessClaims{}

			_, _, err := parser.ParseUnverified(outTokens.AccessToken, accessClaims)

			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(accessClaims.AuthenticatorId)
			ctx.Req.False(accessClaims.IsCertExtendable)
			ctx.Req.True(accessClaims.IsAdmin)
			ctx.Req.NotEmpty(accessClaims.ApiSessionId)
			ctx.Req.NotEmpty(accessClaims.JWTID)
			ctx.Req.Equal(common.TokenTypeAccess, accessClaims.Type)
			ctx.Req.NotEmpty(accessClaims.Subject)
		})
	})

	t.Run("updb with auth request id in query string", func(t *testing.T) {
		ctx.testContextChanged(t)

		client := resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport()))
		client.SetRedirectPolicy(resty.DomainCheckRedirectPolicy("127.0.0.1", "localhost"))
		resp, err := client.R().Get(rpServer.LoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		authRequestId := resp.Header().Get(oidc_auth.AuthRequestIdHeader)
		ctx.Req.NotEmpty(authRequestId)

		opLoginUri := "https://" + resp.RawResponse.Request.URL.Host + "/oidc/login/username?authRequestID=" + authRequestId

		resp, err = client.R().SetFormData(map[string]string{"username": ctx.AdminAuthenticator.Username, "password": ctx.AdminAuthenticator.Password}).Post(opLoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		var outTokens *oidc.Tokens[*oidc.IDTokenClaims]

		select {
		case tokens := <-rpServer.TokenChan:
			outTokens = tokens
		case <-time.After(5 * time.Second):
			ctx.Fail("no tokens received, hit timeout")
		}

		ctx.Req.NotNil(outTokens)
		ctx.Req.NotEmpty(outTokens.IDToken)
		ctx.Req.NotEmpty(outTokens.IDTokenClaims)
		ctx.Req.NotEmpty(outTokens.AccessToken)
		ctx.Req.NotEmpty(outTokens.RefreshToken)

		t.Run("access token has expected values", func(t *testing.T) {
			ctx.testContextChanged(t)
			parser := jwt.NewParser()

			accessClaims := &common.AccessClaims{}

			_, _, err := parser.ParseUnverified(outTokens.AccessToken, accessClaims)

			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(accessClaims.AuthenticatorId)
			ctx.Req.False(accessClaims.IsCertExtendable)
			ctx.Req.True(accessClaims.IsAdmin)
			ctx.Req.NotEmpty(accessClaims.ApiSessionId)
			ctx.Req.NotEmpty(accessClaims.JWTID)
			ctx.Req.Equal(common.TokenTypeAccess, accessClaims.Type)
			ctx.Req.NotEmpty(accessClaims.Subject)
		})
	})

	t.Run("updb with id in query string", func(t *testing.T) {
		ctx.testContextChanged(t)

		client := resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport()))
		client.SetRedirectPolicy(resty.DomainCheckRedirectPolicy("127.0.0.1", "localhost"))
		resp, err := client.R().Get(rpServer.LoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		authRequestId := resp.Header().Get(oidc_auth.AuthRequestIdHeader)
		ctx.Req.NotEmpty(authRequestId)

		opLoginUri := "https://" + resp.RawResponse.Request.URL.Host + "/oidc/login/username?id=" + authRequestId

		resp, err = client.R().SetFormData(map[string]string{"username": ctx.AdminAuthenticator.Username, "password": ctx.AdminAuthenticator.Password}).Post(opLoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		var outTokens *oidc.Tokens[*oidc.IDTokenClaims]

		select {
		case tokens := <-rpServer.TokenChan:
			outTokens = tokens
		case <-time.After(5 * time.Second):
			ctx.Fail("no tokens received, hit timeout")
		}

		ctx.Req.NotNil(outTokens)
		ctx.Req.NotEmpty(outTokens.IDToken)
		ctx.Req.NotEmpty(outTokens.IDTokenClaims)
		ctx.Req.NotEmpty(outTokens.AccessToken)
		ctx.Req.NotEmpty(outTokens.RefreshToken)

		t.Run("access token has expected values", func(t *testing.T) {
			ctx.testContextChanged(t)
			parser := jwt.NewParser()

			accessClaims := &common.AccessClaims{}

			_, _, err := parser.ParseUnverified(outTokens.AccessToken, accessClaims)

			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(accessClaims.AuthenticatorId)
			ctx.Req.False(accessClaims.IsCertExtendable)
			ctx.Req.True(accessClaims.IsAdmin)
			ctx.Req.NotEmpty(accessClaims.ApiSessionId)
			ctx.Req.NotEmpty(accessClaims.JWTID)
			ctx.Req.Equal(common.TokenTypeAccess, accessClaims.Type)
			ctx.Req.NotEmpty(accessClaims.Subject)
		})
	})

	t.Run("cert", func(t *testing.T) {
		ctx.testContextChanged(t)

		ctx.RequireAdminManagementApiLogin()

		_, certAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment("test", false)

		client := resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransportWithClientCert(certAuth.certs, certAuth.key)))
		client.SetRedirectPolicy(resty.DomainCheckRedirectPolicy("127.0.0.1", "localhost"))
		resp, err := client.R().Get(rpServer.LoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		authRequestId := resp.Header().Get(oidc_auth.AuthRequestIdHeader)
		ctx.Req.NotEmpty(authRequestId)

		opLoginUri := "https://" + resp.RawResponse.Request.URL.Host + "/oidc/login/cert"

		resp, err = client.R().SetFormData(map[string]string{"id": authRequestId}).Post(opLoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		var outTokens *oidc.Tokens[*oidc.IDTokenClaims]

		select {
		case tokens := <-rpServer.TokenChan:
			outTokens = tokens
		case <-time.After(5 * time.Second):
			ctx.Fail("no tokens received, hit timeout")
		}

		ctx.Req.NotNil(outTokens)
		ctx.Req.NotEmpty(outTokens.IDToken)
		ctx.Req.NotEmpty(outTokens.IDTokenClaims)
		ctx.Req.NotEmpty(outTokens.AccessToken)
		ctx.Req.NotEmpty(outTokens.RefreshToken)

		t.Run("access token has expected values", func(t *testing.T) {
			ctx.testContextChanged(t)
			parser := jwt.NewParser()

			accessClaims := &common.AccessClaims{}

			_, _, err := parser.ParseUnverified(outTokens.AccessToken, accessClaims)

			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(accessClaims.AuthenticatorId)
			ctx.Req.True(accessClaims.IsCertExtendable)
			ctx.Req.False(accessClaims.IsAdmin)
			ctx.Req.NotEmpty(accessClaims.ApiSessionId)
			ctx.Req.NotEmpty(accessClaims.JWTID)
			ctx.Req.Equal(common.TokenTypeAccess, accessClaims.Type)
			ctx.Req.NotEmpty(accessClaims.Subject)
		})
	})

	t.Run("test cert auth totp ext-jwt", func(t *testing.T) {
		ctx.testContextChanged(t)

		managementApiUrl, err := url.Parse("https://" + ctx.ApiHost + "/edge/management/v1")
		ctx.Req.NoError(err)

		managementApiUrls := []*url.URL{managementApiUrl}

		managementClient := edge_apis.NewManagementApiClient(managementApiUrls, ctx.ControllerConfig.Id.CA(), func(resp chan string) {
			resp <- ""
		})

		adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)

		apiSession, err := managementClient.Authenticate(adminCreds, nil)
		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.NotNil(apiSession)

		ctx.testContextChanged(t)
		jwtSignerCert, _ := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy")

		clientId := "test-client-id-99"
		scope1 := "test-scope-1-99"
		scope2 := "test-scope-2-99"
		extAuthUrl := "https://some.auth.url.example.com/auth"
		createExtJwtParam := external_jwt_signer.NewCreateExternalJWTSignerParams()
		createExtJwtParam.ExternalJWTSigner = &rest_model.ExternalJWTSignerCreate{
			CertPem:         S(nfpem.EncodeToString(jwtSignerCert)),
			Enabled:         B(true),
			Name:            S("Test JWT Signer - Auth Policy"),
			Kid:             S(uuid.NewString()),
			Issuer:          S("test-issuer-99"),
			Audience:        S("test-audience-99"),
			ClientID:        &clientId,
			Scopes:          []string{scope1, scope2},
			ExternalAuthURL: S(extAuthUrl),
		}

		extJwtCreateResp, err := managementClient.API.ExternalJWTSigner.CreateExternalJWTSigner(createExtJwtParam, nil)
		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.Req.NotNil(extJwtCreateResp)

		createAuthPolicyParams := auth_policy.NewCreateAuthPolicyParams()
		createAuthPolicyParams.AuthPolicy = &rest_model.AuthPolicyCreate{
			Name: ToPtr("auth_oidc_test-" + eid.New()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: ToPtr(true),
					Allowed:           ToPtr(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        ToPtr(false),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                ToPtr(true),
					LockoutDurationMinutes: ToPtr(int64(0)),
					MaxAttempts:            ToPtr(int64(5)),
					MinPasswordLength:      ToPtr(int64(5)),
					RequireMixedCase:       ToPtr(false),
					RequireNumberChar:      ToPtr(false),
					RequireSpecialChar:     ToPtr(false),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: ToPtr(extJwtCreateResp.Payload.Data.ID),
				RequireTotp:         ToPtr(true),
			},
		}

		authPolicyCreateResp, err := managementClient.API.AuthPolicy.CreateAuthPolicy(createAuthPolicyParams, nil)
		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.Req.NotNil(authPolicyCreateResp)

		identityName := eid.New()
		identityExternalId := eid.New()
		createIdentityParams := identity2.NewCreateIdentityParams()
		createIdentityParams.Identity = &rest_model.IdentityCreate{
			AuthPolicyID: ToPtr(authPolicyCreateResp.Payload.Data.ID),
			ExternalID:   ToPtr(identityExternalId),
			IsAdmin:      ToPtr(false),
			Name:         ToPtr(identityName),
			Type:         ToPtr(rest_model.IdentityTypeDefault),
		}

		createIdentityResp, err := managementClient.API.Identity.CreateIdentity(createIdentityParams, nil)
		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.Req.NotNil(createIdentityResp)

		identityPassword := eid.New()

		createIdentityUpdbAuthenticator := authenticator2.NewCreateAuthenticatorParams()
		createIdentityUpdbAuthenticator.Authenticator = &rest_model.AuthenticatorCreate{
			CertPem:    "",
			IdentityID: ToPtr(createIdentityResp.Payload.Data.ID),
			Method:     ToPtr("updb"),
			Password:   identityPassword,
			Username:   identityName,
		}

		createIdentityUpdbAuthenticatorResp, err := managementClient.API.Authenticator.CreateAuthenticator(createIdentityUpdbAuthenticator, nil)
		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.Req.NotNil(createIdentityUpdbAuthenticatorResp)

		t.Run("can authenticate via UPDB and see two auth queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			identityClient := resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport()))
			identityClient.SetRedirectPolicy(resty.DomainCheckRedirectPolicy("127.0.0.1", "localhost"))
			resp, err := identityClient.R().Get(rpServer.LoginUri)

			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			authRequestId := resp.Header().Get(oidc_auth.AuthRequestIdHeader)
			ctx.Req.NotEmpty(authRequestId)

			opLoginUri := "https://" + resp.RawResponse.Request.URL.Host + "/oidc/login/username"

			resp, err = identityClient.R().SetHeader("content-type", "application/json").SetBody(map[string]string{"id": authRequestId, "username": identityName, "password": identityPassword}).Post(opLoginUri)

			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			type respBody struct {
				AuthQueries []*rest_model.AuthQueryDetail `json:"authQueries"`
			}

			parsedBody := &respBody{}

			err = json.Unmarshal(resp.Body(), parsedBody)
			ctx.Req.NoError(err)

			ctx.Req.Len(parsedBody.AuthQueries, 2)

			extJwtIdx := -1
			totpIdx := -1

			for i, authQuery := range parsedBody.AuthQueries {
				if authQuery.TypeID == rest_model.AuthQueryTypeEXTDashJWT {
					extJwtIdx = i
				} else if authQuery.TypeID == rest_model.AuthQueryTypeTOTP {
					totpIdx = i
				} else {
					ctx.Req.Failf("unexexpected auth quuery type id encountered: %s", string(authQuery.TypeID))
				}
			}

			ctx.Req.True(extJwtIdx >= 0, "expected extJwtIdx to be set")
			ctx.Req.True(totpIdx >= 0, "expected totpIdx to be set")

			ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].ClientID, clientId)
			ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].Scopes[0], scope1)
			ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].Scopes[1], scope2)
			ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].HTTPURL, extAuthUrl)
		})

	})
}
