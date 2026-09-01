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
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/gorilla/websocket"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/websockets"
	"github.com/openziti/identity"
	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	fabricRestClient "github.com/openziti/ziti/v2/controller/rest_client"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/pkg/errors"
)

// Clients provides authenticated access to a Ziti controller's edge management
// and fabric REST APIs. Edge authentication is delegated to sdk-golang's
// edge_apis.ManagementApiClient; the fabric REST client and the websocket
// management channel are constructed locally because sdk-golang has no
// equivalent.
type Clients struct {
	host string

	Edge   *edge_apis.ZitiEdgeManagement
	Fabric *fabricRestClient.ZitiFabric

	mgmt *edge_apis.ManagementApiClient

	lastAuth    time.Time
	lock        sync.Mutex
	closeNotify chan struct{}
}

// NewManagementClients fetches the controller's well-known CA bundle, builds
// an Edge management client backed by sdk-golang, and constructs a fabric
// REST client that reuses the same HTTP client and authentication writer.
// The returned Clients is unauthenticated; call Authenticate, AuthenticateOidc,
// or SetSessionToken before issuing API calls.
func NewManagementClients(host string) (*Clients, error) {
	if !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}

	caPool, err := loadWellKnownCertPool(host)
	if err != nil {
		return nil, err
	}

	parsedHost, err := url.Parse(host)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse host URL '%v'", host)
	}

	apiUrl := &url.URL{
		Scheme: parsedHost.Scheme,
		Host:   parsedHost.Host,
		Path:   "/edge/management/v1",
	}

	mgmt := edge_apis.NewManagementApiClient([]*url.URL{apiUrl}, caPool, nil)

	fabricRuntime := httptransport.NewWithClient(parsedHost.Host,
		fabricRestClient.DefaultBasePath, fabricRestClient.DefaultSchemes, mgmt.HttpClient)
	fabricRuntime.DefaultAuthentication = mgmt
	fabric := fabricRestClient.New(fabricRuntime, nil)

	return &Clients{
		host:   host,
		Edge:   mgmt.API,
		Fabric: fabric,
		mgmt:   mgmt,
	}, nil
}

// loadWellKnownCertPool fetches the controller's well-known certs and verifies
// the controller serves them itself before returning a CertPool. This
// preserves zitirest's original trust check rather than blindly trusting the
// well-known endpoint.
func loadWellKnownCertPool(host string) (*x509.CertPool, error) {
	wellKnownCerts, _, err := util.GetWellKnownCerts(host, http.Client{})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to retrieve server certificate authority from %v", host)
	}

	trusted, err := util.AreCertsTrusted(host, wellKnownCerts, http.Client{})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to verify well known certs for host %v", host)
	}
	if !trusted {
		return nil, errors.New("server supplied certs not trusted by server, unable to continue")
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(wellKnownCerts) {
		return nil, errors.New("failed to append well-known certs to pool")
	}
	return pool, nil
}

// NewTlsClientConfig returns a TLS configuration rooted at the controller's
// well-known CA pool. Used by callers that need to dial non-HTTP transports
// (e.g. the management websocket) against the same controller.
func (self *Clients) NewTlsClientConfig() *tls.Config {
	return self.mgmt.TlsAwareTransport.GetTlsClientConfig().Clone()
}

// Authenticate performs username/password legacy zt-session authentication.
func (self *Clients) Authenticate(user, password string) error {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.mgmt.SetUseOidc(false)
	if _, err := self.mgmt.Authenticate(edge_apis.NewUpdbCredentials(user, password), nil); err != nil {
		return err
	}

	self.lastAuth = time.Now()
	pfxlog.Logger().Debug("authenticated successfully (legacy)")
	return nil
}

// AuthenticateOidc performs OIDC PKCE authentication with username/password
// credentials. Delegated to sdk-golang's edge_apis flow, which also supports
// TOTP if the controller demands it.
func (self *Clients) AuthenticateOidc(user, password string) error {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.mgmt.SetUseOidc(true)
	if _, err := self.mgmt.Authenticate(edge_apis.NewUpdbCredentials(user, password), nil); err != nil {
		return err
	}

	self.lastAuth = time.Now()
	pfxlog.Logger().Debug("OIDC authentication successful")
	return nil
}

