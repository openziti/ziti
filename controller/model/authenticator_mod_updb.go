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

package model

import (
	"encoding/base64"
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/util/errorz"
	cmap "github.com/orcaman/concurrent-map"
	"time"
)

var _ AuthProcessor = &AuthModuleUpdb{}

type AuthModuleUpdb struct {
	env                       Env
	method                    string
	attemptsByAuthenticatorId cmap.ConcurrentMap //string -> int64
}

func NewAuthModuleUpdb(env Env) *AuthModuleUpdb {
	handler := &AuthModuleUpdb{
		env:                       env,
		method:                    "password",
		attemptsByAuthenticatorId: cmap.New(),
	}

	return handler
}

func (handler *AuthModuleUpdb) CanHandle(method string) bool {
	return method == handler.method
}

func (handler *AuthModuleUpdb) Process(context AuthContext) (string, string, string, error) {
	logger := pfxlog.Logger().WithField("authMethod", handler.method)

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
		return "", "", "", errorz.NewCouldNotValidate(errors.New("username and password fields are required"))
	}

	logger = logger.WithField("username", username)
	authenticator, err := handler.env.GetHandlers().Authenticator.ReadByUsername(username)

	if err != nil {
		logger.WithError(err).Error("could not authenticate, authenticator lookup by username errored")
		return "", "", "", err
	}

	if authenticator == nil {
		logger.WithError(err).Error("could not authenticate, authenticator lookup returned nil")
		return "", "", "", apierror.NewInvalidAuth()
	}

	logger = logger.
		WithField("authenticatorId", authenticator.Id).
		WithField("identityId", authenticator.IdentityId)

	authPolicy, identity, err := getAuthPolicyByIdentityId(handler.env, handler.method, authenticator.Id, authenticator.IdentityId)

	if err != nil {
		logger.WithError(err).Errorf("could not look up auth policy by identity id")
		return "", "", "", apierror.NewInvalidAuth()
	}

	if authPolicy == nil {
		logger.Error("auth policy look up returned nil")
		return "", "", "", apierror.NewInvalidAuth()
	}

	if identity.Disabled {
		logger.
			WithField("disabledAt", identity.DisabledAt).
			WithField("disabledUntil", identity.DisabledUntil).
			Error("authentication failed, identity is disabled")
		return "", "", "", apierror.NewInvalidAuth()
	}

	logger = logger.WithField("authPolicyId", authPolicy.Id)

	if !authPolicy.Primary.Updb.Allowed {
		logger.Error("auth policy does not allow updb authentication")
		return "", "", "", apierror.NewInvalidAuth()
	}

	attempts := int64(0)
	handler.attemptsByAuthenticatorId.Upsert(authenticator.Id, nil, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		if exist {
			prevAttempts := valueInMap.(int64)
			attempts = prevAttempts + 1
			return attempts
		}

		return int64(0)
	})

	if authPolicy.Primary.Updb.MaxAttempts != persistence.UpdbUnlimitedAttemptsLimit && attempts > authPolicy.Primary.Updb.MaxAttempts {
		logger.WithField("attempts", attempts).WithField("maxAttempts", authPolicy.Primary.Updb.MaxAttempts).Error("updb auth failed, max attempts exceeded")

		if err = handler.env.GetHandlers().Identity.Disable(authenticator.IdentityId, time.Duration(authPolicy.Primary.Updb.LockoutDurationMinutes)*time.Minute); err != nil {
			logger.WithError(err).Error("could not lock identity, unhandled error")
		}

		return "", "", "", apierror.NewInvalidAuth()
	}

	updb := authenticator.ToUpdb()

	salt, err := decodeSalt(updb.Salt)

	if err != nil {
		return "", "", "", apierror.NewInvalidAuth()
	}

	hr := handler.env.GetHandlers().Authenticator.ReHashPassword(password, salt)

	if updb.Password != hr.Password {

		return "", "", "", apierror.NewInvalidAuth()
	}

	return updb.IdentityId, "", authenticator.Id, nil
}

func decodeSalt(s string) ([]byte, error) {
	salt := make([]byte, 1024)
	n, err := base64.StdEncoding.Decode(salt, []byte(s))

	if err != nil {
		return nil, err
	}
	return salt[:n], nil
}
