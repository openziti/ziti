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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"reflect"
	"sync"
)

type Stores struct {
	Terminator TerminatorStore
	Router     RouterStore
	Service    ServiceStore
	storeMap   map[string]boltz.CrudStore
	lock       sync.Mutex
	checkables []Checkable
}

type Checkable interface {
	CheckIntegrity(tx *bbolt.Tx, fix bool, errorSink func(err error, fixed bool)) error
}

func (store *Stores) AddCheckable(checkable Checkable) {
	store.lock.Lock()
	defer store.lock.Unlock()
	store.checkables = append(store.checkables, checkable)
}

func (stores *Stores) buildStoreMap() {
	stores.storeMap = map[string]boltz.CrudStore{}
	val := reflect.ValueOf(stores).Elem()
	for i := 0; i < val.NumField(); i++ {
		f := val.Field(i)
		if f.CanInterface() {
			if store, ok := f.Interface().(boltz.CrudStore); ok {
				stores.storeMap[store.GetEntityType()] = store
				stores.AddCheckable(store)
			}
		}
	}
}

func (stores *Stores) GetStoreList() []boltz.CrudStore {
	var result []boltz.CrudStore
	for _, store := range stores.storeMap {
		result = append(result, store)
	}
	return result
}

func (stores *Stores) GetStoreForEntity(entity boltz.Entity) boltz.CrudStore {
	return stores.storeMap[entity.GetEntityType()]
}

func (stores *Stores) GetStoreForEntityType(entityType string) boltz.CrudStore {
	return stores.storeMap[entityType]
}

func (stores *Stores) CheckIntegrity(db boltz.Db, fix bool, errorHandler func(error, bool)) error {
	if fix {
		return db.Update(func(tx *bbolt.Tx) error {
			return stores.CheckIntegrityInTx(db, tx, fix, errorHandler)
		})
	}

	return db.View(func(tx *bbolt.Tx) error {
		return stores.CheckIntegrityInTx(db, tx, fix, errorHandler)
	})
}

func (stores *Stores) CheckIntegrityInTx(db boltz.Db, tx *bbolt.Tx, fix bool, errorHandler func(error, bool)) error {
	if fix {
		pfxlog.Logger().Info("creating database snapshot before attempting to fix data integrity issues")
		if err := db.Snapshot(tx); err != nil {
			return err
		}
	}

	for _, checkable := range stores.checkables {
		if err := checkable.CheckIntegrity(tx, fix, errorHandler); err != nil {
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
