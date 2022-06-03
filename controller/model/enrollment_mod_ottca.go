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
	"crypto/x509"
	"encoding/pem"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/internal/cert"
	fabricApiError "github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/fabric/controller/models"
)

type EnrollModuleOttCa struct {
	env                  Env
	method               string
	fingerprintGenerator cert.FingerprintGenerator
}

func NewEnrollModuleOttCa(env Env) *EnrollModuleOttCa {
	handler := &EnrollModuleOttCa{
		env:                  env,
		method:               persistence.MethodEnrollOttCa,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}

	return handler
}

func (module *EnrollModuleOttCa) CanHandle(method string) bool {
	return method == module.method
}

func (module *EnrollModuleOttCa) Process(ctx EnrollmentContext) (*EnrollmentResult, error) {
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

	if enrollment.CaId == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	ca, err := module.env.GetManagers().Ca.Read(*enrollment.CaId)

	if err != nil {
		return nil, err
	}

	if ca == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	if !ca.IsOttCaEnrollmentEnabled {
		return nil, apierror.NewEnrollmentCaNoLongValid()
	}

	cp := x509.NewCertPool()
	cp.AppendCertsFromPEM([]byte(ca.CertPem))

	vo := x509.VerifyOptions{
		Roots:     cp,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	var validCert *x509.Certificate = nil

	for _, c := range ctx.GetCerts() {
		vc, err := c.Verify(vo)

		if err == nil || vc != nil {
			validCert = c
			break
		}
	}

	if validCert == nil {
		return nil, apierror.NewCertFailedValidation()
	}

	fingerprint := module.fingerprintGenerator.FromCert(validCert)

	certPem := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: validCert.Raw,
	})

	existing, _ := module.env.GetManagers().Authenticator.ReadByFingerprint(fingerprint)

	if existing != nil {
		apiError := apierror.NewCertInUse()
		apiError.Cause = &fabricApiError.GenericCauseError{
			DataMap: map[string]interface{}{
				"fingerprint": fingerprint,
			},
		}
		return nil, apiError
	}

	newAuthenticator := &Authenticator{
		BaseEntity: models.BaseEntity{},
		Method:     persistence.MethodAuthenticatorCert,
		IdentityId: identity.Id,
		SubType: &AuthenticatorCert{
			Fingerprint: fingerprint,
			Pem:         string(certPem),
		},
	}

	err = module.env.GetManagers().Enrollment.ReplaceWithAuthenticator(enrollment.Id, newAuthenticator)

	if err != nil {
		return nil, err
	}

	return &EnrollmentResult{
		Identity:      identity,
		Authenticator: newAuthenticator,
		Content:       map[string]interface{}{},
		TextContent:   []byte(""),
		Status:        200,
	}, nil
}
