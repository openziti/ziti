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
	"fmt"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	EntityTypeServices             = "services"
	FieldServiceTerminatorStrategy = "terminatorStrategy"
)

type Service struct {
	boltz.BaseExtEntity
	TerminatorStrategy string
}

func (entity *Service) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.TerminatorStrategy = bucket.GetStringWithDefault(FieldServiceTerminatorStrategy, "")
}

func (entity *Service) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(FieldServiceTerminatorStrategy, entity.TerminatorStrategy)
}

func (entity *Service) GetEntityType() string {
	return EntityTypeServices
}

type ServiceStore interface {
	store
	LoadOneById(tx *bbolt.Tx, id string) (*Service, error)
}

func newServiceStore(stores *stores) *serviceStoreImpl {
	notFoundErrorFactory := func(id string) error {
		return fmt.Errorf("missing service '%s'", id)
	}

	store := &serviceStoreImpl{
		baseStore: baseStore{
			stores:    stores,
			BaseStore: boltz.NewBaseStore(nil, EntityTypeServices, notFoundErrorFactory, boltz.RootBucket),
		},
	}
	store.InitImpl(store)
	store.AddSymbol(FieldServiceTerminatorStrategy, ast.NodeTypeString)
	store.terminatorsSymbol = store.AddFkSetSymbol(EntityTypeTerminators, stores.terminator)
	return store
}

type serviceStoreImpl struct {
	baseStore
	terminatorsSymbol boltz.EntitySetSymbol
}

func (store *serviceStoreImpl) initializeLinked() {
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

func (store *serviceStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	terminatorIds := store.GetRelatedEntitiesIdList(ctx.Tx(), id, EntityTypeTerminators)
	for _, terminatorId := range terminatorIds {
		if err := store.stores.terminator.DeleteById(ctx, terminatorId); err != nil {
			return err
		}
	}
	return store.BaseStore.DeleteById(ctx, id)
}
