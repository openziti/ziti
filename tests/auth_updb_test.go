//go:build apitests
// +build apitests

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
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/controller/env"
	"github.com/stretchr/testify/require"
	"gopkg.in/resty.v1"
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
	t.Run("test user login should pass", tests.testAuthenticateUpdbTestUserSuccess)
}

func Test_Lockout_Updb(t *testing.T) {
	testContext := NewTestContext(t)
	defer testContext.Teardown()
	testContext.StartServer()
	tests := &authUpdbTests{
		ctx: testContext,
	}

	t.Run("identity should not be disabled, when logging in with correct credentials after exceeding maxAttempts", tests.testAuthenticateUpdbNotLockedAfterMaxAttempts)
	t.Run("identity is disabled, as soon as failed login attempts exceed maxAttempts", tests.testAuthenticateUpdbLockedAfterMaxAttempts)

}

func Test_Lockout_Reset_Updb(t *testing.T) {
	testContext := NewTestContext(t)
	defer testContext.Teardown()
	testContext.StartServer()
	tests := &authUpdbTests{
		ctx: testContext,
	}
	t.Run("login attempts are reset on successful login", tests.testAuthenticateUpdbAttemptsResetOnSuccess)
	t.Run("login should succeed after lockout duration has eleapsed", tests.testAuthenticateUpdbUnlockAfterLockoutDuration)

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
		require.New(t).NotEmpty(resp.Header().Get(env.ZitiSession), fmt.Sprintf("HTTP header %s is empty", env.ZitiSession))
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
		headerToken := resp.Header().Get(env.ZitiSession)
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

func (tests *authUpdbTests) testAuthenticateUpdbTestUserSuccess(t *testing.T) {
	body := gabs.New()
	_, _ = body.SetP(tests.ctx.TestUserAuthenticator.Username, "username")
	_, _ = body.SetP(tests.ctx.TestUserAuthenticator.Password, "password")

	resp, err := tests.ctx.DefaultClientApiClient().R().
		SetHeader("content-type", "application/json").
		SetBody(body.String()).
		Post("authenticate?method=password")

	t.Run("should not have returned an error", func(t *testing.T) {
		require.New(t).NoError(err)
	})

	standardJsonResponseTests(resp, http.StatusOK, t)

	t.Run("returns a session token HTTP headers", func(t *testing.T) {
		require.New(t).NotEmpty(resp.Header().Get(env.ZitiSession), fmt.Sprintf("HTTP header %s is empty", env.ZitiSession))
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
		headerToken := resp.Header().Get(env.ZitiSession)
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

func (tests *authUpdbTests) testAuthenticateUpdbNotLockedAfterMaxAttempts(t *testing.T) {
	r := require.New(t)

	username := tests.ctx.TestUserAuthenticator.Username
	password := tests.ctx.TestUserAuthenticator.Password
	maxAttempts := int(tests.ctx.TestUserAuthPolicy.Primary.Updb.MaxAttempts)

	for attempt := 1; attempt <= (maxAttempts + 1); attempt++ {

		body := gabs.New()
		_, _ = body.SetP(username, "username")
		_, _ = body.SetP(password, "password")

		resp, err := tests.ctx.DefaultClientApiClient().R().
			SetHeader("content-type", "application/json").
			SetBody(body.String()).
			Post("authenticate?method=password")
		r.NoError(err)

		testUserIdentity, err := tests.ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
		r.NoError(err)

		t.Run("Identity must not be disabled, when logging with correct credentials after exceeding maxAttempts", func(t *testing.T) {
			r.Equal(resp.StatusCode(), http.StatusOK)
			r.False(testUserIdentity.Disabled)
		})

	}
}

func (tests *authUpdbTests) testAuthenticateUpdbLockedAfterMaxAttempts(t *testing.T) {
	r := require.New(t)

	username := tests.ctx.TestUserAuthenticator.Username
	maxAttempts := int(tests.ctx.TestUserAuthPolicy.Primary.Updb.MaxAttempts)

	for attempt := 1; attempt <= (maxAttempts + 1); attempt++ {

		body := gabs.New()
		_, _ = body.SetP(username, "username")
		_, _ = body.SetP("wrong_password", "password")

		resp, err := tests.ctx.DefaultClientApiClient().R().
			SetHeader("content-type", "application/json").
			SetBody(body.String()).
			Post("authenticate?method=password")
		r.NoError(err)

		testUserIdentity, err := tests.ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
		r.NoError(err)

		if attempt <= maxAttempts {
			t.Run("Identity should not be disabled, as long as maxAttempts are not exceeded", func(t *testing.T) {
				r.Equal(resp.StatusCode(), http.StatusUnauthorized)
				r.False(testUserIdentity.Disabled)
			})
		} else {
			t.Run("Identity should be disabled, as soon as maxAttempts are exceeded", func(t *testing.T) {
				r.Equal(resp.StatusCode(), http.StatusUnauthorized)
				r.True(testUserIdentity.Disabled)
			})
			t.Run("Identity should have disabledAt and disabledUntil field set", func(t *testing.T) {

				r.NotEmpty(testUserIdentity.DisabledAt)
				r.NotEmpty(testUserIdentity.DisabledUntil)
			})

			t.Run("Identity should disabledUntil field set, according to configured lockoutDuration", func(t *testing.T) {

				lockDuration := testUserIdentity.DisabledUntil.Sub(*testUserIdentity.DisabledAt)
				expectedDuration := int(tests.ctx.TestUserAuthPolicy.Primary.Updb.LockoutDurationMinutes)
				actualDuration := int(lockDuration.Minutes())
				r.Equal(expectedDuration, actualDuration, "lockout duration does not match configured value")

			})

		}
	}
}

func (tests *authUpdbTests) testAuthenticateUpdbAttemptsResetOnSuccess(t *testing.T) {
	r := require.New(t)

	username := tests.ctx.TestUserAuthenticator.Username
	password := "wrong_password"
	maxAttempts := int(tests.ctx.TestUserAuthPolicy.Primary.Updb.MaxAttempts)

	for attempt := 1; attempt <= (maxAttempts + 1); attempt++ {

		body := gabs.New()
		_, _ = body.SetP(username, "username")
		_, _ = body.SetP(password, "password")

		resp, err := tests.ctx.DefaultClientApiClient().R().
			SetHeader("content-type", "application/json").
			SetBody(body.String()).
			Post("authenticate?method=password")
		r.NoError(err)

		testUserIdentity, err := tests.ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
		r.NoError(err)

		// Switch to correct password on the third attempt to reset attempts, then back to wrong password
		switch attempt {
		case 1, 2:
			r.Equal(resp.StatusCode(), http.StatusUnauthorized)
			r.False(testUserIdentity.Disabled)
			if attempt == 2 {
				password = tests.ctx.TestUserAuthenticator.Password
			}
		case 3:
			r.Equal(resp.StatusCode(), http.StatusOK)
			r.False(testUserIdentity.Disabled)
			password = "wrong_password"
		default:
			r.Equal(resp.StatusCode(), http.StatusUnauthorized)
			r.False(testUserIdentity.Disabled)
		}
	}
}

func (tests *authUpdbTests) testAuthenticateUpdbUnlockAfterLockoutDuration(t *testing.T) {
	r := require.New(t)

	username := tests.ctx.TestUserAuthenticator.Username
	password := "wrong_password"
	maxAttempts := int(tests.ctx.TestUserAuthPolicy.Primary.Updb.MaxAttempts)
	lockoutDuration := int(tests.ctx.TestUserAuthPolicy.Primary.Updb.LockoutDurationMinutes)
	var resp *resty.Response

	for attempt := 1; attempt <= (maxAttempts + 1); attempt++ {
		body := gabs.New()
		_, _ = body.SetP(username, "username")
		_, _ = body.SetP(password, "password")

		var err error
		resp, err = tests.ctx.DefaultClientApiClient().R().
			SetHeader("content-type", "application/json").
			SetBody(body.String()).
			Post("authenticate?method=password")
		r.NoError(err)
	}

	testUserIdentity, err := tests.ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
	r.NoError(err)

	t.Run("Identity should be disabled", func(t *testing.T) {
		r.Equal(resp.StatusCode(), http.StatusUnauthorized)
		r.True(testUserIdentity.Disabled)
	})

	// Simulate wait for lockout duration to expire
	time.Sleep((time.Duration(lockoutDuration) * time.Minute) + (5 * time.Second))

	testUserIdentity, err = tests.ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
	r.NoError(err)

	t.Run("Identity should be enabled again", func(t *testing.T) {
		r.False(testUserIdentity.Disabled)
	})

	// Try to login with correct credentials after lockout duration
	body := gabs.New()
	_, _ = body.SetP(username, "username")
	_, _ = body.SetP(tests.ctx.TestUserAuthenticator.Password, "password")

	resp, err = tests.ctx.DefaultClientApiClient().R().
		SetHeader("content-type", "application/json").
		SetBody(body.String()).
		Post("authenticate?method=password")
	r.NoError(err)

	t.Run("UPDB login should succeed", func(t *testing.T) {
		r.Equal(resp.StatusCode(), http.StatusOK)
	})
}
