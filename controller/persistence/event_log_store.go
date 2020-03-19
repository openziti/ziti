/*
	Copyright 2019 NetFoundry, Inc.

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
	FieldEventLogType             = "type"
	FieldEventLogActorType        = "actorType"
	FieldEventLogActorId          = "actorId"
	FieldEventLogEntityType       = "entityType"
	FieldEventLogEntityId         = "entityId"
	FieldEventLogFormattedMessage = "formattedMessage"
	FieldEventLogFormatString     = "formatString"
	FieldEventLogFormatData       = "formatData"
	FieldEventLogData             = "data"
)

type EventLog struct {
	boltz.BaseExtEntity
	Type             string
	ActorType        string
	ActorId          string
	EntityType       string
	EntityId         string
	FormattedMessage string
	FormatString     string
	FormatData       string
	Data             map[string]interface{}
}

func (entity *EventLog) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Type = bucket.GetStringOrError(FieldEventLogType)
	entity.ActorType = bucket.GetStringOrError(FieldEventLogActorType)
	entity.ActorId = bucket.GetStringOrError(FieldEventLogActorId)
	entity.EntityType = bucket.GetStringOrError(FieldEventLogEntityType)
	entity.EntityId = bucket.GetStringOrError(FieldEventLogEntityId)
	entity.FormattedMessage = bucket.GetStringOrError(FieldEventLogFormattedMessage)
	entity.FormatString = bucket.GetStringOrError(FieldEventLogFormatString)
	entity.FormatData = bucket.GetStringOrError(FieldEventLogFormatData)
	entity.Data = bucket.GetMap(FieldEventLogData)
}

func (entity *EventLog) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldEventLogType, entity.Type)
	ctx.SetString(FieldEventLogActorType, entity.ActorType)
	ctx.SetString(FieldEventLogActorId, entity.ActorId)
	ctx.SetString(FieldEventLogEntityType, entity.EntityType)
	ctx.SetString(FieldEventLogEntityId, entity.EntityId)
	ctx.SetString(FieldEventLogFormattedMessage, entity.FormattedMessage)
	ctx.SetString(FieldEventLogFormatString, entity.FormatString)
	ctx.SetString(FieldEventLogFormatData, entity.FormatData)
	ctx.SetMap(FieldEventLogData, entity.Data)
}

func (entity *EventLog) GetEntityType() string {
	return EntityTypeEventLogs
}

type EventLogStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*EventLog, error)
	LoadOneByQuery(tx *bbolt.Tx, query string) (*EventLog, error)
}

func newEventLogStore(stores *stores) *eventLogStoreImpl {
	store := &eventLogStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeEventLogs),
	}
	store.InitImpl(store)
	return store
}

type eventLogStoreImpl struct {
	*baseStore
}

func (store *eventLogStoreImpl) NewStoreEntity() boltz.Entity {
	return &Cluster{}
}

func (store *eventLogStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	store.AddSymbol(FieldEventLogType, ast.NodeTypeString)
	store.AddSymbol(FieldEventLogActorType, ast.NodeTypeString)
	store.AddSymbol(FieldEventLogActorId, ast.NodeTypeString)
	store.AddSymbol(FieldEventLogEntityType, ast.NodeTypeString)
	store.AddSymbol(FieldEventLogEntityId, ast.NodeTypeString)
	store.AddSymbol(FieldEventLogFormattedMessage, ast.NodeTypeString)
	store.AddSymbol(FieldEventLogFormatString, ast.NodeTypeString)
}

func (store *eventLogStoreImpl) initializeLinked() {
	// no linked stores
}

func (store *eventLogStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*EventLog, error) {
	entity := &EventLog{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *eventLogStoreImpl) LoadOneByQuery(tx *bbolt.Tx, query string) (*EventLog, error) {
	entity := &EventLog{}
	if found, err := store.BaseLoadOneByQuery(tx, query, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}
