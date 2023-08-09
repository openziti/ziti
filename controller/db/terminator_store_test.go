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
	"github.com/openziti/fabric/controller/fields"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/boltztest"
	"go.etcd.io/bbolt"
	"math"
	"testing"
	"time"
)

func Test_TerminatorStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	xt.GlobalRegistry().RegisterFactory(&testStrategyFactory{})

	t.Run("test create invalid terminators", ctx.testCreateInvalidTerminators)
	t.Run("test create/delete terminators", ctx.testCreateTerminators)
	t.Run("test create/delete terminators", ctx.testLoadQueryTerminators)
	t.Run("test update terminators", ctx.testUpdateTerminators)
	t.Run("test delete terminators", ctx.testDeleteTerminators)
	t.Run("test patch terminators", ctx.testPatchTerminator)
	t.Run("test sibling lookup", ctx.testFindSiblings)
}

func (ctx *TestContext) testCreateInvalidTerminators(t *testing.T) {
	ctx.NextTest(t)
	defer ctx.cleanupAll()

	terminator := &Terminator{
		Binding: uuid.New().String(),
		Address: uuid.New().String(),
		Service: uuid.New().String(),
		Router:  uuid.New().String(),
	}
	err := boltztest.Update(ctx, terminator)
	ctx.EqualError(err, "cannot update terminator with blank id")

	terminator.Binding = ""
	err = boltztest.Create(ctx, terminator)
	ctx.EqualError(err, "the value '' for 'binding' is invalid: binding is required")

	terminator.Binding = uuid.New().String()
	terminator.Address = ""
	err = boltztest.Create(ctx, terminator)
	ctx.EqualError(err, "the value '' for 'address' is invalid: address is required")

	terminator.Router = ""
	terminator.Address = uuid.New().String()
	err = boltztest.Create(ctx, terminator)
	ctx.EqualError(err, "the value '' for 'router' is invalid: router is required")

	terminator.Service = ""
	terminator.Router = uuid.New().String()
	err = boltztest.Create(ctx, terminator)
	ctx.EqualError(err, "the value '' for 'service' is invalid: service is required")

	service := ctx.requireNewService()
	router := ctx.requireNewRouter()

	terminator.Service = uuid.New().String()
	terminator.Router = router.Id
	err = boltztest.Create(ctx, terminator)
	ctx.EqualError(err, fmt.Sprintf("service with id %v not found", terminator.Service))

	terminator.Service = service.Id
	terminator.Router = uuid.New().String()
	err = boltztest.Create(ctx, terminator)
	ctx.EqualError(err, fmt.Sprintf("router with id %v not found", terminator.Router))
}

type terminatorTestEntities struct {
	service  *Service
	service2 *Service

	router  *Router
	router2 *Router

	terminator  *Terminator
	terminator2 *Terminator
	terminator3 *Terminator
}

func (ctx *TestContext) createTestTerminators() *terminatorTestEntities {
	e := &terminatorTestEntities{}

	e.service = ctx.requireNewService()
	e.router = ctx.requireNewRouter()

	e.terminator = &Terminator{}
	e.terminator.Service = e.service.Id
	e.terminator.Router = e.router.Id
	e.terminator.Binding = uuid.New().String()
	e.terminator.Address = uuid.New().String()
	e.terminator.Cost = 0
	boltztest.RequireCreate(ctx, e.terminator)

	e.router2 = ctx.requireNewRouter()

	e.terminator2 = &Terminator{}
	e.terminator2.Service = e.service.Id
	e.terminator2.Router = e.router2.Id
	e.terminator2.Binding = uuid.New().String()
	e.terminator2.Address = uuid.New().String()
	e.terminator2.Cost = 100
	boltztest.RequireCreate(ctx, e.terminator2)

	e.service2 = ctx.requireNewService()

	e.terminator3 = &Terminator{}
	e.terminator3.Service = e.service2.Id
	e.terminator3.Router = e.router2.Id
	e.terminator3.Binding = uuid.New().String()
	e.terminator3.Address = uuid.New().String()
	e.terminator3.Cost = math.MaxUint16
	boltztest.RequireCreate(ctx, e.terminator3)

	return e
}

