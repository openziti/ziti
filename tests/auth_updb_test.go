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
	"testing"
	"time"

	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/ziti/v2/controller/model"
)

func Test_Authenticate_Updb(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	authPolicy := &model.AuthPolicy{
		Name: "test-auth-policy",
		Primary: model.AuthPolicyPrimary{
			Updb: model.AuthPolicyUpdb{
				Allowed:                true,
				MinPasswordLength:      8,
				MaxAttempts:            int64(3),
				LockoutDurationMinutes: int64(1),
			},
		},
	}
	ctx.Req.NoError(ctx.Managers.CreateAuthPolicy(authPolicy))
	_, testUserAuthenticator, err := ctx.Managers.NewIdentityWithUpdb(authPolicy.Id)
	ctx.Req.NoError(err)

	t.Run("login with invalid password should fail", func(t *testing.T) {
		ctx.testContextChanged(t)

		clientApi := ctx.NewEdgeClientApi(nil)
		creds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, "invalid_password")
		apiSession, err := clientApi.Authenticate(creds, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(apiSession)
	})

	t.Run("login with invalid username should fail", func(t *testing.T) {
		ctx.testContextChanged(t)

		clientApi := ctx.NewEdgeClientApi(nil)
		creds := edge_apis.NewUpdbCredentials("weeewoooweeewooo123", "admin")
		apiSession, err := clientApi.Authenticate(creds, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(apiSession)
	})

	t.Run("login with missing password should fail", func(t *testing.T) {
		ctx.testContextChanged(t)

		clientApi := ctx.NewEdgeClientApi(nil)
		creds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, "")
		apiSession, err := clientApi.Authenticate(creds, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(apiSession)
	})

	t.Run("login with missing username should fail", func(t *testing.T) {
		ctx.testContextChanged(t)

		clientApi := ctx.NewEdgeClientApi(nil)
		creds := edge_apis.NewUpdbCredentials("", ctx.AdminAuthenticator.Password)
		apiSession, err := clientApi.Authenticate(creds, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(apiSession)
	})

	t.Run("admin login should pass", func(t *testing.T) {
		ctx.testContextChanged(t)

		clientApi := ctx.NewEdgeClientApi(nil)
		creds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
		apiSession, err := clientApi.Authenticate(creds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotEmpty(apiSession.GetToken())
		ctx.Req.NotEmpty(apiSession.GetIdentityId())
	})

	t.Run("test user login should pass", func(t *testing.T) {
		ctx.testContextChanged(t)

		clientApi := ctx.NewEdgeClientApi(nil)
		creds := edge_apis.NewUpdbCredentials(testUserAuthenticator.Username, testUserAuthenticator.Password)
		apiSession, err := clientApi.Authenticate(creds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(apiSession)
		ctx.Req.NotEmpty(apiSession.GetToken())
		ctx.Req.NotEmpty(apiSession.GetIdentityId())
	})
}

func Test_Authenticate_Updb_Lockout(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	authPolicy := &model.AuthPolicy{
		Name: "test-auth-policy",
		Primary: model.AuthPolicyPrimary{
			Updb: model.AuthPolicyUpdb{
				Allowed:                true,
				MinPasswordLength:      8,
				MaxAttempts:            int64(3),
				LockoutDurationMinutes: int64(1),
			},
		},
	}
	ctx.Req.NoError(ctx.Managers.CreateAuthPolicy(authPolicy))
	_, testUserAuthenticator, err := ctx.Managers.NewIdentityWithUpdb(authPolicy.Id)
	ctx.Req.NoError(err)

	t.Run("identity should not be disabled when logging in with correct credentials after exceeding maxAttempts", func(t *testing.T) {
		ctx.testContextChanged(t)

		username := testUserAuthenticator.Username
		maxAttempts := int(authPolicy.Primary.Updb.MaxAttempts)
		clientApi := ctx.NewEdgeClientApi(nil)

		for attempt := 1; attempt <= maxAttempts+1; attempt++ {
			creds := edge_apis.NewUpdbCredentials(username, testUserAuthenticator.Password)
			apiSession, err := clientApi.Authenticate(creds, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(apiSession)

			testUserIdentity, err := ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
			ctx.Req.NoError(err)
			ctx.Req.False(testUserIdentity.Disabled)
		}
	})

	t.Run("identity is disabled as soon as failed login attempts exceed maxAttempts", func(t *testing.T) {
		ctx.testContextChanged(t)

		username := testUserAuthenticator.Username
		maxAttempts := int(authPolicy.Primary.Updb.MaxAttempts)
		clientApi := ctx.NewEdgeClientApi(nil)

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			creds := edge_apis.NewUpdbCredentials(username, "wrong_password")
			_, err := clientApi.Authenticate(creds, nil)
			ctx.Req.Error(err)

			testUserIdentity, err := ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
			ctx.Req.NoError(err)

			if attempt < maxAttempts {
				ctx.Req.False(testUserIdentity.Disabled)
			} else {
				ctx.Req.True(testUserIdentity.Disabled)
				ctx.Req.NotEmpty(testUserIdentity.DisabledAt)
				ctx.Req.NotEmpty(testUserIdentity.DisabledUntil)

				lockDuration := testUserIdentity.DisabledUntil.Sub(*testUserIdentity.DisabledAt)
				expectedDuration := int(authPolicy.Primary.Updb.LockoutDurationMinutes)
				ctx.Req.Equal(expectedDuration, int(lockDuration.Minutes()))
			}
		}
	})

	t.Run("disabled identity must not be allowed to login", func(t *testing.T) {
		ctx.testContextChanged(t)

		username := testUserAuthenticator.Username

		testUserIdentity, err := ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
		ctx.Req.NoError(err)
		ctx.Req.True(testUserIdentity.Disabled)
		ctx.Req.NotEmpty(testUserIdentity.DisabledAt)
		ctx.Req.NotEmpty(testUserIdentity.DisabledUntil)

		clientApi := ctx.NewEdgeClientApi(nil)
		creds := edge_apis.NewUpdbCredentials(username, testUserAuthenticator.Password)
		apiSession, err := clientApi.Authenticate(creds, nil)
		ctx.Req.Error(err)
		ctx.Req.Nil(apiSession)
	})
}

func Test_Authenticate_Updb_Lockout_Reset(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	authPolicy := &model.AuthPolicy{
		Name: "test-auth-policy",
		Primary: model.AuthPolicyPrimary{
			Updb: model.AuthPolicyUpdb{
				Allowed:                true,
				MinPasswordLength:      8,
				MaxAttempts:            int64(3),
				LockoutDurationMinutes: int64(1),
			},
		},
	}
	ctx.Req.NoError(ctx.Managers.CreateAuthPolicy(authPolicy))
	_, testUserAuthenticator, err := ctx.Managers.NewIdentityWithUpdb(authPolicy.Id)
	ctx.Req.NoError(err)

	t.Run("login attempts are reset on successful login", func(t *testing.T) {
		ctx.testContextChanged(t)

		username := testUserAuthenticator.Username
		maxAttempts := int(authPolicy.Primary.Updb.MaxAttempts)
		clientApi := ctx.NewEdgeClientApi(nil)
		password := "wrong_password"

		for attempt := 1; attempt <= maxAttempts+1; attempt++ {
			creds := edge_apis.NewUpdbCredentials(username, password)
			apiSession, err := clientApi.Authenticate(creds, nil)

			testUserIdentity, identityErr := ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
			ctx.Req.NoError(identityErr)

			switch attempt {
			case 1, 2:
				ctx.Req.Error(err)
				ctx.Req.Nil(apiSession)
				ctx.Req.False(testUserIdentity.Disabled)
				if attempt == 2 {
					password = testUserAuthenticator.Password
				}
			case 3:
				ctx.Req.NoError(err)
				ctx.Req.NotNil(apiSession)
				ctx.Req.False(testUserIdentity.Disabled)
				password = "wrong_password"
			default:
				ctx.Req.Error(err)
				ctx.Req.Nil(apiSession)
				ctx.Req.False(testUserIdentity.Disabled)
			}
		}
	})
}

func Test_Authenticate_Updb_Lockout_Duration(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	authPolicy := &model.AuthPolicy{
		Name: "test-auth-policy",
		Primary: model.AuthPolicyPrimary{
			Updb: model.AuthPolicyUpdb{
				Allowed:                true,
				MinPasswordLength:      8,
				MaxAttempts:            int64(3),
				LockoutDurationMinutes: int64(1),
			},
		},
	}
	ctx.Req.NoError(ctx.Managers.CreateAuthPolicy(authPolicy))
	testUserIdentity, testUserAuthenticator, err := ctx.Managers.NewIdentityWithUpdb(authPolicy.Id)
	ctx.Req.NoError(err)

	t.Run("login should succeed after lockout duration has elapsed", func(t *testing.T) {
		ctx.testContextChanged(t)

		username := testUserAuthenticator.Username

		// Disable directly with a short duration to avoid a 60s wait from the policy's LockoutDurationMinutes.
		// The re-enable mechanism only depends on DisabledUntil, not on how the lockout was triggered.
		err := ctx.EdgeController.AppEnv.Managers.Identity.Disable(testUserIdentity.Id, 5*time.Second, nil)
		ctx.Req.NoError(err)

		identity, err := ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
		ctx.Req.NoError(err)
		ctx.Req.True(identity.Disabled)

		time.Sleep(5 * time.Second)

		identity, err = ctx.EdgeController.AppEnv.Managers.Identity.ReadByName(username)
		ctx.Req.NoError(err)
		ctx.Req.False(identity.Disabled)

		clientApi := ctx.NewEdgeClientApi(nil)
		creds := edge_apis.NewUpdbCredentials(username, testUserAuthenticator.Password)
		apiSession, err := clientApi.Authenticate(creds, nil)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(apiSession)
	})
}
