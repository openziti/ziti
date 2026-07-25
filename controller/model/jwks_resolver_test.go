package model

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/stretchr/testify/require"
)

func Test_JwksFetchPolicy_CheckIP(t *testing.T) {
	t.Run("with default settings", func(t *testing.T) {
		policy := NewJwksFetchPolicy(config.DefaultJwksFetch())

		t.Run("built-in blocked addresses are blocked", func(t *testing.T) {
			blocked := []string{
				"169.254.169.254", // cloud instance metadata
				"169.254.170.2",   // ECS task metadata
				"fd00:ec2::254",   // AWS IMDS over IPv6
				"169.254.10.10",   // link-local
				"fe80::1",         // link-local
				"224.0.0.1",       // link-local multicast
				"ff02::1",         // link-local multicast
				"0.0.0.0",         // unspecified
				"::",              // unspecified
			}

			for _, address := range blocked {
				t.Run(address, func(t *testing.T) {
					req := require.New(t)

					err := policy.CheckIP(net.ParseIP(address))

					req.Error(err, "%s must be blocked by the built-in tier", address)
				})
			}
		})

		t.Run("private and loopback addresses are allowed, the default posture is compatible", func(t *testing.T) {
			allowed := []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "fc00::1"}

			for _, address := range allowed {
				t.Run(address, func(t *testing.T) {
					req := require.New(t)

					req.NoError(policy.CheckIP(net.ParseIP(address)))
				})
			}
		})

		t.Run("a public address is allowed", func(t *testing.T) {
			req := require.New(t)

			req.NoError(policy.CheckIP(net.ParseIP("93.184.216.34")))
		})
	})

	t.Run("with blockPrivateAddresses enabled", func(t *testing.T) {
		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.BlockPrivateAddresses = true

		policy := NewJwksFetchPolicy(jwksFetch)

		t.Run("private and loopback addresses are blocked", func(t *testing.T) {
			blocked := []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "fc00::1"}

			for _, address := range blocked {
				t.Run(address, func(t *testing.T) {
					req := require.New(t)

					req.Error(policy.CheckIP(net.ParseIP(address)))
				})
			}
		})

		t.Run("a public address is still allowed", func(t *testing.T) {
			req := require.New(t)

			req.NoError(policy.CheckIP(net.ParseIP("93.184.216.34")))
		})
	})

	t.Run("deniedIPs blocks an otherwise allowed public address", func(t *testing.T) {
		req := require.New(t)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.DeniedIPs = mustCidrs(t, "203.0.113.0/24")

		policy := NewJwksFetchPolicy(jwksFetch)

		req.Error(policy.CheckIP(net.ParseIP("203.0.113.5")))
		req.NoError(policy.CheckIP(net.ParseIP("203.0.114.5")))
	})

	t.Run("allowedIPs carves an exception out of blockPrivateAddresses", func(t *testing.T) {
		req := require.New(t)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.BlockPrivateAddresses = true
		jwksFetch.AllowedIPs = mustCidrs(t, "10.1.0.0/16")

		policy := NewJwksFetchPolicy(jwksFetch)

		req.NoError(policy.CheckIP(net.ParseIP("10.1.2.3")), "the carve-out address must be reachable")
		req.Error(policy.CheckIP(net.ParseIP("10.2.2.3")), "a private address outside the carve-out must stay blocked")
	})

	t.Run("allowedIPs cannot override the built-in blocked tier", func(t *testing.T) {
		req := require.New(t)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.AllowedIPs = mustCidrs(t, "169.254.0.0/16", "fd00:ec2::/64")

		policy := NewJwksFetchPolicy(jwksFetch)

		req.Error(policy.CheckIP(net.ParseIP("169.254.169.254")), "the metadata service must never be reachable")
		req.Error(policy.CheckIP(net.ParseIP("fd00:ec2::254")), "the IPv6 metadata service must never be reachable")
	})

	t.Run("allowedIPs cannot override deniedIPs", func(t *testing.T) {
		req := require.New(t)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.DeniedIPs = mustCidrs(t, "10.0.0.0/8")
		jwksFetch.AllowedIPs = mustCidrs(t, "10.1.2.3/32")

		policy := NewJwksFetchPolicy(jwksFetch)

		req.Error(policy.CheckIP(net.ParseIP("10.1.2.3")), "deny wins over allow")
	})

	t.Run("an IPv4-mapped IPv6 address is classified as its IPv4 address", func(t *testing.T) {
		t.Run("a mapped metadata address is blocked", func(t *testing.T) {
			req := require.New(t)

			policy := NewJwksFetchPolicy(config.DefaultJwksFetch())

			req.Error(policy.CheckIP(net.ParseIP("::ffff:169.254.169.254")))
		})

		t.Run("a mapped private address is blocked when blockPrivateAddresses is set", func(t *testing.T) {
			req := require.New(t)

			jwksFetch := config.DefaultJwksFetch()
			jwksFetch.BlockPrivateAddresses = true

			policy := NewJwksFetchPolicy(jwksFetch)

			req.Error(policy.CheckIP(net.ParseIP("::ffff:10.1.2.3")))
		})

		t.Run("a mapped address matches an IPv4 allowedIPs carve-out", func(t *testing.T) {
			req := require.New(t)

			jwksFetch := config.DefaultJwksFetch()
			jwksFetch.BlockPrivateAddresses = true
			jwksFetch.AllowedIPs = mustCidrs(t, "10.1.0.0/16")

			policy := NewJwksFetchPolicy(jwksFetch)

			req.NoError(policy.CheckIP(net.ParseIP("::ffff:10.1.2.3")))
		})

		t.Run("a mapped address matches an IPv4 deniedIPs entry", func(t *testing.T) {
			req := require.New(t)

			jwksFetch := config.DefaultJwksFetch()
			jwksFetch.DeniedIPs = mustCidrs(t, "203.0.113.0/24")

			policy := NewJwksFetchPolicy(jwksFetch)

			req.Error(policy.CheckIP(net.ParseIP("::ffff:203.0.113.5")))
		})
	})

	t.Run("an unparsable address is blocked", func(t *testing.T) {
		req := require.New(t)

		policy := NewJwksFetchPolicy(config.DefaultJwksFetch())

		req.Error(policy.CheckIP(nil), "an address that could not be parsed must never be treated as allowed")
	})
}

