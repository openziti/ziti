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

package persistence

import (
	"fmt"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/db"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/stringz"
	"go.etcd.io/bbolt"
)

func Test_EdgeServiceStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test service parent child relationship", ctx.testServiceParentChild)
	t.Run("test create invalid api services", ctx.testCreateInvalidServices)
	t.Run("test create service", ctx.testCreateServices)
	t.Run("test load/query services", ctx.testLoadQueryServices)
	t.Run("test update services", ctx.testUpdateServices)
	t.Run("test delete services", ctx.testDeleteServices)
}

func (ctx *TestContext) testServiceParentChild(_ *testing.T) {
	fabricService := &db.Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
	}

	ctx.RequireCreate(fabricService)
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
	ctx.Error(ctx.Update(edgeService))
	//
	//err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
	//	query := fmt.Sprintf(`id = "%v" and name = "%v"`, fabricService.Id, edgeService.Name)
	//	ids, _, err := ctx.stores.EdgeService.QueryIds(tx, query)
	//	if err != nil {
	//		return err
	//	}
	//	ctx.Equal(1, len(ids))
	//	ctx.Equal(fabricService.Id, ids[0])
	//	return nil
	//})
	//ctx.NoError(err)
}

func (ctx *TestContext) testCreateInvalidServices(_ *testing.T) {
	defer ctx.CleanupAll()

	identity := ctx.RequireNewIdentity("test-user", false)
	apiSession := NewApiSession(identity.Id)
	ctx.RequireCreate(apiSession)

	edgeService := &EdgeService{
		Service: db.Service{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          eid.New(),
		},
	}

	ctx.RequireCreate(edgeService)
	err := ctx.Create(edgeService)
	ctx.EqualError(err, fmt.Sprintf("an entity of type service already exists with id %v", edgeService.Id))

	edgeService.Id = eid.New()
	err = ctx.Create(edgeService)
	ctx.EqualError(err, fmt.Sprintf("duplicate value '%v' in unique index on services store", edgeService.Name))

	edgeService.Id = eid.New()
	edgeService.Name = eid.New()
	edgeService.TerminatorStrategy = eid.New()
	err = ctx.Create(edgeService)
	ctx.EqualError(err, fmt.Sprintf("terminatorStrategy with name %v not found", edgeService.TerminatorStrategy))
}

func (ctx *TestContext) testCreateServices(_ *testing.T) {
	defer ctx.CleanupAll()

	edgeService := &EdgeService{
		Service: db.Service{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          eid.New(),
		},
	}
	ctx.RequireCreate(edgeService)
	ctx.ValidateBaseline(edgeService)
}

type serviceTestEntities struct {
	servicePolicy *ServicePolicy
	identity1     *Identity
	apiSession1   *ApiSession
	service1      *EdgeService
	service2      *EdgeService
	session1      *Session
	session2      *Session
}

func (ctx *TestContext) createServiceTestEntities() *serviceTestEntities {
	identity1 := ctx.RequireNewIdentity("admin1", true)

	apiSession1 := NewApiSession(identity1.Id)

	role := eid.New()

	ctx.RequireCreate(apiSession1)
	servicePolicy := ctx.requireNewServicePolicy(PolicyTypeDial, ss(), ss(roleRef(role)))

	service1 := &EdgeService{
		Service: db.Service{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          eid.New(),
		},
		RoleAttributes: []string{role},
	}

	ctx.RequireCreate(service1)
	service2 := ctx.RequireNewService(eid.New())

	session1 := NewSession(apiSession1.Id, service1.Id)
	ctx.RequireCreate(session1)

	session2 := NewSession(apiSession1.Id, service2.Id)
	ctx.RequireCreate(session2)

	return &serviceTestEntities{
		servicePolicy: servicePolicy,
		identity1:     identity1,
		apiSession1:   apiSession1,
		service1:      service1,
		service2:      service2,
		session1:      session1,
		session2:      session2,
	}
}

func (ctx *TestContext) testLoadQueryServices(_ *testing.T) {
	ctx.CleanupAll()

	entities := ctx.createServiceTestEntities()

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		service, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		service, err = ctx.stores.EdgeService.LoadOneByName(tx, entities.service1.Name)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		query := fmt.Sprintf(`anyOf(servicePolicies) = "%v"`, entities.servicePolicy.Id)
		ids, _, err := ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.True(stringz.Contains(ids, entities.service1.Id))

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateServices(_ *testing.T) {
	ctx.CleanupAll()
	entities := ctx.createServiceTestEntities()
	earlier := time.Now()
	time.Sleep(time.Millisecond * 50)

	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
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

		err = ctx.stores.EdgeService.Update(boltz.NewMutateContext(tx), service, nil)
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

func (ctx *TestContext) testDeleteServices(_ *testing.T) {
	ctx.CleanupAll()
	entities := ctx.createServiceTestEntities()
	ctx.RequireDelete(entities.service1)
	ctx.RequireDelete(entities.service2)
}
