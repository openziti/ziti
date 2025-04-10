package tests

import (
	"github.com/openziti/edge-api/rest_model"
	"testing"
)

func Test_NetworkJwts(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	t.Run("network-jwts can be retrieved via", func(t *testing.T) {

		t.Run("deprecated root path", func(t *testing.T) {
			ctx.testContextChanged(t)
			jwtRootUrl := "https://" + ctx.ApiHost + "/network-jwts"

			respEnv := &rest_model.ListNetworkJWTsEnvelope{}
			resp, err := ctx.newAnonymousClientApiRequest().SetResult(respEnv).Get(jwtRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())

			ctx.Req.Len(respEnv.Data, 1)
			ctx.Req.NotNil(respEnv.Data[0].Name)
			ctx.Req.Equal("default", *respEnv.Data[0].Name)
			ctx.Req.NotEmpty(respEnv.Data[0].Token)
		})

		t.Run("client path", func(t *testing.T) {
			ctx.testContextChanged(t)
			jwtRootUrl := "https://" + ctx.ApiHost + "/edge/client/v1/network-jwts"

			respEnv := &rest_model.ListNetworkJWTsEnvelope{}
			resp, err := ctx.newAnonymousClientApiRequest().SetResult(respEnv).Get(jwtRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())

			ctx.Req.Len(respEnv.Data, 1)
			ctx.Req.NotNil(respEnv.Data[0].Name)
			ctx.Req.Equal("default", *respEnv.Data[0].Name)
			ctx.Req.NotEmpty(respEnv.Data[0].Token)
		})

		t.Run("management path", func(t *testing.T) {
			ctx.testContextChanged(t)
			jwtRootUrl := "https://" + ctx.ApiHost + "/edge/management/v1/network-jwts"

			respEnv := &rest_model.ListNetworkJWTsEnvelope{}
			resp, err := ctx.newAnonymousClientApiRequest().SetResult(respEnv).Get(jwtRootUrl)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode())

			ctx.Req.Len(respEnv.Data, 1)
			ctx.Req.NotNil(respEnv.Data[0].Name)
			ctx.Req.Equal("default", *respEnv.Data[0].Name)
			ctx.Req.NotEmpty(respEnv.Data[0].Token)
		})
	})
}
