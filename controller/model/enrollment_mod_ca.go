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
	"crypto/x509"
	"fmt"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/internal/cert"
)

type EnrollModuleCa struct {
	env                  Env
	method               string
	fingerprintGenerator cert.FingerprintGenerator
}

func NewEnrollModuleCa(env Env) *EnrollModuleCa {
	handler := &EnrollModuleCa{
		env:                  env,
		method:               persistence.MethodEnrollCa,
		fingerprintGenerator: cert.NewFingerprintGenerator(),
	}

	return handler
}

func (module *EnrollModuleCa) CanHandle(method string) bool {
	return method == module.method
}

func (module *EnrollModuleCa) Process(context EnrollmentContext) (*EnrollmentResult, error) {

	caList, err := module.env.GetHandlers().Ca.HandleQuery("true limit none")

	if err != nil {
		return nil, err
	}

	if len(caList.Cas) == 0 {
		return nil, apierror.NewEnrollmentNoValidCas()
	}

	var enrollmentCa *Ca = nil
	var enrollmentCert *x509.Certificate = nil

	for _, ca := range caList.Cas {
		if ca.IsAutoCaEnrollmentEnabled && ca.IsVerified {
			certPool := x509.NewCertPool()
			certPool.AppendCertsFromPEM([]byte(ca.CertPem))

			verifyOptions := x509.VerifyOptions{
				Roots:     certPool,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			}

			for _, clientCert := range context.GetCerts() {
				validChains, err := clientCert.Verify(verifyOptions)

				if err == nil && validChains != nil {
					enrollmentCert = clientCert
					enrollmentCa = ca
					break
				}
			}

			if enrollmentCa != nil {
				break
			}
		}
	}

	if enrollmentCa == nil {
		return nil, apierror.NewCertFailedValidation()
	}

	fingerprint := module.fingerprintGenerator.FromCert(enrollmentCert)

	existing, _ := module.env.GetHandlers().Authenticator.HandleReadByFingerprint(fingerprint)

	if existing != nil {
		return nil, apierror.NewCertInUse()
	}

	identityId := uuid.New().String()
	identityName := ""

	if context.GetDataAsMap() != nil {
		if dataName, ok := context.GetDataAsMap()["name"]; ok {
			identityName = dataName.(string)
		}
	}

	if identityName == "" {
		identityName = fmt.Sprintf("%s.%s", enrollmentCa.Name, identityId)
	}

	identity := &Identity{
		BaseModelEntityImpl: BaseModelEntityImpl{
			Id: identityId,
		},
		Name:           identityName,
		IdentityTypeId: "device",
		IsDefaultAdmin: false,
		IsAdmin:        false,
	}

	newAuthenticator := &Authenticator{
		BaseModelEntityImpl: BaseModelEntityImpl{},
		Method:              persistence.MethodAuthenticatorCert,
		IdentityId:          identity.Id,
		SubType: &AuthenticatorCert{
			Fingerprint: fingerprint,
		},
	}

	_, _, err = module.env.GetHandlers().Identity.HandleCreateWithAuthenticator(identity, newAuthenticator)

	if err != nil {

		return nil, err
	}

	return &EnrollmentResult{
		Identity:      identity,
		Authenticator: newAuthenticator,
		Content:       []byte{},
		ContentType:   "text/plain",
		Status:        200,
	}, nil
}
