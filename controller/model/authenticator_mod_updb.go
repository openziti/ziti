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

package model

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/db"
	cmap "github.com/orcaman/concurrent-map/v2"
	"time"
)

var _ AuthProcessor = &AuthModuleUpdb{}

const AuthMethodPassword = "password"

type AuthModuleUpdb struct {
	BaseAuthenticator
	attemptsByAuthenticatorId cmap.ConcurrentMap[string, int64]
}

func NewAuthModuleUpdb(env Env) *AuthModuleUpdb {
	return &AuthModuleUpdb{
		BaseAuthenticator: BaseAuthenticator{
			env:    env,
			method: AuthMethodPassword,
		},
		attemptsByAuthenticatorId: cmap.New[int64](),
	}
}

func (module *AuthModuleUpdb) CanHandle(method string) bool {
	return method == module.method
}

func (module *AuthModuleUpdb) Process(context AuthContext) (AuthResult, error) {
	logger := pfxlog.Logger().WithField("authMethod", module.method)

	bundle := &AuthBundle{}

	data := context.GetData()

	username := ""
	password := ""

	if usernameVal := data["username"]; usernameVal != nil {
		username = usernameVal.(string)
	}
	if passwordVal := data["password"]; passwordVal != nil {
		password = passwordVal.(string)
	}

	if username == "" || password == "" {
		reason := "username and password fields are required"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)

		module.DispatchEvent(failEvent)
		return nil, errorz.NewCouldNotValidate(errors.New(reason))
	}

	logger = logger.WithField("username", username)

	var err error
	bundle.Authenticator, err = module.env.GetManagers().Authenticator.ReadByUsername(username)

	if err != nil {
		reason := "could not authenticate, authenticator lookup by username errored"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)

		module.DispatchEvent(failEvent)
		logger.WithError(err).Error(reason)

		return nil, err
	}

	if bundle.Authenticator == nil {
		reason := "could not authenticate, authenticator lookup returned nil"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)

		module.DispatchEvent(failEvent)
		logger.WithError(err).Error(reason)

		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.
		WithField("authenticatorId", bundle.Authenticator.Id).
		WithField("identityId", bundle.Authenticator.IdentityId)

	bundle.AuthPolicy, bundle.Identity, err = getAuthPolicyByIdentityId(module.env, module.method, bundle.Authenticator.Id, bundle.Authenticator.IdentityId)

	if err != nil {
		reason := "could not look up auth policy by identity id"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)

		module.DispatchEvent(failEvent)
		logger.WithError(err).Error(reason)

		return nil, apierror.NewInvalidAuth()
	}

	if bundle.AuthPolicy == nil {
		reason := "auth policy look up returned nil"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)

		module.DispatchEvent(failEvent)
		logger.WithError(err).Error(reason)

		logger.Error(reason)
		return nil, apierror.NewInvalidAuth()
	}

	if bundle.Identity.Disabled {
		reason := fmt.Sprintf("identity is disabled, disabledAt: %v, disabledUntil: %v", bundle.Identity.DisabledAt, bundle.Identity.DisabledUntil)
		failEvent := module.NewAuthEventFailure(context, bundle, reason)

		module.DispatchEvent(failEvent)
		logger.
			WithField("disabledAt", bundle.Identity.DisabledAt).
			WithField("disabledUntil", bundle.Identity.DisabledUntil).
			Error(reason)

		return nil, apierror.NewInvalidAuth()
	}

	logger = logger.WithField("authPolicyId", bundle.AuthPolicy.Id)

	if !bundle.AuthPolicy.Primary.Updb.Allowed {
		reason := fmt.Sprintf("auth policy does not allow updb authentication, authPolicyId: %v", bundle.AuthPolicy.Id)
		failEvent := module.NewAuthEventFailure(context, bundle, reason)

		module.DispatchEvent(failEvent)
		logger.WithField("authPolicyId", bundle.AuthPolicy.Id).Error(reason)
		return nil, apierror.NewInvalidAuth()
	}

	attempts := int64(0)
	module.attemptsByAuthenticatorId.Upsert(bundle.Authenticator.Id, 0, func(exist bool, prevAttempts int64, newValue int64) int64 {
		if exist {
			attempts = prevAttempts + 1
			return attempts
		}

		return 0
	})

	if bundle.AuthPolicy.Primary.Updb.MaxAttempts != db.UpdbUnlimitedAttemptsLimit && attempts > bundle.AuthPolicy.Primary.Updb.MaxAttempts {
		reason := fmt.Sprintf("updb auth failed, max attempts exceeded, attempts: %v, maxAttempts: %v", attempts, bundle.AuthPolicy.Primary.Updb.MaxAttempts)
		failEvent := module.NewAuthEventFailure(context, bundle, reason)

		module.DispatchEvent(failEvent)
		logger.WithField("attempts", attempts).WithField("maxAttempts", bundle.AuthPolicy.Primary.Updb.MaxAttempts).Error(reason)

		duration := time.Duration(bundle.AuthPolicy.Primary.Updb.LockoutDurationMinutes) * time.Minute
		if err = module.env.GetManagers().Identity.Disable(bundle.Authenticator.IdentityId, duration, context.GetChangeContext()); err != nil {
			logger.WithError(err).Error("could not lock identity, unhandled error")
		}

		return nil, apierror.NewInvalidAuth()
	}

	updb := bundle.Authenticator.ToUpdb()

	salt, err := DecodeSalt(updb.Salt)

	if err != nil {
		reason := "could not decode salt"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)

		module.DispatchEvent(failEvent)
		logger.Error(reason)

		return nil, apierror.NewInvalidAuth()
	}

	hr := module.env.GetManagers().Authenticator.ReHashPassword(password, salt)

	if subtle.ConstantTimeCompare([]byte(updb.Password), []byte(hr.Password)) != 1 {
		reason := "could not authenticate, password does not match"
		failEvent := module.NewAuthEventFailure(context, bundle, reason)

		module.DispatchEvent(failEvent)
		logger.Error(reason)

		return nil, apierror.NewInvalidAuth()
	}

	successEvent := module.NewAuthEventSuccess(context, bundle)
	module.DispatchEvent(successEvent)

	return &AuthResultBase{
		identity:        bundle.Identity,
		authenticator:   bundle.Authenticator,
		authenticatorId: bundle.Authenticator.Id,
		env:             module.env,
		authPolicy:      bundle.AuthPolicy,
	}, nil
}

func DecodeSalt(s string) ([]byte, error) {
	salt := make([]byte, 1024)
	n, err := base64.StdEncoding.Decode(salt, []byte(s))

	if err != nil {
		return nil, err
	}
	return salt[:n], nil
}
