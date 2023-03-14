package db

import (
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/controller/xt_smartrouting"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/storage/boltztest"
	"testing"
)

func NewTestContext(t testing.TB) *TestContext {
	xt.GlobalRegistry().RegisterFactory(xt_smartrouting.NewFactory())

	context := &TestContext{}
	context.BaseTestContext = boltztest.NewTestContext(t, context.GetStoreForEntity)
	context.Init()
	return context
}

type TestContext struct {
	stores *Stores
	*boltztest.BaseTestContext
}

func (ctx *TestContext) GetStoreForEntity(entity boltz.Entity) boltz.Store {
	return ctx.stores.GetStoreForEntity(entity)
}

func (ctx *TestContext) Init() {
	ctx.InitDb(Open)

	var err error
	ctx.stores, err = InitStores(ctx.GetDb())
	ctx.NoError(err)
}

func (ctx *TestContext) requireNewService() *Service {
	entity := &Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          uuid.New().String(),
	}
	boltztest.RequireCreate(ctx, entity)
	return entity
}

func (ctx *TestContext) requireNewRouter() *Router {
	entity := &Router{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          uuid.New().String(),
	}
	boltztest.RequireCreate(ctx, entity)
	return entity
}

func (ctx *TestContext) cleanupAll() {
	_ = ctx.GetDb().Update(nil, func(changeCtx boltz.MutateContext) error {
		for _, store := range ctx.stores.storeMap {
			if err := store.DeleteWhere(changeCtx, `true limit none`); err != nil {
				pfxlog.Logger().WithError(err).Errorf("failure while cleaning up %v", store.GetEntityType())
				return err
			}
		}
		return nil
	})
}
