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
	"github.com/openziti/foundation/v2/errorz"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
	"time"
)

type ApiSessionCertificate struct {
	models.BaseEntity
	ApiSession   *ApiSession
	ApiSessionId string
	Subject      string
	Fingerprint  string
	ValidAfter   *time.Time
	ValidBefore  *time.Time
	PEM          string
}

func NewApiSessionCertificate(cert *x509.Certificate) *ApiSessionCertificate {
	ret := &ApiSessionCertificate{
		Subject:     cert.Subject.String(),
		ValidAfter:  &cert.NotBefore,
		ValidBefore: &cert.NotAfter,
	}

	ret.Fingerprint = nfpem.FingerprintFromCertificate(cert)
	ret.PEM = nfpem.EncodeToString(cert)

	return ret
}

func (entity *ApiSessionCertificate) toBoltEntity(tx *bbolt.Tx, env Env) (*db.ApiSessionCertificate, error) {
	if !env.GetStores().ApiSession.IsEntityPresent(tx, entity.ApiSessionId) {
		return nil, errorz.NewFieldError("api session not found", "ApiSessionId", entity.ApiSessionId)
	}

	boltEntity := &db.ApiSessionCertificate{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		ApiSessionId:  entity.ApiSessionId,
		Subject:       entity.Subject,
		Fingerprint:   entity.Fingerprint,
		ValidAfter:    entity.ValidAfter,
		ValidBefore:   entity.ValidBefore,
		PEM:           entity.PEM,
	}

	return boltEntity, nil
}

func (entity *ApiSessionCertificate) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.ApiSessionCertificate, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *ApiSessionCertificate) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.ApiSessionCertificate, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *ApiSessionCertificate) fillFrom(env Env, tx *bbolt.Tx, boltApiSessionCertificate *db.ApiSessionCertificate) error {
	entity.FillCommon(boltApiSessionCertificate)
	entity.Subject = boltApiSessionCertificate.Subject
	entity.Fingerprint = boltApiSessionCertificate.Fingerprint
	entity.ValidAfter = boltApiSessionCertificate.ValidAfter
	entity.ValidBefore = boltApiSessionCertificate.ValidBefore
	entity.PEM = boltApiSessionCertificate.PEM
	entity.ApiSessionId = boltApiSessionCertificate.ApiSessionId

	boltApiSession, err := env.GetStores().ApiSession.LoadOneById(tx, boltApiSessionCertificate.ApiSessionId)
	if err != nil {
		return err
	}
	modelApiSession := &ApiSession{}
	if err := modelApiSession.fillFrom(env, tx, boltApiSession); err != nil {
		return err
	}
	entity.ApiSession = modelApiSession
	return nil
}