func Test_JwksFetchPolicy_CheckHostname(t *testing.T) {
	t.Run("with no host lists configured every host passes", func(t *testing.T) {
		req := require.New(t)

		policy := NewJwksFetchPolicy(config.DefaultJwksFetch())

		req.NoError(policy.CheckHostname("idp.example.com"))
		req.NoError(policy.CheckHostname("10.0.0.5"))
	})

	t.Run("an empty host is blocked", func(t *testing.T) {
		req := require.New(t)

		policy := NewJwksFetchPolicy(config.DefaultJwksFetch())

		req.Error(policy.CheckHostname(""))
	})

	t.Run("deniedHostnames blocks a matching host", func(t *testing.T) {
		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.DeniedHostnames = []string{"blocked.example.com", "*.internal.example.com"}

		policy := NewJwksFetchPolicy(jwksFetch)

		t.Run("an exact match is blocked", func(t *testing.T) {
			req := require.New(t)

			req.Error(policy.CheckHostname("blocked.example.com"))
		})

		t.Run("a wildcard suffix match is blocked", func(t *testing.T) {
			req := require.New(t)

			req.Error(policy.CheckHostname("idp.internal.example.com"))
			req.Error(policy.CheckHostname("deep.idp.internal.example.com"))
		})

		t.Run("the wildcard suffix itself is not matched", func(t *testing.T) {
			req := require.New(t)

			req.NoError(policy.CheckHostname("internal.example.com"), "*.internal.example.com covers subdomains only")
		})

		t.Run("a wildcard matches whole labels only", func(t *testing.T) {
			req := require.New(t)

			// xinternal.example.com ends with internal.example.com as a string, but the
			// wildcard replaces a whole label, so it is a different host
			req.NoError(policy.CheckHostname("xinternal.example.com"))
			req.NoError(policy.CheckHostname("notinternal.example.com"))
		})

		t.Run("an unlisted host passes", func(t *testing.T) {
			req := require.New(t)

			req.NoError(policy.CheckHostname("idp.example.com"))
		})

		t.Run("matching is case insensitive and ignores a trailing dot", func(t *testing.T) {
			req := require.New(t)

			req.Error(policy.CheckHostname("BLOCKED.example.com"))
			req.Error(policy.CheckHostname("blocked.example.com."))
		})
	})

	t.Run("allowedHostnames is exclusive when set", func(t *testing.T) {
		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.AllowedHostnames = []string{"idp.example.com", "*.idp.example.org"}

		policy := NewJwksFetchPolicy(jwksFetch)

		t.Run("a listed host passes", func(t *testing.T) {
			req := require.New(t)

			req.NoError(policy.CheckHostname("idp.example.com"))
			req.NoError(policy.CheckHostname("eu.idp.example.org"))
		})

		t.Run("an unlisted host is blocked", func(t *testing.T) {
			req := require.New(t)

			req.Error(policy.CheckHostname("evil.example.com"))
		})

		t.Run("a literal IP host is blocked", func(t *testing.T) {
			req := require.New(t)

			req.Error(policy.CheckHostname("93.184.216.34"), "an allowedHostnames list can only be satisfied by a name")
		})
	})

	t.Run("deniedHostnames wins over allowedHostnames", func(t *testing.T) {
		req := require.New(t)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.DeniedHostnames = []string{"old.idp.example.com"}
		jwksFetch.AllowedHostnames = []string{"*.idp.example.com"}

		policy := NewJwksFetchPolicy(jwksFetch)

		req.Error(policy.CheckHostname("old.idp.example.com"))
		req.NoError(policy.CheckHostname("new.idp.example.com"))
	})

	t.Run("an allowed host does not authorize a blocked address", func(t *testing.T) {
		req := require.New(t)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.AllowedHostnames = []string{"idp.example.com"}

		policy := NewJwksFetchPolicy(jwksFetch)

		req.NoError(policy.CheckHostname("idp.example.com"))
		req.Error(policy.CheckIP(net.ParseIP("169.254.169.254")), "the hostname gate must never widen the address gate")
	})
}

