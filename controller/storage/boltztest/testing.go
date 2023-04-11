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

package boltztest

import (
	"fmt"
	"github.com/openziti/storage/boltz"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

type TestContext interface {
	GetDb() boltz.Db
	GetStoreForEntity(entity boltz.Entity) boltz.Store
	NextTest(t *testing.T)
	Require() *require.Assertions
	GetReferenceTime() time.Time
}

type StoreFunc func(entity boltz.Entity) boltz.Store

type BaseTestContext struct {
	require.Assertions
	t             testing.TB
	ReferenceTime time.Time
	dbFile        *os.File
	db            boltz.Db
	storeF        StoreFunc
}

func NewTestContext(t testing.TB, storeF StoreFunc) *BaseTestContext {
	return &BaseTestContext{
		Assertions:    *require.New(t),
		t:             t,
		ReferenceTime: time.Now(),
		storeF:        storeF,
	}
}

func (ctx *BaseTestContext) NextTest(t *testing.T) {
	ctx.t = t
	ctx.Assertions = *require.New(t)
}

func (ctx *BaseTestContext) Require() *require.Assertions {
	return &ctx.Assertions
}

func (ctx *BaseTestContext) GetReferenceTime() time.Time {
	return ctx.ReferenceTime
}

func (ctx *BaseTestContext) InitDb(openF func(name string) (boltz.Db, error)) {
	var err error
	ctx.dbFile, err = os.CreateTemp("", "query-bolt-ctx-db")
	ctx.NoError(err)

	err = ctx.dbFile.Close()
	ctx.NoError(err)

	ctx.db, err = openF(ctx.GetDbFile().Name())
	ctx.NoError(err)
}

func (ctx *BaseTestContext) Cleanup() {
	if ctx.GetDb() != nil {
		if err := ctx.GetDb().Close(); err != nil {
			fmt.Printf("error closing bolt db: %v", err)
		}
	}

	if ctx.dbFile != nil {
		if err := os.Remove(ctx.dbFile.Name()); err != nil {
			fmt.Printf("error deleting bolt db file: %v", err)
		}
	}
}

func (ctx *BaseTestContext) GetDb() boltz.Db {
	return ctx.db
}

func (ctx *BaseTestContext) GetDbFile() *os.File {
	return ctx.dbFile
}

func (ctx *BaseTestContext) GetStoreForEntity(entity boltz.Entity) boltz.Store {
	store := ctx.storeF(entity)
	ctx.NotNil(store, "no store found for entity of type: %v", entity.GetEntityType())
	return store
}

func RequireDelete[E boltz.Entity](ctx TestContext, entity E, ignorePaths ...string) {
	err := Delete[E](ctx, entity)
	ctx.Require().NoError(err)
	ValidateDeleted(ctx, entity.GetId(), ignorePaths...)
}

func RequireReload[E boltz.Entity](ctx TestContext, entity E) {
	ctx.Require().NoError(Reload(ctx, entity))
}

func Delete[E boltz.Entity](ctx TestContext, entity E) error {
	return ctx.GetDb().Update(nil, func(mutateContext boltz.MutateContext) error {
		store := GetStoreForEntity(ctx, entity)
		return store.DeleteById(mutateContext, entity.GetId())
	})
}

func GetStoreForEntity[E boltz.Entity](ctx TestContext, entity E) boltz.EntityStore[E] {
	store := ctx.GetStoreForEntity(entity)
	v, ok := store.(boltz.EntityStore[E])
	if !ok {
		panic(errors.Errorf("store for entity type %v is of wrong type %T", entity.GetEntityType(), store))
	}
	return v
}

func Reload[E boltz.Entity](ctx TestContext, entity E) error {
	return ctx.GetDb().View(func(tx *bbolt.Tx) error {
		store := GetStoreForEntity(ctx, entity)
		found, err := store.LoadEntity(tx, entity.GetId(), entity)
		if !found {
			return errors.Errorf("Could not reload %v with id %v", store.GetEntityType(), entity.GetId())
		}
		return err
	})
}

func ValidateDeleted(ctx TestContext, id string, ignorePaths ...string) {
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		return boltz.ValidateDeleted(tx, id, ignorePaths...)
	})
	ctx.Require().NoError(err)
}

func RequireCreate[E boltz.Entity](ctx TestContext, entity E) {
	err := Create(ctx, entity)
	if err != nil {
		fmt.Printf("error: %+v\n", err)
	}
	ctx.Require().NoError(err)
}

