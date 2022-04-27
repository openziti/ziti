//go:build apitests
// +build apitests

/*
	Copyright NetFoundry, Inc.

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
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/sdk-golang/ziti/constants"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func Test_Authenticate_Updb(t *testing.T) {
	testContext := NewTestContext(t)
	defer testContext.Teardown()
	testContext.StartServer()

	tests := &authUpdbTests{
		ctx: testContext,
	}

	t.Run("login with invalid password should fail", tests.testAuthenticateUpdbInvalidPassword)
	t.Run("login with invalid username should fail", tests.testAuthenticateUpdbInvalidUsername)
	t.Run("login with missing password should fail", tests.testAuthenticateUPDBMissingPassword)
	t.Run("login with missing username should fail", tests.testAuthenticateUPDBMissingUsername)
	t.Run("admin login should pass", tests.testAuthenticateUPDBDefaultAdminSuccess)
}

type authUpdbTests struct {
	ctx *TestContext
}

func (tests *authUpdbTests) testAuthenticateUpdbInvalidPassword(t *testing.T) {
	body := gabs.New()
	_, _ = body.SetP(tests.ctx.AdminAuthenticator.Username, "username")
	_, _ = body.SetP("invalid_password", "password")

	resp, err := tests.ctx.DefaultClientApiClient().R().
		SetHeader("content-type", "application/json").
		SetBody(body.String()).
		Post("authenticate?method=password")

	t.Run("should not have returned an error", func(t *testing.T) {
		require.New(t).NoError(err)
	})

	standardErrorJsonResponseTests(resp, "INVALID_AUTH", http.StatusUnauthorized, t)

	t.Run("does not have a session token", func(t *testing.T) {
		require.New(t).Equal("", resp.Header().Get("zt-session"), "expected header zt-session to be empty, got %s", resp.Header().Get("zt-session"))
	})
}

func (tests *authUpdbTests) testAuthenticateUpdbInvalidUsername(t *testing.T) {
	body := gabs.New()
	_, _ = body.SetP("weeewoooweeewooo123", "username")
	_, _ = body.SetP("admin", "password")

	resp, err := tests.ctx.DefaultClientApiClient().R().
		SetHeader("content-type", "application/json").
		SetBody(body.String()).
		Post("authenticate?method=password")

	t.Run("should not have returned an error", func(t *testing.T) {
		require.New(t).NoError(err)
	})

	standardErrorJsonResponseTests(resp, "INVALID_AUTH", http.StatusUnauthorized, t)

	t.Run("does not have a session token", func(t *testing.T) {
		require.New(t).Equal("", resp.Header().Get("zt-session"), "expected header zt-session to be empty, got %s", resp.Header().Get("zt-session"))
	})
}

func (tests *authUpdbTests) testAuthenticateUPDBMissingPassword(t *testing.T) {
	body := gabs.New()
	_, _ = body.SetP(tests.ctx.AdminAuthenticator.Username, "username")

	resp, err := tests.ctx.DefaultClientApiClient().R().
		SetHeader("content-type", "application/json").
		SetBody(body.String()).
		Post("authenticate?method=password")

	t.Run("should not have returned an error", func(t *testing.T) {
		require.New(t).NoError(err)
	})

	standardErrorJsonResponseTests(resp, "COULD_NOT_VALIDATE", http.StatusBadRequest, t)

	t.Run("does not have a session token", func(t *testing.T) {
		require.New(t).Equal("", resp.Header().Get("zt-session"), "expected header zt-session to be empty, got %s", resp.Header().Get("zt-session"))
	})
}

func (tests *authUpdbTests) testAuthenticateUPDBMissingUsername(t *testing.T) {
	body := gabs.New()
	_, _ = body.SetP(tests.ctx.AdminAuthenticator.Password, "password")

	resp, err := tests.ctx.DefaultClientApiClient().R().
		SetHeader("content-type", "application/json").
		SetBody(body.String()).
		Post("authenticate?method=password")

	if err != nil {
		t.Errorf("failed to authenticate via UPDB as default admin: %s", err)
	}

	t.Run("should not have returned an error", func(t *testing.T) {
		require.New(t).NoError(err)
	})

	standardErrorJsonResponseTests(resp, "COULD_NOT_VALIDATE", http.StatusBadRequest, t)

	t.Run("does not have a session token", func(t *testing.T) {
		require.New(t).Equal("", resp.Header().Get("zt-session"), "expected header zt-session to be empty, got %s", resp.Header().Get("zt-session"))
	})
}

func (tests *authUpdbTests) testAuthenticateUPDBDefaultAdminSuccess(t *testing.T) {
	body := gabs.New()
	_, _ = body.SetP(tests.ctx.AdminAuthenticator.Username, "username")
	_, _ = body.SetP(tests.ctx.AdminAuthenticator.Password, "password")

	resp, err := tests.ctx.DefaultClientApiClient().R().
		SetHeader("content-type", "application/json").
		SetBody(body.String()).
		Post("authenticate?method=password")

	t.Run("should not have returned an error", func(t *testing.T) {
		require.New(t).NoError(err)
	})

	standardJsonResponseTests(resp, http.StatusOK, t)

	t.Run("returns a session token HTTP headers", func(t *testing.T) {
		require.New(t).NotEmpty(resp.Header().Get(constants.ZitiSession), fmt.Sprintf("HTTP header %s is empty", constants.ZitiSession))
	})

	t.Run("returns a session token in body", func(t *testing.T) {
		r := require.New(t)
		data, err := gabs.ParseJSON(resp.Body())

		r.NoError(err)

		r.True(data.ExistsP("data.token"), "session token property in 'data.token' as not found")
		r.NotEmpty(data.Path("data.token").String(), "session token property in 'data.token' is empty")
	})

	t.Run("body session token matches HTTP header token", func(t *testing.T) {
		r := require.New(t)
		data, err := gabs.ParseJSON(resp.Body())

		r.NoError(err)

		bodyToken := data.Path("data.token").Data().(string)
		headerToken := resp.Header().Get(constants.ZitiSession)
		r.Equal(bodyToken, headerToken)
	})

	t.Run("returns an identity", func(t *testing.T) {
		r := require.New(t)
		data, err := gabs.ParseJSON(resp.Body())

		r.NoError(err)

		r.True(data.ExistsP("data.identity"), "session token property in 'data.token' as not found")

		_, err = data.ObjectP("data.identity")
		r.NoError(err, "session token property in 'data.token' is empty")
	})
}
