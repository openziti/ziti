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
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
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

func (entity *ApiSessionCertificate) toBoltEntity(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	if !handler.GetEnv().GetStores().ApiSession.IsEntityPresent(tx, entity.ApiSessionId) {
		return nil, errorz.NewFieldError("api session not found", "ApiSessionId", entity.ApiSessionId)
	}

	boltEntity := &persistence.ApiSessionCertificate{
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

func (entity *ApiSessionCertificate) toBoltEntityForCreate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *ApiSessionCertificate) toBoltEntityForUpdate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *ApiSessionCertificate) toBoltEntityForPatch(tx *bbolt.Tx, handler EntityManager, _ boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *ApiSessionCertificate) fillFrom(handler EntityManager, tx *bbolt.Tx, boltEntity boltz.Entity) error {
	boltApiSessionCertificate, ok := boltEntity.(*persistence.ApiSessionCertificate)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model ApiSessionCertificate", reflect.TypeOf(boltEntity))
	}

	entity.FillCommon(boltApiSessionCertificate)
	entity.Subject = boltApiSessionCertificate.Subject
	entity.Fingerprint = boltApiSessionCertificate.Fingerprint
	entity.ValidAfter = boltApiSessionCertificate.ValidAfter
	entity.ValidBefore = boltApiSessionCertificate.ValidBefore
	entity.PEM = boltApiSessionCertificate.PEM
	entity.ApiSessionId = boltApiSessionCertificate.ApiSessionId

	boltApiSession, err := handler.GetEnv().GetStores().ApiSession.LoadOneById(tx, boltApiSessionCertificate.ApiSessionId)
	if err != nil {
		return err
	}
	modelApiSession := &ApiSession{}
	if err := modelApiSession.fillFrom(handler, tx, boltApiSession); err != nil {
		return err
	}
	entity.ApiSession = modelApiSession
	return nil
}