func Test_JwksFetchPolicy_ValidateEndpoint(t *testing.T) {
	t.Run("with default settings", func(t *testing.T) {
		policy := NewJwksFetchPolicy(config.DefaultJwksFetch())

		t.Run("an http or https endpoint is accepted", func(t *testing.T) {
			endpoints := []string{
				"https://idp.example.com/.well-known/jwks.json",
				"http://idp.example.com/.well-known/jwks.json",
				"HTTPS://idp.example.com/jwks",
			}

			for _, endpoint := range endpoints {
				t.Run(endpoint, func(t *testing.T) {
					req := require.New(t)

					req.NoError(policy.ValidateEndpoint(endpoint))
				})
			}
		})

		t.Run("a non-http scheme is rejected", func(t *testing.T) {
			endpoints := []string{"file:///etc/passwd", "ftp://idp.example.com/jwks", "idp.example.com/jwks", ""}

			for _, endpoint := range endpoints {
				t.Run(endpoint, func(t *testing.T) {
					req := require.New(t)

					req.Error(policy.ValidateEndpoint(endpoint))
				})
			}
		})

		t.Run("an endpoint without a host is rejected", func(t *testing.T) {
			req := require.New(t)

			req.Error(policy.ValidateEndpoint("http:///jwks"))
		})

		t.Run("a literal metadata address is rejected", func(t *testing.T) {
			endpoints := []string{
				"http://169.254.169.254/latest/meta-data/",
				"http://169.254.170.2/v2/credentials",
				"http://[fd00:ec2::254]/latest/meta-data/",
				"http://169.254.169.254:8080/jwks",
			}

			for _, endpoint := range endpoints {
				t.Run(endpoint, func(t *testing.T) {
					req := require.New(t)

					req.Error(policy.ValidateEndpoint(endpoint))
				})
			}
		})

		t.Run("a literal private address is accepted, the default posture allows it", func(t *testing.T) {
			req := require.New(t)

			req.NoError(policy.ValidateEndpoint("https://10.1.2.3/jwks"))
		})
	})

	t.Run("with blockPrivateAddresses enabled", func(t *testing.T) {
		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.BlockPrivateAddresses = true

		policy := NewJwksFetchPolicy(jwksFetch)

		t.Run("a literal private address is rejected", func(t *testing.T) {
			req := require.New(t)

			req.Error(policy.ValidateEndpoint("https://10.1.2.3/jwks"))
		})

		t.Run("a hostname is accepted, the dial-time check remains authoritative", func(t *testing.T) {
			req := require.New(t)

			// no DNS resolution happens at create/update time, so a hostname that resolves to
			// a blocked address is caught by the dialer instead
			req.NoError(policy.ValidateEndpoint("https://localhost/jwks"))
		})

		t.Run("a literal private address in allowedIPs is accepted", func(t *testing.T) {
			req := require.New(t)

			carveOut := config.DefaultJwksFetch()
			carveOut.BlockPrivateAddresses = true
			carveOut.AllowedIPs = mustCidrs(t, "10.1.0.0/16")

			req.NoError(NewJwksFetchPolicy(carveOut).ValidateEndpoint("https://10.1.2.3/jwks"))
		})
	})

	t.Run("with host lists configured", func(t *testing.T) {
		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.DeniedHostnames = []string{"old.idp.example.com"}
		jwksFetch.AllowedHostnames = []string{"idp.example.com", "*.idp.example.org"}

		policy := NewJwksFetchPolicy(jwksFetch)

		t.Run("an allowed host is accepted", func(t *testing.T) {
			req := require.New(t)

			req.NoError(policy.ValidateEndpoint("https://idp.example.com/.well-known/jwks.json"))
			req.NoError(policy.ValidateEndpoint("https://eu.idp.example.org/jwks"))
		})

		t.Run("a host outside allowedHostnames is rejected", func(t *testing.T) {
			req := require.New(t)

			req.Error(policy.ValidateEndpoint("https://other.example.com/jwks"))
		})

		t.Run("a denied host is rejected", func(t *testing.T) {
			req := require.New(t)

			req.Error(policy.ValidateEndpoint("https://old.idp.example.com/jwks"))
		})
	})
}

