package tests

import (
	"testing"
)

func Test_Root_Endpoints(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	t.Run("version can be retrieved", func(t *testing.T) {

		t.Run("version can be retrieved from root path plus version", func(t *testing.T) {
			ctx.testContextChanged(t)
			versionRootUrl := "https://" + ctx.ApiHost + "/version"

			resp, err := ctx.newAnonymousClientApiRequest().Get(versionRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())
			ctx.Req.NotEmpty(resp.Body())
		})

		t.Run("version can be retrieved from root path", func(t *testing.T) {
			ctx.testContextChanged(t)
			versionRootUrl := "https://" + ctx.ApiHost + "/"

			resp, err := ctx.newAnonymousClientApiRequest().Get(versionRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())
			ctx.Req.NotEmpty(resp.Body())
		})

		t.Run("version can be retrieved from the base management path plus version", func(t *testing.T) {
			ctx.testContextChanged(t)
			versionRootUrl := "https://" + ctx.ApiHost + "/edge/management/v1/version"

			resp, err := ctx.newAnonymousClientApiRequest().Get(versionRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())
			ctx.Req.NotEmpty(resp.Body())
		})

		t.Run("version can be retrieved from the base management path", func(t *testing.T) {
			ctx.testContextChanged(t)
			versionRootUrl := "https://" + ctx.ApiHost + "/edge/management/v1"

			resp, err := ctx.newAnonymousClientApiRequest().Get(versionRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())
			ctx.Req.NotEmpty(resp.Body())
		})

		t.Run("version can be retrieved from the base client path plus version", func(t *testing.T) {
			ctx.testContextChanged(t)
			versionRootUrl := "https://" + ctx.ApiHost + "/edge/client/v1/version"

			resp, err := ctx.newAnonymousClientApiRequest().Get(versionRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())
			ctx.Req.NotEmpty(resp.Body())
		})

		t.Run("version can be retrieved from the base client path", func(t *testing.T) {
			ctx.testContextChanged(t)
			versionRootUrl := "https://" + ctx.ApiHost + "/edge/client/v1"

			resp, err := ctx.newAnonymousClientApiRequest().Get(versionRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())
			ctx.Req.NotEmpty(resp.Body())
		})
	})

	t.Run(".well-known endpoint can be retrieved", func(t *testing.T) {
		t.Run("well-known/est/ca/certs can be retrieved from root path", func(t *testing.T) {
			ctx.testContextChanged(t)
			caCertsRootUrl := "https://" + ctx.ApiHost + "/.well-known/est/cacerts"

			resp, err := ctx.newAnonymousClientApiRequest().Get(caCertsRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())
			ctx.Req.NotEmpty(resp.Body())
		})

		t.Run("well-known/est/ca/certs can be retrieved from management path", func(t *testing.T) {
			ctx.testContextChanged(t)
			caCertsRootUrl := "https://" + ctx.ApiHost + "/edge/management/v1//.well-known/est/cacerts"

			resp, err := ctx.newAnonymousClientApiRequest().Get(caCertsRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())
			ctx.Req.NotEmpty(resp.Body())
		})

		t.Run("well-known/est/ca/certs can be retrieved from client path", func(t *testing.T) {
			ctx.testContextChanged(t)
			caCertsRootUrl := "https://" + ctx.ApiHost + "/edge/client/v1/.well-known/est/cacerts"

			resp, err := ctx.newAnonymousClientApiRequest().Get(caCertsRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())
			ctx.Req.NotEmpty(resp.Body())
		})
	})
}
