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
	FieldEventualEventType = "type"
	FieldEventualEventData = "data"
)

type EventualEvent struct {
	boltz.BaseExtEntity
	Type string `json:"type"`
	Data []byte `json:"data"`
}

func (entity *EventualEvent) GetEntityType() string {
	return EntityTypeEventualEvents
}

var _ EventualEventStore = (*eventualEventStoreImpl)(nil)

type EventualEventStore interface {
	Store[*EventualEvent]
}

func newEventualEventStore(stores *stores) *eventualEventStoreImpl {
	store := &eventualEventStoreImpl{}
	store.baseStore = newBaseStore[*EventualEvent](stores, store)
	store.InitImpl(store)
	return store
}

type eventualEventStoreImpl struct {
	*baseStore[*EventualEvent]
}

func (store *eventualEventStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.AddSymbol(FieldEventualEventData, ast.NodeTypeOther)
}

func (store *eventualEventStoreImpl) initializeLinked() {}

func (store *eventualEventStoreImpl) NewEntity() *EventualEvent {
	return &EventualEvent{}
}

func (store *eventualEventStoreImpl) FillEntity(entity *EventualEvent, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Type = bucket.GetStringOrError(FieldEventualEventType)
	entity.Data = bucket.Get([]byte(FieldEventualEventData))
}

func (store *eventualEventStoreImpl) PersistEntity(entity *EventualEvent, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldEventualEventType, entity.Type)
	ctx.Bucket.SetError(ctx.Bucket.Put([]byte(FieldEventualEventData), entity.Data))
}