func Test_checkJwksEndpointAllowed(t *testing.T) {
	jwksFetch := config.DefaultJwksFetch()
	jwksFetch.AllowedHostnames = []string{"idp.example.com"}

	policy := NewJwksFetchPolicy(jwksFetch)

	t.Run("a signer without a jwks endpoint is not reported", func(t *testing.T) {
		req := require.New(t)

		req.NoError(checkJwksEndpointAllowed(policy, &db.ExternalJwtSigner{}))
	})

	t.Run("a signer with a blank jwks endpoint is not reported", func(t *testing.T) {
		req := require.New(t)

		endpoint := "  "

		req.NoError(checkJwksEndpointAllowed(policy, &db.ExternalJwtSigner{JwksEndpoint: &endpoint}))
	})

	t.Run("a signer with an allowed jwks endpoint is not reported", func(t *testing.T) {
		req := require.New(t)

		endpoint := "https://idp.example.com/jwks"

		req.NoError(checkJwksEndpointAllowed(policy, &db.ExternalJwtSigner{JwksEndpoint: &endpoint}))
	})

	t.Run("a signer whose jwks endpoint the configuration now refuses is reported", func(t *testing.T) {
		req := require.New(t)

		// the signer was created before the host list was set, so it is orphaned by the
		// current configuration and the operator needs to know at startup
		endpoint := "https://old-idp.example.com/jwks"

		err := checkJwksEndpointAllowed(policy, &db.ExternalJwtSigner{JwksEndpoint: &endpoint})

		req.Error(err)
		req.Contains(err.Error(), "allowedHostnames")
	})

	t.Run("a signer whose jwks endpoint is a blocked address is reported", func(t *testing.T) {
		req := require.New(t)

		endpoint := "http://169.254.169.254/latest/meta-data/"

		req.Error(checkJwksEndpointAllowed(NewJwksFetchPolicy(config.DefaultJwksFetch()),
			&db.ExternalJwtSigner{JwksEndpoint: &endpoint}))
	})
}

