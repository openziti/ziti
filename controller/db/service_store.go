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
	"github.com/openziti/ziti/controller/xt"
	"github.com/openziti/ziti/controller/xt_smartrouting"
	"go.etcd.io/bbolt"
	"time"
)

const (
	EntityTypeServices             = "services"
	FieldServiceTerminatorStrategy = "terminatorStrategy"
	FieldServiceMaxIdleTime        = "maxIdleTime"
)

type Service struct {
	boltz.BaseExtEntity
	Name               string        `json:"name"`
	MaxIdleTime        time.Duration `json:"maxIdleTime"`
	TerminatorStrategy string        `json:"terminatorStrategy"`
}

func (entity *Service) GetEntityType() string {
	return EntityTypeServices
}

func (entity *Service) GetName() string {
	return entity.Name
}

type ServiceStore interface {
	boltz.EntityStore[*Service]
	boltz.EntityStrategy[*Service]
	GetNameIndex() boltz.ReadIndex
	FindByName(tx *bbolt.Tx, name string) (*Service, error)
}

func newServiceStore(stores *stores) *serviceStoreImpl {
	store := &serviceStoreImpl{}
	store.baseStore = baseStore[*Service]{
		stores:    stores,
		BaseStore: boltz.NewBaseStore(NewStoreDefinition[*Service](store)),
	}
	store.InitImpl(store)
	return store
}

type serviceStoreImpl struct {
	baseStore[*Service]
	indexName         boltz.ReadIndex
	terminatorsSymbol boltz.EntitySetSymbol
}

func (store *serviceStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()

	symbolName := store.AddSymbol(FieldName, ast.NodeTypeString)
	store.indexName = store.AddUniqueIndex(symbolName)

	store.AddSymbol(FieldServiceTerminatorStrategy, ast.NodeTypeString)
	store.terminatorsSymbol = store.AddFkSetSymbol(EntityTypeTerminators, store.stores.terminator)
}

func (store *serviceStoreImpl) initializeLinked() {
}

func (store *serviceStoreImpl) GetNameIndex() boltz.ReadIndex {
	return store.indexName
}

func (store *serviceStoreImpl) NewEntity() *Service {
	return &Service{}
}

func (store *serviceStoreImpl) FillEntity(entity *Service, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.TerminatorStrategy = bucket.GetStringWithDefault(FieldServiceTerminatorStrategy, "")
	entity.MaxIdleTime = time.Duration(bucket.GetInt64WithDefault(FieldServiceMaxIdleTime, 0))
}

func (store *serviceStoreImpl) PersistEntity(entity *Service, ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)
	ctx.SetInt64(FieldServiceMaxIdleTime, int64(entity.MaxIdleTime))

	if entity.TerminatorStrategy == "" {
		entity.TerminatorStrategy = xt_smartrouting.Name
	}
	_, changed := ctx.GetAndSetString(FieldServiceTerminatorStrategy, entity.TerminatorStrategy)
	if changed {
		strategy, err := xt.GlobalRegistry().GetStrategy(entity.TerminatorStrategy)
		if err != nil {
			ctx.Bucket.SetError(err)
			return
		}

		if !ctx.IsCreate {
			serviceStore := ctx.Store.(*serviceStoreImpl)
			terminators, err := serviceStore.getTerminators(ctx.Bucket.Tx(), entity.Id)
			if !ctx.Bucket.SetError(err) {
				event := xt.NewStrategyChangeEvent(entity.Id, nil, terminators, nil, nil)
				ctx.Bucket.SetError(strategy.HandleTerminatorChange(event))
			}
		}
	}
}

func (store *serviceStoreImpl) FindByName(tx *bbolt.Tx, name string) (*Service, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		entity, _, err := store.FindById(tx, string(id))
		return entity, err
	}
	return nil, nil
}

func (store *serviceStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	terminatorIds := store.GetRelatedEntitiesIdList(ctx.Tx(), id, EntityTypeTerminators)
	for _, terminatorId := range terminatorIds {
		if err := store.stores.terminator.DeleteById(ctx, terminatorId); err != nil {
			return err
		}
	}
	return store.BaseStore.DeleteById(ctx, id)
}

func (store *serviceStoreImpl) getTerminators(tx *bbolt.Tx, serviceId string) ([]xt.Terminator, error) {
	var terminators []xt.Terminator
	for _, tId := range store.GetRelatedEntitiesIdList(tx, serviceId, EntityTypeTerminators) {
		terminator, _, err := store.stores.terminator.FindById(tx, tId)
		if err != nil {
			return nil, err
		}
		if terminator != nil {
			terminators = append(terminators, terminator)
		}
	}
	return terminators, nil
}
