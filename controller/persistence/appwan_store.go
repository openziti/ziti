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
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetLinkedIds(EntityTypeIdentities, entity.Identities)
	ctx.SetLinkedIds(EntityTypeServices, entity.Services)
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
	symbolIdentities boltz.EntitySymbol
	symbolServices   boltz.EntitySymbol
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
	store.AddLinkCollection(store.symbolServices, store.stores.edgeService.symbolAppwans)
	store.AddLinkCollection(store.symbolIdentities, store.stores.identity.symbolAppwans)
}

func (store *appwanStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Appwan, error) {
	entity := &Appwan{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}
