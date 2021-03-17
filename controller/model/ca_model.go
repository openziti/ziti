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
	"fmt"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
)

type Ca struct {
	models.BaseEntity
	Name                      string
	Fingerprint               string
	CertPem                   string
	IsVerified                bool
	VerificationToken         string
	IsAutoCaEnrollmentEnabled bool
	IsOttCaEnrollmentEnabled  bool
	IsAuthEnabled             bool
	IdentityRoles             []string
	IdentityNameFormat        string
}

func (entity *Ca) fillFrom(_ Handler, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltCa, ok := boltEntity.(*persistence.Ca)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model ca", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltCa)
	entity.Name = boltCa.Name
	entity.Fingerprint = boltCa.Fingerprint
	entity.CertPem = boltCa.CertPem
	entity.IsVerified = boltCa.IsVerified
	entity.VerificationToken = boltCa.VerificationToken
	entity.IsAutoCaEnrollmentEnabled = boltCa.IsAutoCaEnrollmentEnabled
	entity.IsOttCaEnrollmentEnabled = boltCa.IsOttCaEnrollmentEnabled
	entity.IsAuthEnabled = boltCa.IsAuthEnabled
	entity.IdentityRoles = boltCa.IdentityRoles
	entity.IdentityNameFormat = boltCa.IdentityNameFormat
	return nil
}

func (entity *Ca) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (boltz.Entity, error) {
	var fp string

	if entity.CertPem != "" {
		blocks, err := cert.PemChain2Blocks(entity.CertPem)

		if err != nil {
			return nil, errorz.NewFieldError(err.Error(), "certPem", entity.CertPem)
		}

		if len(blocks) == 0 {
			return nil, errorz.NewFieldError("at least one leaf certificate must be supplied", "certPem", entity.CertPem)
		}

		certs, err := cert.Blocks2Certs(blocks)

		if err != nil {
			return nil, errorz.NewFieldError(err.Error(), "certPem", entity.CertPem)
		}

		leaf := certs[0]

		if !leaf.IsCA {
			//return nil, &response.ApiError{
			//	Code:           response.CertificateIsNotCaCode,
			//	Message:        response.CertificateIsNotCaMessage,
			//	HttpStatusCode: http.StatusBadRequest,
			//}
			return nil, errors.New("certificate is not a CA")
		}
		fp = cert.NewFingerprintGenerator().FromCert(certs[0])
	}

	if fp == "" {
		return nil, fmt.Errorf("invalid certificate, could not parse PEM body")
	}

	query := fmt.Sprintf(`fingerprint = "%v"`, fp)
	queryResults, _, err := handler.GetEnv().GetStores().Ca.QueryIds(tx, query)

	if err != nil {
		return nil, err
	}
	if len(queryResults) > 0 {
		return nil, errorz.NewFieldError(fmt.Sprintf("certificate already used as CA %s", queryResults[0]), "certPem", entity.CertPem)
	}

	boltEntity := &persistence.Ca{
		BaseExtEntity:             *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:                      entity.Name,
		CertPem:                   entity.CertPem,
		Fingerprint:               fp,
		IsVerified:                false,
		VerificationToken:         eid.New(),
		IsAuthEnabled:             entity.IsAuthEnabled,
		IsAutoCaEnrollmentEnabled: entity.IsAutoCaEnrollmentEnabled,
		IsOttCaEnrollmentEnabled:  entity.IsOttCaEnrollmentEnabled,
		IdentityRoles:             entity.IdentityRoles,
		IdentityNameFormat:        entity.IdentityNameFormat,
	}

	return boltEntity, nil
}

func (entity *Ca) toBoltEntityForUpdate(_ *bbolt.Tx, _ Handler) (boltz.Entity, error) {
	boltEntity := &persistence.Ca{
		BaseExtEntity:             *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:                      entity.Name,
		IsAuthEnabled:             entity.IsAuthEnabled,
		IsAutoCaEnrollmentEnabled: entity.IsAutoCaEnrollmentEnabled,
		IsOttCaEnrollmentEnabled:  entity.IsOttCaEnrollmentEnabled,
		IsVerified:                entity.IsVerified,
		IdentityRoles:             entity.IdentityRoles,
		IdentityNameFormat:        entity.IdentityNameFormat,
	}

	return boltEntity, nil
}

func (entity *Ca) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler, checker boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntityForUpdate(tx, handler)
}
