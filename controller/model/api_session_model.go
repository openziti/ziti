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
	"github.com/openziti/foundation/util/stringz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"time"
)

type ApiSession struct {
	models.BaseEntity
	Token              string
	IdentityId         string
	Identity           *Identity
	IPAddress          string
	ConfigTypes        map[string]struct{}
	MfaComplete        bool
	MfaRequired        bool
	ExpiresAt          time.Time
	ExpirationDuration time.Duration
	LastActivityAt     time.Time
	AuthenticatorId    string
}

func (entity *ApiSession) toBoltEntity(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	if !handler.GetEnv().GetStores().Identity.IsEntityPresent(tx, entity.IdentityId) {
		return nil, errorz.NewFieldError("identity not found", "IdentityId", entity.IdentityId)
	}

	boltEntity := &persistence.ApiSession{
		BaseExtEntity:   *boltz.NewExtEntity(entity.Id, entity.Tags),
		Token:           entity.Token,
		IdentityId:      entity.IdentityId,
		ConfigTypes:     stringz.SetToSlice(entity.ConfigTypes),
		IPAddress:       entity.IPAddress,
		MfaComplete:     entity.MfaComplete,
		MfaRequired:     entity.MfaRequired,
		AuthenticatorId: entity.AuthenticatorId,
		LastActivityAt:  entity.LastActivityAt,
	}

	return boltEntity, nil
}

func (entity *ApiSession) toBoltEntityForCreate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *ApiSession) toBoltEntityForUpdate(tx *bbolt.Tx, handler EntityManager) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *ApiSession) toBoltEntityForPatch(tx *bbolt.Tx, handler EntityManager, _ boltz.FieldChecker) (boltz.Entity, error) {
	return entity.toBoltEntity(tx, handler)
}

func (entity *ApiSession) fillFrom(handler EntityManager, tx *bbolt.Tx, boltEntity boltz.Entity) error {
	boltApiSession, ok := boltEntity.(*persistence.ApiSession)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model ApiSession", reflect.TypeOf(boltEntity))
	}
	entity.FillCommon(boltApiSession)
	entity.Token = boltApiSession.Token
	entity.IdentityId = boltApiSession.IdentityId
	entity.ConfigTypes = stringz.SliceToSet(boltApiSession.ConfigTypes)
	entity.IPAddress = boltApiSession.IPAddress
	entity.MfaRequired = boltApiSession.MfaRequired
	entity.MfaComplete = boltApiSession.MfaComplete
	entity.ExpiresAt = entity.UpdatedAt.Add(handler.GetEnv().GetConfig().Api.SessionTimeout)
	entity.ExpirationDuration = handler.GetEnv().GetConfig().Api.SessionTimeout
	entity.LastActivityAt = boltApiSession.LastActivityAt
	entity.AuthenticatorId = boltApiSession.AuthenticatorId

	boltIdentity, err := handler.GetEnv().GetStores().Identity.LoadOneById(tx, boltApiSession.IdentityId)
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
