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
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/google/go-cmp/cmp"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

func Test_RouterStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create invalid api routers", ctx.testCreateInvalidRouters)
	t.Run("test create router", ctx.testCreateRouters)
	t.Run("test load/query routers", ctx.testLoadQueryRouters)
	t.Run("test update routers", ctx.testUpdateRouters)
	t.Run("test delete routers", ctx.testDeleteRouters)
}

func (ctx *TestContext) testCreateInvalidRouters(t *testing.T) {
	ctx.Impl.NextTest(t)
	defer ctx.cleanupAll()

	router := &Router{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          uuid.New().String(),
		NoTraversal:   true,
	}

	ctx.RequireCreate(router)
	err := ctx.Create(router)
	ctx.EqualError(err, fmt.Sprintf("an entity of type router already exists with id %v", router.Id))

	router.Id = uuid.New().String()
	err = ctx.Create(router)
	ctx.EqualError(err, fmt.Sprintf("duplicate value '%v' in unique index on routers store", router.Name))
}

func (ctx *TestContext) testCreateRouters(t *testing.T) {
	ctx.Impl.NextTest(t)
	defer ctx.cleanupAll()

	router := &Router{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          uuid.New().String(),
		Cost:          2,
	}
	ctx.RequireCreate(router)
	ctx.ValidateBaseline(router)
}

type routerTestEntities struct {
	router1    *Router
	router2    *Router
	terminator *Terminator
	service    *Service
}

func (ctx *TestContext) createRouterTestEntities() *routerTestEntities {
	router1 := ctx.requireNewRouter()
	router2 := ctx.requireNewRouter()
	service := ctx.requireNewService()

	terminator := &Terminator{
		BaseExtEntity: boltz.BaseExtEntity{
			Id: uuid.New().String(),
		},
		Service: service.Id,
		Router:  router1.Id,
		Binding: "transport",
		Address: "tcp:localhost:22",
	}

	ctx.RequireCreate(terminator)

	return &routerTestEntities{
		router1:    router1,
		router2:    router2,
		service:    service,
		terminator: terminator,
	}
}

func (ctx *TestContext) testLoadQueryRouters(t *testing.T) {
	ctx.Impl.NextTest(t)
	ctx.cleanupAll()

	entities := ctx.createRouterTestEntities()

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		router, err := ctx.stores.Router.LoadOneById(tx, entities.router1.Id)
		ctx.NoError(err)
		ctx.NotNil(router)
		ctx.EqualValues(entities.router1.Id, router.Id)

		router, err = ctx.stores.Router.LoadOneByName(tx, entities.router1.Name)
		ctx.NoError(err)
		ctx.NotNil(router)
		ctx.EqualValues(entities.router1.Id, router.Id)

		query := fmt.Sprintf(`anyOf(terminators) = "%v"`, entities.terminator.Id)
		ids, _, err := ctx.stores.Router.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.router1.Id, ids[0])

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateRouters(t *testing.T) {
	ctx.Impl.NextTest(t)
	ctx.cleanupAll()

	entities := ctx.createRouterTestEntities()
	earlier := time.Now()
	time.Sleep(time.Millisecond * 50)

	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		original, err := ctx.stores.Router.LoadOneById(tx, entities.router1.Id)
		ctx.NoError(err)
		ctx.NotNil(original)

		router, err := ctx.stores.Router.LoadOneById(tx, entities.router1.Id)
		ctx.NoError(err)
		ctx.NotNil(router)

		tags := ctx.CreateTags()
		now := time.Now()
		router.Name = uuid.New().String()
		router.UpdatedAt = earlier
		router.CreatedAt = now
		router.Tags = tags

		err = ctx.stores.Router.Update(boltz.NewMutateContext(tx), router, nil)
		ctx.NoError(err)
		loaded, err := ctx.stores.Router.LoadOneById(tx, entities.router1.Id)
		ctx.NoError(err)
		ctx.NotNil(loaded)
		ctx.EqualValues(original.CreatedAt, loaded.CreatedAt)
		ctx.True(loaded.UpdatedAt.Equal(now) || loaded.UpdatedAt.After(now))
		router.CreatedAt = loaded.CreatedAt
		router.UpdatedAt = loaded.UpdatedAt
		ctx.True(cmp.Equal(router, loaded), cmp.Diff(router, loaded))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testDeleteRouters(t *testing.T) {
	ctx.Impl.NextTest(t)
	ctx.cleanupAll()
	entities := ctx.createRouterTestEntities()
	ctx.RequireDelete(entities.router1)
	ctx.RequireDelete(entities.router2)
}
