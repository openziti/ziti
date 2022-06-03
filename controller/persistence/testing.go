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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/controller/xt_smartrouting"
	tests "github.com/openziti/fabric/tests"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"testing"
	"time"
)

type testConfig struct {
	ctx             *TestContext
	options         *network.Options
	metricsRegistry metrics.Registry
	versionProvider common.VersionProvider
}

func newTestConfig(ctx *TestContext) *testConfig {
	options := network.DefaultOptions()
	options.MinRouterCost = 0

	return &testConfig{
		ctx:             ctx,
		options:         options,
		metricsRegistry: metrics.NewRegistry("test", nil),
		versionProvider: tests.NewVersionProviderTest(),
	}
}

func (self *testConfig) GetId() *identity.TokenId {
	return &identity.TokenId{Token: "test"}
}

func (self *testConfig) GetMetricsRegistry() metrics.Registry {
	return self.metricsRegistry
}

func (self *testConfig) GetOptions() *network.Options {
	return self.options
}

func (self *testConfig) GetCommandDispatcher() command.Dispatcher {
	return nil
}

func (self *testConfig) GetDb() boltz.Db {
	return self.ctx.GetDb()
}

func (self *testConfig) GetMetricsConfig() *metrics.Config {
	return &metrics.Config{
		Source:         "test",
		ReportInterval: time.Minute,
	}
}

func (self *testConfig) GetVersionProvider() common.VersionProvider {
	return self.versionProvider
}

func (self *testConfig) GetCloseNotify() <-chan struct{} {
	return self.ctx.closeNotify
}

type testDbProvider struct {
	ctx *TestContext
}

func (p *testDbProvider) GetDb() boltz.Db {
	return p.ctx.GetDb()
}

func (p *testDbProvider) GetStores() *db.Stores {
	return p.ctx.n.GetStores()
}

func (p *testDbProvider) GetServiceCache() network.Cache {
	return p
}

func (p *testDbProvider) NotifyRouterRenamed(_, _ string) {}

func (p *testDbProvider) RemoveFromCache(_ string) {
}

func (p *testDbProvider) GetManagers() *network.Managers {
	return p.ctx.n.Managers
}

type TestContext struct {
	boltz.BaseTestContext
	db          *db.Db
	n           *network.Network
	stores      *Stores
	closeNotify chan struct{}
}

func NewTestContext(t *testing.T) *TestContext {
	xt.GlobalRegistry().RegisterFactory(xt_smartrouting.NewFactory())

	result := &TestContext{
		BaseTestContext: *boltz.NewTestContext(t),
		closeNotify:     make(chan struct{}, 1),
	}
	result.Impl = result
	return result
}

func (ctx *TestContext) GetNetwork() *network.Network {
	return ctx.n
}

func (ctx *TestContext) Cleanup() {
	close(ctx.closeNotify)
	ctx.BaseTestContext.Cleanup()
}

func (ctx *TestContext) GetStores() *Stores {
	return ctx.stores
}

func (ctx *TestContext) GetDb() boltz.Db {
	return ctx.db
}

func (ctx *TestContext) GetStoreForEntity(entity boltz.Entity) boltz.CrudStore {
	if _, ok := entity.(*db.Service); ok {
		return ctx.n.GetStores().Service
	}
	return ctx.stores.GetStoreForEntity(entity)
}

func (ctx *TestContext) GetDbProvider() DbProvider {
	return &testDbProvider{ctx: ctx}
}

func (ctx *TestContext) Init() {
	ctx.BaseTestContext.InitDbFile()
	ctx.InitWithDbFile(ctx.GetDbFile().Name())
}

func (ctx *TestContext) InitWithDbFile(path string) {
	if path == "" {
		ctx.BaseTestContext.InitDbFile()
		path = ctx.GetDbFile().Name()
	}

	var err error
	ctx.db, err = db.Open(path)
	ctx.NoError(err)

	dbProvider := ctx.GetDbProvider()

	config := newTestConfig(ctx)
	ctx.n, err = network.NewNetwork(config)
	ctx.NoError(err)

	// TODO: setup up single node raft cluster or mock?
	ctx.stores, err = NewBoltStores(dbProvider)
	ctx.NoError(err)

	ctx.NoError(RunMigrations(ctx.GetDb(), ctx.stores))

	ctx.NoError(ctx.stores.EventualEventer.Start(ctx.closeNotify))

}

func (ctx *TestContext) requireNewServicePolicy(policyType PolicyType, identityRoles []string, serviceRoles []string) *ServicePolicy {
	entity := &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		PolicyType:    policyType,
		Semantic:      SemanticAnyOf,
		IdentityRoles: identityRoles,
		ServiceRoles:  serviceRoles,
	}
	ctx.RequireCreate(entity)
	return entity
}

func (ctx *TestContext) RequireNewIdentity(name string, isAdmin bool) *Identity {
	identity := &Identity{
		BaseExtEntity: *boltz.NewExtEntity(eid.New(), nil),
		Name:          name,
		IsAdmin:       isAdmin,
	}
	ctx.RequireCreate(identity)
	return identity
}

func (ctx *TestContext) RequireNewService(name string) *EdgeService {
	edgeService := &EdgeService{
		Service: db.Service{
			BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
			Name:          name,
		},
	}
	ctx.RequireCreate(edgeService)
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
	stores := []boltz.CrudStore{
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
	_ = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		mutateContext := boltz.NewMutateContext(tx)
		for _, store := range stores {
			if err := store.DeleteWhere(mutateContext, `true limit none`); err != nil {
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
