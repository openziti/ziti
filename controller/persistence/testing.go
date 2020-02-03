/*
	Copyright 2019 Netfoundry, Inc.

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
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

type TestContext struct {
	require.Assertions
	t             *testing.T
	dbFile        *os.File
	db            *db.Db
	serviceStore  network.ServiceStore
	routerStore   network.RouterStore
	stores        *Stores
	ReferenceTime time.Time
}

func NewTestContext(t *testing.T) *TestContext {
	return &TestContext{
		Assertions:    *require.New(t),
		t:             t,
		dbFile:        nil,
		db:            nil,
		serviceStore:  nil,
		routerStore:   nil,
		stores:        nil,
		ReferenceTime: time.Now(),
	}
}

func (ctx *TestContext) GetDb() boltz.Db {
	return ctx.db
}

func (ctx *TestContext) GetServiceStore() network.ServiceStore {
	return ctx.serviceStore
}

func (ctx *TestContext) GetServiceCache() network.Cache {
	return ctx
}

func (ctx *TestContext) GetRouterStore() network.RouterStore {
	return ctx.routerStore
}

func (ctx *TestContext) GetStores() *Stores {
	return ctx.stores
}

func (ctx *TestContext) RemoveFromCache(_ string) {
}

func (ctx *TestContext) Init() {
	var err error
	ctx.dbFile, err = ioutil.TempFile("", "query-bolt-ctx-db")
	ctx.NoError(err)

	err = ctx.dbFile.Close()
	ctx.NoError(err)

	ctx.db, err = db.Open(ctx.dbFile.Name())
	ctx.NoError(err)

	ctx.serviceStore = network.NewServiceStore(ctx.db)
	ctx.routerStore = network.NewRouterStore(ctx.db)
	ctx.stores, err = NewBoltStores(ctx)
	ctx.NoError(err)

	ctx.NoError(RunMigrations(ctx, ctx.stores, nil))
}

func (ctx *TestContext) Cleanup() {
	if ctx.db != nil {
		if err := ctx.db.Close(); err != nil {
			fmt.Printf("error closing bolt db: %v", err)
		}
	}

	if ctx.dbFile != nil {
		if err := os.Remove(ctx.dbFile.Name()); err != nil {
			fmt.Printf("error deleting bolt db file: %v", err)
		}
	}
}

func (ctx *TestContext) requireNewServicePolicy(policyType int32, identityRoles []string, serviceRoles []string) *ServicePolicy {
	entity := &ServicePolicy{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: uuid.New().String()},
		Name:               uuid.New().String(),
		PolicyType:         policyType,
		IdentityRoles:      identityRoles,
		ServiceRoles:       serviceRoles,
	}
	ctx.requireCreate(entity)
	return entity
}

func (ctx *TestContext) requireNewIdentity(name string, isAdmin bool) *Identity {
	identity := &Identity{
		BaseEdgeEntityImpl: *NewBaseEdgeEntity(uuid.New().String(), nil),
		Name:               name,
		IsAdmin:            isAdmin,
	}
	ctx.requireCreate(identity)
	return identity
}

func (ctx *TestContext) requireNewService(name string) *EdgeService {
	edgeService := &EdgeService{
		Service: network.Service{
			Id:              uuid.New().String(),
			Binding:         "edge",
			EndpointAddress: "hosted:unclaimed",
			Egress:          "unclaimed",
		},
		Name: name,
	}
	ctx.requireCreate(edgeService)
	return edgeService
}

func (ctx *TestContext) requireDelete(entity boltz.BaseEntity) {
	err := ctx.delete(entity)
	ctx.NoError(err)
	ctx.validateDeleted(entity.GetId())
}

func (ctx *TestContext) requireReload(entity boltz.BaseEntity) {
	ctx.NoError(ctx.reload(entity))
}

func (ctx *TestContext) delete(entity boltz.BaseEntity) error {
	return ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := boltz.NewMutateContext(tx)
		store := ctx.stores.getStoreForEntity(entity)
		if store == nil {
			return errors.Errorf("no store for entity of type '%v'", entity.GetEntityType())
		}
		return store.DeleteById(mutateContext, entity.GetId())
	})
}

func (ctx *TestContext) reload(entity boltz.BaseEntity) error {
	return ctx.GetDb().View(func(tx *bbolt.Tx) error {
		store := ctx.stores.getStoreForEntity(entity)
		if store == nil {
			return errors.Errorf("no store for entity of type '%v'", entity.GetEntityType())
		}
		found, err := store.BaseLoadOneById(tx, entity.GetId(), entity)
		if !found {
			return errors.Errorf("Could not reload %v with id %v", store.GetSingularEntityType(), entity.GetId())
		}
		return err
	})
}

func (ctx *TestContext) validateDeleted(id string) {
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		return boltz.ValidateDeleted(tx, id)
	})
	ctx.NoError(err)
}

func (ctx *TestContext) requireCreate(entity boltz.BaseEntity) {
	err := ctx.create(entity)
	if err != nil {
		fmt.Printf("error: %+v\n", err)
	}
	ctx.NoError(err)
}

func (ctx *TestContext) requireUpdate(entity boltz.BaseEntity) {
	ctx.NoError(ctx.update(entity))
}

func (ctx *TestContext) create(entity boltz.BaseEntity) error {
	return ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := boltz.NewMutateContext(tx)
		store, err := ctx.getStoreForEntity(entity)
		if err != nil {
			return err
		}
		return store.Create(mutateContext, entity)
	})
}

func (ctx *TestContext) update(entity boltz.BaseEntity) error {
	return ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := boltz.NewMutateContext(tx)
		store, err := ctx.getStoreForEntity(entity)
		if err != nil {
			return err
		}
		return store.Update(mutateContext, entity, nil)
	})
}

func (ctx *TestContext) getStoreForEntity(entity boltz.BaseEntity) (boltz.CrudStore, error) {
	var store boltz.CrudStore

	if _, ok := entity.(*network.Service); ok {
		store = ctx.GetServiceStore()
	} else if _, ok := entity.(*network.Router); ok {
		store = ctx.GetRouterStore()
	} else {
		store = ctx.stores.getStoreForEntity(entity)
	}
	if store != nil {
		return store, nil
	}

	return nil, errors.Errorf("no store for entity of type '%v'", entity.GetEntityType())
}

func (ctx *TestContext) validateBaseline(entity BaseEdgeEntity) {
	store := ctx.stores.getStoreForEntity(entity)
	ctx.NotNil(store, "no store for entity of type '%v'", entity.GetEntityType())

	loaded, ok := store.NewStoreEntity().(BaseEdgeEntity)
	ctx.True(ok, "store entity type does not implement BaseEntity: %v", reflect.TypeOf(store.NewStoreEntity()))

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		found, err := store.BaseLoadOneById(tx, entity.GetId(), loaded)
		ctx.NoError(err)
		ctx.Equal(true, found)

		now := time.Now()
		ctx.Equal(entity.GetId(), loaded.GetId())
		ctx.Equal(entity.GetEntityType(), loaded.GetEntityType())
		ctx.True(loaded.GetCreatedAt().Equal(loaded.GetUpdatedAt()))
		ctx.True(loaded.GetCreatedAt().Equal(ctx.ReferenceTime) || loaded.GetCreatedAt().After(ctx.ReferenceTime))
		ctx.True(loaded.GetCreatedAt().Equal(now) || loaded.GetCreatedAt().Before(now))

		return nil
	})
	ctx.NoError(err)

	entity.setCreateAt(loaded.GetCreatedAt())
	entity.setUpdatedAt(loaded.GetUpdatedAt())
	if entity.GetTags() == nil {
		entity.setTags(map[string]interface{}{})
	}

	ctx.True(cmp.Equal(entity, loaded), cmp.Diff(entity, loaded))
}

func (ctx *TestContext) validateUpdated(entity BaseEdgeEntity) {
	store := ctx.stores.getStoreForEntity(entity)
	ctx.NotNil(store, "no store for entity of type '%v'", entity.GetEntityType())

	loaded, ok := store.NewStoreEntity().(BaseEdgeEntity)
	ctx.True(ok, "store entity type does not implement BaseEntity: %v", reflect.TypeOf(store.NewStoreEntity()))

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		found, err := store.BaseLoadOneById(tx, entity.GetId(), loaded)
		ctx.NoError(err)
		ctx.Equal(true, found)

		now := time.Now()
		ctx.Equal(entity.GetId(), loaded.GetId())
		ctx.Equal(entity.GetEntityType(), loaded.GetEntityType())
		ctx.Equal(entity.GetCreatedAt(), loaded.GetCreatedAt())
		ctx.True(loaded.GetCreatedAt().Before(loaded.GetUpdatedAt()))
		ctx.True(loaded.GetUpdatedAt().Equal(now) || loaded.GetUpdatedAt().Before(now))

		return nil
	})
	ctx.NoError(err)

	entity.setCreateAt(loaded.GetCreatedAt())
	entity.setUpdatedAt(loaded.GetUpdatedAt())
	if entity.GetTags() == nil {
		entity.setTags(map[string]interface{}{})
	}

	ctx.True(cmp.Equal(entity, loaded), cmp.Diff(entity, loaded))
}

func (ctx *TestContext) getRelatedIds(entity boltz.BaseEntity, field string) []string {
	var result []string
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		store := ctx.stores.getStoreForEntity(entity)
		if store == nil {
			return errors.Errorf("no store for entity of type '%v'", entity.GetEntityType())
		}
		result = store.GetRelatedEntitiesIdList(tx, entity.GetId(), field)
		return nil
	})
	ctx.NoError(err)
	return result
}

func (ctx *TestContext) createTags() map[string]interface{} {
	return map[string]interface{}{
		"hello":             uuid.New().String(),
		uuid.New().String(): "hello",
		"count":             rand.Int63(),
		"enabled":           rand.Int()%2 == 0,
		uuid.New().String(): int32(27),
		"markerKey":         nil,
	}
}

func (ctx *TestContext) cleanupAll() {
	stores := []boltz.CrudStore{
		ctx.stores.Session,
		ctx.stores.ApiSession,
		ctx.stores.EdgeRouterPolicy,
		ctx.stores.Appwan,
		ctx.GetServiceStore(),
		ctx.stores.EdgeService,
		ctx.stores.Identity,
		ctx.stores.EdgeRouter,
		ctx.stores.Cluster,
		ctx.stores.Config,
	}
	_ = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := boltz.NewMutateContext(tx)
		for _, store := range stores {
			if err := store.DeleteWhere(mutateContext, `true limit none`); err != nil {
				pfxlog.Logger().WithError(err).Errorf("failure while cleaning up %v", store.GetEntityType())
				return err
			}
		}
		return nil
	})
}

func (ctx *TestContext) getIdentityTypeId() string {
	var result string
	err := ctx.db.View(func(tx *bbolt.Tx) error {
		ids, _, err := ctx.stores.IdentityType.QueryIds(tx, "true")
		if err != nil {
			return err
		}
		result = ids[0]
		return nil
	})
	ctx.NoError(err)
	return result
}

func ss(vals ...string) []string {
	return vals
}
