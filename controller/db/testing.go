package db

import (
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/xt"
	"github.com/openziti/ziti/controller/xt_smartrouting"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"testing"
)

func NewTestContext(t testing.TB) *TestContext {
	xt.GlobalRegistry().RegisterFactory(xt_smartrouting.NewFactory())

	context := &TestContext{
		closeNotify: make(chan struct{}, 1),
	}
	context.BaseTestContext = boltztest.NewTestContext(t, context.GetStoreForEntity)
	context.Init()
	return context
}

type TestContext struct {
	*boltztest.BaseTestContext
	stores *Stores
	// n           *network.Network
	closeNotify chan struct{}
}

func (ctx *TestContext) GetStoreForEntity(entity boltz.Entity) boltz.Store {
	return ctx.stores.GetStoreForEntity(entity)
}

func (ctx *TestContext) Init() {
	ctx.InitDb(Open)

	var err error
	ctx.stores, err = InitStores(ctx.GetDb())
	ctx.NoError(err)

	ctx.NoError(RunMigrations(ctx.GetDb(), ctx.stores))
	ctx.NoError(ctx.stores.EventualEventer.Start(ctx.closeNotify))
}

//func (ctx *TestContext) Init() {
//	ctx.BaseTestContext.InitDb(Open)
//
//	//db := ctx.GetDbProvider()
//	//
//	//config := newTestConfig(ctx)
//	//var err error
//	//ctx.n, err = network.NewNetwork(config)
//	//ctx.NoError(err)
//	//
//	//// TODO: setup up single node raft cluster or mock?
//	//ctx.stores, err = NewBoltStores(db)
//	//ctx.NoError(err)
//
//	ctx.NoError(RunMigrations(ctx.GetDb(), ctx.stores))
//
//	ctx.NoError(ctx.stores.EventualEventer.Start(ctx.closeNotify))
//
//}

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

func (ctx *TestContext) newViewTestCtx(tx *bbolt.Tx) boltz.MutateContext {
	return boltz.NewTxMutateContext(change.New().SetChangeAuthorType("test").GetContext(), tx)
}

//func (ctx *TestContext) GetNetwork() *network.Network {
//	return ctx.n
//}

func (ctx *TestContext) Cleanup() {
	close(ctx.closeNotify)
	ctx.BaseTestContext.Cleanup()
}

func (ctx *TestContext) GetStores() *Stores {
	return ctx.stores
}

func (ctx *TestContext) GetDb() boltz.Db {
	return ctx.BaseTestContext.GetDb()
}

//func (ctx *TestContext) GetDbProvider() DbProvider {
//	return &testDbProvider{ctx: ctx}
//}

func (ctx *TestContext) requireNewServicePolicy(policyType PolicyType, identityRoles []string, serviceRoles []string) *ServicePolicy {
	entity := &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		PolicyType:    policyType,
		Semantic:      SemanticAnyOf,
		IdentityRoles: identityRoles,
		ServiceRoles:  serviceRoles,
	}
	boltztest.RequireCreate(ctx, entity)
	return entity
}

func (ctx *TestContext) RequireNewIdentity(name string, isAdmin bool) *Identity {
	identityEntity := &Identity{
		BaseExtEntity: *boltz.NewExtEntity(eid.New(), nil),
		Name:          name,
		IsAdmin:       isAdmin,
	}
	boltztest.RequireCreate(ctx, identityEntity)
	return identityEntity
}

func (ctx *TestContext) RequireNewService(name string) *EdgeService {
	edgeService := &EdgeService{
		Service: Service{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          name,
		},
	}
	boltztest.RequireCreate(ctx, edgeService)
	return edgeService
}

func (ctx *TestContext) getRelatedIds(entity boltz.Entity, field string) []string {
	var result []string
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		store := ctx.stores.GetStoreForEntity(entity)
		if store == nil {
			return errors.Errorf("no store for entity of type '%v'", entity.GetEntityType())
		}
		result = store.GetRelatedEntitiesIdList(tx, entity.GetId(), field)
		return nil
	})
	ctx.NoError(err)
	return result
}

func (ctx *TestContext) CleanupAll() {
	stores := []boltz.Store{
		ctx.stores.Session,
		ctx.stores.ApiSession,
		ctx.stores.Service,
		ctx.stores.EdgeService,
		ctx.stores.Identity,
		ctx.stores.EdgeRouter,
		ctx.stores.Config,
		ctx.stores.Identity,
		ctx.stores.EdgeRouterPolicy,
		ctx.stores.ServicePolicy,
		ctx.stores.ServiceEdgeRouterPolicy,
	}

	_ = ctx.GetDb().Update(change.New().NewMutateContext(), func(mutateCtx boltz.MutateContext) error {
		for _, store := range stores {
			if err := store.DeleteWhere(mutateCtx, `true limit none`); err != nil {
				pfxlog.Logger().WithError(err).Errorf("failure while cleaning up %v", store.GetEntityType())
				return err
			}
		}
		return nil
	})
}

func (ctx *TestContext) getIdentityTypeId() string {
	var result string
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ids, _, err := ctx.stores.IdentityType.QueryIds(tx, "true")
		if err != nil {
			return err
		}
		result = ids[0]
		return nil
	})
	ctx.NoError(err)
	return result
}

func ss(vals ...string) []string {
	return vals
}
