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
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/foundation/util/errorz"
)

var _ AuthProcessor = &AuthModuleUpdb{}

type AuthModuleUpdb struct {
	env    Env
	method string
}

func NewAuthModuleUpdb(env Env) *AuthModuleUpdb {
	handler := &AuthModuleUpdb{
		env:    env,
		method: "password",
	}

	return handler
}

func (handler *AuthModuleUpdb) CanHandle(method string) bool {
	return method == handler.method
}

func (handler *AuthModuleUpdb) Process(context AuthContext) (string, string, error) {
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
		return "", "", errorz.NewCouldNotValidate(errors.New("username and password fields are required"))
	}

	authenticator, err := handler.env.GetHandlers().Authenticator.ReadByUsername(username)

	if err != nil {
		return "", "", err
	}

	if authenticator == nil {
		return "", "", apierror.NewInvalidAuth()
	}

	updb := authenticator.ToUpdb()

	salt, err := decodeSalt(updb.Salt)

	if err != nil {
		return "", "", apierror.NewInvalidAuth()
	}

	hr := handler.env.GetHandlers().Authenticator.ReHashPassword(password, salt)

	if updb.Password != hr.Password {
		return "", "", apierror.NewInvalidAuth()
	}

	return updb.IdentityId, authenticator.Id, nil
}

func decodeSalt(s string) ([]byte, error) {
	salt := make([]byte, 1024)
	n, err := base64.StdEncoding.Decode(salt, []byte(s))

	if err != nil {
		return nil, err
	}
	return salt[:n], nil
}