func (ctx *TestContext) testCreateTerminators(t *testing.T) {
	ctx.NextTest(t)
	defer ctx.cleanupAll()

	e := ctx.createTestTerminators()

	boltztest.ValidateBaseline(ctx, e.terminator)
	boltztest.ValidateBaseline(ctx, e.terminator2)
	boltztest.ValidateBaseline(ctx, e.terminator3)

	terminatorIds := ctx.GetRelatedIds(e.service, EntityTypeTerminators)
	ctx.EqualValues(2, len(terminatorIds))
	ctx.True(stringz.Contains(terminatorIds, e.terminator.Id))
	ctx.True(stringz.Contains(terminatorIds, e.terminator2.Id))

	terminatorIds = ctx.GetRelatedIds(e.router, EntityTypeTerminators)
	ctx.EqualValues(1, len(terminatorIds))
	ctx.EqualValues(e.terminator.Id, terminatorIds[0])

	terminatorIds = ctx.GetRelatedIds(e.router2, EntityTypeTerminators)
	ctx.EqualValues(2, len(terminatorIds))
	ctx.True(stringz.Contains(terminatorIds, e.terminator2.Id))
	ctx.True(stringz.Contains(terminatorIds, e.terminator3.Id))

	terminatorIds = ctx.GetRelatedIds(e.service2, EntityTypeTerminators)
	ctx.EqualValues(1, len(terminatorIds))
	ctx.EqualValues(e.terminator3.Id, terminatorIds[0])

}

