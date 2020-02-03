/*
	Copyright 2019 Netfoundry, Inc.

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
	"errors"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

type Appwan struct {
	BaseEdgeEntityImpl
	Name       string
	Identities []string
	Services   []string
}

func (entity *Appwan) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Identities = bucket.GetStringList(EntityTypeIdentities)
	entity.Services = bucket.GetStringList(EntityTypeServices)
}

func (entity *Appwan) SetValues(ctx *boltz.PersistContext) {
	ctx.Bucket.SetError(errors.New("the appwan entity type is deprecated. it may be read, but not written"))
}

func (entity *Appwan) GetEntityType() string {
	return EntityTypeAppwans
}

type AppwanStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*Appwan, error)
}

func newAppwanStore(stores *stores) *appwanStoreImpl {
	store := &appwanStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeAppwans),
	}
	store.InitImpl(store)
	return store
}

type appwanStoreImpl struct {
	*baseStore

	indexName        boltz.ReadIndex
	symbolIdentities boltz.EntitySetSymbol
	symbolServices   boltz.EntitySetSymbol
}

func (store *appwanStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &Appwan{}
}

func (store *appwanStoreImpl) initializeLocal() {
	store.addBaseFields()

	store.indexName = store.addUniqueNameField()
	store.symbolServices = store.AddFkSetSymbol(EntityTypeServices, store.stores.edgeService)
	store.symbolIdentities = store.AddFkSetSymbol(EntityTypeIdentities, store.stores.identity)
}

func (store *appwanStoreImpl) initializeLinked() {
}

func (store *appwanStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Appwan, error) {
	entity := &Appwan{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
