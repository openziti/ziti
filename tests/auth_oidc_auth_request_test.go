//go:build apitests

/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
)

func Test_OIDC_AuthRequest_Expiration(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	clientHelper := ctx.NewEdgeClientApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	loginUrl := "https://" + ctx.ApiHost + "/oidc/login"

	t.Run("GET auth-queries carries the auth request expiresAt and expirationSeconds", func(t *testing.T) {
		ctx.NextTest(t)
		body := &struct {
			ExpiresAt         *strfmt.DateTime `json:"expiresAt"`
			ExpirationSeconds *int64           `json:"expirationSeconds"`
		}{}
		before := time.Now().Truncate(time.Second)
		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		resp, err := result.Client.R().
			SetHeader("accept", oidc_auth.JsonContentType).
			SetResult(body).
			Get(loginUrl + "/auth-queries?id=" + result.AuthRequestId)

		after := time.Now()
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		ctx.Req.NotNil(body.ExpirationSeconds, "expirationSeconds missing, body: %s", resp.String())
		ctx.Req.Equal(int64(common.DefaultAuthRequestDuration.Seconds()), *body.ExpirationSeconds)
		ctx.Req.NotNil(body.ExpiresAt, "expiresAt missing, body: %s", resp.String())
		expiresAt := time.Time(*body.ExpiresAt)
		ctx.Req.False(expiresAt.Before(before.Add(common.DefaultAuthRequestDuration)), "expiresAt %s is earlier than the auth request could have been created", expiresAt)
		ctx.Req.False(expiresAt.After(after.Add(common.DefaultAuthRequestDuration)), "expiresAt %s is later than the auth request could have been created", expiresAt)
	})
}

func Test_OIDC_AuthRequest_HtmlPasswordLoginWithTotp(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	clientHelper := ctx.NewEdgeClientApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	loginUrl := "https://" + ctx.ApiHost + "/oidc/login"

	t.Run("POST password as HTML renders the TOTP page for an identity with TOTP enrolled", func(t *testing.T) {
		ctx.NextTest(t)
		_, err := clientHelper.Authenticate(adminCreds, nil)
		ctx.Req.NoError(err)
		_, _, err = clientHelper.EnrollTotpMfa()
		ctx.Req.NoError(err)
		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		resp, err := result.Client.R().
			SetHeader("accept", oidc_auth.HtmlContentType).
			SetHeader("content-type", oidc_auth.FormContentType).
			SetFormData(map[string]string{
				"id":       result.AuthRequestId,
				"username": ctx.AdminAuthenticator.Username,
				"password": ctx.AdminAuthenticator.Password,
			}).
			Post(loginUrl + "/password")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		ctx.Req.Contains(resp.String(), `action="/oidc/login/totp"`, "expected the TOTP page")
		ctx.Req.Contains(resp.String(), "display: none;", "expected no error on the TOTP page")
	})
}

