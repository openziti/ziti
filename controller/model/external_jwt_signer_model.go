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
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/boltz"
	nfpem "github.com/openziti/foundation/util/pem"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
)

type ExternalJwtSigner struct {
	models.BaseEntity
	Name            string
	CertPem         string
	Enabled         bool
	ExternalAuthUrl *string
	UseExternalId   bool
	ClaimsProperty  *string

	CommonName  string
	Fingerprint string
	NotAfter    time.Time
	NotBefore   time.Time
}

func (entity *ExternalJwtSigner) toBoltEntity() (boltz.Entity, error) {
	signerCerts := nfpem.PemStringToCertificates(entity.CertPem)

	if len(signerCerts) != 1 {
		return nil, apierror.NewInvalidCertificatePem()
	}

	signerCert := signerCerts[0]

	fingerprint := nfpem.FingerprintFromCertificate(signerCert)

	signer := &persistence.ExternalJwtSigner{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:            entity.Name,
		Fingerprint:     fingerprint,
		CertPem:         entity.CertPem,
		CommonName:      signerCert.Subject.CommonName,
		NotAfter:        &signerCert.NotAfter,
		NotBefore:       &signerCert.NotBefore,
		Enabled:         entity.Enabled,
		ExternalAuthUrl: entity.ExternalAuthUrl,
		UseExternalId:   entity.UseExternalId,
		ClaimsProperty:  entity.ClaimsProperty,
	}

	return signer, nil
}

func (entity *ExternalJwtSigner) toBoltEntityForCreate(*bbolt.Tx, Handler) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ExternalJwtSigner) toBoltEntityForUpdate(*bbolt.Tx, Handler) (boltz.Entity, error) {
	return entity.toBoltEntity()
}

func (entity *ExternalJwtSigner) toBoltEntityForPatch(*bbolt.Tx, Handler, boltz.FieldChecker) (boltz.Entity, error) {
	signer := &persistence.ExternalJwtSigner{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:            entity.Name,
		CertPem:         entity.CertPem,
		Enabled:         entity.Enabled,
		ExternalAuthUrl: entity.ExternalAuthUrl,
		UseExternalId:   entity.UseExternalId,
		ClaimsProperty:  entity.ClaimsProperty,
	}

	if entity.CertPem != "" {
		signerCerts := nfpem.PemStringToCertificates(entity.CertPem)

		if len(signerCerts) != 1 {
			return nil, apierror.NewInvalidCertificatePem()
		}

		signerCert := signerCerts[0]
		fingerprint := nfpem.FingerprintFromCertificate(signerCert)

		signer.CommonName = signerCert.Subject.CommonName
		signer.NotBefore = &signerCert.NotBefore
		signer.NotAfter = &signerCert.NotAfter
		signer.Fingerprint = fingerprint
	}

	return signer, nil
}

func (entity *ExternalJwtSigner) fillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltExternalJwtSigner, ok := boltEntity.(*persistence.ExternalJwtSigner)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model ExternalJwtSigner", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltExternalJwtSigner)
	entity.Name = boltExternalJwtSigner.Name
	entity.CommonName = boltExternalJwtSigner.CommonName
	entity.CertPem = boltExternalJwtSigner.CertPem
	entity.Fingerprint = boltExternalJwtSigner.Fingerprint
	entity.Enabled = boltExternalJwtSigner.Enabled
	entity.NotBefore = *boltExternalJwtSigner.NotBefore
	entity.NotAfter = *boltExternalJwtSigner.NotAfter
	entity.ExternalAuthUrl = boltExternalJwtSigner.ExternalAuthUrl
	entity.ClaimsProperty = boltExternalJwtSigner.ClaimsProperty
	entity.UseExternalId = boltExternalJwtSigner.UseExternalId
	return nil
}
