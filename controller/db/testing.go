package db

import (
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/controller/xt_smartrouting"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
	"testing"
)

func NewTestContext(t *testing.T) *TestContext {
	xt.GlobalRegistry().RegisterFactory(xt_smartrouting.NewFactory())

	context := &TestContext{
		BaseTestContext: *boltz.NewTestContext(t),
	}
	context.Impl = context
	context.Init()
	return context
}

type TestContext struct {
	db     boltz.Db
	stores *Stores
	boltz.BaseTestContext
}

func (ctx *TestContext) GetStoreForEntity(entity boltz.Entity) boltz.CrudStore {
	return ctx.stores.GetStoreForEntity(entity)
}

func (ctx *TestContext) GetDb() boltz.Db {
	return ctx.db
}

func (ctx *TestContext) Init() {
	ctx.InitDbFile()

	var err error
	ctx.db, err = Open(ctx.GetDbFile().Name(), false)
	ctx.NoError(err)

	ctx.stores, err = InitStores(ctx.GetDb())
	ctx.NoError(err)
}

func (ctx *TestContext) requireNewService() *Service {
	entity := &Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          uuid.New().String(),
	}
	ctx.RequireCreate(entity)
	return entity
}

func (ctx *TestContext) requireNewRouter() *Router {
	entity := &Router{
		BaseExtEntity: boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:          uuid.New().String(),
	}
	ctx.RequireCreate(entity)
	return entity
}

func (ctx *TestContext) cleanupAll() {
	_ = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := boltz.NewMutateContext(tx)
		for _, store := range ctx.stores.storeMap {
			if err := store.DeleteWhere(mutateContext, `true limit none`); err != nil {
				pfxlog.Logger().WithError(err).Errorf("failure while cleaning up %v", store.GetEntityType())
				return err
			}
		}
		return nil
	})
}
