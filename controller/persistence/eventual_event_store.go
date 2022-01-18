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
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldEventualEventType = "type"
	FieldEventualEventData = "data"
)

type EventualEvent struct {
	boltz.BaseExtEntity
	Type string
	Data []byte
}

func (entity *EventualEvent) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Type = bucket.GetStringOrError(FieldEventualEventType)
	entity.Data = bucket.Get([]byte(FieldEventualEventData))

}

func (entity *EventualEvent) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldEventualEventType, entity.Type)
	ctx.Bucket.SetError(ctx.Bucket.Put([]byte(FieldEventualEventData), entity.Data))
}

func (entity *EventualEvent) GetEntityType() string {
	return EntityTypeEventualEvents
}

type EventualEventStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*EventualEvent, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*EventualEvent, error)
}

func newEventualEventStore(stores *stores) *eventualEventStoreImpl {
	store := &eventualEventStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeEventualEvents),
	}
	store.InitImpl(store)
	return store
}

type eventualEventStoreImpl struct {
	*baseStore
	indexName         boltz.ReadIndex
	symbolEnrollments boltz.EntitySetSymbol
}

func (store *eventualEventStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*EventualEvent, error) {
	entity := &EventualEvent{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *eventualEventStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*EventualEvent, error) {
	entity := &EventualEvent{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *eventualEventStoreImpl) NewStoreEntity() boltz.Entity {
	return &EventualEvent{}
}

func (store *eventualEventStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.AddSymbol(FieldEventualEventData, ast.NodeTypeOther)
}

func (store *eventualEventStoreImpl) initializeLinked() {
}
