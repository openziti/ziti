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
	"fmt"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/internal/cert"
	"github.com/netfoundry/ziti-fabric/controller/models"
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

	caList, err := module.env.GetHandlers().Ca.Query("true limit none")

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

	if enrollmentCert == nil {
		return nil, apierror.NewCertFailedValidation()
	}

	fingerprint := module.fingerprintGenerator.FromCert(enrollmentCert)

	certPem := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: enrollmentCert.Raw,
	})

	existing, _ := module.env.GetHandlers().Authenticator.ReadByFingerprint(fingerprint)

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

	identType, err := module.env.GetHandlers().IdentityType.ReadByName("Device")
	if err != nil {
		return nil, err
	}

	identity := &Identity{
		BaseEntity: models.BaseEntity{
			Id: identityId,
		},
		Name:           identityName,
		IdentityTypeId: identType.Id,
		IsDefaultAdmin: false,
		IsAdmin:        false,
		RoleAttributes: enrollmentCa.IdentityRoles,
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

	_, _, err = module.env.GetHandlers().Identity.CreateWithAuthenticator(identity, newAuthenticator)

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
