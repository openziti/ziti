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

package zitirest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	openApiRuntime "github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/gorilla/websocket"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/websockets"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_management_api_client/authentication"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/controller/api"
	"github.com/openziti/ziti/v2/controller/env"
	fabricRestClient "github.com/openziti/ziti/v2/controller/rest_client"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/pkg/errors"
)

const (
	oidcRedirectURI = "http://localhost:8080/auth/callback"
	oidcClientID    = "native"
)

// Clients provides authenticated access to the Ziti controller's management APIs.
// It supports both legacy zt-session authentication and OIDC bearer token authentication.
type Clients struct {
	host           string
	wellKnownCerts []byte
	token          concurrenz.AtomicValue[string]
	Fabric         *fabricRestClient.ZitiFabric
	Edge           *rest_management_api_client.ZitiEdgeManagement

	FabricRuntime *httptransport.Runtime
	EdgeRuntime   *httptransport.Runtime
	lastAuth      time.Time
	lock          sync.Mutex

	refreshToken concurrenz.AtomicValue[string]
	oidcMode     bool
	closeNotify  chan struct{}
}

func (self *Clients) NewTlsClientConfig() *tls.Config {
	rootCaPool := x509.NewCertPool()
	rootCaPool.AppendCertsFromPEM(self.wellKnownCerts)

	return &tls.Config{
		RootCAs: rootCaPool,
	}
}

func (self *Clients) AuthenticateIfNeeded(user, password string, maxTimeBetweenLogins time.Duration) error {
	self.lock.Lock()
	doAuth := time.Since(self.lastAuth) > maxTimeBetweenLogins
	self.lock.Unlock()
	if doAuth {
		return self.Authenticate(user, password)
	}
	return nil
}

func (self *Clients) Authenticate(user, password string) error {
	self.lock.Lock()
	defer self.lock.Unlock()

	ctx, cancelF := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelF()

	result, err := self.Edge.Authentication.Authenticate(&authentication.AuthenticateParams{
		Auth: &rest_model.Authenticate{
			Username: rest_model.Username(user),
			Password: rest_model.Password(password),
		},
		Method:  "password",
		Context: ctx,
	})
	if err != nil {
		var authErr util.ApiErrorPayload
		if errors.As(err, &authErr) {
			out, _ := json.Marshal(authErr)
			fmt.Println(string(out))
		}
		return err
	}

	self.SetSessionToken(*result.Payload.Data.Token)
	pfxlog.Logger().WithField("token", self.token.Load()).Debug("authenticated successfully")
	self.lastAuth = time.Now()

	return nil
}

func (self *Clients) AuthenticateRequest(request openApiRuntime.ClientRequest, registry strfmt.Registry) error {
	if self.oidcMode {
		return request.SetHeaderParam("Authorization", "Bearer "+self.token.Load())
	}
	return request.SetHeaderParam(api.ZitiSession, self.token.Load())
}

func (self *Clients) SetSessionToken(token string) {
	self.token.Store(token)
}

