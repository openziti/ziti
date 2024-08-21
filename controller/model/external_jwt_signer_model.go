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
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
	"time"
)

type ExternalJwtSigner struct {
	models.BaseEntity
	Name            string
	CertPem         *string
	JwksEndpoint    *string
	Kid             *string
	Enabled         bool
	ExternalAuthUrl *string
	UseExternalId   bool
	ClaimsProperty  *string
	Issuer          *string
	Audience        *string
	ClientId        *string
	Scopes          []string

	CommonName  string
	Fingerprint *string
	NotAfter    time.Time
	NotBefore   time.Time
}

func (entity *ExternalJwtSigner) toBoltEntity() (*db.ExternalJwtSigner, error) {
	signer := &db.ExternalJwtSigner{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:            entity.Name,
		CertPem:         entity.CertPem,
		JwksEndpoint:    entity.JwksEndpoint,
		Enabled:         entity.Enabled,
		ExternalAuthUrl: entity.ExternalAuthUrl,
		UseExternalId:   entity.UseExternalId,
		ClaimsProperty:  entity.ClaimsProperty,
		Kid:             entity.Kid,
		Issuer:          entity.Issuer,
		Audience:        entity.Audience,
		ClientId:        entity.ClientId,
		Scopes:          entity.Scopes,
	}

	if entity.CertPem != nil && *entity.CertPem != "" {
		signerCerts := nfpem.PemStringToCertificates(*entity.CertPem)

		if len(signerCerts) != 1 {
			return nil, apierror.NewInvalidCertificatePem()
		}

		signerCert := signerCerts[0]
		fingerprint := nfpem.FingerprintFromCertificate(signerCert)

		signer.CommonName = signerCert.Subject.CommonName
		signer.NotBefore = &signerCert.NotBefore
		signer.NotAfter = &signerCert.NotAfter
		signer.Fingerprint = &fingerprint
	}
	return signer, nil
}

func (entity *ExternalJwtSigner) toBoltEntityForCreate(*bbolt.Tx, Env) (*db.ExternalJwtSigner, error) {
	return entity.toBoltEntity()
}

func (entity *ExternalJwtSigner) toBoltEntityForUpdate(*bbolt.Tx, Env, boltz.FieldChecker) (*db.ExternalJwtSigner, error) {
	return entity.toBoltEntity()
}

func (entity *ExternalJwtSigner) fillFrom(_ Env, _ *bbolt.Tx, boltExternalJwtSigner *db.ExternalJwtSigner) error {
	entity.FillCommon(boltExternalJwtSigner)
	entity.Name = boltExternalJwtSigner.Name
	entity.CommonName = boltExternalJwtSigner.CommonName
	entity.CertPem = boltExternalJwtSigner.CertPem
	entity.JwksEndpoint = boltExternalJwtSigner.JwksEndpoint
	entity.Fingerprint = boltExternalJwtSigner.Fingerprint
	entity.Enabled = boltExternalJwtSigner.Enabled

	entity.NotBefore = timeOrEmpty(boltExternalJwtSigner.NotBefore)
	entity.NotAfter = timeOrEmpty(boltExternalJwtSigner.NotAfter)
	entity.ExternalAuthUrl = boltExternalJwtSigner.ExternalAuthUrl
	entity.ClaimsProperty = boltExternalJwtSigner.ClaimsProperty
	entity.UseExternalId = boltExternalJwtSigner.UseExternalId
	entity.Kid = boltExternalJwtSigner.Kid
	entity.Issuer = boltExternalJwtSigner.Issuer
	entity.Audience = boltExternalJwtSigner.Audience
	entity.ClientId = boltExternalJwtSigner.ClientId
	entity.Scopes = boltExternalJwtSigner.Scopes
	return nil
}

func timeOrEmpty(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}

	return *t
}
