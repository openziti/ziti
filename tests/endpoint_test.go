package tests

import (
	"testing"
)

// Test_Endpoints does HTTP testing against public entry URLs to ensure they continue to function.
// Non-prefixed paths are deprecated, but some older clients do not use the edge/client/v1 path.
// The .well-known path has many handlers among different APIs and tests for those should exist
// perpetuity.
func Test_Endpoints(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	t.Run("non-prefixed path defaults for enrollment", func(t *testing.T) {
		ctx.testContextChanged(t)

		rootPathClient, _, _ := ctx.NewClientComponents("/")

		resp, err := rootPathClient.R().Post("https://" + ctx.ApiHost + "/enroll")

		ctx.Req.NoError(err)
		ctx.Req.Equal(400, resp.StatusCode())
		ctx.Req.Equal("application/json", resp.Header().Get("Content-Type"))
		ctx.Req.NotEmpty(resp.Body())
	})

	t.Run("non-prefixed path defaults for authentication", func(t *testing.T) {
		ctx.testContextChanged(t)

		rootPathClient, _, _ := ctx.NewClientComponents("/")

		resp, err := rootPathClient.R().Post("https://" + ctx.ApiHost + "/authenticate")

		ctx.Req.NoError(err)
		ctx.Req.Equal(400, resp.StatusCode())
		ctx.Req.Equal("application/json", resp.Header().Get("Content-Type"))
		ctx.Req.NotEmpty(resp.Body())
	})

	t.Run("oidc-configuration works on .well-known", func(t *testing.T) {
		ctx.testContextChanged(t)

		rootPathClient, _, _ := ctx.NewClientComponents("/")

		resp, err := rootPathClient.R().Get("https://" + ctx.ApiHost + "/.well-known/openid-configuration")

		ctx.Req.NoError(err)
		ctx.Req.Equal(200, resp.StatusCode())
		ctx.Req.Equal("application/json", resp.Header().Get("Content-Type"))
		ctx.Req.NotEmpty(resp.Body())
	})

	t.Run("est castore works on .well-known", func(t *testing.T) {
		ctx.testContextChanged(t)

		rootPathClient, _, _ := ctx.NewClientComponents("/")

		resp, err := rootPathClient.R().Get("https://" + ctx.ApiHost + "/.well-known/est/cacerts")

		ctx.Req.NoError(err)
		ctx.Req.Equal(200, resp.StatusCode())
		ctx.Req.Equal("application/pkcs7-mime", resp.Header().Get("Content-Type"))
		ctx.Req.NotEmpty(resp.Body())
	})
}
