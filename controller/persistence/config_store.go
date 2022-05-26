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
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	FieldConfigData            = "data"
	FieldConfigType            = "type"
	FieldConfigIdentityService = "identityServices"
)

func newConfig(name string, configType string, data map[string]interface{}) *Config {
	return &Config{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
		Type:          configType,
		Data:          data,
	}
}

type Config struct {
	boltz.BaseExtEntity
	Name string
	Type string
	Data map[string]interface{}
}

func (entity *Config) GetName() string {
	return entity.Name
}

func (entity *Config) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.Type = bucket.GetStringOrError(FieldConfigType)
	entity.Data = bucket.GetMap(FieldConfigData)
}

func (entity *Config) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetString(FieldConfigType, entity.Type)
	ctx.SetMap(FieldConfigData, entity.Data)

	if ctx.ProceedWithSet(FieldConfigData) && entity.Data == nil {
		ctx.Bucket.SetError(errorz.NewFieldError("data is required", "data", nil))
	}
}

func (entity *Config) GetEntityType() string {
	return EntityTypeConfigs
}

type ConfigStore interface {
	NameIndexedStore
	LoadOneById(tx *bbolt.Tx, id string) (*Config, error)
	LoadOneByName(tx *bbolt.Tx, name string) (*Config, error)
}

func newConfigsStore(stores *stores) *configStoreImpl {
	store := &configStoreImpl{
		baseStore: newBaseStore(stores, EntityTypeConfigs),
	}
	store.InitImpl(store)
	return store
}

type configStoreImpl struct {
	*baseStore

	indexName              boltz.ReadIndex
	symbolType             boltz.EntitySymbol
	symbolServices         boltz.EntitySetSymbol
	symbolIdentityServices boltz.EntitySetSymbol
	identityServicesLinks  *boltz.LinkedSetSymbol
}

func (store *configStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *configStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.indexName = store.addUniqueNameField()
	store.symbolType = store.AddFkSymbol(FieldConfigType, store.stores.configType)
	store.AddMapSymbol(FieldConfigData, ast.NodeTypeAnyType, FieldConfigData)
	store.symbolServices = store.AddFkSetSymbol(db.EntityTypeServices, store.stores.edgeService)
	store.symbolIdentityServices = store.AddSetSymbol(FieldConfigIdentityService, ast.NodeTypeOther)
	store.identityServicesLinks = &boltz.LinkedSetSymbol{EntitySymbol: store.symbolIdentityServices}
}

func (store *configStoreImpl) initializeLinked() {
	store.AddFkIndex(store.symbolType, store.stores.configType.symbolConfigs)
	store.AddLinkCollection(store.symbolServices, store.stores.edgeService.symbolConfigs)
}

func (store *configStoreImpl) NewStoreEntity() boltz.Entity {
	return &Config{}
}

func (store *configStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Config, error) {
	entity := &Config{}
	if err := store.baseLoadOneById(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *configStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*Config, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
	}
	return nil, nil
}

func (store *configStoreImpl) Update(ctx boltz.MutateContext, entity boltz.Entity, checker boltz.FieldChecker) error {
	if err := store.createServiceChangeEvents(ctx.Tx(), entity.GetId()); err != nil {
		return err
	}
	return store.baseStore.Update(ctx, entity, checker)
}

func (store *configStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	if err := store.createServiceChangeEvents(ctx.Tx(), id); err != nil {
		return err
	}

	err := store.symbolIdentityServices.Map(ctx.Tx(), []byte(id), func(mapCtx *boltz.MapContext) {
		keys, err := boltz.DecodeStringSlice(mapCtx.Value())
		if err != nil {
			mapCtx.SetError(err)
			return
		}
		identityId := keys[0]
		serviceId := keys[1]
		err = store.stores.identity.removeServiceConfigs(ctx.Tx(), identityId, func(identityServiceId, _, configId string) bool {
			return identityServiceId == serviceId && configId == id
		})
		if err != nil {
			mapCtx.SetError(err)
			return
		}
	})
	if err != nil {
		return err
	}
	return store.baseStore.DeleteById(ctx, id)
}

func (store *configStoreImpl) createServiceChangeEvents(tx *bbolt.Tx, configId string) error {
	eh := &serviceEventHandler{}

	id := []byte(configId)
	err := store.symbolServices.Map(tx, id, func(ctx *boltz.MapContext) {
		eh.addServiceUpdatedEvent(store.baseStore, tx, ctx.Value())
	})

	if err != nil {
		return err
	}

	return store.symbolIdentityServices.Map(tx, id, func(mapCtx *boltz.MapContext) {
		keys, err := boltz.DecodeStringSlice(mapCtx.Value())
		if err != nil {
			mapCtx.SetError(err)
			return
		}
		identityId := keys[0]
		serviceId := keys[1]
		eh.addServiceEvent(tx, []byte(identityId), []byte(serviceId), ServiceUpdated)
	})
}
