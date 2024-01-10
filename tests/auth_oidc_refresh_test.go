package tests

//
//import (
//	"bytes"
//	"encoding/json"
//	"github.com/go-resty/resty/v2"
//	"github.com/openziti/ziti/common"
//	"github.com/openziti/ziti/controller/oidc_auth"
//	"github.com/zitadel/oidc/v2/pkg/oidc"
//	"io"
//	"net/http"
//	"testing"
//	"time"
//)
//
//func Test_Authenticate_OIDC_Refresh(t *testing.T) {
//	ctx := NewTestContext(t)
//	defer ctx.Teardown()
//	ctx.StartServer()
//
//	rpServer, err := newOidcTestRp(ctx.ApiHost)
//	ctx.Req.NoError(err)
//
//	rpServer.Start()
//	defer rpServer.Stop()
//
//	//clientApiUrl, err := url.Parse("https://" + ctx.ApiHost + EdgeClientApiPath)
//	//ctx.Req.NoError(err)
//	//
//	//managementApiUrl, err := url.Parse("https://" + ctx.ApiHost + EdgeManagementApiPath)
//	//ctx.Req.NoError(err)
//
//	t.Run("authenticate", func(t *testing.T) {
//		ctx.testContextChanged(t)
//
//		client := resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport()))
//		client.SetRedirectPolicy(resty.DomainCheckRedirectPolicy("127.0.0.1", "localhost"))
//		resp, err := client.R().Get(rpServer.LoginUri)
//
//		ctx.Req.NoError(err)
//		ctx.Req.Equal(http.StatusOK, resp.StatusCode())
//
//		authRequestId := resp.Header().Get(oidc_auth.AuthRequestIdHeader)
//		ctx.Req.NotEmpty(authRequestId)
//
//		opLoginUri := "https://" + resp.RawResponse.Request.URL.Host + "/oidc/login/username"
//
//		resp, err = client.R().SetFormData(map[string]string{"id": authRequestId, "username": ctx.AdminAuthenticator.Username, "password": ctx.AdminAuthenticator.Password}).Post(opLoginUri)
//
//		ctx.Req.NoError(err)
//		ctx.Req.Equal(http.StatusOK, resp.StatusCode())
//
//		var outTokens *oidc.Tokens[*common.IdTokenClaims]
//
//		select {
//		case tokens := <-rpServer.TokenChan:
//			outTokens = tokens
//		case <-time.After(5 * time.Second):
//			ctx.Fail("no tokens received, hit timeout")
//		}
//
//		ctx.Req.NotNil(outTokens)
//		ctx.Req.NotEmpty(outTokens.IDToken)
//		ctx.Req.NotEmpty(outTokens.IDTokenClaims)
//		ctx.Req.NotEmpty(outTokens.AccessToken)
//		ctx.Req.NotEmpty(outTokens.RefreshToken)
//
//		t.Run("attempt to exchange tokens", func(t *testing.T) {
//			refreshToken := outTokens.RefreshToken
//			clientID := "native"
//
//			// OIDC token endpoint for your provider
//			tokenEndpoint := "https://" + resp.RawResponse.Request.URL.Host + "/oidc/token"
//
//			// Prepare the request body
//			requestBody, _ := json.Marshal(map[string]string{
//				"grant_type":    "refresh_token",
//				"refresh_token": refreshToken,
//				"client_id":     clientID,
//			})
//
//			// Create a new HTTP request
//			req, err := http.NewRequest("POST", tokenEndpoint, bytes.NewBuffer(requestBody))
//			if err != nil {
//				panic(err)
//			}
//
//			// Set appropriate headers
//			req.Header.Set("Content-Type", "application/json")
//
//			// Make the HTTP request
//			client := &http.Client{}
//			resp, err := client.Do(req)
//			if err != nil {
//				panic(err)
//			}
//			defer resp.Body.Close()
//
//			// Read and unmarshal the response body
//			body, err := io.ReadAll(resp.Body)
//			if err != nil {
//				panic(err)
//			}
//
//			var tokenResponse oidc.TokenExchangeResponse
//			err = json.Unmarshal(body, &tokenResponse)
//			if err != nil {
//				panic(err)
//			}
//
//			println(tokenResponse)
//
//		})
//	})
//}
