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
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/identity"

	"github.com/openziti/xweb/v3"
	"github.com/openziti/ziti/v2/controller/api"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
	"github.com/openziti/ziti/v2/controller/response"
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
	return strings.HasPrefix(r.URL.Path, h.RootPath())
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

	// allowedHostnames are operator-approved exact hostnames a wildcard server-cert SAN may be expanded
	// to as OIDC issuers (see getPossibleIssuers). Parsed like redirectURIs below.
	var allowedHostnames []string
	if allowedVal, ok := options["allowedHostnames"]; ok {
		list, ok := allowedVal.([]interface{})
		if !ok {
			return nil, fmt.Errorf("edge-oidc 'allowedHostnames' must be a list of hostnames, got %T", allowedVal)
		}
		for _, item := range list {
			hostname, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("edge-oidc 'allowedHostnames' entries must be strings, got %T", item)
			}
			hostname = strings.TrimSpace(hostname)
			if hostname == "" {
				continue
			}
			if strings.Contains(hostname, "*") {
				return nil, fmt.Errorf("edge-oidc 'allowedHostnames' entries must be exact hostnames, not patterns: %q", hostname)
			}
			// OIDC issuer comparison is case-sensitive (RFC 8414), so normalize to lower case to match how
			// hostnames are presented and to avoid emitting a mixed-case iss claim that strict relying
			// parties would reject. Warn loudly so the operator notices.
			if lower := strings.ToLower(hostname); lower != hostname {
				pfxlog.Logger().Warnf("edge-oidc allowedHostnames entry %q contains uppercase characters; normalizing to %q (OIDC issuer hostnames are case-sensitive per RFC 8414)", hostname, lower)
				hostname = lower
			}
			allowedHostnames = append(allowedHostnames, hostname)
		}
	}

	issuers := getPossibleIssuers(serverConfig.Identity, serverConfig.BindPoints, allowedHostnames)

	oidcConfig := oidc_auth.NewConfig(issuers, cert, key)
	oidcConfig.Identity = serverConfig.Identity
	oidcConfig.AccessTokenDuration = ae.GetConfig().Edge.Oidc.AccessTokenDuration
	oidcConfig.RefreshTokenDuration = ae.GetConfig().Edge.Oidc.RefreshTokenDuration
	oidcConfig.IdTokenDuration = ae.GetConfig().Edge.Oidc.IdTokenDuration
	oidcConfig.RevocationMinTokenLifetime = ae.GetConfig().Edge.Oidc.RevocationMinTokenLifetime
	oidcConfig.RevocationBucketInterval = ae.GetConfig().Edge.Oidc.RevocationBucketInterval
	oidcConfig.RevocationBucketMaxSize = ae.GetConfig().Edge.Oidc.RevocationBucketMaxSize
	oidcConfig.RevocationMaxQueued = ae.GetConfig().Edge.Oidc.RevocationMaxQueued

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
	oidcApi.handler = api.TimeoutHandler(api.WrapCorsHandler(oidcApi.handler), 10*time.Second, apierror.NewTimeoutError(), response.EdgeResponseMapper{})

	return oidcApi, nil
}

// getPossibleIssuers inspects the API server's identity and bind points for addresses, SAN DNS, and SAN IP entries
// that denote valid issuers. It returns a list of hostname:port combinations as a slice. It handles converting
// :443 to explicit and implicit ports for clients that may silently remove :443.
//
// A wildcard DNS SAN (e.g. "*.example.com") is never emitted as a literal issuer. Instead it is expanded only
// to the operator-approved allowedHostnames it actually covers (per x509 wildcard matching), keeping the set
// of valid OIDC issuers a closed, concrete list.
func getPossibleIssuers(id identity.Identity, bindPoints []xweb.BindPoint, allowedHostnames []string) []oidc_auth.Issuer {
	const (
		DefaultTlsPort = "443"
	)

	// The expected issuer's list is a combination of the following:
	// - all explicit expected bind point address ip or hostname and ports
	// - the IP and DNS SANs from all server certs + the port from the bind point address
	// - allowedHostnames covered by a wildcard DNS SAN + the port from the bind point address
	issuerMap := map[string]struct{}{}
	portMap := map[string]struct{}{}

	for _, bindPoint := range bindPoints {
		host, port, err := net.SplitHostPort(bindPoint.ServerAddress())
		if err != nil {
			continue

		}
		portMap[port] = struct{}{}

		if port == DefaultTlsPort {
			issuerMap[host] = struct{}{}
		}

		issuerMap[bindPoint.ServerAddress()] = struct{}{}
	}

	var ports []string
	for port := range portMap {
		ports = append(ports, port)
	}

	// addHost adds host:port issuers for every configured port (plus the bare host for the default TLS port,
	// for clients that silently drop :443).
	addHost := func(host string) {
		for _, port := range ports {
			issuerMap[net.JoinHostPort(host, port)] = struct{}{}
			if port == DefaultTlsPort {
				issuerMap[host] = struct{}{}
			}
		}
	}

	matchedAllowed := map[string]bool{}
	sawWildcard := false

	for _, curServerCertChain := range id.GetX509ActiveServerCertChains() {
		if len(curServerCertChain) == 0 {
			continue
		}
		curServerCert := curServerCertChain[0]

		for _, dnsName := range curServerCert.DNSNames {
			if strings.HasPrefix(dnsName, "*.") {
				// A wildcard SAN cannot be a usable issuer URL. Expand it only to allowedHostnames it
				// actually covers; with no allowlist it contributes no issuers.
				sawWildcard = true
				matcher := &x509.Certificate{DNSNames: []string{dnsName}}
				for _, allowed := range allowedHostnames {
					if matcher.VerifyHostname(allowed) == nil {
						addHost(allowed)
						matchedAllowed[allowed] = true
					}
				}
				continue
			}

			// Concrete SANs are always issuers; allowedHostnames only constrains wildcard expansion. The
			// match below just records that this allowlist entry is covered by a SAN (suppressing the
			// "not covered" warning below); it does not gate the concrete SAN.
			addHost(dnsName)
			for _, allowed := range allowedHostnames {
				if strings.EqualFold(allowed, dnsName) {
					matchedAllowed[allowed] = true
				}
			}
		}

		for _, ipAddr := range curServerCert.IPAddresses {
			addHost(ipAddr.String())
		}
	}

	if sawWildcard && len(allowedHostnames) == 0 {
		pfxlog.Logger().Warn("a server certificate has a wildcard DNS SAN but no edge-oidc 'allowedHostnames' are configured; the wildcard will not be used as an OIDC issuer")
	}
	for _, allowed := range allowedHostnames {
		if !matchedAllowed[allowed] {
			pfxlog.Logger().Warnf("edge-oidc allowedHostnames entry %q is not covered by any active server certificate SAN; ignoring", allowed)
		}
	}

	var issuers []oidc_auth.Issuer
	for address := range issuerMap {
		issuer, err := oidc_auth.NewIssuer(address)

		if err != nil {
			continue
		}
		issuers = append(issuers, issuer)
	}

	return issuers
}
