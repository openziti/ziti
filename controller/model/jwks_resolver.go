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

package model

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"

	"github.com/openziti/jwks"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/db"
)

// builtInBlockedCidrs are always refused for a JWKS fetch and cannot be re-enabled by
// configuration. They are the addresses that turn the controller's network position into a
// credential oracle, plus the addresses that have no meaning as an IdP endpoint.
var builtInBlockedCidrs = []string{
	"169.254.0.0/16",    // IPv4 link-local, includes cloud instance metadata (169.254.169.254) and ECS task metadata (169.254.170.2)
	"fe80::/10",         // IPv6 link-local
	"fd00:ec2::254/128", // AWS instance metadata over IPv6, inside unique-local space
	"224.0.0.0/24",      // IPv4 link-local multicast
	"ff02::/16",         // IPv6 link-local multicast
	"0.0.0.0/32",        // IPv4 unspecified
	"::/128",            // IPv6 unspecified
}

// JwksFetchConfig returns the [edge.externalJwtSigners.jwksFetch] settings from the
// environment, falling back to the defaults when no edge configuration is present.
func JwksFetchConfig(env Env) config.JwksFetch {
	if cfg := env.GetConfig(); cfg != nil && cfg.Edge != nil {
		return cfg.Edge.ExternalJwtSigners.JwksFetch
	}

	return config.DefaultJwksFetch()
}

// JwksFetchPolicy decides whether the controller may fetch a given JWKS endpoint. The
// endpoint URL is operator-supplied, so without this the controller's network position would
// be reachable by whoever can write an external JWT signer.
//
// A hop is fetched only if it passes both gates, and both gates are applied to the initial
// request and to every redirect. Neither gate can authorize what the other refuses: host
// matching only ever narrows what may be fetched.
//
// CheckHostname is the hostname gate, applied to the URL's hostname:
//
//  1. deniedHostnames  - blocked
//  2. allowedHostnames - when non-empty and the host does not match, blocked
//  3. anything else - passes
//
// CheckIP is the address gate, applied to the address being connected to, first-match-wins,
// deny before allow:
//
//  1. builtInBlockedCidrs - blocked, not overridable
//  2. deniedIPs     - blocked, not overridable by allowedIPs
//  3. allowedIPs    - allowed, a carve-out of tier 4 only
//  4. blockPrivateAddresses and the address is private or loopback - blocked
//  5. anything else       - allowed
type JwksFetchPolicy struct {
	blockPrivateAddresses bool
	builtInBlocked        []*net.IPNet
	deniedIPs             []*net.IPNet
	allowedIPs            []*net.IPNet
	deniedHostnames       []string
	allowedHostnames      []string
}

// NewJwksFetchPolicy returns a JwksFetchPolicy built from the [edge.externalJwtSigners.jwksFetch]
// configuration section.
func NewJwksFetchPolicy(cfg config.JwksFetch) *JwksFetchPolicy {
	result := &JwksFetchPolicy{
		blockPrivateAddresses: cfg.BlockPrivateAddresses,
		deniedIPs:             cfg.DeniedIPs,
		allowedIPs:            cfg.AllowedIPs,
		deniedHostnames:       cfg.DeniedHostnames,
		allowedHostnames:      cfg.AllowedHostnames,
	}

	for _, cidr := range builtInBlockedCidrs {
		_, ipNet, err := net.ParseCIDR(cidr)

		if err != nil {
			// builtInBlockedCidrs is a compile-time constant list, a parse failure is a bug
			panic(fmt.Errorf("invalid built-in blocked CIDR %s: %w", cidr, err))
		}

		result.builtInBlocked = append(result.builtInBlocked, ipNet)
	}

	return result
}

// CheckHostname returns nil if the controller may fetch from the given URL hostname, and an
// error otherwise. It is applied to the initial endpoint and to every redirect hop.
//
// Hostname matching narrows only. A hostname that passes here is still subject to CheckIP, and
// a caller may be able to reach the same target under a different name, so this is a filter on
// top of the address gate rather than a boundary of its own.
func (self *JwksFetchPolicy) CheckHostname(hostname string) error {
	normalized := config.NormalizeHostname(hostname)

	if normalized == "" {
		return fmt.Errorf("jwks endpoint url must include a hostname")
	}

	if matchesHostname(self.deniedHostnames, normalized) {
		return fmt.Errorf("jwks endpoint hostname %s is not permitted, it matches [edge.externalJwtSigners.jwksFetch.deniedHostnames]", normalized)
	}

	if len(self.allowedHostnames) > 0 && !matchesHostname(self.allowedHostnames, normalized) {
		return fmt.Errorf("jwks endpoint hostname %s is not permitted, [edge.externalJwtSigners.jwksFetch.allowedHostnames] is set and does not include it", normalized)
	}

	return nil
}