func (ctx *TestContext) testLoadQueryTerminators(t *testing.T) {
	ctx.NextTest(t)
	defer ctx.cleanupAll()

	e := ctx.createTestTerminators()

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		loadedTerminator, found, err := ctx.stores.Terminator.FindById(tx, e.terminator.Id)
		ctx.NoError(err)
		ctx.True(found)
		ctx.NotNil(loadedTerminator)
		ctx.EqualValues(e.terminator.Id, loadedTerminator.Id)
		ctx.EqualValues(e.terminator.Service, loadedTerminator.Service)
		ctx.EqualValues(e.terminator.Router, loadedTerminator.Router)
		ctx.EqualValues(e.terminator.Binding, loadedTerminator.Binding)
		ctx.EqualValues(e.terminator.Address, loadedTerminator.Address)
		ctx.EqualValues(e.terminator.Cost, loadedTerminator.Cost)

		ids, _, err := ctx.stores.Terminator.QueryIds(tx, fmt.Sprintf(`service = "%v"`, e.service.Id))
		ctx.NoError(err)
		ctx.EqualValues(2, len(ids))
		ctx.True(stringz.Contains(ids, e.terminator.Id))
		ctx.True(stringz.Contains(ids, e.terminator2.Id))

		ids, _, err = ctx.stores.Terminator.QueryIds(tx, fmt.Sprintf(`router = "%v"`, e.router2.Id))
		ctx.NoError(err)
		ctx.EqualValues(2, len(ids))
		ctx.True(stringz.Contains(ids, e.terminator2.Id))
		ctx.True(stringz.Contains(ids, e.terminator3.Id))

		ids, _, err = ctx.stores.Service.QueryIds(tx, fmt.Sprintf(`anyOf(terminators) = "%v"`, e.terminator.Id))
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.True(stringz.Contains(ids, e.service.Id))

		ids, _, err = ctx.stores.Router.QueryIds(tx, fmt.Sprintf(`anyOf(terminators) = "%v"`, e.terminator.Id))
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.True(stringz.Contains(ids, e.router.Id))

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateTerminators(t *testing.T) {
	ctx.NextTest(t)
	defer ctx.cleanupAll()

	e := ctx.createTestTerminators()

	terminator := e.terminator
	boltztest.RequireReload(ctx, terminator)

	time.Sleep(time.Millisecond * 10) // ensure updatedAt is after createdAt

	terminator.Service = e.service2.Id
	terminator.Router = e.router2.Id
	terminator.Binding = uuid.New().String()
	terminator.Address = uuid.New().String()
	terminator.Tags = ctx.CreateTags()
	boltztest.RequireUpdate(ctx, terminator)

	terminator.Service = e.service.Id // service should not be updated
	boltztest.ValidateUpdated(ctx, terminator)

}

func (ctx *TestContext) testDeleteTerminators(t *testing.T) {
	ctx.NextTest(t)
	defer ctx.cleanupAll()

	e := ctx.createTestTerminators()

	boltztest.RequireDelete(ctx, e.terminator3)
	boltztest.RequireDelete(ctx, e.router2)

	boltztest.ValidateDeleted(ctx, e.terminator2.Id)
	boltztest.ValidateDeleted(ctx, e.terminator3.Id)

	boltztest.RequireDelete(ctx, e.service)
	boltztest.ValidateDeleted(ctx, e.terminator.Id)
}

func (ctx *TestContext) testPatchTerminator(*testing.T) {
	service := ctx.requireNewService()
	router := ctx.requireNewRouter()

	terminator := &Terminator{}
	terminator.Service = service.Id
	terminator.Router = router.Id
	terminator.Binding = uuid.New().String()
	terminator.Address = uuid.New().String()
	terminator.Cost = 0
	terminator.PeerData = map[uint32][]byte{
		1: {1, 2, 3},
		2: {3, 4, 5},
	}
	boltztest.RequireCreate(ctx, terminator)
	boltztest.ValidateBaseline(ctx, terminator)

	terminator.Cost = 100
	checker := fields.UpdatedFieldsMap{
		FieldTerminatorCost: struct{}{},
	}
	boltztest.RequirePatch(ctx, terminator, checker)
	boltztest.ValidateUpdated(ctx, terminator)

	terminator.PeerData = map[uint32][]byte{
		1: {7, 8, 9},
	}

	checker = fields.UpdatedFieldsMap{
		FieldServerPeerData: struct{}{},
	}
	boltztest.RequirePatch(ctx, terminator, checker)
	boltztest.ValidateUpdated(ctx, terminator)
}

func (ctx *TestContext) testFindSiblings(*testing.T) {
	service := ctx.requireNewService()
	router := ctx.requireNewRouter()

	terminator := &Terminator{}
	terminator.Service = service.Id
	terminator.Router = router.Id
	terminator.Binding = uuid.NewString()
	terminator.Address = uuid.NewString()
	terminator.InstanceId = uuid.NewString()
	boltztest.RequireCreate(ctx, terminator)

	terminator2 := &Terminator{}
	terminator2.Service = service.Id
	terminator2.Router = router.Id
	terminator2.Binding = terminator.Binding
	terminator2.Address = uuid.NewString()
	terminator2.InstanceId = terminator.InstanceId
	boltztest.RequireCreate(ctx, terminator2)

	terminator3 := &Terminator{}
	terminator3.Service = service.Id
	terminator3.Router = router.Id
	terminator3.Binding = terminator.Binding
	terminator3.Address = uuid.NewString()
	terminator3.InstanceId = uuid.NewString()
	boltztest.RequireCreate(ctx, terminator3)

	var siblings []*Terminator
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		siblings, err = ctx.stores.Terminator.GetTerminatorsInIdentityGroup(tx, terminator.Id)
		return err
	})
	ctx.NoError(err)
	ctx.Equal(1, len(siblings))
	ctx.Equal(terminator2.Id, siblings[0].Id)
}

type testStrategyFactory struct{}

func (t testStrategyFactory) GetStrategyName() string {
	return "smartrouting"
}

func (t testStrategyFactory) NewStrategy() xt.Strategy {
	return &testStrategy{}
}

type testStrategy struct{}

func (t testStrategy) Select(terminators []xt.CostedTerminator) (xt.CostedTerminator, error) {
	return terminators[0], nil
}

func (t testStrategy) HandleTerminatorChange(xt.StrategyChangeEvent) error {
	return nil
}

func (t testStrategy) NotifyEvent(xt.TerminatorEvent) {
}