// AuthenticateOidc performs OIDC authentication using the OAuth 2.0 PKCE flow
// with username/password credentials. After successful authentication, the access
// token is used for subsequent API requests via the Authorization: Bearer header.
func (self *Clients) AuthenticateOidc(username, password string) error {
	self.lock.Lock()
	defer self.lock.Unlock()

	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("failed to create cookie jar: %w", err)
	}

	transport := self.newHttpTransport()

	// Client for requests that should not follow redirects to the callback URI
	redirectClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if strings.HasPrefix(req.URL.String(), oidcRedirectURI) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// Client for normal requests (follows all redirects)
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		Jar:       jar,
	}

	pkce, err := generatePkceParams()
	if err != nil {
		return fmt.Errorf("failed to generate PKCE parameters: %w", err)
	}

	state := generateRandomBase64(16)
	nonce := generateRandomBase64(32)

	// Step 1: Initiate OIDC authorization flow
	authURL := self.host + "/oidc/authorize?" + url.Values{
		"client_id":             {oidcClientID},
		"response_type":         {"code"},
		"scope":                 {"openid offline_access"},
		"state":                 {state},
		"code_challenge":        {pkce.challenge},
		"code_challenge_method": {pkce.method},
		"redirect_uri":          {oidcRedirectURI},
		"nonce":                 {nonce},
	}.Encode()

	resp, err := httpClient.Get(authURL)
	if err != nil {
		return fmt.Errorf("OIDC authorize request failed: %w", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OIDC authorize failed with status %d", resp.StatusCode)
	}

	authRequestID := resp.Header.Get("auth-request-id")
	if authRequestID == "" {
		return errors.New("OIDC authorize response missing auth-request-id header")
	}

	// Step 2: Submit credentials
	loginURL := self.host + "/oidc/login/password"
	loginResp, err := redirectClient.PostForm(loginURL, url.Values{
		"id":       {authRequestID},
		"username": {username},
		"password": {password},
	})
	if err != nil {
		return fmt.Errorf("OIDC login request failed: %w", err)
	}
	_, _ = io.ReadAll(loginResp.Body)
	_ = loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusFound {
		return fmt.Errorf("OIDC login failed with status %d (expected 302)", loginResp.StatusCode)
	}

	// Extract authorization code from redirect Location
	locationStr := loginResp.Header.Get("Location")
	locationURL, err := url.Parse(locationStr)
	if err != nil {
		return fmt.Errorf("failed to parse OIDC redirect URL: %w", err)
	}

	code := locationURL.Query().Get("code")
	if code == "" {
		return errors.New("OIDC redirect missing authorization code")
	}

	if locationURL.Query().Get("state") != state {
		return errors.New("OIDC state mismatch")
	}

	// Step 3: Exchange authorization code for tokens
	tokens, err := self.exchangeOidcTokens(httpClient, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {oidcClientID},
		"code":          {code},
		"code_verifier": {pkce.verifier},
		"redirect_uri":  {oidcRedirectURI},
	})
	if err != nil {
		return fmt.Errorf("OIDC token exchange failed: %w", err)
	}

	self.token.Store(tokens.AccessToken)
	self.refreshToken.Store(tokens.RefreshToken)
	self.oidcMode = true
	self.lastAuth = time.Now()

	pfxlog.Logger().Debug("OIDC authentication successful")
	return nil
}

// refreshOidcTokens uses the stored refresh token to obtain a new access token.
func (self *Clients) refreshOidcTokens() error {
	self.lock.Lock()
	defer self.lock.Unlock()

	currentRefreshToken := self.refreshToken.Load()
	if currentRefreshToken == "" {
		return errors.New("no refresh token available")
	}

	httpClient := &http.Client{
		Transport: self.newHttpTransport(),
		Timeout:   30 * time.Second,
	}

	tokens, err := self.exchangeOidcTokens(httpClient, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {oidcClientID},
		"refresh_token": {currentRefreshToken},
		"scope":         {"openid offline_access"},
	})
	if err != nil {
		return err
	}

	self.token.Store(tokens.AccessToken)
	if tokens.RefreshToken != "" {
		self.refreshToken.Store(tokens.RefreshToken)
	}
	self.lastAuth = time.Now()

	pfxlog.Logger().Debug("OIDC session refreshed successfully")
	return nil
}

// exchangeOidcTokens posts the given form values to the OIDC token endpoint
// and returns the parsed token response.
func (self *Clients) exchangeOidcTokens(httpClient *http.Client, form url.Values) (*oidcTokenResponse, error) {
	tokenURL := self.host + "/oidc/oauth/token"
	resp, err := httpClient.PostForm(tokenURL, form)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokens oidcTokenResponse
	if err = json.Unmarshal(body, &tokens); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokens.AccessToken == "" {
		return nil, errors.New("token response missing access_token")
	}

	return &tokens, nil
}

// StartSessionRefresh starts a background goroutine that periodically refreshes
// the API session. In OIDC mode it uses the refresh token; in legacy mode it
// re-authenticates with the provided credentials. Call StopSessionRefresh to
// stop the goroutine.
func (self *Clients) StartSessionRefresh(interval time.Duration, username, password string) {
	self.lock.Lock()
	if self.closeNotify != nil {
		close(self.closeNotify)
	}
	self.closeNotify = make(chan struct{})
	done := self.closeNotify
	self.lock.Unlock()

	go self.runSessionRefresh(interval, username, password, done)
}

