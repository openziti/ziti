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
)

const (
	TotpMinLength = 4
	TotpMaxLength = 6
)

type Mfa struct {
	models.BaseEntity
	IsVerified    bool
	IdentityId    string
	Identity      *Identity
	Secret        string
	RecoveryCodes []string
}

func (entity *Mfa) toBoltEntity(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	if !handler.GetEnv().GetStores().Identity.IsEntityPresent(tx, entity.IdentityId) {
		return nil, errorz.NewFieldError("identity not found", "IdentityId", entity.IdentityId)
	}

	boltEntity := &persistence.Mfa{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		IsVerified:    entity.IsVerified,
		IdentityId:    entity.IdentityId,
		RecoveryCodes: entity.RecoveryCodes,
		Secret:        entity.Secret,
	}

	return boltEntity, nil
}

func (entity *Mfa) toBoltEntityForCreate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *Mfa) toBoltEntityForUpdate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *Mfa) toBoltEntityForPatch(tx *bbolt.Tx, handler EntityManager, checker boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *Mfa) fillFrom(handler EntityManager, tx *bbolt.Tx, boltEntity boltz.Entity) error {
	boltMfa, ok := boltEntity.(*persistence.Mfa)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model Mfa", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltMfa)
	entity.IsVerified = boltMfa.IsVerified
	entity.IdentityId = boltMfa.IdentityId
	entity.RecoveryCodes = boltMfa.RecoveryCodes
	entity.Secret = boltMfa.Secret
	boltIdentity, err := handler.GetEnv().GetStores().Identity.LoadOneById(tx, boltMfa.IdentityId)
	if err != nil {
		return err
	}
	modelIdentity := &Identity{}
	if err := modelIdentity.fillFrom(handler, tx, boltIdentity); err != nil {
		return err
	}
	entity.Identity = modelIdentity
	return nil
}
