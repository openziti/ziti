// +build apitests

/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/Jeffail/gabs"
	"net/http"
	"testing"
)

func Test_Authenticate_Updb(t *testing.T) {
	testContext := NewTestContext(t)
	defer testContext.teardown()
	testContext.startServer()

	tests := &authUpdbTests{
		ctx: testContext,
	}

	t.Run("login with invalid password should fail", tests.testAuthenticateUpdbInvalidPassword)
	t.Run("login with missing password should fail", tests.testAuthenticateUPDBMissingPassword)
	t.Run("login with missing username should fail", tests.testAuthenticateUPDBMissingUsername)
	t.Run("admin login should pass", tests.testAuthenticateUPDBDefaultAdminSuccess)
}

type authUpdbTests struct {
	ctx *TestContext
}

func (tests *authUpdbTests) testAuthenticateUpdbInvalidPassword(t *testing.T) {
	t.Skip("verify if test or functionality is wrong")
	body := gabs.New()
	_, _ = body.SetP(tests.ctx.AdminUsername, "username")
	_, _ = body.SetP("invalid_password", "password")

	resp, err := tests.ctx.DefaultClient().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body.String()).
		Post("/authenticate?method=password")

	if err != nil {
		t.Errorf("failed to authenticate via updb as default admin: %s", err)
	}

	t.Run("ReturnsForbidden", func(t *testing.T) {
		if resp.StatusCode() != http.StatusForbidden {
			t.Errorf("expected status code %d got %d", http.StatusForbidden, resp.StatusCode())
		}
	})

	t.Run("HasNoSessionHeader", func(t *testing.T) {
		if resp.Header().Get("zt-session") != "" {
			t.Errorf("expected header zt-session to not be empty, got %s", resp.Header().Get("zt-session"))
		}
	})
}

func (tests *authUpdbTests) testAuthenticateUPDBMissingPassword(t *testing.T) {
	body := gabs.New()
	_, _ = body.SetP(tests.ctx.AdminUsername, "username")

	resp, err := tests.ctx.DefaultClient().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body.String()).
		Post("/authenticate?method=password")

	if err != nil {
		t.Errorf("failed to authenticate via UPDB as default admin: %s", err)
	}

	t.Run("ReturnsBadRequest", func(t *testing.T) {
		if resp.StatusCode() != http.StatusBadRequest {
			t.Errorf("expected status code %d got %d", http.StatusBadRequest, resp.StatusCode())
		}
	})

	t.Run("HasNoSessionHeader", func(t *testing.T) {
		if resp.Header().Get("zt-session") != "" {
			t.Errorf("expected header zt-session to not be empty, got %s", resp.Header().Get("zt-session"))
		}
	})
}

func (tests *authUpdbTests) testAuthenticateUPDBMissingUsername(t *testing.T) {
	body := gabs.New()
	_, _ = body.SetP(tests.ctx.AdminPassword, "password")

	resp, err := tests.ctx.DefaultClient().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body.String()).
		Post("/authenticate?method=password")

	if err != nil {
		t.Errorf("failed to authenticate via UPDB as default admin: %s", err)
	}

	t.Run("ReturnsBadRequest", func(t *testing.T) {
		if resp.StatusCode() != http.StatusBadRequest {
			t.Errorf("expected status code %d got %d", http.StatusBadRequest, resp.StatusCode())
		}
	})

	t.Run("HasNoSessionHeader", func(t *testing.T) {
		if resp.Header().Get("zt-session") != "" {
			t.Errorf("expected header zt-session to not be empty, got %s", resp.Header().Get("zt-session"))
		}
	})
}

func (tests *authUpdbTests) testAuthenticateUPDBDefaultAdminSuccess(t *testing.T) {
	body := gabs.New()
	_, _ = body.SetP(tests.ctx.AdminUsername, "username")
	_, _ = body.SetP(tests.ctx.AdminPassword, "password")

	resp, err := tests.ctx.DefaultClient().R().
		SetHeader("Content-Type", "application/json").
		SetBody(body.String()).
		Post("/authenticate?method=password")

	if err != nil {
		t.Errorf("failed to authenticate via UPDB as default admin: %s", err)
	}

	t.Run("ReturnsOk", func(t *testing.T) {
		if resp.StatusCode() != http.StatusOK {
			t.Errorf("expected status code %d got %d", http.StatusOK, resp.StatusCode())
		}
	})

	t.Run("HasSessionHeader", func(t *testing.T) {
		if resp.Header().Get("zt-session") == "" {
			t.Error("expected header zt-session to not be empty")
		}
	})
}