// matchesHostname reports whether a normalized hostname matches any of the given normalized
// patterns. A pattern is either an exact hostname, or a "*.suffix" wildcard that matches any
// subdomain of suffix at any depth but never suffix itself: "*.sub.host.com" matches
// "idp.sub.host.com" and "a.b.sub.host.com", but not "sub.host.com".
func matchesHostname(patterns []string, hostname string) bool {
	for _, pattern := range patterns {
		if suffix, isWildcard := strings.CutPrefix(pattern, "*."); isWildcard {
			if strings.HasSuffix(hostname, "."+suffix) {
				return true
			}

			continue
		}

		if hostname == pattern {
			return true
		}
	}

	return false
}

// CheckIP returns nil if the controller may connect to the given address for a JWKS fetch,
// and an error describing which tier refused it otherwise. An address that cannot be
// classified is refused.
func (self *JwksFetchPolicy) CheckIP(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("jwks endpoint address could not be parsed")
	}

	// classify IPv4 and IPv4-mapped IPv6 addresses identically
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}

	if containsIP(self.builtInBlocked, ip) {
		return fmt.Errorf("jwks endpoint address %s is not permitted, it is a metadata, link-local or unspecified address", ip)
	}

	if containsIP(self.deniedIPs, ip) {
		return fmt.Errorf("jwks endpoint address %s is not permitted, it matches [edge.externalJwtSigners.jwksFetch.deniedIPs]", ip)
	}

	if containsIP(self.allowedIPs, ip) {
		return nil
	}

	if self.blockPrivateAddresses && (ip.IsPrivate() || ip.IsLoopback()) {
		return fmt.Errorf("jwks endpoint address %s is not permitted, private and loopback addresses are blocked by [edge.externalJwtSigners.jwksFetch.blockPrivateAddresses]", ip)
	}

	return nil
}

// ValidateEndpoint checks an operator-supplied jwksEndpoint URL at create/update time so an
// obviously unusable endpoint is reported immediately instead of failing later during a
// fetch. It rejects a scheme other than http/https, a missing host, and a host that is a
// literal blocked IP address.
//
// This is advisory: no name resolution happens here, so a hostname that resolves to a blocked
// address passes this check and is refused by the dialer at fetch time. The dial-time check
// remains the authoritative one.
func (self *JwksFetchPolicy) ValidateEndpoint(endpoint string) error {
	target, err := url.Parse(strings.TrimSpace(endpoint))

	if err != nil {
		return fmt.Errorf("could not parse jwks endpoint url: %w", err)
	}

	if err = validateJwksEndpointScheme(target); err != nil {
		return err
	}

	host := target.Hostname()

	if err = self.CheckHostname(host); err != nil {
		return err
	}

	if ip := net.ParseIP(host); ip != nil {
		return self.CheckIP(ip)
	}

	return nil
}

// checkJwksEndpointAllowed returns an error when a signer's jwksEndpoint is one the given
// policy refuses. A signer with no jwksEndpoint, or a blank one, is not reported.
func checkJwksEndpointAllowed(policy *JwksFetchPolicy, signer *db.ExternalJwtSigner) error {
	if signer.JwksEndpoint == nil || strings.TrimSpace(*signer.JwksEndpoint) == "" {
		return nil
	}

	return policy.ValidateEndpoint(*signer.JwksEndpoint)
}

// CheckDialAddress applies CheckIP to a host:port address as it is about to be dialed. The
// address has already been resolved at that point, which is what makes the check
// rebinding-safe: the address that is connected to is the address that is checked.
func (self *JwksFetchPolicy) CheckDialAddress(address string) error {
	host, _, err := net.SplitHostPort(address)

	if err != nil {
		return fmt.Errorf("could not parse jwks endpoint dial address %s: %w", address, err)
	}

	return self.CheckIP(net.ParseIP(host))
}

