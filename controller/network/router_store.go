/*
	Copyright 2019 NetFoundry, Inc.

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

package network

import (
	"fmt"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

const (
	EntityTypeRouters = "routers"
	FieldFingerprint  = "fingerprint"
)

func (router *Router) GetId() string {
	return router.Id
}

func (router *Router) SetId(id string) {
	router.Id = id
}

func (router *Router) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	router.Fingerprint = bucket.GetStringOrError(FieldFingerprint)
}

func (router *Router) SetValues(ctx *boltz.PersistContext) {
	ctx.SetString(FieldFingerprint, router.Fingerprint)
}

func (router *Router) GetEntityType() string {
	return EntityTypeRouters
}

type RouterStore interface {
	boltz.CrudStore
	create(router *Router) error
	update(router *Router) error
	remove(id string) error
	all() ([]*Router, error)
	loadOneById(id string) (*Router, error)
	LoadOneById(tx *bbolt.Tx, id string) (*Router, error)
}

func NewRouterStore(db boltz.Db) RouterStore {
	notFoundErrorFactory := func(id string) error {
		return fmt.Errorf("missing router '%s'", id)
	}

	store := &routerStoreImpl{
		db:        db,
		BaseStore: boltz.NewBaseStore(nil, EntityTypeRouters, notFoundErrorFactory, boltz.RootBucket),
	}
	store.InitImpl(store)
	return store
}

type routerStoreImpl struct {
	db boltz.Db
	*boltz.BaseStore
}

func (store *routerStoreImpl) NewStoreEntity() boltz.BaseEntity {
	return &Router{}
}

func (store *routerStoreImpl) create(router *Router) error {
	return store.db.Update(func(tx *bbolt.Tx) error {
		return store.Create(boltz.NewMutateContext(tx), router)
	})
}

func (store *routerStoreImpl) update(router *Router) error {
	return store.db.Update(func(tx *bbolt.Tx) error {
		return store.Update(boltz.NewMutateContext(tx), router, nil)
	})
}

func (store *routerStoreImpl) remove(id string) error {
	return store.db.Update(func(tx *bbolt.Tx) error {
		return store.DeleteById(boltz.NewMutateContext(tx), id)
	})
}

func (store *routerStoreImpl) loadOneById(id string) (router *Router, err error) {
	err = store.db.View(func(tx *bbolt.Tx) error {
		router, err = store.LoadOneById(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	if router == nil {
		return nil, fmt.Errorf("missing router '%s'", id)
	}
	return
}

func (store *routerStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*Router, error) {
	router := &Router{}
	if found, err := store.BaseLoadOneById(tx, id, router); !found || err != nil {
		return nil, err
	}
	return router, nil
}

func (store *routerStoreImpl) all() ([]*Router, error) {
	routers := make([]*Router, 0)
	err := store.db.View(func(tx *bbolt.Tx) error {
		ids, _, err := store.QueryIds(tx, "true")
		if err != nil {
			return err
		}
		for _, id := range ids {
			router, err := store.LoadOneById(tx, string(id))
			if err != nil {
				return err
			}
			routers = append(routers, router)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return routers, nil
}