func RequireUpdate[E boltz.Entity](ctx TestContext, entity E) {
	ctx.Require().NoError(Update(ctx, entity))
}

func RequirePatch[E boltz.Entity](ctx TestContext, entity E, checker boltz.FieldChecker) {
	ctx.Require().NoError(Patch(ctx, entity, checker))
}

func Create[E boltz.Entity](ctx TestContext, entity E) error {
	return ctx.GetDb().Update(nil, func(mutateContext boltz.MutateContext) error {
		store := GetStoreForEntity(ctx, entity)
		return store.Create(mutateContext, entity)
	})
}

func Update[E boltz.Entity](ctx TestContext, entity E) error {
	return ctx.GetDb().Update(nil, func(mutateContext boltz.MutateContext) error {
		store := GetStoreForEntity(ctx, entity)
		return store.Update(mutateContext, entity, nil)
	})
}

func Patch[E boltz.Entity](ctx TestContext, entity E, checker boltz.FieldChecker) error {
	return ctx.GetDb().Update(nil, func(mutateContext boltz.MutateContext) error {
		store := GetStoreForEntity(ctx, entity)
		return store.Update(mutateContext, entity, checker)
	})
}

func ValidateBaseline[E boltz.ExtEntity](ctx TestContext, entity E, opts ...cmp.Option) {
	store := GetStoreForEntity(ctx, entity)
	var loaded boltz.ExtEntity
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		var found bool
		loaded, found, err = store.FindById(tx, entity.GetId())
		ctx.Require().NoError(err)
		ctx.Require().Equal(true, found)

		now := time.Now()
		ctx.Require().Equal(entity.GetId(), loaded.GetId())
		ctx.Require().Equal(entity.GetEntityType(), loaded.GetEntityType())
		ctx.Require().True(loaded.GetCreatedAt().Equal(loaded.GetUpdatedAt()))
		ctx.Require().True(loaded.GetCreatedAt().Equal(ctx.GetReferenceTime()) || loaded.GetCreatedAt().After(ctx.GetReferenceTime()))
		ctx.Require().True(loaded.GetCreatedAt().Equal(now) || loaded.GetCreatedAt().Before(now))

		return nil
	})
	ctx.Require().NoError(err)

	entity.SetCreatedAt(loaded.GetCreatedAt())
	entity.SetUpdatedAt(loaded.GetUpdatedAt())
	if entity.GetTags() == nil {
		entity.SetTags(map[string]interface{}{})
	}

	ctx.Require().True(cmp.Equal(entity, loaded, opts...), cmp.Diff(entity, loaded))
}

func ValidateUpdated[E boltz.ExtEntity](ctx TestContext, entity E) {
	store := GetStoreForEntity(ctx, entity)
	loaded := store.GetEntityStrategy().NewEntity()

	var found bool
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		found, err = store.LoadEntity(tx, entity.GetId(), loaded)
		return err
	})
	ctx.Require().NoError(err)
	ctx.Require().Equal(true, found)

	now := time.Now()
	ctx.Require().Equal(entity.GetId(), loaded.GetId())
	ctx.Require().Equal(entity.GetEntityType(), loaded.GetEntityType())
	ctx.Require().Equal(entity.GetCreatedAt(), loaded.GetCreatedAt())
	ctx.Require().True(loaded.GetCreatedAt().Before(loaded.GetUpdatedAt()), "%v should be before %v", loaded.GetCreatedAt(), loaded.GetUpdatedAt())
	ctx.Require().True(loaded.GetUpdatedAt().Equal(now) || loaded.GetUpdatedAt().Before(now))

	entity.SetCreatedAt(loaded.GetCreatedAt())
	entity.SetUpdatedAt(loaded.GetUpdatedAt())
	if entity.GetTags() == nil {
		entity.SetTags(map[string]interface{}{})
	}

	ctx.Require().True(cmp.Equal(entity, loaded), cmp.Diff(entity, loaded))
}

func (ctx *BaseTestContext) GetRelatedIds(entity boltz.Entity, field string) []string {
	var result []string
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		store := ctx.GetStoreForEntity(entity)
		result = store.GetRelatedEntitiesIdList(tx, entity.GetId(), field)
		return nil
	})
	ctx.NoError(err)
	return result
}

func (ctx *BaseTestContext) CreateTags() map[string]interface{} {
	return map[string]interface{}{
		"hello":             uuid.New().String(),
		uuid.New().String(): "hello",
		"count":             rand.Int63(),
		"enabled":           rand.Int()%2 == 0,
		uuid.New().String(): int32(27),
		"markerKey":         nil,
	}
}
