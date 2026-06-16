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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"

	"github.com/google/go-cmp/cmp"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
)

func Test_ServiceStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create invalid api services", ctx.testCreateInvalidServices)
	t.Run("test create service", ctx.testCreateServices)
	t.Run("test fabric-only services", ctx.testFabricOnlyServices)
	t.Run("test load/query services", ctx.testLoadQueryServices)
	t.Run("test load/query services by policy", ctx.testLoadQueryServicesByPolicy)
	t.Run("test update services", ctx.testUpdateServices)
	t.Run("test delete services", ctx.testDeleteServices)
	t.Run("test delete services with sessions", ctx.testDeleteServicesWithSessions)
}

func (ctx *TestContext) testFabricOnlyServices(t *testing.T) {
	ctx.NextTest(t)
	ctx.CleanupAll()

	// Fabric and edge services share a single store. A fabric-only service
	// (IsFabricOnly=true) should be filterable via the isFabricOnly field.
	fabricService := &Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		IsFabricOnly:  true,
	}

	boltztest.RequireCreate(ctx, fabricService)
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ctx.True(ctx.stores.Service.IsEntityPresent(tx, fabricService.GetId()))

		_, count, err := ctx.stores.Service.QueryIds(tx, "true")
		ctx.NoError(err)
		ctx.Equal(int64(1), count)

		ids, count, err := ctx.stores.Service.QueryIds(tx, "isFabricOnly = true")
		ctx.NoError(err)
		ctx.Equal(int64(1), count)
		ctx.Equal(fabricService.Id, ids[0])

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testCreateInvalidServices(t *testing.T) {
	ctx.NextTest(t)
	defer ctx.CleanupAll()

	service := &Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          uuid.New().String(),
	}

	boltztest.RequireCreate(ctx, service)
	err := boltztest.Create(ctx, service)
	ctx.EqualError(err, fmt.Sprintf("an entity of type service already exists with id %v", service.Id))

	service.Id = uuid.New().String()
	err = boltztest.Create(ctx, service)
	ctx.EqualError(err, fmt.Sprintf("duplicate value '%v' in unique index on services store", service.Name))

	service.Id = uuid.New().String()
	service.Name = uuid.New().String()
	service.TerminatorStrategy = uuid.New().String()
	err = boltztest.Create(ctx, service)
	ctx.EqualError(err, fmt.Sprintf("terminatorStrategy with name %v not found", service.TerminatorStrategy))
}

func (ctx *TestContext) testCreateServices(t *testing.T) {
	ctx.NextTest(t)
	defer ctx.CleanupAll()

	service := &Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          uuid.New().String(),
	}
	boltztest.RequireCreate(ctx, service)
	boltztest.ValidateBaseline(ctx, service)
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
	service2.Tags = map[string]interface{}{
		"location": "NY",
	}
	boltztest.RequireUpdate(ctx, service2)
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

	boltztest.RequireCreate(ctx, terminator)

	return &serviceTestEntities{
		service1:   service1,
		service2:   service2,
		router:     router,
		terminator: terminator,
	}
}

func (ctx *TestContext) testLoadQueryServices(t *testing.T) {
	ctx.NextTest(t)
	ctx.CleanupAll()

	entities := ctx.createServiceTestEntities()

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		service, _, err := ctx.stores.Service.FindById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		service, err = ctx.stores.Service.FindByName(tx, entities.service1.Name)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		query := fmt.Sprintf(`anyOf(terminators) = "%v"`, entities.terminator.Id)
		ids, _, err := ctx.stores.Service.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.service1.Id, ids[0])

		query = `tags.location = "NY"`
		ids, _, err = ctx.stores.Service.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.service2.Id, ids[0])

		query = `tags.location = null`
		ids, _, err = ctx.stores.Service.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.service1.Id, ids[0])

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateServices(t *testing.T) {
	ctx.NextTest(t)
	ctx.CleanupAll()
	entities := ctx.createServiceTestEntities()
	earlier := time.Now()
	time.Sleep(time.Millisecond * 50)

	err := ctx.GetDb().Update(nil, func(changeCtx boltz.MutateContext) error {
		original, _, err := ctx.stores.Service.FindById(changeCtx.Tx(), entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(original)

		service, _, err := ctx.stores.Service.FindById(changeCtx.Tx(), entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)

		tags := ctx.CreateTags()
		now := time.Now()
		service.Name = uuid.New().String()
		service.UpdatedAt = earlier
		service.CreatedAt = now
		service.Tags = tags

		err = ctx.stores.Service.Update(changeCtx, service, nil)
		ctx.NoError(err)
		loaded, _, err := ctx.stores.Service.FindById(changeCtx.Tx(), entities.service1.Id)
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
	ctx.NextTest(t)

	ctx.CleanupAll()
	entities := ctx.createServiceTestEntities()
	boltztest.RequireDelete(ctx, entities.service1)
	boltztest.RequireDelete(ctx, entities.service2)
}

type serviceSessionTestEntities struct {
	servicePolicy *ServicePolicy
	identity1     *Identity
	apiSession1   *ApiSession
	service1      *Service
	service2      *Service
	session1      *Session
	session2      *Session
}

func (ctx *TestContext) createServiceSessionTestEntities() *serviceSessionTestEntities {
	identity1 := ctx.RequireNewIdentity("admin1", true)

	apiSession1 := NewApiSession(identity1.Id)

	role := eid.New()

	boltztest.RequireCreate(ctx, apiSession1)
	servicePolicy := ctx.requireNewServicePolicy(PolicyTypeDial, ss(), ss(roleRef(role)))

	service1 := &Service{
		BaseExtEntity:  boltz.BaseExtEntity{Id: eid.New()},
		Name:           eid.New(),
		RoleAttributes: []string{role},
	}

	boltztest.RequireCreate(ctx, service1)
	service2 := ctx.RequireNewService(eid.New())

	session1 := NewSession(apiSession1.Id, service1.Id)
	boltztest.RequireCreate(ctx, session1)

	session2 := NewSession(apiSession1.Id, service2.Id)
	boltztest.RequireCreate(ctx, session2)

	return &serviceSessionTestEntities{
		servicePolicy: servicePolicy,
		identity1:     identity1,
		apiSession1:   apiSession1,
		service1:      service1,
		service2:      service2,
		session1:      session1,
		session2:      session2,
	}
}

func (ctx *TestContext) testLoadQueryServicesByPolicy(t *testing.T) {
	ctx.NextTest(t)
	ctx.CleanupAll()

	entities := ctx.createServiceSessionTestEntities()

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		service, err := ctx.stores.Service.LoadById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		query := fmt.Sprintf(`anyOf(servicePolicies) = "%v"`, entities.servicePolicy.Id)
		ids, _, err := ctx.stores.Service.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.True(stringz.Contains(ids, entities.service1.Id))

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testDeleteServicesWithSessions(t *testing.T) {
	ctx.NextTest(t)
	ctx.CleanupAll()
	entities := ctx.createServiceSessionTestEntities()
	boltztest.RequireDelete(ctx, entities.service1, apiSessionsSessionsIdxPath)
	boltztest.RequireDelete(ctx, entities.service2, apiSessionsSessionsIdxPath)
}
