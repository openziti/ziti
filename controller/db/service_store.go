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

package db

import (
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/controller/xt_smartrouting"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	EntityTypeServices             = "services"
	FieldServiceTerminatorStrategy = "terminatorStrategy"
)

type Service struct {
	boltz.BaseExtEntity
	Name               string
	TerminatorStrategy string
}

func (entity *Service) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(FieldName)
	entity.TerminatorStrategy = bucket.GetStringWithDefault(FieldServiceTerminatorStrategy, "")
}

func (entity *Service) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldName, entity.Name)

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

func (entity *Service) GetEntityType() string {
	return EntityTypeServices
}

type ServiceStore interface {
	store
	GetNameIndex() boltz.ReadIndex
	LoadOneById(tx *bbolt.Tx, id string) (*Service, error)
	LoadOneByName(tx *bbolt.Tx, name string) (*Service, error)
}

func newServiceStore(stores *stores) *serviceStoreImpl {
	notFoundErrorFactory := func(id string) error {
		return boltz.NewNotFoundError(boltz.GetSingularEntityType(EntityTypeServices), "id", id)
	}

	store := &serviceStoreImpl{
		baseStore: baseStore{
			stores:    stores,
			BaseStore: boltz.NewBaseStore(EntityTypeServices, notFoundErrorFactory, boltz.RootBucket),
		},
	}
	store.InitImpl(store)
	return store
}

type serviceStoreImpl struct {
	baseStore
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

func (store *serviceStoreImpl) NewStoreEntity() boltz.Entity {
	return &Service{}
}

func (store *serviceStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Service, error) {
	entity := &Service{}
	if found, err := store.BaseLoadOneById(tx, id, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *serviceStoreImpl) LoadOneByName(tx *bbolt.Tx, name string) (*Service, error) {
	id := store.indexName.Read(tx, []byte(name))
	if id != nil {
		return store.LoadOneById(tx, string(id))
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
		terminator, err := store.stores.terminator.LoadOneById(tx, tId)
		if err != nil {
			return nil, err
		}
		terminators = append(terminators, terminator)
	}
	return terminators, nil
}
