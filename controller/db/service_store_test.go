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
	"fmt"
	"github.com/google/uuid"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

func Test_ServiceStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create invalid api services", ctx.testCreateInvalidServices)
	t.Run("test create service", ctx.testCreateServices)
	t.Run("test load/query services", ctx.testLoadQueryServices)
	t.Run("test update services", ctx.testUpdateServices)
	t.Run("test delete services", ctx.testDeleteServices)
}

func (ctx *TestContext) testCreateInvalidServices(t *testing.T) {
	ctx.Impl.NextTest(t)
	defer ctx.cleanupAll()

	service := &Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          uuid.New().String(),
	}

	ctx.RequireCreate(service)
	err := ctx.Create(service)
	ctx.EqualError(err, fmt.Sprintf("an entity of type service already exists with id %v", service.Id))

	service.Id = uuid.New().String()
	err = ctx.Create(service)
	ctx.EqualError(err, fmt.Sprintf("duplicate value '%v' in unique index on services store", service.Name))

	service.Id = uuid.New().String()
	service.Name = uuid.New().String()
	service.TerminatorStrategy = uuid.New().String()
	err = ctx.Create(service)
	ctx.EqualError(err, fmt.Sprintf("terminatorStrategy with name %v not found", service.TerminatorStrategy))
}

func (ctx *TestContext) testCreateServices(t *testing.T) {
	ctx.Impl.NextTest(t)
	defer ctx.cleanupAll()

	service := &Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          uuid.New().String(),
	}
	ctx.RequireCreate(service)
	ctx.ValidateBaseline(service)
}

type serviceTestEntities struct {
	service1   *Service
	service2   *Service
	terminator *Terminator
	router     *Router
}

func (ctx *TestContext) createServiceTestEntities() *serviceTestEntities {
	service1 := ctx.requireNewService()
	service2 := ctx.requireNewService()
	router := ctx.requireNewRouter()

	terminator := &Terminator{
		BaseExtEntity: boltz.BaseExtEntity{
			Id: uuid.New().String(),
		},
		Service: service1.Id,
		Router:  router.Id,
		Binding: "transport",
		Address: "tcp:localhost:22",
	}

	ctx.RequireCreate(terminator)

	return &serviceTestEntities{
		service1:   service1,
		service2:   service2,
		router:     router,
		terminator: terminator,
	}
}

func (ctx *TestContext) testLoadQueryServices(t *testing.T) {
	ctx.Impl.NextTest(t)
	ctx.cleanupAll()

	entities := ctx.createServiceTestEntities()

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		service, err := ctx.stores.Service.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		service, err = ctx.stores.Service.LoadOneByName(tx, entities.service1.Name)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		query := fmt.Sprintf(`anyOf(terminators) = "%v"`, entities.terminator.Id)
		ids, _, err := ctx.stores.Service.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.service1.Id, ids[0])

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateServices(t *testing.T) {
	ctx.Impl.NextTest(t)
	ctx.cleanupAll()
	entities := ctx.createServiceTestEntities()
	earlier := time.Now()
	time.Sleep(time.Millisecond * 50)

	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		original, err := ctx.stores.Service.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(original)

		service, err := ctx.stores.Service.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)

		tags := ctx.CreateTags()
		now := time.Now()
		service.Name = uuid.New().String()
		service.UpdatedAt = earlier
		service.CreatedAt = now
		service.Tags = tags

		err = ctx.stores.Service.Update(boltz.NewMutateContext(tx), service, nil)
		ctx.NoError(err)
		loaded, err := ctx.stores.Service.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(loaded)
		ctx.EqualValues(original.CreatedAt, loaded.CreatedAt)
		ctx.True(loaded.UpdatedAt.Equal(now) || loaded.UpdatedAt.After(now))
		service.CreatedAt = loaded.CreatedAt
		service.UpdatedAt = loaded.UpdatedAt
		ctx.True(cmp.Equal(service, loaded), cmp.Diff(service, loaded))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testDeleteServices(t *testing.T) {
	ctx.Impl.NextTest(t)

	ctx.cleanupAll()
	entities := ctx.createServiceTestEntities()
	ctx.RequireDelete(entities.service1)
	ctx.RequireDelete(entities.service2)
}
