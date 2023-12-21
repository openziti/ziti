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
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/change"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

func Test_EdgeServiceStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test service parent child relationship", ctx.testEdgeServiceParentChild)
	t.Run("test create invalid api services", ctx.testCreateInvalidEdgeServices)
	t.Run("test create service", ctx.testCreateEdgeServices)
	t.Run("test load/query services", ctx.testLoadQueryEdgeServices)
	t.Run("test update services", ctx.testUpdateEdgeServices)
	t.Run("test delete services", ctx.testDeleteEdgeServices)
}

func (ctx *TestContext) testEdgeServiceParentChild(_ *testing.T) {
	fabricService := &Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
	}

	boltztest.RequireCreate(ctx, fabricService)
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ctx.False(ctx.stores.EdgeService.IsEntityPresent(tx, fabricService.GetId()))

		_, count, err := ctx.stores.EdgeService.QueryIds(tx, "true")
		ctx.NoError(err)
		ctx.Equal(int64(0), count)
		return nil
	})
	ctx.NoError(err)

	edgeService := &EdgeService{
		Service: *fabricService,
	}

	// Supporting extending a base type gets complicated with lower level indexes, since you're mixing an update
	// in the parent with a create in the child. Not worth fixing right now
	ctx.Error(boltztest.Update(ctx, edgeService))
	//
	//err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
	//	query := fmt.Sprintf(`id = "%v" and name = "%v"`, fabricService.Id, edgeService.Name)
	//	ids, _, err := ctx.stores.EdgeService.QueryEntities(tx, query)
	//	if err != nil {
	//		return err
	//	}
	//	ctx.Equal(1, len(ids))
	//	ctx.Equal(fabricService.Id, ids[0])
	//	return nil
	//})
	//ctx.NoError(err)
}

func (ctx *TestContext) testCreateInvalidEdgeServices(_ *testing.T) {
	defer ctx.CleanupAll()

	identity := ctx.RequireNewIdentity("test-user", false)
	apiSession := NewApiSession(identity.Id)
	boltztest.RequireCreate(ctx, apiSession)

	edgeService := &EdgeService{
		Service: Service{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          eid.New(),
		},
	}

	boltztest.RequireCreate(ctx, edgeService)
	err := boltztest.Create(ctx, edgeService)
	ctx.EqualError(err, fmt.Sprintf("an entity of type service already exists with id %v", edgeService.Id))

	edgeService.Id = eid.New()
	err = boltztest.Create(ctx, edgeService)
	ctx.EqualError(err, fmt.Sprintf("duplicate value '%v' in unique index on services store", edgeService.Name))

	edgeService.Id = eid.New()
	edgeService.Name = eid.New()
	edgeService.TerminatorStrategy = eid.New()
	err = boltztest.Create(ctx, edgeService)
	ctx.EqualError(err, fmt.Sprintf("terminatorStrategy with name %v not found", edgeService.TerminatorStrategy))
}

func (ctx *TestContext) testCreateEdgeServices(_ *testing.T) {
	defer ctx.CleanupAll()

	edgeService := &EdgeService{
		Service: Service{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          eid.New(),
		},
	}
	boltztest.RequireCreate(ctx, edgeService)
	boltztest.ValidateBaseline(ctx, edgeService)
}

type edgeServiceTestEntities struct {
	servicePolicy *ServicePolicy
	identity1     *Identity
	apiSession1   *ApiSession
	service1      *EdgeService
	service2      *EdgeService
	session1      *Session
	session2      *Session
}

func (ctx *TestContext) createEdgeServiceTestEntities() *edgeServiceTestEntities {
	identity1 := ctx.RequireNewIdentity("admin1", true)

	apiSession1 := NewApiSession(identity1.Id)

	role := eid.New()

	boltztest.RequireCreate(ctx, apiSession1)
	servicePolicy := ctx.requireNewServicePolicy(PolicyTypeDial, ss(), ss(roleRef(role)))

	service1 := &EdgeService{
		Service: Service{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          eid.New(),
		},
		RoleAttributes: []string{role},
	}

	boltztest.RequireCreate(ctx, service1)
	service2 := ctx.RequireNewService(eid.New())
	service2.Tags = map[string]interface{}{
		"location": "NY",
	}
	boltztest.RequireUpdate(ctx, service2)

	session1 := NewSession(apiSession1.Id, service1.Id)
	boltztest.RequireCreate(ctx, session1)

	session2 := NewSession(apiSession1.Id, service2.Id)
	boltztest.RequireCreate(ctx, session2)

	return &edgeServiceTestEntities{
		servicePolicy: servicePolicy,
		identity1:     identity1,
		apiSession1:   apiSession1,
		service1:      service1,
		service2:      service2,
		session1:      session1,
		session2:      session2,
	}
}

func (ctx *TestContext) testLoadQueryEdgeServices(_ *testing.T) {
	ctx.CleanupAll()

	entities := ctx.createEdgeServiceTestEntities()

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		service, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		query := fmt.Sprintf(`anyOf(servicePolicies) = "%v"`, entities.servicePolicy.Id)
		ids, _, err := ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.True(stringz.Contains(ids, entities.service1.Id))

		query = `tags.location = "NY"`
		ids, _, err = ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.service2.Id, ids[0])

		query = `tags.location = null`
		ids, _, err = ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.service1.Id, ids[0])

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateEdgeServices(_ *testing.T) {
	ctx.CleanupAll()
	entities := ctx.createEdgeServiceTestEntities()
	earlier := time.Now()
	time.Sleep(time.Millisecond * 50)

	mutateCtx := change.New().NewMutateContext()
	err := ctx.GetDb().Update(mutateCtx, func(mutateCtx boltz.MutateContext) error {
		tx := mutateCtx.Tx()
		original, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(original)

		service, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)

		tags := ctx.CreateTags()
		now := time.Now()
		service.Name = eid.New()
		service.UpdatedAt = earlier
		service.CreatedAt = now
		service.Tags = tags

		err = ctx.stores.EdgeService.Update(mutateCtx, service, nil)
		ctx.NoError(err)
		loaded, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
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

func (ctx *TestContext) testDeleteEdgeServices(_ *testing.T) {
	ctx.CleanupAll()
	entities := ctx.createEdgeServiceTestEntities()
	boltztest.RequireDelete(ctx, entities.service1, apiSessionsSessionsIdxPath)
	boltztest.RequireDelete(ctx, entities.service2, apiSessionsSessionsIdxPath)
}