func Test_OIDC_AuthRequest_UnknownId(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	clientHelper := ctx.NewEdgeClientApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	loginUrl := "https://" + ctx.ApiHost + "/oidc/login"

	for _, method := range []string{"password", "username", "cert", "ext-jwt"} {
		t.Run("POST "+method+" returns 401 UNAUTHORIZED for an unknown auth request id", func(t *testing.T) {
			ctx.NextTest(t)
			apiErr := &rest_model.APIError{}

			resp, err := ctx.newAnonymousClientApiRequest().
				SetHeader("accept", oidc_auth.JsonContentType).
				SetHeader("content-type", oidc_auth.JsonContentType).
				SetBody(map[string]string{
					"id":       uuid.NewString(),
					"username": ctx.AdminAuthenticator.Username,
					"password": ctx.AdminAuthenticator.Password,
				}).
				Post(loginUrl + "/" + method)

			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
			ctx.Req.NoError(apiErr.UnmarshalBinary(resp.Body()))
			ctx.Req.Equal(errorz.UnauthorizedCode, apiErr.Code)
		})
	}

	for _, method := range []string{"password", "username", "cert", "ext-jwt"} {
		t.Run("GET "+method+" as JSON returns 401 UNAUTHORIZED for an unknown auth request id", func(t *testing.T) {
			ctx.NextTest(t)
			apiErr := &rest_model.APIError{}

			resp, err := ctx.newAnonymousClientApiRequest().
				SetHeader("accept", oidc_auth.JsonContentType).
				Get(loginUrl + "/" + method + "?authRequestID=" + uuid.NewString())

			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
			ctx.Req.NoError(apiErr.UnmarshalBinary(resp.Body()), "expected a JSON API error, body: %s", resp.String())
			ctx.Req.Equal(errorz.UnauthorizedCode, apiErr.Code)
		})
	}

	for _, method := range []string{"password", "username"} {
		t.Run("GET "+method+" as HTML returns 401 with a timed out message for an unknown auth request id", func(t *testing.T) {
			ctx.NextTest(t)

			resp, err := ctx.newAnonymousClientApiRequest().
				SetHeader("accept", oidc_auth.HtmlContentType).
				Get(loginUrl + "/" + method + "?authRequestID=" + uuid.NewString())

			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
			ctx.Req.Contains(resp.Header().Get("content-type"), oidc_auth.HtmlContentType)
			ctx.Req.Contains(resp.String(), "Login timed out. Return to your application to start a new login.")
		})
	}

	for _, method := range []string{"cert", "ext-jwt"} {
		t.Run("GET "+method+" as HTML returns 401 for an unknown auth request id", func(t *testing.T) {
			ctx.NextTest(t)

			resp, err := ctx.newAnonymousClientApiRequest().
				SetHeader("accept", oidc_auth.HtmlContentType).
				Get(loginUrl + "/" + method + "?authRequestID=" + uuid.NewString())

			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		})
	}

	t.Run("GET username as HTML renders the login page for a live auth request id", func(t *testing.T) {
		ctx.NextTest(t)
		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		resp, err := result.Client.R().
			SetHeader("accept", oidc_auth.HtmlContentType).
			Get(loginUrl + "/username?authRequestID=" + result.AuthRequestId)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		ctx.Req.Equal(result.AuthRequestId, resp.Header().Get(oidc_auth.AuthRequestIdHeader))
		ctx.Req.NotContains(resp.String(), "Login timed out")
	})

	t.Run("GET auth-queries returns 401 UNAUTHORIZED for an unknown auth request id", func(t *testing.T) {
		ctx.NextTest(t)
		apiErr := &rest_model.APIError{}

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", oidc_auth.JsonContentType).
			Get(loginUrl + "/auth-queries?id=" + uuid.NewString())

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		ctx.Req.NoError(apiErr.UnmarshalBinary(resp.Body()), "expected a JSON API error, body: %s", resp.String())
		ctx.Req.Equal(errorz.UnauthorizedCode, apiErr.Code)
	})

	t.Run("POST totp as JSON returns 401 UNAUTHORIZED for an unknown auth request id", func(t *testing.T) {
		ctx.NextTest(t)
		apiErr := &rest_model.APIError{}

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", oidc_auth.JsonContentType).
			SetHeader("content-type", oidc_auth.JsonContentType).
			SetBody(map[string]string{"id": uuid.NewString(), "code": "123456"}).
			Post(loginUrl + "/totp")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		ctx.Req.NoError(apiErr.UnmarshalBinary(resp.Body()))
		ctx.Req.Equal(errorz.UnauthorizedCode, apiErr.Code)
	})

	for _, code := range []string{"", "12345678901234"} {
		t.Run("POST totp as JSON returns 401 UNAUTHORIZED for an unknown auth request id with code '"+code+"'", func(t *testing.T) {
			ctx.NextTest(t)
			apiErr := &rest_model.APIError{}

			resp, err := ctx.newAnonymousClientApiRequest().
				SetHeader("accept", oidc_auth.JsonContentType).
				SetHeader("content-type", oidc_auth.JsonContentType).
				SetBody(map[string]string{"id": uuid.NewString(), "code": code}).
				Post(loginUrl + "/totp")

			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
			ctx.Req.NoError(apiErr.UnmarshalBinary(resp.Body()))
			ctx.Req.Equal(errorz.UnauthorizedCode, apiErr.Code)
		})
	}

	t.Run("POST totp as HTML returns 401 without internal error text for an unknown auth request id", func(t *testing.T) {
		ctx.NextTest(t)

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", oidc_auth.HtmlContentType).
			SetHeader("content-type", oidc_auth.FormContentType).
			SetFormData(map[string]string{"id": uuid.NewString(), "code": "123456"}).
			Post(loginUrl + "/totp")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		ctx.Req.NotContains(resp.String(), "request not found")
	})

	t.Run("POST totp as JSON returns 401 UNAUTHORIZED for an auth request without primary authentication", func(t *testing.T) {
		ctx.NextTest(t)
		apiErr := &rest_model.APIError{}
		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		resp, err := result.Client.R().
			SetHeader("accept", oidc_auth.JsonContentType).
			SetHeader("content-type", oidc_auth.JsonContentType).
			SetBody(map[string]string{"id": result.AuthRequestId, "code": "123456"}).
			Post(loginUrl + "/totp")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		ctx.Req.NoError(apiErr.UnmarshalBinary(resp.Body()))
		ctx.Req.Equal(errorz.UnauthorizedCode, apiErr.Code)
	})

	t.Run("POST totp enroll returns 401 UNAUTHORIZED for an unknown auth request id", func(t *testing.T) {
		ctx.NextTest(t)
		apiErr := &rest_model.APIError{}

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", oidc_auth.JsonContentType).
			SetHeader("content-type", oidc_auth.JsonContentType).
			SetBody(map[string]string{"id": uuid.NewString()}).
			Post(loginUrl + "/totp/enroll")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		ctx.Req.NoError(apiErr.UnmarshalBinary(resp.Body()))
		ctx.Req.Equal(errorz.UnauthorizedCode, apiErr.Code)
	})

	t.Run("DELETE totp enroll returns 401 UNAUTHORIZED for an unknown auth request id", func(t *testing.T) {
		ctx.NextTest(t)
		apiErr := &rest_model.APIError{}

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", oidc_auth.JsonContentType).
			SetHeader("content-type", oidc_auth.JsonContentType).
			SetBody(map[string]string{"id": uuid.NewString(), "code": "123456"}).
			Delete(loginUrl + "/totp/enroll")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		ctx.Req.NoError(apiErr.UnmarshalBinary(resp.Body()))
		ctx.Req.Equal(errorz.UnauthorizedCode, apiErr.Code)
	})

	t.Run("POST totp enroll verify returns 401 UNAUTHORIZED for an unknown auth request id", func(t *testing.T) {
		ctx.NextTest(t)
		apiErr := &rest_model.APIError{}

		resp, err := ctx.newAnonymousClientApiRequest().
			SetHeader("accept", oidc_auth.JsonContentType).
			SetHeader("content-type", oidc_auth.JsonContentType).
			SetBody(map[string]string{"id": uuid.NewString(), "code": "123456"}).
			Post(loginUrl + "/totp/enroll/verify")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(), "unexpected status, body: %s", resp.String())
		ctx.Req.NoError(apiErr.UnmarshalBinary(resp.Body()))
		ctx.Req.Equal(errorz.UnauthorizedCode, apiErr.Code)
	})
}
