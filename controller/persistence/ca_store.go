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
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldCaFingerprint                    = "fingerprint"
	FieldCaCertPem                        = "certPem"
	FieldCaIsVerified                     = "isVerified"
	FieldCaVerificationToken              = "verificationToken"
	FieldCaIsAutoCaEnrollmentEnabled      = "isAutoCaEnrollmentEnabled"
	FieldCaIsOttCaEnrollmentEnabled       = "isOttCaEnrollmentEnabled"
	FieldCaIsAuthEnabled                  = "isAuthEnabled"
	FieldCaIdentityNameFormat             = "identityNameFormat"
	FieldCaEnrollments                    = "enrollments"
	FieldCaExternalIdClaim                = "externalIdClaim"
	FieldCaExternalIdClaimLocation        = "externalIdClaim.location"
	FieldCaExternalIdClaimIndex           = "externalIdClaim.index"
	FieldCaExternalIdClaimMatcher         = "externalIdClaim.matcher"
	FieldCaExternalIdClaimMatcherCriteria = "externalIdClaim.matcherCriteria"
	FieldCaExternalIdClaimParser          = "externalIdClaim.parser"
	FieldCaExternalIdClaimParserCriteria  = "externalIdClaim.parserSeparator"
)

const (
	ExternalIdClaimLocCommonName = "COMMON_NAME"
	ExternalIdClaimLocSanUri     = "SAN_URI"
	ExternalIdClaimLocSanEmail   = "SAN_EMAIL"

	ExternalIdClaimMatcherAll    = "ALL"
	ExternalIdClaimMatcherSuffix = "SUFFIX"
	ExternalIdClaimMatcherPrefix = "PREFIX"
	ExternalIdClaimMatcherScheme = "SCHEME"

	ExternalIdClaimParserNone  = "NONE"
	ExternalIdClaimParserSplit = "SPLIT"
)

type Ca struct {
	boltz.BaseExtEntity
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
	ExternalIdClaim           *ExternalIdClaim
}

type ExternalIdClaim struct {
	Location        string
	Matcher         string
	MatcherCriteria string
	Parser          string
	ParserCriteria  string
	Index           int64
}

func (entity *Ca) GetName() string {
	return entity.Name
}

func (entity *Ca) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Fingerprint = bucket.GetStringOrError(FieldCaFingerprint)
	entity.CertPem = bucket.GetStringOrError(FieldCaCertPem)
	entity.IsVerified = bucket.GetBoolWithDefault(FieldCaIsVerified, false)
	entity.VerificationToken = bucket.GetStringOrError(FieldCaVerificationToken)
	entity.IsAutoCaEnrollmentEnabled = bucket.GetBoolWithDefault(FieldCaIsAutoCaEnrollmentEnabled, false)
	entity.IsOttCaEnrollmentEnabled = bucket.GetBoolWithDefault(FieldCaIsOttCaEnrollmentEnabled, false)
	entity.IsAuthEnabled = bucket.GetBoolWithDefault(FieldCaIsAuthEnabled, false)
	entity.IdentityRoles = bucket.GetStringList(FieldIdentityRoles)
	entity.IdentityNameFormat = bucket.GetStringWithDefault(FieldCaIdentityNameFormat, "")

	if externalField := bucket.GetBucket(FieldCaExternalIdClaim); externalField != nil {
		entity.ExternalIdClaim = &ExternalIdClaim{}
		entity.ExternalIdClaim.Location = externalField.GetStringWithDefault(FieldCaExternalIdClaimLocation, "")
		entity.ExternalIdClaim.Matcher = externalField.GetStringWithDefault(FieldCaExternalIdClaimMatcher, "")
		entity.ExternalIdClaim.MatcherCriteria = externalField.GetStringWithDefault(FieldCaExternalIdClaimMatcherCriteria, "")
		entity.ExternalIdClaim.Parser = externalField.GetStringWithDefault(FieldCaExternalIdClaimParser, "")
		entity.ExternalIdClaim.ParserCriteria = externalField.GetStringWithDefault(FieldCaExternalIdClaimParserCriteria, "")
		entity.ExternalIdClaim.Index = externalField.GetInt64WithDefault(FieldCaExternalIdClaimIndex, 0)
	}

}

