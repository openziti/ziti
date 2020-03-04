/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/validation"
	"github.com/netfoundry/ziti-edge/crypto"
	"github.com/netfoundry/ziti-edge/internal/cert"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/xeipuuv/gojsonschema"
)

type EnrollModuleUpdb struct {
	env                  Env
	method               string
	fingerprintGenerator cert.FingerprintGenerator
}

func NewEnrollModuleUpdb(env Env) *EnrollModuleUpdb {
	handler := &EnrollModuleUpdb{
		env:                  env,
		method:               persistence.MethodEnrollUpdb,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}

	return handler
}

func (module *EnrollModuleUpdb) CanHandle(method string) bool {
	return method == module.method
}

func (module *EnrollModuleUpdb) Process(ctx EnrollmentContext) (*EnrollmentResult, error) {
	enrollment, err := module.env.GetHandlers().Enrollment.ReadByToken(ctx.GetToken())
	if err != nil {
		return nil, err
	}

	if enrollment == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	identity, err := module.env.GetHandlers().Identity.Read(enrollment.IdentityId)

	if err != nil {
		return nil, err
	}

	if identity == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	data := ctx.GetDataAsMap()

	result, err := module.env.GetSchemas().GetEnrollUpdbPost().Validate(gojsonschema.NewGoLoader(data))

	if err != nil {
		return nil, err
	}

	if !result.Valid() {
		return nil, validation.NewValidationError(result.Errors()[0])
	}

	password := ""

	val, ok := data["password"]
	if !ok {
		return nil, apierror.NewUnhandled()
	}
	password = val.(string)

	hash := crypto.Hash(password)

	encodedPassword := base64.StdEncoding.EncodeToString(hash.Hash)
	encodedSalt := base64.StdEncoding.EncodeToString(hash.Salt)

	newAuthenticator := &Authenticator{
		BaseEntity: models.BaseEntity{
			Id: uuid.New().String(),
		},
		Method:     persistence.MethodAuthenticatorUpdb,
		IdentityId: enrollment.IdentityId,
		SubType: &AuthenticatorUpdb{
			Username: *enrollment.Username,
			Password: encodedPassword,
			Salt:     encodedSalt,
		},
	}

	err = module.env.GetHandlers().Enrollment.ReplaceWithAuthenticator(enrollment.Id, newAuthenticator)

	if err != nil {
		return nil, err
	}

	return &EnrollmentResult{
		Identity:      identity,
		Authenticator: newAuthenticator,
		Content:       []byte("{}"),
		ContentType:   "application/json",
		Status:        200,
	}, nil

}
