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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/fabric/controller/models"
	"github.com/sirupsen/logrus"
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

// Process will attempt to verify a client certificate bundle (supplied via the TLS handshake) with
// known CAs. The first certificate must be the client certificate and all subsequent certificates
// are treated as untrusted intermediates. If a verifying CA has `externalIdClaim` configuration present,
// the claim will be searched for. If it resolves, the values will be used as the `externalId` for the resulting
// identity. Subsequent authentications will match the certificate `externalId`. If not present, a certificate
// authenticator will be created where the fingerprint of the certificate will be matched on subsequent authentications.
func (module *EnrollModuleCa) Process(context EnrollmentContext) (*EnrollmentResult, error) {
	log := pfxlog.Logger().WithField("method", module.method)
	caList, err := module.env.GetManagers().Ca.Query("true limit none")

	if err != nil {
		return nil, err
	}

	if len(caList.Cas) == 0 {
		log.Error("attempting enrollment with no CAs present in the system")
		return nil, apierror.NewEnrollmentNoValidCas()
	}

	clientCerts := context.GetCerts()

	if len(clientCerts) == 0 {
		log.Error("attempting enrollment with no client certificates presented")
		return nil, apierror.NewCertFailedValidation()
	}

	log = log.WithField("intermediateCount", len(clientCerts)-1)

	clientCert := clientCerts[0]

	intermediatePool := x509.NewCertPool()

	for _, intermediateCert := range clientCerts[1:] {
		intermediatePool.AddCert(intermediateCert)
	}

	var enrollmentCa *Ca = nil

	//number of cas checked
	caCheckCount := 0

	for _, ca := range caList.Cas {
		if ca.IsAutoCaEnrollmentEnabled && ca.IsVerified {
			caCheckCount = caCheckCount + 1
			certPool := x509.NewCertPool()
			certPool.AppendCertsFromPEM([]byte(ca.CertPem))

			verifyOptions := x509.VerifyOptions{
				Roots:         certPool,
				Intermediates: intermediatePool,
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			}

			validChains, err := clientCert.Verify(verifyOptions)

			if err == nil && validChains != nil {
				enrollmentCa = ca
				break
			}

			if enrollmentCa != nil {
				break
			}
		}
	}

	if enrollmentCa == nil {
		log.WithField("caCheckCount", caCheckCount).Error("failed enrollment, no matching CA found")
		return nil, apierror.NewCertFailedValidation()
	}

	externalId, err := enrollmentCa.GetExternalId(clientCert)

	if err != nil {
		log.WithError(err).Error("error retrieving externalId from clientCert")
		return nil, apierror.NewMissingCertClaim()
	}

	log = log.WithField("externalId", externalId)

	if externalId != "" {
		return module.completeExternalIdEnrollment(log, context, enrollmentCa, clientCert, externalId)
	}

	return module.completeCertAuthenticatorEnrollment(log, context, enrollmentCa, clientCert)
}

// completeCertAuthenticatorEnrollment will result in the creation of an identity with an associated certificate
// authenticator. The certificate is identified by its fingerprint. Generally useful for identities that can
// store private keys inside of hardware modules (i.e. Android, iOS, HSMs, TPMs, etc.)
func (module *EnrollModuleCa) completeCertAuthenticatorEnrollment(log *logrus.Entry, context EnrollmentContext, ca *Ca, enrollmentCert *x509.Certificate) (*EnrollmentResult, error) {
	fingerprint := module.fingerprintGenerator.FromCert(enrollmentCert)

	log = log.WithField("fingerprint", fingerprint).WithField("subMethod", "authenticator")

	certPem := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: enrollmentCert.Raw,
	})

	existing, _ := module.env.GetManagers().Authenticator.ReadByFingerprint(fingerprint)

	if existing != nil {
		log.Error("enrollment failed, fingerprint already in use")
		return nil, apierror.NewCertInUse()
	}

	identityId := eid.New()
	requestedName := ""

	log = log.WithField("identityId", identityId)

	if context.GetDataAsMap() != nil {
		if dataName, ok := context.GetDataAsMap()["name"]; ok {
			requestedName = dataName.(string)
		}
	}

	if requestedName == "" {
		requestedName = identityId
	}

	log = log.WithField("requestedName", requestedName)

	identityName := module.getIdentityName(ca, enrollmentCert, requestedName, identityId)

	log = log.WithField("determinedName", identityName)

	identType, err := module.env.GetManagers().IdentityType.ReadByName("Device")
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
		RoleAttributes: ca.IdentityRoles,
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

	_, authenticatorId, err := module.env.GetManagers().Identity.CreateWithAuthenticator(identity, newAuthenticator)

	if err != nil {
		log.WithError(err).Error("failed to create identity with authenticator")
		return nil, err
	}

	log.WithField("authenticatorId", authenticatorId).Info("identity and authenticator created, enrollment success")

	return &EnrollmentResult{
		Identity:      identity,
		Authenticator: newAuthenticator,
		Content:       map[string]interface{}{},
		TextContent:   []byte(""),
		Status:        200,
	}, nil
}

