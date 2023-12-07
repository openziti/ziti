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
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
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

func (entity *Mfa) toBoltEntity(tx *bbolt.Tx, env Env) (*db.Mfa, error) {
	if !env.GetStores().Identity.IsEntityPresent(tx, entity.IdentityId) {
		return nil, errorz.NewFieldError("identity not found", "IdentityId", entity.IdentityId)
	}

	boltEntity := &db.Mfa{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		IsVerified:    entity.IsVerified,
		IdentityId:    entity.IdentityId,
		RecoveryCodes: entity.RecoveryCodes,
		Secret:        entity.Secret,
	}

	return boltEntity, nil
}

func (entity *Mfa) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.Mfa, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *Mfa) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.Mfa, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *Mfa) fillFrom(env Env, tx *bbolt.Tx, boltMfa *db.Mfa) error {
	entity.FillCommon(boltMfa)
	entity.IsVerified = boltMfa.IsVerified
	entity.IdentityId = boltMfa.IdentityId
	entity.RecoveryCodes = boltMfa.RecoveryCodes
	entity.Secret = boltMfa.Secret
	boltIdentity, err := env.GetStores().Identity.LoadOneById(tx, boltMfa.IdentityId)
	if err != nil {
		return err
	}
	modelIdentity := &Identity{}
	if err = modelIdentity.fillFrom(env, tx, boltIdentity); err != nil {
		return err
	}
	entity.Identity = modelIdentity
	return nil
}
