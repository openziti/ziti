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

package persistence

import (
	"github.com/google/uuid"
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldMfaIdentity      = "identity"
	FieldMfaIsVerified    = "isVerified"
	FieldMfaRecoveryCodes = "recoveryCodes"
	FieldMfaSecret        = "secret"
	FieldMfaSalt          = "salt"
)

type Mfa struct {
	boltz.BaseExtEntity
	IdentityId    string
	IsVerified    bool
	Secret        string
	Salt          string
	RecoveryCodes []string
}

func NewMfa(identityId string) *Mfa {
	return &Mfa{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		IdentityId:    identityId,
		Salt:          uuid.New().String(),
		IsVerified:    false,
	}
}

func (entity *Mfa) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.IdentityId = bucket.GetStringOrError(FieldMfaIdentity)
	entity.IsVerified = bucket.GetBoolWithDefault(FieldMfaIsVerified, false)
	entity.RecoveryCodes = bucket.GetStringList(FieldMfaRecoveryCodes)
	entity.Salt = bucket.GetStringOrError(FieldMfaSalt)
	entity.Secret = bucket.GetStringWithDefault(FieldMfaSecret, "")
}

func (entity *Mfa) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldMfaIdentity, entity.IdentityId)
	ctx.SetBool(FieldMfaIsVerified, entity.IsVerified)
	ctx.SetStringList(FieldMfaRecoveryCodes, entity.RecoveryCodes)
	ctx.SetString(FieldMfaSalt, entity.Salt)
	ctx.SetString(FieldMfaSecret, entity.Secret)
}

func (entity *Mfa) GetEntityType() string {
	return EntityTypeMfas
}

type MfaStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*Mfa, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*Mfa, error)
}

func newMfaStore(stores *stores) *MfaStoreImpl {
	store := &MfaStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeMfas),
	}

	store.InitImpl(store)
	return store
}

type SecretStore interface {
	GetSecret() []byte

}

type MfaStoreImpl struct {
	*baseStore
	symbolIdentity boltz.EntitySymbol
}

func (store *MfaStoreImpl) NewStoreEntity() boltz.Entity {
	return &Mfa{}
}

func (store *MfaStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.symbolIdentity = store.AddFkSymbol(FieldMfaIdentity, store.stores.identity)

	store.AddFkConstraint(store.symbolIdentity, false, boltz.CascadeDelete)
}

func (store *MfaStoreImpl) initializeLinked() {
}

func (store *MfaStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Mfa, error) {
	entity := &Mfa{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *MfaStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*Mfa, error) {
	entity := &Mfa{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