func (self *Clients) runSessionRefresh(interval time.Duration, username, password string, done chan struct{}) {
	log := pfxlog.Logger()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			var err error
			if self.oidcMode {
				err = self.refreshOidcTokens()
			} else {
				err = self.Authenticate(username, password)
			}
			if err != nil {
				log.WithError(err).Error("session refresh failed")
			}
		case <-done:
			return
		}
	}
}

// StopSessionRefresh stops the background session refresh goroutine.
func (self *Clients) StopSessionRefresh() {
	self.lock.Lock()
	defer self.lock.Unlock()
	if self.closeNotify != nil {
		close(self.closeNotify)
		self.closeNotify = nil
	}
}

type oidcTokenResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    float64 `json:"expires_in"`
	IDToken      string  `json:"id_token"`
}

type pkceParams struct {
	verifier  string
	challenge string
	method    string
}

func generatePkceParams() (*pkceParams, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)

	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return &pkceParams{
		verifier:  verifier,
		challenge: challenge,
		method:    "S256",
	}, nil
}

func generateRandomBase64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (self *Clients) NewWsMgmtChannel(bindHandler channel.BindHandler) (channel.Channel, error) {
	log := pfxlog.Logger()

	baseUrl := self.host + "/" + string(util.FabricAPI)
	wsUrl := strings.ReplaceAll(baseUrl, "http", "ws") + "/v1/ws-api"
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		TLSClientConfig:  self.NewTlsClientConfig(),
		HandshakeTimeout: 10 * time.Second,
	}

	result := http.Header{}
	result.Set(env.ZitiSession, self.token.Load())

	conn, resp, err := dialer.Dial(wsUrl, result)
	if err != nil {
		if resp != nil {
			if body, rerr := io.ReadAll(resp.Body); rerr == nil {
				log.WithError(err).Errorf("response body [%v]", string(body))
			}
		} else {
			log.WithError(err).Error("websocket dial returned error")
		}
		return nil, err
	}

	id := &identity.TokenId{Token: "mgmt"}
	underlayFactory := websockets.NewUnderlayFactory(id, conn, nil)

	ch, err := channel.NewChannel("mgmt", underlayFactory, bindHandler, nil)
	if err != nil {
		return nil, err
	}
	return ch, nil
}

func (self *Clients) LoadWellKnownCerts() error {
	if !strings.HasPrefix(self.host, "http") {
		self.host = "https://" + self.host
	}

	wellKnownCerts, _, err := util.GetWellKnownCerts(self.host, http.Client{})
	if err != nil {
		return errors.Wrapf(err, "unable to retrieve server certificate authority from %v", self.host)
	}

	certsTrusted, err := util.AreCertsTrusted(self.host, wellKnownCerts, http.Client{})
	if err != nil {
		return errors.Wrapf(err, "unable to verify well known certs for host %v", self.host)
	}

	if !certsTrusted {
		return errors.New("server supplied certs not trusted by server, unable to continue")
	}

	self.wellKnownCerts = wellKnownCerts
	return nil
}

func (self *Clients) newHttpTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 10 * time.Second,
		}).DialContext,

		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       self.NewTlsClientConfig(),
	}
}

func (self *Clients) newRestClientTransport() *http.Client {
	return &http.Client{
		Transport: self.newHttpTransport(),
		Timeout:   10 * time.Second,
	}
}

func NewManagementClients(host string) (*Clients, error) {
	if !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}

	clients := &Clients{
		host: host,
	}

	if err := clients.LoadWellKnownCerts(); err != nil {
		return nil, err
	}

	httpClient := clients.newRestClientTransport()

	parsedHost, err := url.Parse(host)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse host URL '%v'", host)
	}

	clients.FabricRuntime = httptransport.NewWithClient(parsedHost.Host,
		fabricRestClient.DefaultBasePath, fabricRestClient.DefaultSchemes, httpClient)

	clients.EdgeRuntime = httptransport.NewWithClient(parsedHost.Host,
		rest_management_api_client.DefaultBasePath, rest_management_api_client.DefaultSchemes, httpClient)

	clients.Fabric = fabricRestClient.New(clients.FabricRuntime, nil)
	clients.Edge = rest_management_api_client.New(clients.EdgeRuntime, nil)

	clients.FabricRuntime.DefaultAuthentication = clients
	clients.EdgeRuntime.DefaultAuthentication = clients

	return clients, nil
}
