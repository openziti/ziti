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
	"context"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"reflect"
	"sync"
)

func NewStoreDefinition[E boltz.ExtEntity](strategy boltz.EntityStrategy[E]) boltz.StoreDefinition[E] {
	entityType := strategy.NewEntity().GetEntityType()
	return boltz.StoreDefinition[E]{
		EntityType:     entityType,
		EntityStrategy: strategy,
		BasePath:       []string{RootBucket},
		EntityNotFoundF: func(id string) error {
			return boltz.NewNotFoundError(boltz.GetSingularEntityType(entityType), "id", id)
		},
	}
}

type Stores struct {
	Terminator TerminatorStore
	Router     RouterStore
	Service    ServiceStore
	storeMap   map[string]boltz.Store
	lock       sync.Mutex
	checkables []boltz.Checkable
}

func (store *Stores) AddCheckable(checkable boltz.Checkable) {
	store.lock.Lock()
	defer store.lock.Unlock()
	store.checkables = append(store.checkables, checkable)
}

func (stores *Stores) buildStoreMap() {
	stores.storeMap = map[string]boltz.Store{}
	val := reflect.ValueOf(stores).Elem()
	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		if f.CanInterface() {
			if store, ok := f.Interface().(boltz.Store); ok {
				stores.storeMap[store.GetEntityType()] = store
				stores.AddCheckable(store)
			}
		}
	}
}

func (stores *Stores) GetStoreList() []boltz.Store {
	var result []boltz.Store
	for _, store := range stores.storeMap {
		result = append(result, store)
	}
	return result
}

func (stores *Stores) GetStoreForEntity(entity boltz.Entity) boltz.Store {
	return stores.storeMap[entity.GetEntityType()]
}

func (stores *Stores) GetStoreForEntityType(entityType string) boltz.Store {
	return stores.storeMap[entityType]
}

func (stores *Stores) CheckIntegrity(db boltz.Db, ctx context.Context, fix bool, errorHandler func(error, bool)) error {

	if fix {
		changeCtx := boltz.NewMutateContext(ctx)
		return db.Update(changeCtx, func(changeCtx boltz.MutateContext) error {
			return stores.CheckIntegrityInTx(db, changeCtx, fix, errorHandler)
		})
	}

	return db.View(func(tx *bbolt.Tx) error {
		changeCtx := boltz.NewTxMutateContext(ctx, tx)
		return stores.CheckIntegrityInTx(db, changeCtx, fix, errorHandler)
	})
}

func (stores *Stores) CheckIntegrityInTx(db boltz.Db, ctx boltz.MutateContext, fix bool, errorHandler func(error, bool)) error {
	if fix {
		pfxlog.Logger().Info("creating database snapshot before attempting to fix data integrity issues")
		if err := db.Snapshot(ctx.Tx()); err != nil {
			return err
		}
	}

	for _, checkable := range stores.checkables {
		if err := checkable.CheckIntegrity(ctx, fix, errorHandler); err != nil {
			return err
		}
	}
	return nil
}

type stores struct {
	terminator *terminatorStoreImpl
	router     *routerStoreImpl
	service    *serviceStoreImpl
}

func InitStores(db boltz.Db) (*Stores, error) {
	internalStores := &stores{}

	internalStores.terminator = newTerminatorStore(internalStores)
	internalStores.router = newRouterStore(internalStores)
	internalStores.service = newServiceStore(internalStores)

	stores := &Stores{
		Terminator: internalStores.terminator,
		Router:     internalStores.router,
		Service:    internalStores.service,
	}

	stores.buildStoreMap()

	internalStores.terminator.initializeLocal()
	internalStores.router.initializeLocal()
	internalStores.service.initializeLocal()

	internalStores.terminator.initializeLinked()
	internalStores.router.initializeLinked()
	internalStores.service.initializeLinked()

	mm := boltz.NewMigratorManager(db)
	if err := mm.Migrate("fabric", CurrentDbVersion, internalStores.migrate); err != nil {
		return nil, err
	}

	return stores, nil
}
