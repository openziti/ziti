/*
	Copyright 2020 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldCaFingerprint               = "fingerprint"
	FieldCaCertPem                   = "certPem"
	FieldCaIsVerified                = "isVerified"
	FieldCaVerificationToken         = "verificationToken"
	FieldCaIsAutoCaEnrollmentEnabled = "isAutoCaEnrollmentEnabled"
	FieldCaIsOttCaEnrollmentEnabled  = "isOttCaEnrollmentEnabled"
	FieldCaIsAuthEnabled             = "isAuthEnabled"
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
	indexName boltz.ReadIndex
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
	// TODO: Delete enrollment certs
	return store.baseStore.DeleteById(ctx, id)
}
