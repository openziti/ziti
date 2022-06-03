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
	"encoding/pem"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
)

type EnrollModuleOtt struct {
	env                  Env
	method               string
	fingerprintGenerator cert.FingerprintGenerator
}

func NewEnrollModuleOtt(env Env) *EnrollModuleOtt {
	handler := &EnrollModuleOtt{
		env:                  env,
		method:               persistence.MethodEnrollOtt,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}

	return handler
}

func (module *EnrollModuleOtt) CanHandle(method string) bool {
	return method == module.method
}

func (module *EnrollModuleOtt) Process(ctx EnrollmentContext) (*EnrollmentResult, error) {
	enrollment, err := module.env.GetManagers().Enrollment.ReadByToken(ctx.GetToken())
	if err != nil {
		return nil, err
	}

	if enrollment == nil || enrollment.IdentityId == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	identity, err := module.env.GetManagers().Identity.Read(*enrollment.IdentityId)

	if err != nil {
		return nil, err
	}

	if identity == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	csrPem := ctx.GetDataAsByteArray()

	csr, err := cert.ParseCsrPem(csrPem)

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	certRaw, err := module.env.GetApiClientCsrSigner().SignCsr(csr, &cert.SigningOpts{})

	if err != nil {
		apiErr := apierror.NewCouldNotProcessCsr()
		apiErr.Cause = err
		apiErr.AppendCause = true
		return nil, apiErr
	}

	fp := module.fingerprintGenerator.FromRaw(certRaw)

	certPem := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certRaw,
	})

	newAuthenticator := &Authenticator{
		BaseEntity: models.BaseEntity{
			Id: eid.New(),
		},
		Method:     persistence.MethodAuthenticatorCert,
		IdentityId: *enrollment.IdentityId,
		SubType: &AuthenticatorCert{
			Fingerprint: fp,
			Pem:         string(certPem),
		},
	}

	err = module.env.GetManagers().Enrollment.ReplaceWithAuthenticator(enrollment.Id, newAuthenticator)

	if err != nil {
		return nil, err
	}

	content := &rest_model.EnrollmentCerts{
		Cert: string(certPem),
	}

	return &EnrollmentResult{
		Identity:      identity,
		Authenticator: newAuthenticator,
		Content:       content,
		TextContent:   certPem,
		Status:        200,
	}, nil

}