// AuthenticateIfNeeded re-authenticates only if it has been longer than
// maxTimeBetweenLogins since the last successful authentication.
func (self *Clients) AuthenticateIfNeeded(user, password string, maxTimeBetweenLogins time.Duration) error {
	self.lock.Lock()
	doAuth := time.Since(self.lastAuth) > maxTimeBetweenLogins
	self.lock.Unlock()
	if doAuth {
		return self.Authenticate(user, password)
	}
	return nil
}

// SetSessionToken installs a pre-acquired legacy session token without
// running an authentication flow. Used by tests that authenticate via a
// separate path and only need a typed REST client wrapper.
func (self *Clients) SetSessionToken(token string) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.mgmt.SetUseOidc(false)
	var session edge_apis.ApiSession = edge_apis.NewApiSessionLegacy(token)
	self.mgmt.ApiSession.Store(&session)
	self.lastAuth = time.Now()
}

// StartSessionRefresh starts a background goroutine that periodically renews
// the API session. In OIDC mode the refresh token is used; in legacy mode the
// goroutine re-authenticates with the provided credentials. Call
// StopSessionRefresh to stop the goroutine.
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

// StopSessionRefresh stops the background session refresh goroutine.
func (self *Clients) StopSessionRefresh() {
	self.lock.Lock()
	defer self.lock.Unlock()
	if self.closeNotify != nil {
		close(self.closeNotify)
		self.closeNotify = nil
	}
}

func (self *Clients) runSessionRefresh(interval time.Duration, username, password string, done chan struct{}) {
	log := pfxlog.Logger()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := self.refreshSession(username, password); err != nil {
				log.WithError(err).Error("session refresh failed")
			}
		case <-done:
			return
		}
	}
}

// refreshSession renews the API session. In OIDC mode it uses sdk-golang's
// refresh-token exchange; in legacy mode it re-authenticates, matching the
// original zitirest behavior so long-lived tests survive even past session
// expiry.
func (self *Clients) refreshSession(username, password string) error {
	current := self.mgmt.GetCurrentApiSession()
	if current != nil && current.GetType() == edge_apis.ApiSessionTypeOidc {
		self.lock.Lock()
		defer self.lock.Unlock()

		refreshed, err := self.mgmt.AuthEnabledApi.RefreshApiSession(current, self.mgmt.HttpClient)
		if err != nil {
			return err
		}
		self.mgmt.ApiSession.Store(&refreshed)
		self.lastAuth = time.Now()
		pfxlog.Logger().Debug("OIDC session refreshed successfully")
		return nil
	}
	return self.Authenticate(username, password)
}

// NewWsMgmtChannel opens a websocket connection to the controller's fabric
// management channel endpoint and wraps it in a channel.Channel using the
// supplied bind handler. The current session token is sent as either
// `zt-session` (legacy) or `Authorization: Bearer ...` (OIDC) depending on the
// active session type.
func (self *Clients) NewWsMgmtChannel(bindHandler channel.BindHandler) (channel.Channel, error) {
	log := pfxlog.Logger()

	baseUrl := self.host + "/" + string(util.FabricAPI)
	wsUrl := strings.ReplaceAll(baseUrl, "http", "ws") + "/v1/ws-api"
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		TLSClientConfig:  self.NewTlsClientConfig(),
		HandshakeTimeout: 10 * time.Second,
	}

	headers := http.Header{}
	if session := self.mgmt.GetCurrentApiSession(); session != nil {
		if name, value := session.GetAccessHeader(); name != "" {
			headers.Set(name, value)
		}
	}

	conn, resp, err := dialer.Dial(wsUrl, headers)
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

	ch, err := channel.NewSingleChannel("mgmt", underlayFactory, bindHandler, nil)
	if err != nil {
		return nil, err
	}
	return ch, nil
}