// containsIP reports whether any of the given networks contains the given address.
func containsIP(ipNets []*net.IPNet, ip net.IP) bool {
	for _, ipNet := range ipNets {
		if ipNet.Contains(ip) {
			return true
		}
	}

	return false
}

var _ jwks.Resolver = (*HardenedJwksResolver)(nil)

// HardenedJwksResolver fetches JWKS responses over HTTP(S) with the constraints an
// operator-supplied URL requires: a bounded total time, a bounded number of redirects, an
// http/https-only scheme, and a JwksFetchPolicy check of every address that is connected to.
//
// The address check is installed as the dialer's control function, so it is the single choke
// point for the first request and for every redirect hop: it runs after DNS resolution and
// before the connection is made, which is what a check of the URL's hostname cannot do.
type HardenedJwksResolver struct {
	client *http.Client
	policy *JwksFetchPolicy
}

// NewHardenedJwksResolver returns a HardenedJwksResolver configured from the
// [edge.externalJwtSigners.jwksFetch] configuration section.
func NewHardenedJwksResolver(cfg config.JwksFetch) *HardenedJwksResolver {
	if cfg.Timeout <= 0 {
		cfg.Timeout = config.DefaultJwksFetchTimeout
	}

	if cfg.MaxRedirects < 0 {
		cfg.MaxRedirects = config.DefaultJwksFetchMaxRedirects
	}

	policy := NewJwksFetchPolicy(cfg)

	dialer := &net.Dialer{
		Timeout: cfg.Timeout,
		Control: func(_ string, address string, _ syscall.RawConn) error {
			return policy.CheckDialAddress(address)
		},
	}

	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: cfg.Timeout,
		// JWKS fetches are infrequent, so take a fresh, policy-checked connection every time
		// rather than reusing one
		DisableKeepAlives: true,
	}

	maxRedirects := cfg.MaxRedirects

	client := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("jwks endpoint exceeded the maximum of %d redirect(s)", maxRedirects)
			}

			if err := validateJwksEndpointScheme(request.URL); err != nil {
				return err
			}

			// every hop is checked on its own, an allowed first hop does not carry over
			return policy.CheckHostname(request.URL.Hostname())
		},
	}

	return &HardenedJwksResolver{
		client: client,
		policy: policy,
	}
}

// Get implements jwks.Resolver. It returns the parsed JWKS response and the raw response body.
func (self *HardenedJwksResolver) Get(endpoint string) (*jwks.Response, []byte, error) {
	target, err := url.Parse(strings.TrimSpace(endpoint))

	if err != nil {
		return nil, nil, fmt.Errorf("could not parse jwks endpoint url: %w", err)
	}

	if err = validateJwksEndpointScheme(target); err != nil {
		return nil, nil, err
	}

	if err = self.policy.CheckHostname(target.Hostname()); err != nil {
		return nil, nil, err
	}

	request, err := http.NewRequest(http.MethodGet, target.String(), nil)

	if err != nil {
		return nil, nil, fmt.Errorf("could not create jwks endpoint request: %w", err)
	}

	request.Header.Set("accept", "application/json")

	response, err := self.client.Do(request)

	if err != nil {
		return nil, nil, err
	}

	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("could not fetch JWKS, status code was not 200 OK, got %d", response.StatusCode)
	}

	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("content-type"), ";")[0]))

	if contentType != "application/json" && contentType != "application/jwk-set+json" && contentType != "application/jwk+json" {
		return nil, nil, fmt.Errorf("invalid content type %s, expected application/json", contentType)
	}

	body, err := io.ReadAll(response.Body)

	if err != nil {
		return nil, nil, fmt.Errorf("could not read jwks response: %w", err)
	}

	jwksResponse := &jwks.Response{}

	if err = json.Unmarshal(body, jwksResponse); err != nil {
		return nil, nil, fmt.Errorf("could not parse jwks response: %w", err)
	}

	return jwksResponse, body, nil
}

// validateJwksEndpointScheme allows only http and https. Anything else either cannot be
// fetched or would let the URL scheme choose the transport.
func validateJwksEndpointScheme(target *url.URL) error {
	switch strings.ToLower(target.Scheme) {
	case "http", "https":
		return nil
	}

	return fmt.Errorf("invalid jwks endpoint scheme %s, only http and https are supported", target.Scheme)
}
