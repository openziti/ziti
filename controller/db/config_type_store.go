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
	"fmt"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/ast"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const (
	FieldConfigTypeSchema = "schema"
	FieldConfigTypeTarget = "target"

	// ConfigTypeTargetService indicates a config type used for service configuration.
	ConfigTypeTargetService = "service"
	// ConfigTypeTargetRouter indicates a config type used for router configuration.
	ConfigTypeTargetRouter = "router"
	// ConfigTypeTargetOther indicates a config type used for purposes other than service or router configuration.
	ConfigTypeTargetOther = "other"
)

// validConfigTypeTargets is the set of allowed values for ConfigType.Target.
var validConfigTypeTargets = map[string]struct{}{
	ConfigTypeTargetService: {},
	ConfigTypeTargetRouter:  {},
	ConfigTypeTargetOther:   {},
}

func newConfigType(name string) *ConfigType {
	return &ConfigType{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
		Target:        ConfigTypeTargetService,
	}
}

type ConfigType struct {
	boltz.BaseExtEntity
	Name   string                 `json:"name"`
	Schema map[string]interface{} `json:"schema"`
	Target string                 `json:"target"`
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
	store.AddSymbol(FieldConfigTypeTarget, ast.NodeTypeString)
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
	entity.Target = bucket.GetStringOrError(FieldConfigTypeTarget)
}

func (store *configTypeStoreImpl) PersistEntity(entity *ConfigType, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)

	if ctx.FieldChecker == nil || ctx.FieldChecker.IsUpdated(FieldConfigTypeSchema) {
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

	if ctx.FieldChecker == nil || ctx.FieldChecker.IsUpdated(FieldConfigTypeTarget) {
		if _, ok := validConfigTypeTargets[entity.Target]; !ok {
			ctx.Bucket.SetError(errorz.NewFieldError(
				fmt.Sprintf("invalid target %q, must be one of: service, router, other", entity.Target),
				FieldConfigTypeTarget, entity.Target))
			return
		}
		if !ctx.IsCreate {
			if existing := ctx.Bucket.GetString(FieldConfigTypeTarget); existing != nil && *existing != entity.Target {
				ctx.Bucket.SetError(errorz.NewFieldError(
					"target is immutable and cannot be changed after creation",
					FieldConfigTypeTarget, entity.Target))
				return
			}
		}
		ctx.SetString(FieldConfigTypeTarget, entity.Target)
	}
}

func (store *configTypeStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*ConfigType, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadById(tx, string(id))
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
