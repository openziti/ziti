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
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/models"
	"go.etcd.io/bbolt"
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

func (entity *ApiSession) toBoltEntity(tx *bbolt.Tx, env Env) (*db.ApiSession, error) {
	if !env.GetStores().Identity.IsEntityPresent(tx, entity.IdentityId) {
		return nil, errorz.NewFieldError("identity not found", "IdentityId", entity.IdentityId)
	}

	boltEntity := &db.ApiSession{
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

func (entity *ApiSession) toBoltEntityForCreate(tx *bbolt.Tx, env Env) (*db.ApiSession, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *ApiSession) toBoltEntityForUpdate(tx *bbolt.Tx, env Env, _ boltz.FieldChecker) (*db.ApiSession, error) {
	return entity.toBoltEntity(tx, env)
}

func (entity *ApiSession) fillFrom(env Env, tx *bbolt.Tx, boltApiSession *db.ApiSession) error {
	entity.FillCommon(boltApiSession)
	entity.Token = boltApiSession.Token
	entity.IdentityId = boltApiSession.IdentityId
	entity.ConfigTypes = stringz.SliceToSet(boltApiSession.ConfigTypes)
	entity.IPAddress = boltApiSession.IPAddress
	entity.MfaRequired = boltApiSession.MfaRequired
	entity.MfaComplete = boltApiSession.MfaComplete
	entity.ExpiresAt = entity.UpdatedAt.Add(env.GetConfig().Api.SessionTimeout)
	entity.ExpirationDuration = env.GetConfig().Api.SessionTimeout
	entity.LastActivityAt = boltApiSession.LastActivityAt
	entity.AuthenticatorId = boltApiSession.AuthenticatorId

	boltIdentity, err := env.GetStores().Identity.LoadOneById(tx, boltApiSession.IdentityId)
	if err != nil {
		return err
	}
	modelIdentity := &Identity{}
	if err := modelIdentity.fillFrom(env, tx, boltIdentity); err != nil {
		return err
	}
	entity.Identity = modelIdentity
	return nil
}
