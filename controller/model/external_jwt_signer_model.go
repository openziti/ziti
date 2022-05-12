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
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	nfpem "github.com/openziti/foundation/util/pem"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
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

	CommonName  string
	Fingerprint *string
	NotAfter    time.Time
	NotBefore   time.Time
}

func (entity *ExternalJwtSigner) toBoltEntity() (boltz.Entity, error) {
	var fingerprint string
	var signerCert *x509.Certificate

	if entity.CertPem != nil {
		signerCerts := nfpem.PemStringToCertificates(*entity.CertPem)
		if len(signerCerts) != 1 {
			return nil, apierror.NewInvalidCertificatePem()
		}

		signerCert = signerCerts[0]

		fingerprint = nfpem.FingerprintFromCertificate(signerCert)
	}

	signer := &persistence.ExternalJwtSigner{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:            entity.Name,
		Fingerprint:     &fingerprint,
		CertPem:         entity.CertPem,
		JwksEndpoint:    entity.JwksEndpoint,
		Enabled:         entity.Enabled,
		ExternalAuthUrl: entity.ExternalAuthUrl,
		UseExternalId:   entity.UseExternalId,
		ClaimsProperty:  entity.ClaimsProperty,
		Kid:             entity.Kid,
		Issuer:          entity.Issuer,
		Audience:        entity.Audience,
	}

	if entity.CertPem != nil {
		signer.CommonName = signerCert.Subject.CommonName
		signer.NotAfter = &signerCert.NotAfter
		signer.NotBefore = &signerCert.NotBefore
	}

	return signer, nil
}

func (entity *ExternalJwtSigner) toBoltEntityForCreate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ExternalJwtSigner) toBoltEntityForUpdate(*bbolt.Tx, EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ExternalJwtSigner) toBoltEntityForPatch(*bbolt.Tx, EntityManager, boltz.FieldChecker) (boltz.Entity, error) {
	signer := &persistence.ExternalJwtSigner{
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
		Audience:        entity.Issuer,
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

func (entity *ExternalJwtSigner) fillFrom(_ EntityManager, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltExternalJwtSigner, ok := boltEntity.(*persistence.ExternalJwtSigner)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model ExternalJwtSigner", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltExternalJwtSigner)
	entity.Name = boltExternalJwtSigner.Name
	entity.CommonName = boltExternalJwtSigner.CommonName
	entity.CertPem = boltExternalJwtSigner.CertPem
	entity.JwksEndpoint = boltExternalJwtSigner.JwksEndpoint
	entity.Fingerprint = boltExternalJwtSigner.Fingerprint
	entity.Enabled = boltExternalJwtSigner.Enabled
	entity.NotBefore = *boltExternalJwtSigner.NotBefore
	entity.NotAfter = *boltExternalJwtSigner.NotAfter
	entity.ExternalAuthUrl = boltExternalJwtSigner.ExternalAuthUrl
	entity.ClaimsProperty = boltExternalJwtSigner.ClaimsProperty
	entity.UseExternalId = boltExternalJwtSigner.UseExternalId
	entity.Kid = boltExternalJwtSigner.Kid
	entity.Issuer = boltExternalJwtSigner.Issuer
	entity.Audience = boltExternalJwtSigner.Audience
	return nil
}
