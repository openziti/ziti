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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	httptransport "github.com/go-openapi/runtime/client"
	"github.com/gorilla/websocket"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/websockets"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_management_api_client/authentication"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/controller/env"
	fabric_rest_client "github.com/openziti/ziti/controller/rest_client"
	"github.com/openziti/ziti/ziti/util"
	"github.com/pkg/errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Clients struct {
	host           string
	wellKnownCerts []byte
	token          string
	Fabric         *fabric_rest_client.ZitiFabric
	Edge           *rest_management_api_client.ZitiEdgeManagement

	FabricRuntime *httptransport.Runtime
	EdgeRuntime   *httptransport.Runtime
}

func (self *Clients) NewTlsClientConfig() *tls.Config {
	rootCaPool := x509.NewCertPool()
	rootCaPool.AppendCertsFromPEM(self.wellKnownCerts)

	return &tls.Config{
		RootCAs: rootCaPool,
	}
}

func (self *Clients) Authenticate(user, password string) error {
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
	pfxlog.Logger().WithField("token", self.token).Debug("authenticated successfully")
	self.SetSessionToken(*result.Payload.Data.Token)
	return nil
}

func (self *Clients) SetSessionToken(token string) {
	self.token = token
	self.FabricRuntime.DefaultAuthentication = &util.EdgeManagementAuth{
		Token: self.token,
	}

	self.EdgeRuntime.DefaultAuthentication = &util.EdgeManagementAuth{
		Token: self.token,
	}

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
	result.Set(env.ZitiSession, self.token)

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

	wellKnownCerts, _, err := util.GetWellKnownCerts(self.host)
	if err != nil {
		return errors.Wrapf(err, "unable to retrieve server certificate authority from %v", self.host)
	}

	certsTrusted, err := util.AreCertsTrusted(self.host, wellKnownCerts)
	if err != nil {
		return errors.Wrapf(err, "unable to verify well known certs for host %v", self.host)
	}

	if !certsTrusted {
		return errors.New("server supplied certs not trusted by server, unable to continue")
	}

	self.wellKnownCerts = wellKnownCerts
	return nil
}

func (self *Clients) newRestClientTransport() *http.Client {
	httpClientTransport := &http.Transport{
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

	httpClient := &http.Client{
		Transport: httpClientTransport,
		Timeout:   10 * time.Second,
	}
	return httpClient
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
		fabric_rest_client.DefaultBasePath, fabric_rest_client.DefaultSchemes, httpClient)

	clients.EdgeRuntime = httptransport.NewWithClient(parsedHost.Host,
		rest_management_api_client.DefaultBasePath, rest_management_api_client.DefaultSchemes, httpClient)

	clients.Fabric = fabric_rest_client.New(clients.FabricRuntime, nil)
	clients.Edge = rest_management_api_client.New(clients.EdgeRuntime, nil)

	return clients, nil
}
