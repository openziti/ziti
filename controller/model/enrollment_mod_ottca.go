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
	"crypto/x509"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/controller/apierror"
	fabricApiError "github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"time"
)

type EnrollModuleOttCa struct {
	env                  Env
	method               string
	fingerprintGenerator cert.FingerprintGenerator
}

func NewEnrollModuleOttCa(env Env) *EnrollModuleOttCa {
	return &EnrollModuleOttCa{
		env:                  env,
		method:               db.MethodEnrollOttCa,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}
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

	if enrollment.ExpiresAt == nil || enrollment.ExpiresAt.IsZero() || enrollment.ExpiresAt.Before(time.Now()) {
		return nil, apierror.NewEnrollmentExpired()
	}

	identity, err := module.env.GetManagers().Identity.Read(*enrollment.IdentityId)

	if err != nil {
		return nil, err
	}

	if identity == nil {
		return nil, apierror.NewInvalidEnrollmentToken()
	}

	ctx.GetChangeContext().
		SetChangeAuthorType(change.AuthorTypeIdentity).
		SetChangeAuthorId(identity.Id).
		SetChangeAuthorName(identity.Name)

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

	rootPool := x509.NewCertPool()
	rootPool.AppendCertsFromPEM([]byte(ca.CertPem))

	intermediatePool := x509.NewCertPool()

	certs := ctx.GetCerts()

	if len(certs) == 0 {
		return nil, apierror.NewFailedCertificateValidation()
	}
	peer := certs[0]
	chain := certs[1:]

	for _, c := range chain {
		intermediatePool.AddCert(c)
	}

	verifyOptions := x509.VerifyOptions{
		Roots:         rootPool,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		Intermediates: intermediatePool,
	}

	validChains, err := peer.Verify(verifyOptions)

	if err != nil || len(validChains) == 0 {
		return nil, apierror.NewFailedCertificateValidation()
	}

	chainPem := ""
	for _, c := range validChains[0] {
		newPem, _ := cert.RawToPem(c.Raw)
		chainPem += string(newPem) + "\n"
	}

	fingerprint := module.fingerprintGenerator.FromCert(peer)

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
		Method:     db.MethodAuthenticatorCert,
		IdentityId: identity.Id,
		SubType: &AuthenticatorCert{
			Fingerprint:       fingerprint,
			Pem:               chainPem,
			IsIssuedByNetwork: false,
		},
	}

	err = module.env.GetManagers().Enrollment.ReplaceWithAuthenticator(enrollment.Id, newAuthenticator, ctx.GetChangeContext())

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