func Test_HardenedJwksResolver_Get(t *testing.T) {
	const jwksBody = `{"keys":[{"kty":"RSA","kid":"test-kid","n":"AQAB","e":"AQAB"}]}`

	t.Run("a valid JWKS endpoint resolves", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		resolver := NewHardenedJwksResolver(config.DefaultJwksFetch())

		response, body, err := resolver.Get(server.URL + "/jwks")

		req.NoError(err)
		req.Len(response.Keys, 1)
		req.Equal("test-kid", response.Keys[0].KeyId)
		req.Equal(jwksBody, string(body))
	})

	t.Run("a blocked address is refused without contacting the endpoint", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.BlockPrivateAddresses = true

		resolver := NewHardenedJwksResolver(jwksFetch)

		_, _, err := resolver.Get(server.URL + "/jwks")

		req.Error(err)
		req.Contains(err.Error(), "is not permitted")
		req.Zero(server.requestCount(), "the request must be stopped before it reaches the endpoint")
	})

	t.Run("a hostname that resolves to a blocked address is refused at dial time", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		// the URL host is a name, so only the resolved address can be checked - this is the
		// DNS rebinding case, and it is why enforcement lives in the dialer
		endpoint, err := server.urlWithHost("localhost")
		req.NoError(err)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.BlockPrivateAddresses = true

		resolver := NewHardenedJwksResolver(jwksFetch)

		_, _, err = resolver.Get(endpoint)

		req.Error(err)
		req.Contains(err.Error(), "is not permitted")
		req.Zero(server.requestCount(), "the request must be stopped before it reaches the endpoint")
	})

	t.Run("a redirect to a blocked address is refused", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		resolver := NewHardenedJwksResolver(config.DefaultJwksFetch())

		_, _, err := resolver.Get(server.URL + "/redirect-to-metadata")

		req.Error(err, "the metadata address must be blocked on a redirect hop, not just on the first hop")
		req.Contains(err.Error(), "is not permitted", "the redirect hop must be refused by the address policy")
	})

	t.Run("a host that is not in allowedHostnames is refused without contacting the endpoint", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		endpoint, err := server.urlWithHost("localhost")
		req.NoError(err)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.AllowedHostnames = []string{"idp.example.com"}

		resolver := NewHardenedJwksResolver(jwksFetch)

		_, _, err = resolver.Get(endpoint)

		req.Error(err)
		req.Contains(err.Error(), "allowedHostnames")
		req.Zero(server.requestCount(), "the request must be stopped before it reaches the endpoint")
	})

	t.Run("a host in deniedHostnames is refused without contacting the endpoint", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		endpoint, err := server.urlWithHost("localhost")
		req.NoError(err)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.DeniedHostnames = []string{"localhost"}

		resolver := NewHardenedJwksResolver(jwksFetch)

		_, _, err = resolver.Get(endpoint)

		req.Error(err)
		req.Contains(err.Error(), "deniedHostnames")
		req.Zero(server.requestCount())
	})

	t.Run("a host in allowedHostnames is fetched", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		endpoint, err := server.urlWithHost("localhost")
		req.NoError(err)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.AllowedHostnames = []string{"localhost"}

		resolver := NewHardenedJwksResolver(jwksFetch)

		response, _, err := resolver.Get(endpoint)

		req.NoError(err)
		req.Len(response.Keys, 1)
	})

	t.Run("a redirect to a host that is not in allowedHostnames is refused", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		endpoint, err := server.urlWithHost("localhost")
		req.NoError(err)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.AllowedHostnames = []string{"localhost"}

		resolver := NewHardenedJwksResolver(jwksFetch)

		// the first hop is an allowed host, the redirect target is not - each hop is checked
		// on its own
		_, _, err = resolver.Get(strings.Replace(endpoint, "/jwks", "/redirect-to-unlisted-host", 1))

		req.Error(err)
		req.Contains(err.Error(), "allowedHostnames")
	})

	t.Run("a redirect within the cap is followed", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		resolver := NewHardenedJwksResolver(config.DefaultJwksFetch())

		response, _, err := resolver.Get(server.URL + "/redirect-to-jwks")

		req.NoError(err)
		req.Len(response.Keys, 1)
	})

	t.Run("exceeding maxRedirects is an error", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.MaxRedirects = 2

		resolver := NewHardenedJwksResolver(jwksFetch)

		_, _, err := resolver.Get(server.URL + "/redirect-loop")

		req.Error(err)
		req.Contains(err.Error(), "redirect")
	})

	t.Run("maxRedirects of zero refuses to follow any redirect", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.MaxRedirects = 0

		resolver := NewHardenedJwksResolver(jwksFetch)

		_, _, err := resolver.Get(server.URL + "/redirect-to-jwks")

		req.Error(err)
		req.Contains(err.Error(), "redirect")
	})

	t.Run("a slow endpoint is cut off at the timeout", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		jwksFetch := config.DefaultJwksFetch()
		jwksFetch.Timeout = 100 * time.Millisecond

		resolver := NewHardenedJwksResolver(jwksFetch)

		start := time.Now()
		_, _, err := resolver.Get(server.URL + "/hang")
		elapsed := time.Since(start)

		req.Error(err)
		req.Less(elapsed, 10*time.Second, "the fetch must be bounded by the configured timeout")
	})

	t.Run("a non-http scheme is refused", func(t *testing.T) {
		endpoints := []string{
			"file:///etc/passwd",
			"ftp://example.com/jwks",
			"gopher://example.com:70/jwks",
			"/no/scheme/at/all",
		}

		for _, endpoint := range endpoints {
			t.Run(endpoint, func(t *testing.T) {
				req := require.New(t)

				resolver := NewHardenedJwksResolver(config.DefaultJwksFetch())

				_, _, err := resolver.Get(endpoint)

				req.Error(err)
			})
		}
	})

	t.Run("an unparsable url is refused", func(t *testing.T) {
		req := require.New(t)

		resolver := NewHardenedJwksResolver(config.DefaultJwksFetch())

		_, _, err := resolver.Get("http://[::1")

		req.Error(err)
	})

	t.Run("a non-200 status is an error", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		resolver := NewHardenedJwksResolver(config.DefaultJwksFetch())

		_, _, err := resolver.Get(server.URL + "/not-found")

		req.Error(err)
	})

	t.Run("a non-json content type is an error", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		resolver := NewHardenedJwksResolver(config.DefaultJwksFetch())

		_, _, err := resolver.Get(server.URL + "/html")

		req.Error(err)
	})

	t.Run("a body that is not a JWKS response is an error", func(t *testing.T) {
		req := require.New(t)

		server := newTestJwksServer(t, jwksBody)

		resolver := NewHardenedJwksResolver(config.DefaultJwksFetch())

		_, _, err := resolver.Get(server.URL + "/not-json")

		req.Error(err)
	})
}

