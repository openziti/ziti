package db

import (
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
	"github.com/openziti/ziti/v2/controller/xt"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
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
	ctx.stores, err = InitStores(ctx.GetDb(), command.NoOpRateLimiter{}, nil)
	ctx.NoError(err)

	ctx.NoError(RunMigrations(ctx.GetDb(), ctx.stores, nil))
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

func (ctx *TestContext) RequireNewService(name string) *Service {
	edgeService := &Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
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

// CleanupAll resets the db to its post-migration state, deleting all test-created
// entities and restoring migration-seeded data such as identity types, well-known
// config types and the default auth policy.
func (ctx *TestContext) CleanupAll() {
	err := ctx.GetDb().Update(change.New().NewMutateContext(), func(mutateCtx boltz.MutateContext) error {
		tx := mutateCtx.Tx()
		if tx.Bucket([]byte(RootBucket)) != nil {
			if err := tx.DeleteBucket([]byte(RootBucket)); err != nil {
				return err
			}
		}
		root, err := tx.CreateBucketIfNotExists([]byte(RootBucket))
		if err != nil {
			return err
		}

		storeList := ctx.stores.getStoresForInit()
		sort.Slice(storeList, func(i, j int) bool {
			if storeList[i].IsChildStore() == storeList[j].IsChildStore() {
				return storeList[i].GetEntityType() < storeList[j].GetEntityType()
			}
			return !storeList[i].IsChildStore()
		})

		errorHolder := &errorz.ErrorHolderImpl{}
		for _, store := range storeList {
			store.initializeIndexes(tx, errorHolder)
		}
		if errorHolder.HasError() {
			return errorHolder.GetError()
		}

		// the db is fresh, so we can run the migration initialize step directly instead
		// of going through the migration manager
		migrations := &Migrations{stores: ctx.stores}
		step := &boltz.MigrationStep{
			Component: "edge",
			Ctx:       mutateCtx,
		}
		version := migrations.initialize(step)
		if step.HasError() {
			return step.GetError()
		}

		versionsBucket := boltz.NewTypedBucket(nil, root).GetOrCreateBucket("versions")
		versionsBucket.SetInt64(step.Component, int64(version), nil)
		return versionsBucket.GetError()
	})
	ctx.NoError(err)
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
