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

package boltz

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
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

type TestContext interface {
	GetDb() Db
	GetStoreForEntity(entity Entity) CrudStore
	NextTest(t *testing.T)
}

type BaseTestContext struct {
	require.Assertions
	t             testing.TB
	ReferenceTime time.Time
	Impl          TestContext
	dbFile        *os.File
}

func NewTestContext(t testing.TB) *BaseTestContext {
	return &BaseTestContext{
		Assertions:    *require.New(t),
		t:             t,
		ReferenceTime: time.Now(),
	}
}

func (ctx *BaseTestContext) NextTest(t *testing.T) {
	ctx.t = t
	ctx.Assertions = *require.New(t)
}

func (ctx *BaseTestContext) InitDbFile() {
	var err error
	ctx.dbFile, err = ioutil.TempFile("", "query-bolt-ctx-db")
	ctx.NoError(err)

	err = ctx.dbFile.Close()
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

func (ctx *BaseTestContext) GetDb() Db {
	return ctx.Impl.GetDb()
}

func (ctx *BaseTestContext) GetDbFile() *os.File {
	return ctx.dbFile
}

func (ctx *BaseTestContext) GetStoreForEntity(entity Entity) CrudStore {
	store := ctx.Impl.GetStoreForEntity(entity)
	ctx.NotNil(store, "no store found for entity of type: %v", entity.GetEntityType())
	return store
}

func (ctx *BaseTestContext) RequireDelete(entity Entity) {
	err := ctx.Delete(entity)
	ctx.NoError(err)
	ctx.ValidateDeleted(entity.GetId())
}

func (ctx *BaseTestContext) RequireReload(entity Entity) {
	ctx.NoError(ctx.Reload(entity))
}

func (ctx *BaseTestContext) Delete(entity Entity) error {
	return ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := NewMutateContext(tx)
		store := ctx.GetStoreForEntity(entity)
		return store.DeleteById(mutateContext, entity.GetId())
	})
}

func (ctx *BaseTestContext) Reload(entity Entity) error {
	return ctx.GetDb().View(func(tx *bbolt.Tx) error {
		store := ctx.GetStoreForEntity(entity)
		found, err := store.BaseLoadOneById(tx, entity.GetId(), entity)
		if !found {
			return errors.Errorf("Could not reload %v with id %v", store.GetEntityType(), entity.GetId())
		}
		return err
	})
}

func (ctx *BaseTestContext) ValidateDeleted(id string, ignorePaths ...string) {
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		return ValidateDeleted(tx, id, ignorePaths...)
	})
	ctx.NoError(err)
}

func (ctx *BaseTestContext) RequireCreate(entity Entity) {
	err := ctx.Create(entity)
	if err != nil {
		fmt.Printf("error: %+v\n", err)
	}
	ctx.NoError(err)
}

func (ctx *BaseTestContext) RequireUpdate(entity Entity) {
	ctx.NoError(ctx.Update(entity))
}

func (ctx *BaseTestContext) RequirePatch(entity Entity, checker FieldChecker) {
	ctx.NoError(ctx.Patch(entity, checker))
}

func (ctx *BaseTestContext) Create(entity Entity) error {
	return ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := NewMutateContext(tx)
		store := ctx.GetStoreForEntity(entity)
		return store.Create(mutateContext, entity)
	})
}

func (ctx *BaseTestContext) Update(entity Entity) error {
	return ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := NewMutateContext(tx)
		store := ctx.GetStoreForEntity(entity)
		return store.Update(mutateContext, entity, nil)
	})
}

func (ctx *BaseTestContext) Patch(entity Entity, checker FieldChecker) error {
	return ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := NewMutateContext(tx)
		store := ctx.GetStoreForEntity(entity)
		return store.Update(mutateContext, entity, checker)
	})
}

func (ctx *BaseTestContext) ValidateBaseline(entity ExtEntity, opts ...cmp.Option) {
	store := ctx.GetStoreForEntity(entity)
	loaded, ok := store.NewStoreEntity().(ExtEntity)
	ctx.True(ok, "store entity type does not implement Entity: %v", reflect.TypeOf(store.NewStoreEntity()))

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

	entity.SetCreatedAt(loaded.GetCreatedAt())
	entity.SetUpdatedAt(loaded.GetUpdatedAt())
	if entity.GetTags() == nil {
		entity.SetTags(map[string]interface{}{})
	}

	ctx.True(cmp.Equal(entity, loaded, opts...), cmp.Diff(entity, loaded))
}

func (ctx *BaseTestContext) ValidateUpdated(entity ExtEntity) {
	store := ctx.GetStoreForEntity(entity)
	loaded, ok := store.NewStoreEntity().(ExtEntity)
	ctx.True(ok, "store entity type does not implement Entity: %v", reflect.TypeOf(store.NewStoreEntity()))

	var found bool
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		found, err = store.BaseLoadOneById(tx, entity.GetId(), loaded)
		return err
	})
	ctx.NoError(err)
	ctx.Equal(true, found)

	now := time.Now()
	ctx.Equal(entity.GetId(), loaded.GetId())
	ctx.Equal(entity.GetEntityType(), loaded.GetEntityType())
	ctx.Equal(entity.GetCreatedAt(), loaded.GetCreatedAt())
	ctx.True(loaded.GetCreatedAt().Before(loaded.GetUpdatedAt()), "%v should be before %v", loaded.GetCreatedAt(), loaded.GetUpdatedAt())
	ctx.True(loaded.GetUpdatedAt().Equal(now) || loaded.GetUpdatedAt().Before(now))

	entity.SetCreatedAt(loaded.GetCreatedAt())
	entity.SetUpdatedAt(loaded.GetUpdatedAt())
	if entity.GetTags() == nil {
		entity.SetTags(map[string]interface{}{})
	}

	ctx.True(cmp.Equal(entity, loaded), cmp.Diff(entity, loaded))
}

func (ctx *BaseTestContext) GetRelatedIds(entity Entity, field string) []string {
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
