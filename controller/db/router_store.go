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
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
)

import (
	"go.etcd.io/bbolt"
)

const (
	EntityTypeRouters      = "routers"
	FieldRouterFingerprint = "fingerprint"
)

type Router struct {
	boltz.BaseExtEntity
	Fingerprint *string
}

func (entity *Router) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Fingerprint = bucket.GetString(FieldRouterFingerprint)
}

func (entity *Router) SetValues(ctx *boltz.PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetStringP(FieldRouterFingerprint, entity.Fingerprint)
}

func (entity *Router) GetEntityType() string {
	return EntityTypeRouters
}

type RouterStore interface {
	boltz.CrudStore
	LoadOneById(tx *bbolt.Tx, id string) (*Router, error)
}

func newRouterStore(stores *stores) *routerStoreImpl {
	notFoundErrorFactory := func(id string) error {
		return boltz.NewNotFoundError(boltz.GetSingularEntityType(EntityTypeRouters), "id", id)
	}

	store := &routerStoreImpl{
		baseStore: baseStore{
			stores:    stores,
			BaseStore: boltz.NewBaseStore(EntityTypeRouters, notFoundErrorFactory, boltz.RootBucket),
		},
	}
	store.InitImpl(store)
	return store
}

type routerStoreImpl struct {
	baseStore
	terminatorsSymbol boltz.EntitySetSymbol
}

func (store *routerStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	store.AddSymbol(FieldRouterFingerprint, ast.NodeTypeString)
	store.terminatorsSymbol = store.AddFkSetSymbol(EntityTypeTerminators, store.stores.terminator)
}

func (store *routerStoreImpl) initializeLinked() {
}

func (store *routerStoreImpl) NewStoreEntity() boltz.Entity {
	return &Router{}
}

func (store *routerStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Router, error) {
	entity := &Router{}
	if found, err := store.BaseLoadOneById(tx, id, entity); !found || err != nil {
		return nil, err
	}
	return entity, nil
}

func (store *routerStoreImpl) DeleteById(ctx boltz.MutateContext, id string) error {
	terminatorIds := store.GetRelatedEntitiesIdList(ctx.Tx(), id, EntityTypeTerminators)
	for _, terminatorId := range terminatorIds {
		if err := store.stores.terminator.DeleteById(ctx, terminatorId); err != nil {
			return err
		}
	}
	return store.BaseStore.DeleteById(ctx, id)
}
