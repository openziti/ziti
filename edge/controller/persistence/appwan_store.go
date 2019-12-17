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
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldAppwanServices   = "services"
	FieldAppwanIdentities = "identities"
)

type Appwan struct {
	BaseEdgeEntityImpl
	Name       string
	Identities []string
	Services   []string
}

func NewAppwan(name string) *Appwan {
	return &Appwan{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: uuid.New().String()},
		Name:               name,
	}
}

func (entity *Appwan) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Identities = bucket.GetStringList(FieldAppwanIdentities)
	entity.Services = bucket.GetStringList(FieldAppwanServices)
}

func (entity *Appwan) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetLinkedIds(FieldAppwanIdentities, entity.Identities)
	ctx.SetLinkedIds(FieldAppwanServices, entity.Services)
}

func (entity *Appwan) GetEntityType() string {
	return EntityTypeAppwans
}

type AppwanStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*Appwan, error)
	LoadOneByName(tx *bbolt.Tx, id string) (*Appwan, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*Appwan, error)
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
	store.symbolServices = store.AddFkSetSymbol(FieldAppwanServices, store.stores.edgeService)
	store.symbolIdentities = store.AddFkSetSymbol(FieldAppwanIdentities, store.stores.identity)
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

func (store *appwanStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*Appwan, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *appwanStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*Appwan, error) {
	entity := &Appwan{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