// completeExternalIdEnrollment creates an identity without static authenticators (i.e. cert fingerprint mapping).
// Instead, the `externalId` field on the created identity will match the `externalId` retrieved from the enrolling
// certificate as defined by the validating Ca. This allows certificate to rotate, as long as the `externalId` claim
// can be extracted. An example of this is SPIFFE Ids stored as a SAN URI.
func (module *EnrollModuleCa) completeExternalIdEnrollment(log *logrus.Entry, context EnrollmentContext, ca *Ca, enrollmentCert *x509.Certificate, externalId string) (*EnrollmentResult, error) {
	identityId := eid.New()

	log = log.WithField("identityId", identityId).WithField("subMethod", "externalId")

	requestedName := ""

	if context.GetDataAsMap() != nil {
		if dataName, ok := context.GetDataAsMap()["name"]; ok {
			requestedName = dataName.(string)
		}
	}

	if requestedName == "" {
		requestedName = identityId
	}

	log = log.WithField("requestedName", requestedName)

	identityName := module.getIdentityName(ca, enrollmentCert, requestedName, identityId)

	log = log.WithField("determinedName", identityName)

	identType, err := module.env.GetManagers().IdentityType.ReadByName("Device")
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
		RoleAttributes: ca.IdentityRoles,
	}

	identity.ExternalId = &externalId

	_, err = module.env.GetManagers().Identity.Create(identity)

	if err != nil {
		log.WithError(err).Error("failed to create identity, enrollment failed")
		return nil, err
	}

	log.Info("identity created, enrollment success")

	return &EnrollmentResult{
		Identity:      identity,
		Authenticator: nil,
		Content:       map[string]interface{}{},
		TextContent:   []byte(""),
		Status:        200,
	}, nil
}

// getIdentityName returns a unique identity name based taking into account:
//	1) the requested name from the enrolling identity
//  2) the name formatting requirements determined by the enrolling CA
//  3) the uniqueness of the name that is a result of 1 and 2
//
//  The requested name is only used if the CA's name format allows it to be used.
//
//  If the resulting name is not unique a six digit zero-padded numerical suffix is appended (i.e. 000001).
func (module *EnrollModuleCa) getIdentityName(ca *Ca, enrollmentCert *x509.Certificate, requestedName string, identityId string) string {
	formatter := NewIdentityNameFormatter(ca, enrollmentCert, requestedName, identityId)
	nameFormat := ca.IdentityNameFormat

	if nameFormat == "" {
		nameFormat = DefaultCaIdentityNameFormat
	}

	identityName := formatter.Format(nameFormat)

	identityNameIsValid := false
	suffixCount := 0
	for !identityNameIsValid {
		//check for name collisions append 4 digit incrementing number to end till ok
		entity, _ := module.env.GetManagers().Identity.readEntityByQuery(fmt.Sprintf(`%s="%s"`, persistence.FieldName, identityName))

		if entity != nil {
			suffixCount = suffixCount + 1
			identityName = identityName + fmt.Sprintf("%06d", suffixCount)
		} else {
			identityNameIsValid = true
		}
	}

	return identityName
}

func NewIdentityNameFormatter(ca *Ca, clientCert *x509.Certificate, identityName, identityId string) *Formatter {
	return NewFormatter(map[string]string{
		FormatSymbolCaName:        ca.Name,
		FormatSymbolCaId:          ca.Id,
		FormatSymbolCommonName:    clientCert.Subject.CommonName,
		FormatSymbolRequestedName: identityName,
		FormatSymbolIdentityId:    identityId,
	})
}
