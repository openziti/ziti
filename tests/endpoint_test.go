package tests

import (
	"encoding/json"
	"github.com/openziti/edge-api/rest_model"
	"net/http"
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

	t.Run("oidc-configuration does not work on root .well-known", func(t *testing.T) {
		ctx.testContextChanged(t)

		rootPathClient, _, _ := ctx.NewClientComponents("/")

		resp, err := rootPathClient.R().Get("https://" + ctx.ApiHost + "/.well-known/openid-configuration")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusNotFound, resp.StatusCode())
	})

	t.Run("oidc-configuration works on oidc/.well-known", func(t *testing.T) {
		ctx.testContextChanged(t)

		rootPathClient, _, _ := ctx.NewClientComponents("/")

		resp, err := rootPathClient.R().Get("https://" + ctx.ApiHost + "/oidc/.well-known/openid-configuration")

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

	t.Run("the version endpoint", func(t *testing.T) {

		t.Run("responds on root /version", func(t *testing.T) {
			ctx.testContextChanged(t)

			rootResp, err := ctx.newAnonymousClientApiRequest().Get("https://" + ctx.ApiHost + "/version")
			ctx.Req.NoError(err)
			ctx.Req.Equal(200, rootResp.StatusCode())

			t.Run("has the proper values", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.Req.Equal("application/json", rootResp.Header().Get("Content-Type"))

				data := &rest_model.Version{}
				envelope := &rest_model.Empty{
					Data: data,
				}

				err = json.Unmarshal(rootResp.Body(), envelope)
				ctx.Req.NoError(err)

				ctx.Req.NotEmpty(envelope)
				ctx.Req.NotEmpty(envelope.Data)
				ctx.Req.NotEmpty(data)
				ctx.Req.NotEmpty(data.Version)
				ctx.Req.NotEmpty(data.APIVersions)
				ctx.Req.NotEmpty(data.BuildDate)
				ctx.Req.NotEmpty(data.Capabilities)
				ctx.Req.NotEmpty(data.Revision)
				ctx.Req.NotEmpty(data.RuntimeVersion)

				ctx.Req.Contains(data.APIVersions, "edge")
				ctx.Req.Contains(data.APIVersions["edge"], "v1")
				ctx.Req.Equal(*data.APIVersions["edge"]["v1"].Path, "/edge/client/v1")

				ctx.Req.Contains(data.APIVersions, "edge-client")
				ctx.Req.Contains(data.APIVersions["edge-client"], "v1")
				ctx.Req.Equal(*data.APIVersions["edge-client"]["v1"].Path, "/edge/client/v1")

				ctx.Req.Contains(data.APIVersions, "edge-management")
				ctx.Req.Contains(data.APIVersions["edge-management"], "v1")
				ctx.Req.Equal(*data.APIVersions["edge-management"]["v1"].Path, "/edge/management/v1")

				ctx.Req.Contains(data.APIVersions, "edge-oidc")
				ctx.Req.Contains(data.APIVersions["edge-oidc"], "v1")
				ctx.Req.Equal(*data.APIVersions["edge-oidc"]["v1"].Path, "/oidc")

				ctx.Req.Contains(data.APIVersions, "health-checks")
				ctx.Req.Contains(data.APIVersions["health-checks"], "v1")
				ctx.Req.Equal(*data.APIVersions["health-checks"]["v1"].Path, "/health-checks/v1")
			})

			t.Run("responds on /edge/client/v1/version", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := ctx.newAnonymousClientApiRequest().Get("version")
				ctx.Req.NoError(err)
				ctx.Req.Equal(200, resp.StatusCode())
				ctx.Req.Equal(rootResp.Body(), resp.Body())
			})

			t.Run("responds on /edge/management/v1/version", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := ctx.newAnonymousManagementApiRequest().Get("version")
				ctx.Req.NoError(err)
				ctx.Req.Equal(200, resp.StatusCode())
				ctx.Req.Equal(rootResp.Body(), resp.Body())
			})
		})

	})
}
