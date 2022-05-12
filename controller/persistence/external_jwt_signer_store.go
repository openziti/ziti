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
	"fmt"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"strings"
	"time"
)

const (
	FieldExternalJwtSignerFingerprint     = "fingerprint"
	FieldExternalJwtSignerCertPem         = "certPem"
	FieldExternalJwtSignerJwksEndpoint    = "jwksEndpoint"
	FieldExternalJwtSignerCommonName      = "commonName"
	FieldExternalJwtSignerNotAfter        = "notAfter"
	FieldExternalJwtSignerNotBefore       = "notBefore"
	FieldExternalJwtSignerEnabled         = "enabled"
	FieldExternalJwtSignerExternalAuthUrl = "externalAuthUrl"
	FieldExternalJwtSignerAuthPolicies    = "authPolicies"
	FieldExternalJwtSignerClaimsProperty  = "claimsProperty"
	FieldExternalJwtSignerUseExternalId   = "useExternalId"
	FieldExternalJwtSignerKid             = "kid"
	FieldExternalJwtSignerIssuer          = "issuer"
	FieldExternalJwtSignerAudience        = "audience"

	DefaultClaimsProperty = "sub"
)

type ExternalJwtSigner struct {
	boltz.BaseExtEntity
	Name            string
	Fingerprint     *string
	Kid             *string
	CertPem         *string
	JwksEndpoint    *string
	CommonName      string
	NotAfter        *time.Time
	NotBefore       *time.Time
	Enabled         bool
	ExternalAuthUrl *string
	ClaimsProperty  *string
	UseExternalId   bool
	Issuer          *string
	Audience        *string
}

func (entity *ExternalJwtSigner) GetName() string {
	return entity.Name
}

func (entity *ExternalJwtSigner) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringWithDefault(FieldName, "")
	entity.CertPem = bucket.GetString(FieldExternalJwtSignerCertPem)
	entity.JwksEndpoint = bucket.GetString(FieldExternalJwtSignerJwksEndpoint)
	entity.Fingerprint = bucket.GetString(FieldExternalJwtSignerFingerprint)
	entity.Kid = bucket.GetString(FieldExternalJwtSignerKid)
	entity.CommonName = bucket.GetStringWithDefault(FieldExternalJwtSignerCommonName, "")
	entity.NotAfter = bucket.GetTime(FieldExternalJwtSignerNotAfter)
	entity.NotBefore = bucket.GetTime(FieldExternalJwtSignerNotBefore)
	entity.Enabled = bucket.GetBoolWithDefault(FieldExternalJwtSignerEnabled, false)
	entity.ExternalAuthUrl = bucket.GetString(FieldExternalJwtSignerExternalAuthUrl)
	entity.ClaimsProperty = bucket.GetString(FieldExternalJwtSignerClaimsProperty)
	entity.UseExternalId = bucket.GetBoolWithDefault(FieldExternalJwtSignerUseExternalId, false)
	entity.Issuer = bucket.GetString(FieldExternalJwtSignerIssuer)
	entity.Audience = bucket.GetString(FieldExternalJwtSignerAudience)
}

func (entity *ExternalJwtSigner) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetStringP(FieldExternalJwtSignerCertPem, entity.CertPem)
	ctx.SetStringP(FieldExternalJwtSignerJwksEndpoint, entity.JwksEndpoint)
	ctx.SetStringP(FieldExternalJwtSignerFingerprint, entity.Fingerprint)
	ctx.SetStringP(FieldExternalJwtSignerKid, entity.Kid)
	ctx.SetString(FieldExternalJwtSignerCommonName, entity.CommonName)
	ctx.SetTimeP(FieldExternalJwtSignerNotAfter, entity.NotAfter)
	ctx.SetTimeP(FieldExternalJwtSignerNotBefore, entity.NotBefore)
	ctx.SetBool(FieldExternalJwtSignerEnabled, entity.Enabled)
	ctx.SetBool(FieldExternalJwtSignerUseExternalId, entity.UseExternalId)

	if entity.ExternalAuthUrl != nil && strings.TrimSpace(*entity.ExternalAuthUrl) == "" {
		entity.ExternalAuthUrl = nil
	}
	ctx.SetStringP(FieldExternalJwtSignerExternalAuthUrl, entity.ExternalAuthUrl)

	if entity.Issuer != nil && strings.TrimSpace(*entity.Issuer) == "" {
		entity.Issuer = nil
	}
	ctx.SetStringP(FieldExternalJwtSignerIssuer, entity.Issuer)

	if entity.Audience != nil && strings.TrimSpace(*entity.Audience) == "" {
		entity.Audience = nil
	}
	ctx.SetStringP(FieldExternalJwtSignerAudience, entity.Audience)

	if entity.ClaimsProperty == nil || strings.TrimSpace(*entity.ClaimsProperty) == "" {
		ctx.SetString(FieldExternalJwtSignerClaimsProperty, DefaultClaimsProperty)
	} else {
		ctx.SetStringP(FieldExternalJwtSignerClaimsProperty, entity.ClaimsProperty)
	}

	jwksEndpoint := ctx.Bucket.GetString(FieldExternalJwtSignerJwksEndpoint)
	certPem := ctx.Bucket.GetString(FieldExternalJwtSignerCertPem)

	if (jwksEndpoint == nil || *jwksEndpoint == "") && (certPem == nil || *certPem == "") {
		ctx.Bucket.SetError(apierror.NewBadRequestFieldError(*errorz.NewFieldError("jwksEndpoint or certPem is required", "certPem", certPem)))
	}

	if jwksEndpoint != nil && certPem != nil {
		existingName := FieldExternalJwtSignerCertPem
		existingValue := certPem
		if jwksEndpoint != nil {
			existingName = FieldExternalJwtSignerJwksEndpoint
			existingValue = jwksEndpoint
		}
		ctx.Bucket.SetError(apierror.NewBadRequestFieldError(*errorz.NewFieldError("only one of jwksEndpoint or certPem may be defined", existingName, existingValue)))
	}
}

