package tests

import (
	service2 "github.com/openziti/edge-api/rest_client_api_client/service"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"net/url"
	"testing"
)

func Test_Authenticate_OIDC_Refresh(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	rpServer, err := newOidcTestRp(ctx.ApiHost)
	ctx.Req.NoError(err)

	rpServer.Start()
	defer rpServer.Stop()

	clientApiUrl, err := url.Parse("https://" + ctx.ApiHost + EdgeClientApiPath)
	ctx.Req.NoError(err)

	client := edge_apis.NewClientApiClient([]*url.URL{clientApiUrl}, ctx.ControllerConfig.Id.CA(), nil)

	client.Credentials = edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	client.SetUseOidc(true)

	t.Run("can authenticate to obtain a new token backed API Session", func(t *testing.T) {
		ctx.testContextChanged(t)

		apiSession, err := client.Authenticate(client.Credentials, nil)
		ctx.Req.NoError(err)
		ctx.Req.IsType(&edge_apis.ApiSessionOidc{}, apiSession)

		t.Run("can refresh the token backed API Session", func(t *testing.T) {
			ctx.testContextChanged(t)

			newSession, err := client.API.RefreshApiSession(apiSession, client.HttpClient)

			ctx.Req.NoError(err)
			ctx.NotNil(newSession)
			ctx.NotNil(newSession.GetToken())
			ctx.NotEqual(newSession.GetToken(), apiSession.GetToken())

			t.Run("can use the new token backed API Session", func(t *testing.T) {
				ctx.testContextChanged(t)
				client.ApiSession.Store(&newSession)

				params := service2.NewListServicesParams()
				result, err := client.API.Service.ListServices(params, nil)
				ctx.Req.NoError(err)
				ctx.NotNil(result)
			})
		})
	})

}
