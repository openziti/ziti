package db

import (
	"github.com/google/go-cmp/cmp"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/change"
	"go.etcd.io/bbolt"
	"testing"
)

func Test_IdentityStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test identity service configs", ctx.testIdentityServiceConfigs)
}

func (ctx *TestContext) testIdentityServiceConfigs(_ *testing.T) {
	service1 := ctx.RequireNewService(eid.New())
	service2 := ctx.RequireNewService(eid.New())
	identity := ctx.RequireNewIdentity(eid.New(), false)
	boltztest.RequireReload(ctx, identity)
	boltztest.ValidateBaseline(ctx, identity)

	clientConfigTypeId := ""
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		clientConfigTypeId = string(ctx.stores.ConfigType.GetNameIndex().Read(tx, []byte("intercept.v1")))
		return nil
	})
	ctx.NoError(err)

	serverConfigTypeId := ""
	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		serverConfigTypeId = string(ctx.stores.ConfigType.GetNameIndex().Read(tx, []byte("host.v1")))
		return nil
	})
	ctx.NoError(err)

	config := newConfig(eid.New(), clientConfigTypeId, map[string]interface{}{
		"hostname": "foo.yourcompany.com",
		"port":     int64(22),
	})
	boltztest.RequireCreate(ctx, config)

	config2 := newConfig(eid.New(), clientConfigTypeId, map[string]interface{}{
		"hostname": "bar.yourcompany.com",
		"port":     int64(23),
	})
	boltztest.RequireCreate(ctx, config2)

	config3 := newConfig(eid.New(), serverConfigTypeId, map[string]interface{}{
		"hostname": "baz.yourcompany.com",
		"port":     int64(24),
	})
	boltztest.RequireCreate(ctx, config3)

	mutateCtx := change.New().NewMutateContext()
	err = ctx.GetDb().Update(mutateCtx, func(mutateCtx boltz.MutateContext) error {
		identity.ServiceConfigs = map[string]map[string]string{
			service1.Id: {
				config.Type:  config.Id,
				config3.Type: config3.Id,
			},
		}
		return ctx.stores.Identity.Update(mutateCtx, identity, boltz.MapFieldChecker{
			FieldIdentityServiceConfigs: struct{}{},
		})
	})
	ctx.NoError(err)
	ctx.validateServiceConfigs(identity)

	mutateCtx = change.New().NewMutateContext()
	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		serviceConfigs := ctx.getServiceConfigs(tx, identity.Id, "all")
		ctx.Equal(1, len(serviceConfigs))
		serviceMap, ok := serviceConfigs[service1.Id]
		ctx.True(ok)
		ctx.Equal(2, len(serviceMap))
		cfg, ok := serviceMap[config.Type]
		ctx.True(ok)
		ctx.Equal(config.Data, cfg)

		cfg, ok = serviceMap[config3.Type]
		ctx.True(ok)
		ctx.Equal(config3.Data, cfg)

		return nil
	})
	ctx.NoError(err)

	err = ctx.GetDb().Update(mutateCtx, func(mutateCtx boltz.MutateContext) error {
		identity.ServiceConfigs = map[string]map[string]string{
			service2.Id: {
				config.Type:  config.Id,
				config3.Type: config3.Id,
			},
		}

		return ctx.stores.Identity.Update(mutateCtx, identity, boltz.MapFieldChecker{
			FieldIdentityServiceConfigs: struct{}{},
		})
	})

	ctx.NoError(err)
	ctx.validateServiceConfigs(identity)

	mutateCtx = change.New().NewMutateContext()
	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		serviceConfigs := ctx.getServiceConfigs(tx, identity.Id, "all")
		ctx.Equal(1, len(serviceConfigs))
		serviceMap, ok := serviceConfigs[service2.Id]
		ctx.True(ok)
		ctx.Equal(2, len(serviceMap))
		cfg, ok := serviceMap[config.Type]
		ctx.True(ok)
		ctx.Equal(config.Data, cfg)

		cfg, ok = serviceMap[config3.Type]
		ctx.True(ok)
		ctx.Equal(config3.Data, cfg)

		return nil
	})

	ctx.NoError(err)

	err = ctx.GetDb().Update(mutateCtx, func(mutateCtx boltz.MutateContext) error {
		identity.ServiceConfigs = map[string]map[string]string{
			service2.Id: {
				config.Type: config2.Id,
			},
		}

		return ctx.stores.Identity.Update(mutateCtx, identity, boltz.MapFieldChecker{
			FieldIdentityServiceConfigs: struct{}{},
		})
	})

	ctx.NoError(err)
	ctx.validateServiceConfigs(identity)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		serviceConfigs := ctx.getServiceConfigs(tx, identity.Id, "all")
		ctx.Equal(1, len(serviceConfigs))
		serviceMap, ok := serviceConfigs[service2.Id]
		ctx.True(ok)
		ctx.Equal(1, len(serviceMap))
		cfg, ok := serviceMap[config.Type]
		ctx.True(ok)
		ctx.Equal(config2.Data, cfg)

		return nil
	})

	ctx.NoError(err)

}

func (ctx *TestContext) validateServiceConfigs(identity *Identity) {
	compareIdentity := &Identity{
		BaseExtEntity: boltz.BaseExtEntity{
			Id: identity.Id,
		},
	}
	boltztest.RequireReload(ctx, compareIdentity)
	ctx.Require().True(cmp.Equal(identity.ServiceConfigs, compareIdentity.ServiceConfigs),
		cmp.Diff(identity.ServiceConfigs, compareIdentity.ServiceConfigs))

}

func (ctx *TestContext) getServiceConfigs(tx *bbolt.Tx, identityId string, configTypes ...string) map[string]map[string]map[string]interface{} {
	configTypeMap := map[string]struct{}{}
	for _, configType := range configTypes {
		configTypeMap[configType] = struct{}{}
	}
	return ctx.stores.Identity.LoadServiceConfigsByServiceAndType(tx, identityId, configTypeMap)
}