// testJwksServer is a local endpoint that serves the routes the resolver tests exercise and
// counts the requests that reach it.
type testJwksServer struct {
	*httptest.Server
	requests atomic.Int32
}

// requestCount returns how many requests reached the server.
func (self *testJwksServer) requestCount() int {
	return int(self.requests.Load())
}

// urlWithHost returns the server's URL with its host replaced, keeping the port.
func (self *testJwksServer) urlWithHost(host string) (string, error) {
	parsed, err := url.Parse(self.URL)

	if err != nil {
		return "", err
	}

	parsed.Host = net.JoinHostPort(host, parsed.Port())

	return parsed.String() + "/jwks", nil
}

// newTestJwksServer starts a JWKS endpoint for the duration of the test.
func newTestJwksServer(t *testing.T, jwksBody string) *testJwksServer {
	result := &testJwksServer{}

	// released on test cleanup so the /hang route does not outlive the test
	done := make(chan struct{})

	result.Server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		result.requests.Add(1)

		switch request.URL.Path {
		case "/jwks":
			writer.Header().Set("content-type", "application/json")
			_, _ = writer.Write([]byte(jwksBody))
		case "/redirect-to-jwks":
			http.Redirect(writer, request, "/jwks", http.StatusFound)
		case "/redirect-loop":
			http.Redirect(writer, request, "/redirect-loop", http.StatusFound)
		case "/redirect-to-metadata":
			http.Redirect(writer, request, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
		case "/redirect-to-unlisted-host":
			http.Redirect(writer, request, "http://unlisted.example.com/jwks", http.StatusFound)
		case "/hang":
			<-done
		case "/html":
			writer.Header().Set("content-type", "text/html")
			_, _ = writer.Write([]byte("<html></html>"))
		case "/not-json":
			writer.Header().Set("content-type", "application/json")
			_, _ = writer.Write([]byte("this is not json"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))

	t.Cleanup(func() {
		close(done)
		result.Close()
	})

	return result
}

// mustCidrs parses CIDRs for test setup, failing the test if any cannot be parsed.
func mustCidrs(t *testing.T, values ...string) []*net.IPNet {
	t.Helper()

	var result []*net.IPNet

	for _, value := range values {
		_, ipNet, err := net.ParseCIDR(value)

		if err != nil {
			t.Fatalf("could not parse test CIDR %s: %v", value, err)
		}

		result = append(result, ipNet)
	}

	return result
}
