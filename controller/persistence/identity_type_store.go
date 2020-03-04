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
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

type IdentityType struct {
	boltz.BaseExtEntity
	Name string
}

func (entity *IdentityType) GetName() string {
	return entity.Name
}

func (entity *IdentityType) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
}

func (entity *IdentityType) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
}

func (entity *IdentityType) GetEntityType() string {
	return EntityTypeIdentityTypes
}

type IdentityTypeStore interface {
	NameIndexedStore
	LoadOneById(tx *bbolt.Tx, id string) (*IdentityType, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*IdentityType, error)
}

func newIdentityTypeStore(stores *stores) *IdentityTypeStoreImpl {
	store := &IdentityTypeStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeIdentityTypes),
	}
	store.InitImpl(store)
	return store
}

type IdentityTypeStoreImpl struct {
	*baseStore
	indexName boltz.ReadIndex
}

func (store *IdentityTypeStoreImpl) NewStoreEntity() boltz.Entity {
	return &IdentityType{}
}

func (store *IdentityTypeStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()
}

func (store *IdentityTypeStoreImpl) initializeLinked() {
	// no links
}

func (store *IdentityTypeStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *IdentityTypeStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*IdentityType, error) {
	entity := &IdentityType{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *IdentityTypeStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*IdentityType, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *IdentityTypeStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*IdentityType, error) {
	entity := &IdentityType{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
