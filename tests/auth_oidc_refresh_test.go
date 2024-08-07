package tests

import (
	service2 "github.com/openziti/edge-api/rest_client_api_client/service"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/controller/oidc_auth"
	"github.com/zitadel/oidc/v2/pkg/oidc"
	"golang.org/x/oauth2"
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

		t.Run("can refresh the token backed API Session via Refresh Token Request", func(t *testing.T) {
			ctx.testContextChanged(t)

			oidcApiSession := apiSession.(*edge_apis.ApiSessionOidc)

			req := &oidc.RefreshTokenRequest{
				RefreshToken: oidcApiSession.OidcTokens.RefreshToken,
				ClientID:     common.ClaimAudienceOpenZiti,
				Scopes:       []string{"openid", "offline_access"},
			}

			enc := oidc.NewEncoder()
			dst := map[string][]string{}
			err := enc.Encode(req, dst)
			ctx.Req.NoError(err)

			dst["grant_type"] = []string{string(req.GrantType())}

			newTokens := &oidc.TokenExchangeResponse{}
			cReq := ctx.newAnonymousClientApiRequest().SetHeader("content-type", oidc_auth.FormContentType).SetMultiValueFormData(dst).SetResult(newTokens)

			resp, err := cReq.Post("https://" + ctx.ApiHost + "/oidc/oauth/token")
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(200, resp.StatusCode(), "expected status %d, got %d, request: %s", 200, resp.StatusCode(), resp.String())

			t.Run("can use the new token backed API Session", func(t *testing.T) {

				newApiSession := &edge_apis.ApiSessionOidc{
					OidcTokens: &oidc.Tokens[*oidc.IDTokenClaims]{
						Token: &oauth2.Token{
							AccessToken:  newTokens.AccessToken,
							TokenType:    newTokens.TokenType,
							RefreshToken: newTokens.RefreshToken,
						},
						IDTokenClaims: oidcApiSession.OidcTokens.IDTokenClaims,
						IDToken:       oidcApiSession.OidcTokens.IDToken,
					},
				}

				newApiSessionI := edge_apis.ApiSession(newApiSession)
				ctx.testContextChanged(t)
				client.ApiSession.Store(&newApiSessionI)

				params := service2.NewListServicesParams()
				result, err := client.API.Service.ListServices(params, nil)
				ctx.Req.NoError(err)
				ctx.NotNil(result)
			})
		})

		t.Run("can refresh the token backed API Session via Token Exchange Request", func(t *testing.T) {
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
