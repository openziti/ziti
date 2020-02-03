/*
	Copyright 2020 Netfoundry, Inc.

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
	"encoding/json"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const (
	FieldConfigTypeSchema = "schema"
)

func newConfigType(name string) *ConfigType {
	return &ConfigType{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: uuid.New().String()},
		Name:               name,
	}
}

type ConfigType struct {
	BaseEdgeEntityImpl
	Name   string
	Schema map[string]interface{}
}

func (entity *ConfigType) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	marshalledSchema := bucket.GetString(FieldConfigTypeSchema)
	if marshalledSchema != nil {
		entity.Schema = map[string]interface{}{}
		bucket.SetError(json.Unmarshal([]byte(*marshalledSchema), &entity.Schema))
	}
}

func (entity *ConfigType) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)

	if len(entity.Schema) > 0 {
		marshalled, err := json.Marshal(entity.Schema)
		if err != nil {
			ctx.Bucket.SetError(err)
			return
		}
		ctx.SetString(FieldConfigTypeSchema, string(marshalled))
	} else {
		ctx.SetStringP(FieldConfigTypeSchema, nil)
	}
}

func (entity *ConfigType) GetEntityType() string {
	return EntityTypeConfigTypes
}

type ConfigTypeStore interface {
	Store
	LoadOneById(tx *bbolt.Tx, id string) (*ConfigType, error)
	LoadOneByName(tx *bbolt.Tx, name string) (*ConfigType, error)
	GetNameIndex() boltz.ReadIndex
	GetName(tx *bbolt.Tx, id string) *string
}

func newConfigTypesStore(stores *stores) *configTypeStoreImpl {
	store := &configTypeStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeConfigTypes),
	}
	store.InitImpl(store)
	return store
}

type configTypeStoreImpl struct {
	*baseStore

	indexName     boltz.ReadIndex
	symbolConfigs boltz.EntitySetSymbol
}

func (store *configTypeStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *configTypeStoreImpl) initializeLocal() {
	store.addBaseFields()
	store.indexName = store.addUniqueNameField()
	store.symbolConfigs = store.AddFkSetSymbol(EntityTypeConfigs, store.stores.config)
	store.AddSymbol(FieldConfigTypeSchema, ast.NodeTypeString)
}

func (store *configTypeStoreImpl) initializeLinked() {
}

func (store *configTypeStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &ConfigType{}
}

func (store *configTypeStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*ConfigType, error) {
	entity := &ConfigType{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *configTypeStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*ConfigType, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *configTypeStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	if bucket := store.GetEntityBucket(ctx.Tx(), []byte(id)); bucket != nil {
		if !bucket.IsStringListEmpty(EntityTypeConfigs) {
			return errors.Errorf("cannot delete config type %v, as configs of that type exist", id)
		}
	}

	return store.BaseStore.DeleteById(ctx, id)
}
