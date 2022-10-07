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
	"github.com/google/uuid"
	"github.com/openziti/storage/ast"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"os"
	"testing"
)

const (
	entityTypeFoo = "foos"
)

type foo struct {
	BaseExtEntity
	Name string
}

func (entity *foo) LoadValues(_ CrudStore, bucket *TypedBucket) {
	entity.LoadBaseValues(bucket)
	entity.Name = bucket.GetStringOrError(fieldName)
}

func (entity *foo) SetValues(ctx *PersistContext) {
	entity.SetBaseValues(ctx)
	ctx.SetString(fieldName, entity.Name)
}

func (entity *foo) GetEntityType() string {
	return entityTypeFoo
}

func newFooStore() *fooStoreImpl {
	store := &fooStoreImpl{
		BaseStore: NewBaseStore(entityTypeFoo, func(id string) error {
			return errors.Errorf("entity of type %v with id %v not found", entityTypeFoo, id)
		}, "stores"),
	}
	store.InitImpl(store)
	return store
}

type fooStoreImpl struct {
	*BaseStore
}

func (store *fooStoreImpl) NewStoreEntity() Entity {
	return &foo{}
}

func (store *fooStoreImpl) initializeLocal() {
	store.AddExtEntitySymbols()
	symbolName := store.AddSymbol(fieldName, ast.NodeTypeString)
	store.AddUniqueIndex(symbolName)
	store.AddConstraint(NewSystemEntityEnforcementConstraint(store))
}

func (store *fooStoreImpl) LoadOneById(tx *bbolt.Tx, id string) (*foo, error) {
	entity := &foo{}
	if found, err := store.BaseLoadOneById(tx, id, entity); !found || err != nil {
		return nil, err
	}

	return entity, nil
}

type systemEntitiesTest struct {
	dbTest
	fooStore *fooStoreImpl
}

func (test *systemEntitiesTest) init() {
	test.dbTest.init()
	var err error
	test.dbFile, err = os.CreateTemp("", "query-bolt-test-db")
	test.NoError(err)
	test.NoError(test.dbFile.Close())
	test.db, err = bbolt.Open(test.dbFile.Name(), 0, bbolt.DefaultOptions)
	test.NoError(err)

	test.fooStore = newFooStore()
	test.fooStore.initializeLocal()

	err = test.db.Update(func(tx *bbolt.Tx) error {
		test.fooStore.InitializeIndexes(tx, test)
		return nil
	})
	test.NoError(err)
}

func (test *systemEntitiesTest) newFoo() *foo {
	return &foo{
		BaseExtEntity: BaseExtEntity{
			Id: uuid.NewString(),
		},
		Name: uuid.NewString(),
	}
}

func TestSystemEntities_NonSystem(t *testing.T) {
	test := &systemEntitiesTest{}
	test.Assertions = require.New(t)
	test.init()
	defer test.cleanup()

	store := test.fooStore

	foo := test.newFoo()
	tx, err := test.db.Begin(true)
	test.NoError(err)

	defer func() {
		err = tx.Commit()
		test.NoError(err)
	}()

	ctx := NewMutateContext(tx)
	test.NoError(store.Create(ctx, foo))

	fooCheck, err := store.LoadOneById(tx, foo.Id)
	test.NoError(err)
	test.Equal(foo.Id, fooCheck.Id)
	test.Equal(foo.Name, fooCheck.Name)
	test.Equal(foo.IsSystem, fooCheck.IsSystem)

	foo.Name = uuid.NewString()
	test.NoError(store.Update(ctx, foo, nil))

	fooCheck, err = store.LoadOneById(tx, foo.Id)
	test.NoError(err)
	test.Equal(foo.Id, fooCheck.Id)
	test.Equal(foo.Name, fooCheck.Name)
	test.Equal(foo.IsSystem, fooCheck.IsSystem)

	test.NoError(store.DeleteById(ctx, foo.Id))
}

func TestSystemEntities_System(t *testing.T) {
	test := &systemEntitiesTest{}
	test.Assertions = require.New(t)
	test.init()
	defer test.cleanup()

	store := test.fooStore

	foo := test.newFoo()
	foo.IsSystem = true

	tx, err := test.db.Begin(true)
	test.NoError(err)

	defer func() {
		err = tx.Commit()
		test.NoError(err)
	}()

	ctx := NewMutateContext(tx).GetSystemContext()
	test.NoError(store.Create(ctx, foo))

	fooCheck, err := store.LoadOneById(tx, foo.Id)
	test.NoError(err)
	test.Equal(foo.Id, fooCheck.Id)
	test.Equal(foo.Name, fooCheck.Name)
	test.Equal(foo.IsSystem, fooCheck.IsSystem)

	foo.Name = uuid.NewString()
	test.NoError(store.Update(ctx, foo, nil))

	fooCheck, err = store.LoadOneById(tx, foo.Id)
	test.NoError(err)
	test.Equal(foo.Id, fooCheck.Id)
	test.Equal(foo.Name, fooCheck.Name)
	test.Equal(foo.IsSystem, fooCheck.IsSystem)

	test.NoError(store.DeleteById(ctx, foo.Id))
}

