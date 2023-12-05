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
	"encoding/json"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/eid"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const (
	FieldConfigTypeSchema = "schema"
)

func newConfigType(name string) *ConfigType {
	return &ConfigType{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
	}
}

type ConfigType struct {
	boltz.BaseExtEntity
	Name   string                 `json:"name"`
	Schema map[string]interface{} `json:"schema"`
}

func (entity *ConfigType) GetName() string {
	return entity.Name
}

func (entity *ConfigType) GetEntityType() string {
	return EntityTypeConfigTypes
}

var _ ConfigTypeStore = (*configTypeStoreImpl)(nil)

type ConfigTypeStore interface {
	Store[*ConfigType]
	NameIndexed
	LoadOneByName(tx *bbolt.Tx, name string) (*ConfigType, error)
	GetName(tx *bbolt.Tx, id string) *string
}

func newConfigTypesStore(stores *stores) *configTypeStoreImpl {
	store := &configTypeStoreImpl{}
	store.baseStore = newBaseStore[*ConfigType](stores, store)
	store.InitImpl(store)
	return store
}

type configTypeStoreImpl struct {
	*baseStore[*ConfigType]

	indexName     boltz.ReadIndex
	symbolConfigs boltz.EntitySetSymbol
}

func (store *configTypeStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *configTypeStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()
	store.symbolConfigs = store.AddFkSetSymbol(EntityTypeConfigs, store.stores.config)
	store.AddSymbol(FieldConfigTypeSchema, ast.NodeTypeString)
}

func (store *configTypeStoreImpl) initializeLinked() {
}

func (store *configTypeStoreImpl) NewEntity() *ConfigType {
	return &ConfigType{}
}

func (store *configTypeStoreImpl) FillEntity(entity *ConfigType, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	marshalledSchema := bucket.GetString(FieldConfigTypeSchema)
	if marshalledSchema != nil {
		entity.Schema = map[string]interface{}{}
		bucket.SetError(json.Unmarshal([]byte(*marshalledSchema), &entity.Schema))
	}
}

func (store *configTypeStoreImpl) PersistEntity(entity *ConfigType, ctx *boltz.PersistContext) {
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
