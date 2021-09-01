package persistence

import (
	"fmt"
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/pkg/errors"
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
	service := ctx.RequireNewService(eid.New())
	identity := ctx.RequireNewIdentity(eid.New(), false)

	clientConfigTypeId := ""
	err := ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		clientConfigTypeId = string(ctx.stores.ConfigType.GetNameIndex().Read(tx, []byte("ziti-tunneler-client.v1")))
		return nil
	})
	ctx.NoError(err)

	serverConfigTypeId := ""
	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		serverConfigTypeId = string(ctx.stores.ConfigType.GetNameIndex().Read(tx, []byte("ziti-tunneler-server.v1")))
		return nil
	})
	ctx.NoError(err)

	config := newConfig(eid.New(), clientConfigTypeId, map[string]interface{}{
		"hostname": "foo.yourcompany.com",
		"port":     int64(22),
	})
	ctx.RequireCreate(config)

	config2 := newConfig(eid.New(), clientConfigTypeId, map[string]interface{}{
		"hostname": "bar.yourcompany.com",
		"port":     int64(23),
	})
	ctx.RequireCreate(config2)

	config3 := newConfig(eid.New(), serverConfigTypeId, map[string]interface{}{
		"hostname": "baz.yourcompany.com",
		"port":     int64(24),
	})
	ctx.RequireCreate(config3)

	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		err := ctx.stores.Identity.AssignServiceConfigs(tx, identity.Id,
			ServiceConfig{ServiceId: service.Id, ConfigId: config.Id},
			ServiceConfig{ServiceId: service.Id, ConfigId: config2.Id})
		ctx.EqualError(err, fmt.Sprintf("multiple service configs provided for identity %v of config type %v", identity.Id, clientConfigTypeId))
		return nil
	})
	ctx.NoError(err)

	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		err := ctx.stores.Identity.AssignServiceConfigs(tx, identity.Id, ServiceConfig{ServiceId: service.Id, ConfigId: config.Id})
		ctx.NoError(err)

		serviceConfigs := ctx.getServiceConfigs(tx, identity.Id, "all")
		ctx.Equal(1, len(serviceConfigs))
		serviceMap, ok := serviceConfigs[service.Id]
		ctx.True(ok)
		ctx.Equal(1, len(serviceMap))
		_, ok = serviceMap[config.Type]
		ctx.True(ok)

		identityServices := ctx.getIdentityServices(tx, config.Id)
		ctx.Equal(1, len(identityServices))
		ctx.Equal(identity.Id, identityServices[0].identityId)
		ctx.Equal(service.Id, identityServices[0].serviceId)

		return nil
	})
	ctx.NoError(err)

	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		err := ctx.stores.Identity.RemoveServiceConfigs(tx, identity.Id, ServiceConfig{ServiceId: service.Id, ConfigId: config.Id})
		ctx.NoError(err)

		serviceConfigs := ctx.getServiceConfigs(tx, identity.Id, "all")
		ctx.Equal(0, len(serviceConfigs))

		identityServices := ctx.getIdentityServices(tx, config.Id)
		ctx.Equal(0, len(identityServices))
		return nil
	})
	ctx.NoError(err)

	ctx.RequireDelete(config)
	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		serviceConfigs := ctx.getServiceConfigs(tx, identity.Id, "all")
		ctx.Equal(0, len(serviceConfigs))
		return nil
	})
	ctx.NoError(err)

	ctx.RequireCreate(config)

	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		err := ctx.stores.Identity.AssignServiceConfigs(tx, identity.Id,
			ServiceConfig{ServiceId: service.Id, ConfigId: config.Id},
			ServiceConfig{ServiceId: service.Id, ConfigId: config3.Id})
		ctx.NoError(err)

		serviceConfigs := ctx.getServiceConfigs(tx, identity.Id, "all")
		ctx.Equal(1, len(serviceConfigs))
		serviceMap, ok := serviceConfigs[service.Id]
		ctx.True(ok)
		ctx.Equal(2, len(serviceMap))
		_, ok = serviceMap[config.Type]
		ctx.True(ok)
		_, ok = serviceMap[config3.Type]
		ctx.True(ok)

		serviceConfigs = ctx.getServiceConfigs(tx, identity.Id, serverConfigTypeId)
		ctx.Equal(1, len(serviceConfigs))
		serviceMap, ok = serviceConfigs[service.Id]
		ctx.True(ok)
		ctx.Equal(1, len(serviceMap))
		_, ok = serviceMap[config3.Type]
		ctx.True(ok)

		identityServices := ctx.getIdentityServices(tx, config.Id)
		ctx.Equal(1, len(identityServices))
		ctx.Equal(identity.Id, identityServices[0].identityId)
		ctx.Equal(service.Id, identityServices[0].serviceId)

		return nil
	})
	ctx.NoError(err)

	ctx.RequireDelete(identity)

	err = ctx.GetDb().Update(func(tx *bbolt.Tx) error {
		identityServices := ctx.getIdentityServices(tx, config.Id)
		ctx.Equal(0, len(identityServices))

		identityServices = ctx.getIdentityServices(tx, config3.Id)
		ctx.Equal(0, len(identityServices))
		return nil
	})

	ctx.NoError(err)
}

func (ctx *TestContext) getIdentityServices(tx *bbolt.Tx, configId string) []testIdentityServices {
	var result []testIdentityServices
	identityServiceSymbol := ctx.stores.Config.GetSymbol(FieldConfigIdentityService).(boltz.EntitySetSymbol)
	err := identityServiceSymbol.Map(tx, []byte(configId), func(ctx *boltz.MapContext) {
		decoded, err := boltz.DecodeStringSlice(ctx.Value())
		if err != nil {
			ctx.SetError(err)
			return
		}
		if len(decoded) != 2 {
			ctx.SetError(errors.Errorf("expected 2 fields, got %v", len(decoded)))
		}
		result = append(result, testIdentityServices{identityId: decoded[0], serviceId: decoded[1]})
	})
	ctx.NoError(err)
	return result
}

func (ctx *TestContext) getServiceConfigs(tx *bbolt.Tx, identityId string, configTypes ...string) map[string]map[string]map[string]interface{} {
	configTypeMap := map[string]struct{}{}
	for _, configType := range configTypes {
		configTypeMap[configType] = struct{}{}
	}
	return ctx.stores.Identity.LoadServiceConfigsByServiceAndType(tx, identityId, configTypeMap)
}

type testIdentityServices struct {
	identityId string
	serviceId  string
}