func (entity *Ca) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetString(FieldCaFingerprint, entity.Fingerprint)
	ctx.SetString(FieldCaCertPem, entity.CertPem)
	ctx.SetBool(FieldCaIsVerified, entity.IsVerified)
	ctx.SetString(FieldCaVerificationToken, entity.VerificationToken)
	ctx.SetBool(FieldCaIsAutoCaEnrollmentEnabled, entity.IsAutoCaEnrollmentEnabled)
	ctx.SetBool(FieldCaIsOttCaEnrollmentEnabled, entity.IsOttCaEnrollmentEnabled)
	ctx.SetBool(FieldCaIsAuthEnabled, entity.IsAuthEnabled)
	ctx.SetStringList(FieldIdentityRoles, entity.IdentityRoles)
	ctx.SetString(FieldCaIdentityNameFormat, entity.IdentityNameFormat)

	if entity.ExternalIdClaim != nil {
		externalField := ctx.Bucket.GetOrCreateBucket(FieldCaExternalIdClaim)
		externalField.SetString(FieldCaExternalIdClaimLocation, entity.ExternalIdClaim.Location, ctx.FieldChecker)
		externalField.SetString(FieldCaExternalIdClaimMatcher, entity.ExternalIdClaim.Matcher, ctx.FieldChecker)
		externalField.SetString(FieldCaExternalIdClaimMatcherCriteria, entity.ExternalIdClaim.MatcherCriteria, ctx.FieldChecker)
		externalField.SetString(FieldCaExternalIdClaimParser, entity.ExternalIdClaim.Parser, ctx.FieldChecker)
		externalField.SetString(FieldCaExternalIdClaimParserCriteria, entity.ExternalIdClaim.ParserCriteria, ctx.FieldChecker)
		externalField.SetInt64(FieldCaExternalIdClaimIndex, entity.ExternalIdClaim.Index, ctx.FieldChecker)
	} else {
		_ = ctx.Bucket.DeleteBucket([]byte(FieldCaExternalIdClaim))
	}

}

func (entity *Ca) GetEntityType() string {
	return EntityTypeCas
}

type CaStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*Ca, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*Ca, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*Ca, error)
}

func newCaStore(stores *stores) *caStoreImpl {
	store := &caStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeCas),
	}
	store.InitImpl(store)
	return store
}

type caStoreImpl struct {
	*baseStore
	indexName         boltz.ReadIndex
	symbolEnrollments boltz.EntitySetSymbol
}

func (store *caStoreImpl) NewStoreEntity() boltz.Entity {
	return &Ca{}
}

func (store *caStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()
	store.AddSymbol(FieldCaFingerprint, ast.NodeTypeString)
	store.AddSymbol(FieldCaIsVerified, ast.NodeTypeBool)
	store.AddSymbol(FieldCaVerificationToken, ast.NodeTypeString)
	store.AddSymbol(FieldCaIsAutoCaEnrollmentEnabled, ast.NodeTypeBool)
	store.AddSymbol(FieldCaIsOttCaEnrollmentEnabled, ast.NodeTypeBool)
	store.AddSymbol(FieldCaIsAuthEnabled, ast.NodeTypeBool)
	store.AddSetSymbol(FieldIdentityRoles, ast.NodeTypeString)
	store.symbolEnrollments = store.AddFkSetSymbol(FieldCaEnrollments, store.stores.enrollment)

}

func (store *caStoreImpl) initializeLinked() {
}

func (store *caStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Ca, error) {
	entity := &Ca{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *caStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*Ca, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *caStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*Ca, error) {
	entity := &Ca{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *caStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	for _, enrollmentId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, FieldCaEnrollments) {
		if err := store.stores.enrollment.DeleteById(ctx, enrollmentId); err != nil {
			return err
		}
	}

	return store.baseStore.DeleteById(ctx, id)
}

func (store *caStoreImpl) Update(ctx boltz.MutateContext, entity boltz.Entity, checker boltz.FieldChecker) error {
	return store.baseStore.Update(ctx, entity, checker)
}
