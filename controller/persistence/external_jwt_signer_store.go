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
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
	"time"
)

const (
	FieldExternalJwtSignerFingerprint     = "fingerprint"
	FieldExternalJwtSignerCertPem         = "certPem"
	FieldExternalJwtSignerCommonName      = "commonName"
	FieldExternalJwtSignerNotAfter        = "notAfter"
	FieldExternalJwtSignerNotBefore       = "notBefore"
	FieldExternalJwtSignerEnabled         = "enabled"
	FieldExternalJwtSignerExternalAuthUrl = "externalAuthUrl"
	FieldExternalJwtSignerAuthPolicies    = "authPolicies"
	FieldExternalJwtSignerClaimsProperty  = "claimsProperty"
	FieldExternalJwtSignerUseExternalId   = "useExternalId"

	DefaultClaimsProperty = "sub"
)

type ExternalJwtSigner struct {
	boltz.BaseExtEntity
	Name            string
	Fingerprint     string
	CertPem         string
	CommonName      string
	NotAfter        *time.Time
	NotBefore       *time.Time
	Enabled         bool
	ExternalAuthUrl *string
	ClaimsProperty  *string
	UseExternalId   bool
}

func (entity *ExternalJwtSigner) GetName() string {
	return entity.Name
}

func (entity *ExternalJwtSigner) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringWithDefault(FieldName, "")
	entity.CertPem = bucket.GetStringWithDefault(FieldExternalJwtSignerCertPem, "")
	entity.Fingerprint = bucket.GetStringWithDefault(FieldExternalJwtSignerFingerprint, "")
	entity.CommonName = bucket.GetStringWithDefault(FieldExternalJwtSignerCommonName, "")
	entity.NotAfter = bucket.GetTime(FieldExternalJwtSignerNotAfter)
	entity.NotBefore = bucket.GetTime(FieldExternalJwtSignerNotBefore)
	entity.Enabled = bucket.GetBoolWithDefault(FieldExternalJwtSignerEnabled, false)
	entity.ExternalAuthUrl = bucket.GetString(FieldExternalJwtSignerExternalAuthUrl)
	entity.ClaimsProperty = bucket.GetString(FieldExternalJwtSignerClaimsProperty)
	entity.UseExternalId = bucket.GetBoolWithDefault(FieldExternalJwtSignerUseExternalId, false)
}

func (entity *ExternalJwtSigner) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetString(FieldExternalJwtSignerCertPem, entity.CertPem)
	ctx.SetString(FieldExternalJwtSignerFingerprint, entity.Fingerprint)
	ctx.SetString(FieldExternalJwtSignerCommonName, entity.CommonName)
	ctx.SetTimeP(FieldExternalJwtSignerNotAfter, entity.NotAfter)
	ctx.SetTimeP(FieldExternalJwtSignerNotBefore, entity.NotBefore)
	ctx.SetBool(FieldExternalJwtSignerEnabled, entity.Enabled)
	ctx.SetStringP(FieldExternalJwtSignerExternalAuthUrl, entity.ExternalAuthUrl)
	ctx.SetBool(FieldExternalJwtSignerUseExternalId, entity.UseExternalId)

	if entity.ClaimsProperty == nil || *entity.ClaimsProperty == "" {
		ctx.SetString(FieldExternalJwtSignerClaimsProperty, DefaultClaimsProperty)
	} else {
		ctx.SetStringP(FieldExternalJwtSignerClaimsProperty, entity.ClaimsProperty)
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
}

func (store *externalJwtSignerStoreImpl) NewStoreEntity() boltz.Entity {
	return &ExternalJwtSigner{}
}

func (store *externalJwtSignerStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()
	store.symbolFingerprint = store.AddSymbol(FieldExternalJwtSignerFingerprint, ast.NodeTypeString)
	store.fingerprintIndex = store.AddUniqueIndex(store.symbolFingerprint)

	store.AddSymbol(FieldExternalJwtSignerFingerprint, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerCertPem, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerCommonName, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerNotAfter, ast.NodeTypeDatetime)
	store.AddSymbol(FieldExternalJwtSignerNotBefore, ast.NodeTypeDatetime)
	store.AddSymbol(FieldExternalJwtSignerEnabled, ast.NodeTypeBool)
	store.AddSymbol(FieldExternalJwtSignerClaimsProperty, ast.NodeTypeString)
	store.AddSymbol(FieldExternalJwtSignerUseExternalId, ast.NodeTypeBool)

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
