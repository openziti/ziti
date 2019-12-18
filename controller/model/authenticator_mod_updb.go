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

package model

import (
	"encoding/base64"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/crypto"
	"net/http"
)

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

func (handler *AuthModuleUpdb) Process(context AuthContext) (string, error) {
	jsonParsed, err := gabs.Consume(context.GetData())

	if err != nil {
		return "", &apierror.ApiError{
			Code:    apierror.CouldNotParseBodyCode,
			Message: apierror.CouldNotParseBodyMessage,
			Cause:   err,
			Status:  http.StatusBadRequest,
		}
	}

	username := ""
	password := ""

	if jsonParsed.Exists("username") {
		username, _ = jsonParsed.Path("username").Data().(string)
	}

	if jsonParsed.Exists("password") {
		password, _ = jsonParsed.Path("password").Data().(string)
	}

	if username == "" || password == "" {
		return "", &apierror.ApiError{
			Code:    apierror.CouldNotValidateCode,
			Message: apierror.CouldNotValidateMessage,
			Cause:   fmt.Errorf("username and password fields are required"),
			Status:  http.StatusBadRequest,
		}
	}

	authenticator, err := handler.env.GetHandlers().Authenticator.HandleReadByUsername(username)

	if err != nil {
		return "", err
	}

	if authenticator == nil {
		return "", apierror.NewInvalidAuth()
	}

	updb := authenticator.ToUpdb()

	salt, err := decodeSalt(updb.Salt)

	if err != nil {
		return "", apierror.NewInvalidAuth()
	}

	hr := crypto.ReHash(password, salt)

	target := base64.StdEncoding.EncodeToString(hr.Hash)

	if updb.Password != target {
		return "", apierror.NewInvalidAuth()
	}

	return updb.IdentityId, nil
}

func decodeSalt(s string) ([]byte, error) {
	salt := make([]byte, 1024)
	n, err := base64.StdEncoding.Decode(salt, []byte(s))

	if err != nil {
		return nil, err
	}
	return salt[:n], nil
}
