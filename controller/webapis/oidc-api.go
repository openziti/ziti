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

package webapis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/openziti/identity"
	"net"
	"net/http"
	"strings"

	"github.com/openziti/xweb/v2"
	"github.com/openziti/ziti/controller/api"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/oidc_auth"
)

var _ xweb.ApiHandlerFactory = &OidcApiFactory{}

type OidcApiFactory struct {
	InitFunc func(*OidcApiHandler) error
	appEnv   *env.AppEnv
}

func (factory OidcApiFactory) Validate(config *xweb.InstanceConfig) error {
	return nil
}

func NewOidcApiFactory(appEnv *env.AppEnv) *OidcApiFactory {
	return &OidcApiFactory{
		appEnv: appEnv,
	}
}

func (factory OidcApiFactory) Binding() string {
	return OidcApiBinding
}

func (factory OidcApiFactory) New(serverConfig *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {
	oidcApi, err := NewOidcApiHandler(serverConfig, factory.appEnv, options)

	if err != nil {
		return nil, err
	}

	if factory.InitFunc != nil {
		if err := factory.InitFunc(oidcApi); err != nil {
			return nil, fmt.Errorf("error running on init func: %v", err)
		}
	}

	return oidcApi, nil
}

type OidcApiHandler struct {
	handler http.Handler
	appEnv  *env.AppEnv
	options map[interface{}]interface{}
}

func (h OidcApiHandler) Binding() string {
	return OidcApiBinding
}

func (h OidcApiHandler) Options() map[interface{}]interface{} {
	return h.options
}

func (h OidcApiHandler) RootPath() string {
	return "/oidc"
}

func (h OidcApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, h.RootPath()) || r.URL.Path == oidc_auth.WellKnownOidcConfiguration
}

func (h OidcApiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	h.handler.ServeHTTP(writer, request)
}

func (h OidcApiHandler) IsDefault() bool {
	return false
}

func NewOidcApiHandler(serverConfig *xweb.ServerConfig, ae *env.AppEnv, options map[interface{}]interface{}) (*OidcApiHandler, error) {
	oidcApi := &OidcApiHandler{
		options: options,
		appEnv:  ae,
	}

	serverCert := serverConfig.Identity.ServerCert()

	cert := serverCert[0].Leaf
	key := serverCert[0].PrivateKey

	issuers := getPossibleIssuers(serverConfig.Identity, serverConfig.BindPoints)

	oidcConfig := oidc_auth.NewConfig(issuers, cert, key)
	oidcConfig.Identity = serverConfig.Identity

	if secretVal, ok := options["secret"]; ok {
		if secret, ok := secretVal.(string); ok {
			secret = strings.TrimSpace(secret)
			if secret != "" {
				oidcConfig.TokenSecret = secret
			}
		}
	}

	if oidcConfig.TokenSecret == "" {
		bytes := make([]byte, 32)
		_, err := rand.Read(bytes)
		if err != nil {
			return nil, fmt.Errorf("could not generate random secret: %w", err)
		}

		oidcConfig.TokenSecret = hex.EncodeToString(bytes)
	}

	if redirectVal, ok := options["redirectURIs"]; ok {
		if redirects, ok := redirectVal.([]interface{}); ok {
			for _, redirectVal := range redirects {
				if redirect, ok := redirectVal.(string); ok {
					oidcConfig.RedirectURIs = append(oidcConfig.RedirectURIs, redirect)
				}
			}
		}
	}

	if postLogoutVal, ok := options["postLogoutURIs"]; ok {
		if postLogs, ok := postLogoutVal.([]interface{}); ok {
			for _, postLogVal := range postLogs {
				if postLog, ok := postLogVal.(string); ok {
					oidcConfig.PostLogoutURIs = append(oidcConfig.PostLogoutURIs, postLog)
				}
			}
		}
	}

	// add defaults
	if len(oidcConfig.RedirectURIs) == 0 {
		oidcConfig.RedirectURIs = append(oidcConfig.RedirectURIs, "openziti://auth/callback")
		oidcConfig.RedirectURIs = append(oidcConfig.RedirectURIs, "https://127.0.0.1:*/auth/callback")
		oidcConfig.RedirectURIs = append(oidcConfig.RedirectURIs, "http://127.0.0.1:*/auth/callback")
		oidcConfig.RedirectURIs = append(oidcConfig.RedirectURIs, "https://localhost:*/auth/callback")
		oidcConfig.RedirectURIs = append(oidcConfig.RedirectURIs, "http://localhost:*/auth/callback")
	}

	if len(oidcConfig.PostLogoutURIs) == 0 {
		oidcConfig.PostLogoutURIs = append(oidcConfig.PostLogoutURIs, "openziti://auth/logout")
		oidcConfig.PostLogoutURIs = append(oidcConfig.PostLogoutURIs, "https://127.0.0.1:*/auth/logout")
		oidcConfig.PostLogoutURIs = append(oidcConfig.PostLogoutURIs, "http://127.0.0.1:*/auth/logout")
		oidcConfig.PostLogoutURIs = append(oidcConfig.PostLogoutURIs, "https://localhost:*/auth/logout")
		oidcConfig.PostLogoutURIs = append(oidcConfig.PostLogoutURIs, "http://localhost:*/auth/logout")
	}

	var err error
	oidcApi.handler, err = oidc_auth.NewNativeOnlyOP(context.Background(), ae, oidcConfig)

	if err != nil {
		return nil, err
	}
	oidcApi.handler = api.WrapCorsHandler(oidcApi.handler)

	return oidcApi, nil
}

// getPossibleIssuers inspects the API server's identity and bind points for addresses, SAN DNS, and SAN IP entries
// that denote valid issuers. It returns a list of hostname:port combinations as a slice. It handles converting
// :443 to explicit and implicit ports for clients that may silently remove :443
func getPossibleIssuers(id identity.Identity, bindPoints []*xweb.BindPointConfig) []string {
	const (
		DefaultTlsPort = "443"
	)

	// The expected issuer's list is a combination of the following:
	// - all explicit expected bind point address ip or hostname and ports
	// - the IP and DNS SANs from all server certs + the port from the bind point address
	issuerMap := map[string]struct{}{}
	portMap := map[string]struct{}{}

	for _, bindPoint := range bindPoints {
		host, port, err := net.SplitHostPort(bindPoint.Address)
		if err != nil {
			continue

		}
		portMap[port] = struct{}{}

		if port == DefaultTlsPort {
			issuerMap[host] = struct{}{}
		}

		issuerMap[bindPoint.Address] = struct{}{}
	}

	var ports []string
	for port := range portMap {
		ports = append(ports, port)
	}

	for _, curServerCertChain := range id.GetX509ActiveServerCertChains() {
		if len(curServerCertChain) == 0 {
			continue
		}
		curServerCert := curServerCertChain[0]
		for _, dnsName := range curServerCert.DNSNames {
			for _, port := range ports {
				newIssuer := net.JoinHostPort(dnsName, port)
				issuerMap[newIssuer] = struct{}{}
				if port == DefaultTlsPort {
					issuerMap[dnsName] = struct{}{}
				}
			}
		}

		for _, ipAddr := range curServerCert.IPAddresses {
			for _, port := range ports {
				ipStr := ipAddr.String()
				newIssuer := net.JoinHostPort(ipStr, port)
				issuerMap[newIssuer] = struct{}{}
				if port == DefaultTlsPort {
					issuerMap[ipStr] = struct{}{}
				}
			}
		}
	}

	var issuers []string
	for hostPort := range issuerMap {
		issuers = append(issuers, hostPort)
	}

	return issuers
}
