package tests

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/openziti/ziti/controller/oidc_auth"
	"github.com/zitadel/oidc/v2/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v2/pkg/http"
	"github.com/zitadel/oidc/v2/pkg/oidc"
	"net"
	"net/http"
	"net/http/cookiejar"
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
	clientID := "native"
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

func Test_Authenticate_Distributed_Auth(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	rpServer, err := newOidcTestRp(ctx.ApiHost)
	ctx.Req.NoError(err)

	rpServer.Start()
	defer rpServer.Stop()

	//clientApiUrl, err := url.Parse("https://" + ctx.ApiHost + EdgeClientApiPath)
	//ctx.Req.NoError(err)
	//
	//managementApiUrl, err := url.Parse("https://" + ctx.ApiHost + EdgeManagementApiPath)
	//ctx.Req.NoError(err)

	t.Run("updb", func(t *testing.T) {
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
	})

	t.Run("cert", func(t *testing.T) {
		ctx.testContextChanged(t)

		ctx.RequireAdminManagementApiLogin()

		_, certAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment("test", false)

		client := resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransportWithClientCert(certAuth.cert, certAuth.key)))
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
	})
}
