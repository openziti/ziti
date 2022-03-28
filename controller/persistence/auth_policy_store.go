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
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	DefaultUpdbMinPasswordLength = int64(5)
	DefaultUpdbMaxAttempts       = int64(5)
	DefaultAuthPolicyId          = "default"

	UpdbIndefiniteLockout      = int64(0)
	UpdbUnlimitedAttemptsLimit = int64(0)

	FieldAuthPolicyPrimaryCertAllowed           = "primary.cert.allowed"
	FieldAuthPolicyPrimaryCertAllowExpiredCerts = "primary.cert.allowExpiredCerts"

	FieldAuthPolicyPrimaryUpdbAllowed                = "primary.updb.allowed"
	FiledAuthPolicyPrimaryUpdbMinPasswordLength      = "primary.updb.minPasswordLength"
	FieldAuthPolicyPrimaryUpdbRequireSpecialChar     = "primary.updb.requireSpecialChar"
	FieldAuthPolicyPrimaryUpdbRequireNumberChar      = "primary.updb.requireNumberChar"
	FieldAuthPolicyPrimaryUpdbRequireMixedCase       = "primary.updb.requireMixedCase"
	FieldAuthPolicyPrimaryUpdbMaxAttempts            = "primary.updb.maxAttempts"
	FieldAuthPolicyPrimaryUpdbLockoutDurationMinutes = "primary.updb.lockoutDurationMinutes"

	FieldAuthPolicyPrimaryExtJwtAllowed        = "primary.extJwt.allowed"
	FieldAuthPolicyPrimaryExtJwtAllowedSigners = "primary.extJwt.allowedSigners"

	FieldAuthSecondaryPolicyRequireTotp          = "secondary.requireTotp"
	FieldAuthSecondaryPolicyRequiredExtJwtSigner = "secondary.requireExtJwtSigner"
)

type AuthPolicy struct {
	boltz.BaseExtEntity
	Name string

	Primary   AuthPolicyPrimary
	Secondary AuthPolicySecondary
}

type AuthPolicyPrimary struct {
	Cert   AuthPolicyCert
	Updb   AuthPolicyUpdb
	ExtJwt AuthPolicyExtJwt
}

type AuthPolicySecondary struct {
	RequireTotp          bool
	RequiredExtJwtSigner *string
}

type AuthPolicyCert struct {
	Allowed           bool
	AllowExpiredCerts bool
}

type AuthPolicyExtJwt struct {
	Allowed              bool
	AllowedExtJwtSigners []string
}

type AuthPolicyUpdb struct {
	Allowed                bool
	MinPasswordLength      int64
	RequireSpecialChar     bool
	RequireNumberChar      bool
	RequireMixedCase       bool
	MaxAttempts            int64
	LockoutDurationMinutes int64
}

func (entity *AuthPolicy) GetName() string {
	return entity.Name
}

func (entity *AuthPolicy) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)

	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Primary.Updb.Allowed = bucket.GetBoolWithDefault(FieldAuthPolicyPrimaryUpdbAllowed, true)
	entity.Primary.Updb.MinPasswordLength = bucket.GetInt64WithDefault(FiledAuthPolicyPrimaryUpdbMinPasswordLength, DefaultUpdbMinPasswordLength)
	entity.Primary.Updb.RequireSpecialChar = bucket.GetBoolWithDefault(FieldAuthPolicyPrimaryUpdbRequireSpecialChar, false)
	entity.Primary.Updb.RequireNumberChar = bucket.GetBoolWithDefault(FieldAuthPolicyPrimaryUpdbRequireNumberChar, false)
	entity.Primary.Updb.RequireMixedCase = bucket.GetBoolWithDefault(FieldAuthPolicyPrimaryUpdbRequireMixedCase, false)
	entity.Primary.Updb.MaxAttempts = bucket.GetInt64WithDefault(FieldAuthPolicyPrimaryUpdbMaxAttempts, DefaultUpdbMaxAttempts)
	entity.Primary.Updb.LockoutDurationMinutes = bucket.GetInt64WithDefault(FieldAuthPolicyPrimaryUpdbLockoutDurationMinutes, 0)

	entity.Primary.Cert.Allowed = bucket.GetBoolWithDefault(FieldAuthPolicyPrimaryCertAllowed, true)
	entity.Primary.Cert.AllowExpiredCerts = bucket.GetBoolWithDefault(FieldAuthPolicyPrimaryCertAllowExpiredCerts, true)

	entity.Primary.ExtJwt.Allowed = bucket.GetBoolWithDefault(FieldAuthPolicyPrimaryExtJwtAllowed, true)
	entity.Primary.ExtJwt.AllowedExtJwtSigners = bucket.GetStringList(FieldAuthPolicyPrimaryExtJwtAllowedSigners)

	entity.Secondary.RequireTotp = bucket.GetBoolWithDefault(FieldAuthSecondaryPolicyRequireTotp, false)
	entity.Secondary.RequiredExtJwtSigner = bucket.GetString(FieldAuthSecondaryPolicyRequiredExtJwtSigner)
}