func (entity *ExternalJwtSigner) GetEntityType() string {
	return EntityTypeExternalJwtSigners
}

type ExternalJwtSignerStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*ExternalJwtSigner, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*ExternalJwtSigner, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*ExternalJwtSigner, error)
}

func newExternalJwtSignerStore(stores *stores) *externalJwtSignerStoreImpl {
	store := &externalJwtSignerStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeExternalJwtSigners),
	}
	store.InitImpl(store)
	return store
}

type externalJwtSignerStoreImpl struct {
	*baseStore
	indexName          boltz.ReadIndex
	symbolFingerprint  boltz.EntitySymbol
	symbolEnrollments  boltz.EntitySetSymbol
	symbolAuthPolicies boltz.EntitySetSymbol
	fingerprintIndex   boltz.ReadIndex
	symbolKid          boltz.EntitySymbol
	kidIndex           boltz.ReadIndex
	symbolIssuer       boltz.EntitySymbol
	issuerIndex        boltz.ReadIndex
}

func (store *externalJwtSignerStoreImpl) NewStoreEntity() boltz.Entity {
	return &ExternalJwtSigner{}
}

func (store *externalJwtSignerStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()

	store.symbolFingerprint = store.AddSymbol(FieldExternalJwtSignerFingerprint, ast.NodeTypeString)
	store.fingerprintIndex = store.AddNullableUniqueIndex(store.symbolFingerprint)

	store.symbolKid = store.AddSymbol(FieldExternalJwtSignerKid, ast.NodeTypeString)
	store.kidIndex = store.AddNullableUniqueIndex(store.symbolKid)

	store.symbolIssuer = store.AddSymbol(FieldExternalJwtSignerIssuer, ast.NodeTypeString)
	store.issuerIndex = store.AddUniqueIndex(store.symbolIssuer)

	store.AddSymbol(FieldExternalJwtSignerCertPem, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerCommonName, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerNotAfter, ast.NodeTypeDatetime)
	store.AddSymbol(FieldExternalJwtSignerNotBefore, ast.NodeTypeDatetime)
	store.AddSymbol(FieldExternalJwtSignerEnabled, ast.NodeTypeBool)
	store.AddSymbol(FieldExternalJwtSignerClaimsProperty, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerUseExternalId, ast.NodeTypeBool)
	store.AddSymbol(FieldExternalJwtSignerAudience, ast.NodeTypeString)

	store.symbolAuthPolicies = store.AddFkSetSymbol(FieldExternalJwtSignerAuthPolicies, store.stores.authPolicy)
}

func (store *externalJwtSignerStoreImpl) initializeLinked() {
}

func (store *externalJwtSignerStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*ExternalJwtSigner, error) {
	entity := &ExternalJwtSigner{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *externalJwtSignerStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*ExternalJwtSigner, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *externalJwtSignerStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*ExternalJwtSigner, error) {
	entity := &ExternalJwtSigner{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
func (store *externalJwtSignerStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	ids, _, err := store.stores.authPolicy.QueryIds(ctx.Tx(), fmt.Sprintf(`anyOf(%s) = "%s"`, FieldAuthPolicyPrimaryExtJwtAllowedSigners, id))

	if err != nil {
		return err
	}

	if len(ids) > 0 {
		return boltz.NewReferenceByIdsError(EntityTypeExternalJwtSigners, id, EntityTypeAuthPolicies, ids, FieldAuthPolicyPrimaryExtJwtAllowedSigners)
	}

	err = store.BaseStore.DeleteById(ctx, id)

	return err
}