func TestSystemEntities_NonSystemCreateSystem(t *testing.T) {
	test := &systemEntitiesTest{}
	test.Assertions = require.New(t)
	test.init()
	defer test.cleanup()

	store := test.fooStore

	foo := test.newFoo()
	foo.IsSystem = true

	tx, err := test.db.Begin(true)
	test.NoError(err)

	defer func() {
		err = tx.Commit()
		test.NoError(err)
	}()

	ctx := NewMutateContext(tx)
	err = store.Create(ctx, foo)
	test.EqualError(err, fmt.Sprintf("cannot create system foo in a non-system context (id=%v)", foo.Id))
}

func TestSystemEntities_NonSystemUpdateSystem(t *testing.T) {
	test := &systemEntitiesTest{}
	test.Assertions = require.New(t)
	test.init()
	defer test.cleanup()

	store := test.fooStore

	foo := test.newFoo()
	foo.IsSystem = true

	tx, err := test.db.Begin(true)
	test.NoError(err)

	defer func() {
		err = tx.Commit()
		test.NoError(err)
	}()

	ctx := NewMutateContext(tx).GetSystemContext()
	test.NoError(store.Create(ctx, foo))

	fooCheck, err := store.LoadOneById(tx, foo.Id)
	test.NoError(err)
	test.Equal(foo.Id, fooCheck.Id)
	test.Equal(foo.Name, fooCheck.Name)
	test.Equal(foo.IsSystem, fooCheck.IsSystem)

	foo.Name = uuid.NewString()
	err = store.Update(NewMutateContext(tx), foo, nil)
	test.EqualError(err, fmt.Sprintf("ENTITY_CAN_NOT_BE_UPDATED: The entity requested for update can not be updated: cannot update system foo in a non-system context (id=%v)", foo.Id))
}

func TestSystemEntities_NonSystemDeleteSystem(t *testing.T) {
	test := &systemEntitiesTest{}
	test.Assertions = require.New(t)
	test.init()
	defer test.cleanup()

	store := test.fooStore

	foo := test.newFoo()
	foo.IsSystem = true

	tx, err := test.db.Begin(true)
	test.NoError(err)

	defer func() {
		err = tx.Commit()
		test.NoError(err)
	}()

	ctx := NewMutateContext(tx).GetSystemContext()
	test.NoError(store.Create(ctx, foo))

	fooCheck, err := store.LoadOneById(tx, foo.Id)
	test.NoError(err)
	test.Equal(foo.Id, fooCheck.Id)
	test.Equal(foo.Name, fooCheck.Name)
	test.Equal(foo.IsSystem, fooCheck.IsSystem)

	foo.Name = uuid.NewString()
	test.NoError(store.Update(ctx, foo, nil))

	fooCheck, err = store.LoadOneById(tx, foo.Id)
	test.NoError(err)
	test.Equal(foo.Id, fooCheck.Id)
	test.Equal(foo.Name, fooCheck.Name)
	test.Equal(foo.IsSystem, fooCheck.IsSystem)

	err = store.DeleteById(NewMutateContext(tx), foo.Id)
	test.EqualError(err, fmt.Sprintf("ENTITY_CAN_NOT_BE_DELETED: The entity requested for delete can not be deleted: cannot delete system foo in a non-system context (id=%v)", foo.Id))
}

func TestSystemEntities_NonSystemConvertToSystem(t *testing.T) {
	test := &systemEntitiesTest{}
	test.Assertions = require.New(t)
	test.init()
	defer test.cleanup()

	store := test.fooStore

	foo := test.newFoo()

	tx, err := test.db.Begin(true)
	test.NoError(err)

	defer func() {
		err = tx.Commit()
		test.NoError(err)
	}()

	ctx := NewMutateContext(tx)
	test.NoError(store.Create(ctx, foo))

	fooCheck, err := store.LoadOneById(tx, foo.Id)
	test.NoError(err)
	test.Equal(foo.Id, fooCheck.Id)
	test.Equal(foo.Name, fooCheck.Name)
	test.Equal(foo.IsSystem, fooCheck.IsSystem)

	foo.Name = uuid.NewString()
	foo.IsSystem = true
	test.NoError(store.Update(NewMutateContext(tx), foo, nil))

	fooCheck, err = store.LoadOneById(tx, foo.Id)
	test.NoError(err)
	test.Equal(foo.Id, fooCheck.Id)
	test.Equal(foo.Name, fooCheck.Name)
	test.Equal(false, fooCheck.IsSystem)
}

func TestSystemEntities_NonSystemConvertFromSystem(t *testing.T) {
	test := &systemEntitiesTest{}
	test.Assertions = require.New(t)
	test.init()
	defer test.cleanup()

	store := test.fooStore

	foo := test.newFoo()
	foo.IsSystem = true

	tx, err := test.db.Begin(true)
	test.NoError(err)

	defer func() {
		err = tx.Commit()
		test.NoError(err)
	}()

	ctx := NewMutateContext(tx).GetSystemContext()
	test.NoError(store.Create(ctx, foo))

	fooCheck, err := store.LoadOneById(tx, foo.Id)
	test.NoError(err)
	test.Equal(foo.Id, fooCheck.Id)
	test.Equal(foo.Name, fooCheck.Name)
	test.Equal(foo.IsSystem, fooCheck.IsSystem)

	foo.Name = uuid.NewString()
	foo.IsSystem = false
	err = store.Update(NewMutateContext(tx), foo, nil)
	test.EqualError(err, fmt.Sprintf("ENTITY_CAN_NOT_BE_UPDATED: The entity requested for update can not be updated: cannot update system foo in a non-system context (id=%v)", foo.Id))
}