func (entity *AuthPolicy) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)

	if entity.Primary.Updb.LockoutDurationMinutes < 0 {
		entity.Primary.Updb.LockoutDurationMinutes = UpdbIndefiniteLockout
	}

	if entity.Primary.Updb.MaxAttempts < 0 {
		entity.Primary.Updb.MaxAttempts = UpdbUnlimitedAttemptsLimit
	}

	if entity.Primary.Updb.MinPasswordLength < DefaultUpdbMinPasswordLength {
		entity.Primary.Updb.MinPasswordLength = DefaultUpdbMinPasswordLength
	}

	ctx.SetString(FieldName, entity.Name)

	ctx.SetBool(FieldAuthPolicyPrimaryCertAllowed, entity.Primary.Cert.Allowed)
	ctx.SetBool(FieldAuthPolicyPrimaryCertAllowExpiredCerts, entity.Primary.Cert.AllowExpiredCerts)

	ctx.SetBool(FieldAuthPolicyPrimaryUpdbAllowed, entity.Primary.Updb.Allowed)
	ctx.SetInt64(FiledAuthPolicyPrimaryUpdbMinPasswordLength, entity.Primary.Updb.MinPasswordLength)
	ctx.SetBool(FieldAuthPolicyPrimaryUpdbRequireSpecialChar, entity.Primary.Updb.RequireSpecialChar)
	ctx.SetBool(FieldAuthPolicyPrimaryUpdbRequireNumberChar, entity.Primary.Updb.RequireNumberChar)
	ctx.SetBool(FieldAuthPolicyPrimaryUpdbRequireMixedCase, entity.Primary.Updb.RequireMixedCase)
	ctx.SetInt64(FieldAuthPolicyPrimaryUpdbMaxAttempts, entity.Primary.Updb.MaxAttempts)
	ctx.SetInt64(FieldAuthPolicyPrimaryUpdbLockoutDurationMinutes, entity.Primary.Updb.LockoutDurationMinutes)

	ctx.SetBool(FieldAuthPolicyPrimaryExtJwtAllowed, entity.Primary.ExtJwt.Allowed)
	ctx.SetStringList(FieldAuthPolicyPrimaryExtJwtAllowedSigners, entity.Primary.ExtJwt.AllowedExtJwtSigners)

	ctx.SetBool(FieldAuthSecondaryPolicyRequireTotp, entity.Secondary.RequireTotp)
	ctx.SetStringP(FieldAuthSecondaryPolicyRequiredExtJwtSigner, entity.Secondary.RequiredExtJwtSigner)
}

func (entity *AuthPolicy) GetEntityType() string {
	return EntityTypeAuthPolicies
}

type AuthPolicyStore interface {
	NameIndexedStore
	LoadOneById(tx *bbolt.Tx, id string) (*AuthPolicy, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*AuthPolicy, error)
}

func newAuthPolicyStore(stores *stores) *AuthPolicyStoreImpl {
	store := &AuthPolicyStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeAuthPolicies),
	}
	store.InitImpl(store)
	return store
}

type AuthPolicyStoreImpl struct {
	*baseStore
	indexName                             boltz.ReadIndex
	symbolExtJwtSignerId                  boltz.EntitySymbol
	symbolPrimaryAllowedExtJwtSigners     boltz.EntitySetSymbol
	symbolSecondaryRequiredExtJwtSignerId boltz.EntitySymbol
}

func (store *AuthPolicyStoreImpl) NewStoreEntity() boltz.Entity {
	return &AuthPolicy{}
}

func (store *AuthPolicyStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()

	store.AddSymbol(FieldAuthPolicyPrimaryCertAllowed, ast.NodeTypeBool)
	store.AddSymbol(FieldAuthPolicyPrimaryCertAllowExpiredCerts, ast.NodeTypeBool)

	store.AddSymbol(FieldAuthPolicyPrimaryUpdbAllowed, ast.NodeTypeBool)
	store.AddSymbol(FiledAuthPolicyPrimaryUpdbMinPasswordLength, ast.NodeTypeInt64)
	store.AddSymbol(FieldAuthPolicyPrimaryUpdbRequireSpecialChar, ast.NodeTypeBool)
	store.AddSymbol(FieldAuthPolicyPrimaryUpdbRequireNumberChar, ast.NodeTypeInt64)
	store.AddSymbol(FieldAuthPolicyPrimaryUpdbRequireMixedCase, ast.NodeTypeBool)
	store.AddSymbol(FieldAuthPolicyPrimaryUpdbMaxAttempts, ast.NodeTypeBool)

	store.AddSymbol(FieldAuthPolicyPrimaryExtJwtAllowed, ast.NodeTypeBool)

	store.AddSymbol(FieldAuthSecondaryPolicyRequireTotp, ast.NodeTypeBool)
	store.AddSymbol(FieldAuthSecondaryPolicyRequiredExtJwtSigner, ast.NodeTypeString)

	store.symbolPrimaryAllowedExtJwtSigners = store.AddFkSetSymbol(FieldAuthPolicyPrimaryExtJwtAllowedSigners, store.stores.externalJwtSigner)

	store.symbolSecondaryRequiredExtJwtSignerId = store.AddFkSymbol(FieldAuthSecondaryPolicyRequiredExtJwtSigner, store.stores.externalJwtSigner)
	store.AddFkConstraint(store.symbolSecondaryRequiredExtJwtSignerId, true, boltz.CascadeNone)
}

func (store *AuthPolicyStoreImpl) initializeLinked() {
	store.AddNullableFkIndex(store.symbolPrimaryAllowedExtJwtSigners, store.stores.externalJwtSigner.symbolAuthPolicies)
}

func (store *AuthPolicyStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *AuthPolicyStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*AuthPolicy, error) {
	entity := &AuthPolicy{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *AuthPolicyStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*AuthPolicy, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *AuthPolicyStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*AuthPolicy, error) {
	entity := &AuthPolicy{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
