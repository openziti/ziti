package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func Test_OidcDiscoveryEndpoints(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	t.Run("discovery response contains openziti_endpoints", func(t *testing.T) {
		ctx.testContextChanged(t)

		discoveryUrl := "https://" + ctx.ApiHost + "/oidc/.well-known/openid-configuration"
		resp, err := ctx.newAnonymousClientApiRequest().Get(discoveryUrl)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		var discovery map[string]interface{}
		err = json.Unmarshal(resp.Body(), &discovery)
		ctx.Req.NoError(err)

		t.Run("standard OIDC fields are still present", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.Contains(discovery, "issuer")
			ctx.Req.Contains(discovery, "token_endpoint")
			ctx.Req.Contains(discovery, "authorization_endpoint")
		})

		t.Run("openziti_endpoints field exists and is an object", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.Contains(discovery, "openziti_endpoints")

			endpoints, ok := discovery["openziti_endpoints"].(map[string]interface{})
			ctx.Req.True(ok, "openziti_endpoints should be a JSON object")

			issuer, ok := discovery["issuer"].(string)
			ctx.Req.True(ok, "issuer should be a string")

			expectedEndpoints := map[string]string{
				"password":           issuer + "/login/password",
				"cert":               issuer + "/login/cert",
				"ext_jwt":            issuer + "/login/ext-jwt",
				"totp":               issuer + "/login/totp",
				"totp_enroll":        issuer + "/login/totp/enroll",
				"totp_enroll_verify": issuer + "/login/totp/enroll/verify",
				"auth_queries":       issuer + "/login/auth-queries",
			}

			for key, expectedUrl := range expectedEndpoints {
				t.Run(key+" endpoint is present", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.Req.Contains(endpoints, key)
					url, ok := endpoints[key].(string)
					ctx.Req.True(ok, "%q should be a string", key)
					ctx.Req.True(strings.HasPrefix(url, "https://"), "%q should be an absolute https URL", key)
					ctx.Req.Equal(expectedUrl, url)
				})
			}
		})
	})
}

// Test_OidcDiscoveryEndpoints_DualServers verifies that when the controller exposes the
// edge-oidc API on two web servers with different ports, the OIDC discovery document
// returned by each server contains endpoint URLs that reflect the port the client
// connected to.
func Test_OidcDiscoveryEndpoints_DualServers(t *testing.T) {
	ctx := NewTestContextWithConfigSet(t, DualOidcServers)
	defer ctx.Teardown()
	ctx.StartServer()

	primaryHost := "127.0.0.1:1281"
	secondaryHost := "127.0.0.1:1282"

	for _, host := range []string{primaryHost, secondaryHost} {
		t.Run("discovery on "+host, func(t *testing.T) {
			ctx.testContextChanged(t)

			client := ctx.NewRestClientWithDefaults()
			discoveryUrl := "https://" + host + "/oidc/.well-known/openid-configuration"
			resp, err := client.R().Get(discoveryUrl)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			var discovery map[string]interface{}
			err = json.Unmarshal(resp.Body(), &discovery)
			ctx.Req.NoError(err)

			issuer, ok := discovery["issuer"].(string)
			ctx.Req.True(ok, "issuer should be a string")
			ctx.Req.Contains(issuer, host, "issuer should contain the host the client connected to")

			endpoints, ok := discovery["openziti_endpoints"].(map[string]interface{})
			ctx.Req.True(ok, "openziti_endpoints should be a JSON object")

			for _, key := range []string{"password", "cert", "ext_jwt", "totp", "totp_enroll", "totp_enroll_verify", "auth_queries"} {
				t.Run(key+" uses correct host", func(t *testing.T) {
					ctx.testContextChanged(t)
					url, ok := endpoints[key].(string)
					ctx.Req.True(ok, "%q should be a string", key)
					ctx.Req.True(strings.HasPrefix(url, issuer+"/login/"),
						"%q URL %q should start with issuer %q", key, url, issuer)
				})
			}
		})
	}
}
