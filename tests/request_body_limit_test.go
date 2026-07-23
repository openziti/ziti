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
	"bytes"
	"net/http"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/openziti/ziti/controller/api"
)

// Test_RequestBodyLimit locks in the pre-auth request body cap. The controller web APIs
// buffer each request body into memory before any authentication check, so bodies larger
// than api.MaxRequestBodySize must be rejected with 413 instead of being buffered.
func Test_RequestBodyLimit(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	oversized := bytes.Repeat([]byte("a"), api.MaxRequestBodySize+1)

	t.Run("oversized body to the unauthenticated client API enroll endpoint returns 413", func(t *testing.T) {
		ctx.testContextChanged(t)

		resp, err := ctx.newAnonymousClientApiRequest().SetBody(oversized).Post("/enroll")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusRequestEntityTooLarge, resp.StatusCode(),
			"an oversized body must be rejected before it is buffered, got body: %s", resp.String())
	})

	t.Run("oversized body to the unauthenticated management API authenticate endpoint returns 413", func(t *testing.T) {
		ctx.testContextChanged(t)

		resp, err := ctx.newAnonymousManagementApiRequest().SetBody(oversized).Post("/authenticate?method=password")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusRequestEntityTooLarge, resp.StatusCode(),
			"an oversized body must be rejected before it is buffered, got body: %s", resp.String())
	})

	t.Run("oversized chunked body without a Content-Length header returns 413", func(t *testing.T) {
		ctx.testContextChanged(t)

		// a reader body makes resty send chunked transfer encoding, so the server cannot
		// reject on the Content-Length header and must stop reading at the cap instead
		resp, err := ctx.newAnonymousClientApiRequest().SetBody(bytes.NewReader(oversized)).Post("/enroll")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusRequestEntityTooLarge, resp.StatusCode(),
			"an oversized chunked body must be rejected once the cap is hit, got body: %s", resp.String())
	})

	t.Run("oversized body to the unauthenticated OIDC login endpoint returns 413", func(t *testing.T) {
		ctx.testContextChanged(t)

		client := resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport()))

		resp, err := client.R().
			SetHeader("content-type", "application/json").
			SetBody(oversized).
			Post("https://" + ctx.ApiHost + "/oidc/login/username")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusRequestEntityTooLarge, resp.StatusCode(),
			"an oversized body must be rejected before it is buffered, got body: %s", resp.String())
	})

	t.Run("normal-size body still reaches the API handlers", func(t *testing.T) {
		ctx.testContextChanged(t)

		resp, err := ctx.newAnonymousManagementApiRequest().
			SetBody(`{"username":"bogus","password":"bogus"}`).
			Post("/authenticate?method=password")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode(),
			"a small body must pass the cap and reach normal authentication handling")
	})
}
