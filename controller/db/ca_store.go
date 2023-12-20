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
	Name                      string           `json:"name"`
	Fingerprint               string           `json:"fingerprint"`
	CertPem                   string           `json:"certPem"`
	IsVerified                bool             `json:"isVerified"`
	VerificationToken         string           `json:"verificationToken"`
	IsAutoCaEnrollmentEnabled bool             `json:"isAutoCaEnrollmentEnabled"`
	IsOttCaEnrollmentEnabled  bool             `json:"isOttCaEnrollmentEnabled"`
	IsAuthEnabled             bool             `json:"isAuthEnabled"`
	IdentityRoles             []string         `json:"identityRoles"`
	IdentityNameFormat        string           `json:"identityNameFormat"`
	ExternalIdClaim           *ExternalIdClaim `json:"externalIdClaim"`
}

type ExternalIdClaim struct {
	Location        string `json:"location"`
	Matcher         string `json:"matcher"`
	MatcherCriteria string `json:"matcherCriteria"`
	Parser          string `json:"parser"`
	ParserCriteria  string `json:"parserCriteria"`
	Index           int64  `json:"index"`
}

func (entity *Ca) GetName() string {
	return entity.Name
}

func (entity *Ca) GetEntityType() string {
	return EntityTypeCas
}

var _ CaStore = (*caStoreImpl)(nil)

type CaStore interface {
	Store[*Ca]
}

func newCaStore(stores *stores) *caStoreImpl {
	store := &caStoreImpl{}
	store.baseStore = newBaseStore[*Ca](stores, store)
	store.InitImpl(store)
	return store
}

type caStoreImpl struct {
	*baseStore[*Ca]
	indexName         boltz.ReadIndex
	symbolEnrollments boltz.EntitySetSymbol
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

func (store *caStoreImpl) initializeLinked() {}

func (store *caStoreImpl) NewEntity() *Ca {
	return &Ca{}
}

func (store *caStoreImpl) FillEntity(entity *Ca, bucket *boltz.TypedBucket) {
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

func (store *caStoreImpl) PersistEntity(entity *Ca, ctx *boltz.PersistContext) {
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

func (store *caStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	for _, enrollmentId := range store.GetRelatedEntitiesIdList(ctx.Tx(), id, FieldCaEnrollments) {
		if err := store.stores.enrollment.DeleteById(ctx, enrollmentId); err != nil {
			return err
		}
	}

	return store.baseStore.DeleteById(ctx, id)
}
