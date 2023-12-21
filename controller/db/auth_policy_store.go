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

package db

import (
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
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
	Name string `json:"name"`

	Primary   AuthPolicyPrimary   `json:"primary"`
	Secondary AuthPolicySecondary `json:"secondary"`
}

type AuthPolicyPrimary struct {
	Cert   AuthPolicyCert   `json:"cert"`
	Updb   AuthPolicyUpdb   `json:"updb"`
	ExtJwt AuthPolicyExtJwt `json:"extJwt"`
}

type AuthPolicySecondary struct {
	RequireTotp          bool    `json:"requireTotp"`
	RequiredExtJwtSigner *string `json:"requiredExtJwtSigner"`
}

type AuthPolicyCert struct {
	Allowed           bool `json:"allowed"`
	AllowExpiredCerts bool `json:"allowExpiredCerts"`
}

type AuthPolicyExtJwt struct {
	Allowed              bool     `json:"allowed"`
	AllowedExtJwtSigners []string `json:"allowedExtJwtSigners"`
}

type AuthPolicyUpdb struct {
	Allowed                bool  `json:"allowed"`
	MinPasswordLength      int64 `json:"minPasswordLength"`
	RequireSpecialChar     bool  `json:"requireSpecialChar"`
	RequireNumberChar      bool  `json:"requireNumberChar"`
	RequireMixedCase       bool  `json:"requireMixedCase"`
	MaxAttempts            int64 `json:"maxAttempts"`
	LockoutDurationMinutes int64 `json:"lockoutDurationMinutes"`
}

func (entity *AuthPolicy) GetName() string {
	return entity.Name
}

func (entity *AuthPolicy) GetEntityType() string {
	return EntityTypeAuthPolicies
}

var _ AuthPolicyStore = (*AuthPolicyStoreImpl)(nil)

type AuthPolicyStore interface {
	NameIndexed
	Store[*AuthPolicy]
}

func newAuthPolicyStore(stores *stores) *AuthPolicyStoreImpl {
	store := &AuthPolicyStoreImpl{}
	store.baseStore = newBaseStore[*AuthPolicy](stores, store)
	store.InitImpl(store)
	return store
}

type AuthPolicyStoreImpl struct {
	*baseStore[*AuthPolicy]
	indexName                             boltz.ReadIndex
	symbolPrimaryAllowedExtJwtSigners     boltz.EntitySetSymbol
	symbolSecondaryRequiredExtJwtSignerId boltz.EntitySymbol
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

func (store *AuthPolicyStoreImpl) NewEntity() *AuthPolicy {
	return &AuthPolicy{}
}

func (store *AuthPolicyStoreImpl) FillEntity(entity *AuthPolicy, bucket *boltz.TypedBucket) {
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

func (store *AuthPolicyStoreImpl) PersistEntity(entity *AuthPolicy, ctx *boltz.PersistContext) {
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
