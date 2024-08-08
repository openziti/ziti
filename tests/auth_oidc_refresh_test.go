package tests

import (
	"github.com/golang-jwt/jwt/v5"
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

		oidcApiSession := apiSession.(*edge_apis.ApiSessionOidc)
		jwtParser := jwt.NewParser()

		idClaims := &jwt.MapClaims{}
		idToken, _, err := jwtParser.ParseUnverified(oidcApiSession.OidcTokens.IDToken, idClaims)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(idToken)
		originalIdAudiences, err := idToken.Claims.GetAudience()
		ctx.Req.NoError(err)
		ctx.Req.Len(originalIdAudiences, 1, "expected 1 audience")

		accessClaims := &jwt.MapClaims{}
		acccessToken, _, err := jwtParser.ParseUnverified(oidcApiSession.OidcTokens.AccessToken, accessClaims)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(acccessToken)
		originalAccessAudiences, err := acccessToken.Claims.GetAudience()
		ctx.Req.NoError(err)
		ctx.Req.Len(originalAccessAudiences, 1, "expected 1 audience")
		ctx.Req.NotEmpty(originalAccessAudiences[0], "expected access audience to be a non-empty string")

		refreshClaims := &jwt.MapClaims{}
		refreshToken, _, err := jwtParser.ParseUnverified(oidcApiSession.OidcTokens.RefreshToken, refreshClaims)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(refreshToken)
		originalRefreshAudiences, err := refreshToken.Claims.GetAudience()
		ctx.Req.NoError(err)
		ctx.Req.Len(originalRefreshAudiences, 1, "expected 1 audience")
		ctx.Req.NotEmpty(originalRefreshAudiences[0], "expected refresh audience to be a non-empty string")

		ctx.Req.Equal(originalIdAudiences[0], originalAccessAudiences[0], "id token and access token audiences do not match, id: %s, access: %s", originalIdAudiences[0], originalAccessAudiences[0])
		ctx.Req.Equal(originalAccessAudiences[0], originalRefreshAudiences[0], "access token and refresh token audiences do not match, access: %s, refresh: %s", originalAccessAudiences[0], originalRefreshAudiences[0])

		originalAccessScopes := (*accessClaims)["scopes"]
		ctx.Req.Len(originalAccessScopes, 2)
		ctx.Req.Contains(originalAccessScopes, oidc.ScopeOpenID)
		ctx.Req.Contains(originalAccessScopes, oidc.ScopeOfflineAccess)

		originalRefreshScopes := (*refreshClaims)["scopes"]
		ctx.Req.Len(originalRefreshScopes, 2)
		ctx.Req.Contains(originalRefreshScopes, oidc.ScopeOpenID)
		ctx.Req.Contains(originalRefreshScopes, oidc.ScopeOfflineAccess)

		t.Run("can refresh the token backed API Session via Refresh Token Request", func(t *testing.T) {
			ctx.testContextChanged(t)

			oidcApiSession := apiSession.(*edge_apis.ApiSessionOidc)

			req := &oidc.RefreshTokenRequest{
				RefreshToken: oidcApiSession.OidcTokens.RefreshToken,
				ClientID:     oidcApiSession.OidcTokens.IDTokenClaims.ClientID,
				Scopes:       []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess},
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

			t.Run("the new token has the correct audience and scopes", func(t *testing.T) {
				ctx.testContextChanged(t)

				newAccessClaims := &jwt.MapClaims{}
				newAccessToken, _, err := jwtParser.ParseUnverified(newTokens.AccessToken, newAccessClaims)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(newAccessToken)
				newAccessAudiences, err := newAccessToken.Claims.GetAudience()
				ctx.Req.NoError(err)
				ctx.Req.Len(newAccessAudiences, 1, "expected 1 audience")
				ctx.Equal(originalAccessAudiences[0], newAccessAudiences[0])

				newRefreshClaims := &jwt.MapClaims{}
				newRefreshToken, _, err := jwtParser.ParseUnverified(newTokens.RefreshToken, newRefreshClaims)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(newRefreshToken)
				newRefreshAudiences, err := newRefreshToken.Claims.GetAudience()
				ctx.Req.NoError(err)
				ctx.Req.Len(newRefreshAudiences, 1, "expected 1 audience")
				ctx.Equal(originalRefreshAudiences[0], newRefreshAudiences[0])

				newAccessScopes := (*newAccessClaims)["scopes"]
				ctx.Req.Len(newAccessScopes, 2)
				ctx.Req.Contains(newAccessScopes, oidc.ScopeOpenID)
				ctx.Req.Contains(newAccessScopes, oidc.ScopeOfflineAccess)

				newRefreshScopes := (*newRefreshClaims)["scopes"]
				ctx.Req.Len(newRefreshScopes, 2)
				ctx.Req.Contains(newRefreshScopes, oidc.ScopeOpenID)
				ctx.Req.Contains(newRefreshScopes, oidc.ScopeOfflineAccess)

			})

			t.Run("can use the new token backed API Session", func(t *testing.T) {
				ctx.testContextChanged(t)
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

		t.Run("can not refresh the token backed API Session via Refresh Token Request with the wrong client id", func(t *testing.T) {
			ctx.testContextChanged(t)

			oidcApiSession := apiSession.(*edge_apis.ApiSessionOidc)

			req := &oidc.RefreshTokenRequest{
				RefreshToken: oidcApiSession.OidcTokens.RefreshToken,
				ClientID:     "123",
				Scopes:       []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess},
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
			ctx.Req.Equal(400, resp.StatusCode(), "expected status %d, got %d, request: %s", 200, resp.StatusCode(), resp.String())
		})

		t.Run("can not refresh the token backed API Session via Refresh Token Request by swapping client ids", func(t *testing.T) {
			ctx.testContextChanged(t)

			oidcApiSession := apiSession.(*edge_apis.ApiSessionOidc)

			clientId := common.ClaimAudienceOpenZiti
			if oidcApiSession.OidcTokens.IDTokenClaims.ClientID == common.ClaimAudienceOpenZiti {
				clientId = common.ClaimLegacyNative
			}
			req := &oidc.RefreshTokenRequest{
				RefreshToken: oidcApiSession.OidcTokens.RefreshToken,
				ClientID:     clientId,
				Scopes:       []string{oidc.ScopeOpenID, oidc.ScopeOfflineAccess},
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
			ctx.Req.Equal(400, resp.StatusCode(), "expected status %d, got %d, request: %s", 200, resp.StatusCode(), resp.String())
		})

		t.Run("can not refresh the token backed API Session via Refresh Token Request with invalid scopes", func(t *testing.T) {
			ctx.testContextChanged(t)

			oidcApiSession := apiSession.(*edge_apis.ApiSessionOidc)

			req := &oidc.RefreshTokenRequest{
				RefreshToken: oidcApiSession.OidcTokens.RefreshToken,
				ClientID:     oidcApiSession.OidcTokens.IDTokenClaims.ClientID,
				Scopes:       []string{"myscope"},
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
			ctx.Req.Equal(400, resp.StatusCode(), "expected status %d, got %d, request: %s", 200, resp.StatusCode(), resp.String())
		})

		t.Run("can refresh the token backed API Session via Token Exchange Request", func(t *testing.T) {
			ctx.testContextChanged(t)

			newSession, err := client.API.RefreshApiSession(apiSession, client.HttpClient)

			ctx.Req.NoError(err)
			ctx.NotNil(newSession)
			ctx.NotNil(newSession.GetToken())
			ctx.NotEqual(newSession.GetToken(), apiSession.GetToken())
			ctx.IsType(newSession, &edge_apis.ApiSessionOidc{})

			newTokens := newSession.(*edge_apis.ApiSessionOidc)

			t.Run("the new token has the correct audiences", func(t *testing.T) {
				ctx.testContextChanged(t)

				newAccessClaims := &jwt.MapClaims{}
				newAccessToken, _, err := jwtParser.ParseUnverified(newTokens.OidcTokens.AccessToken, newAccessClaims)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(newAccessToken)
				newAccessAudiences, err := newAccessToken.Claims.GetAudience()
				ctx.Req.NoError(err)
				ctx.Req.Len(newAccessAudiences, 1, "expected 1 audience")
				ctx.Equal(originalAccessAudiences[0], newAccessAudiences[0])

				newRefreshClaims := &jwt.MapClaims{}
				newRefreshToken, _, err := jwtParser.ParseUnverified(newTokens.OidcTokens.RefreshToken, newRefreshClaims)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(newRefreshToken)
				newRefreshAudiences, err := newRefreshToken.Claims.GetAudience()
				ctx.Req.NoError(err)
				ctx.Req.Len(newRefreshAudiences, 1, "expected 1 audience")
				ctx.Equal(originalRefreshAudiences[0], newRefreshAudiences[0])
			})

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
