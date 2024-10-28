package model

import (
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"testing"
)

func TestIdentityManager(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test identity config overrides service delete", ctx.testIdentityConfigOverridesServiceDelete)
	t.Run("test identity config overrides identity delete", ctx.testIdentityConfigOverridesIdentityDelete)
}

func (ctx *TestContext) testIdentityConfigOverridesServiceDelete(t *testing.T) {
	ctx.requireNewEdgeRouter()
	identity := ctx.requireNewIdentity(false)

	cfg1 := ctx.requireNewConfig("host.v1", map[string]any{
		"address":  "localhost",
		"port":     8080,
		"protocol": "tcp",
	})

	service := ctx.requireNewService(cfg1.Id)
	service.RoleAttributes = []string{eid.New()}
	ctx.NoError(ctx.managers.EdgeService.Update(service, nil, change.New()))

	ctx.requireNewServicePolicy(db.PolicyTypeDialName, ss("#all"), ss("#all"))
	ctx.requireNewServicePolicy(db.PolicyTypeBindName, ss("#all"), ss("#all"))
	ctx.requireNewEdgeRouterPolicy(ss("#all"), ss("#all"))
	ctx.requireNewServiceNewEdgeRouterPolicy(ss("#all"), ss("#all"))

	cfg2 := ctx.requireNewConfig("host.v1", map[string]any{
		"address":  "localhost",
		"port":     8080,
		"protocol": "tcp",
	})

	err := ctx.managers.Identity.AssignServiceConfigs(identity.Id, []ServiceConfig{
		{
			Service: service.Id,
			Config:  cfg2.Id,
		},
	}, change.New())
	ctx.NoError(err)

	query, err := ast.Parse(ctx.GetStores().EdgeService, "true")
	ctx.NoError(err)

	result, err := ctx.managers.EdgeService.PublicQueryForIdentity(identity, map[string]struct{}{"all": {}}, query)
	ctx.NoError(err)
	ctx.Equal(1, len(result.Services))

	err = ctx.managers.Config.Delete(cfg1.Id, change.New())
	ctx.Nil(err)

	err = ctx.managers.Config.Delete(cfg2.Id, change.New())
	ctx.NotNil(err)

	err = ctx.managers.EdgeService.Delete(service.Id, change.New())
	ctx.NoError(err)

	err = ctx.managers.Config.Delete(cfg2.Id, change.New())
	ctx.NoError(err)

	boltztest.ValidateDeleted(ctx, service.Id)
	boltztest.ValidateDeleted(ctx, cfg1.Id)
	boltztest.ValidateDeleted(ctx, cfg2.Id)
}

func (ctx *TestContext) testIdentityConfigOverridesIdentityDelete(t *testing.T) {
	ctx.requireNewEdgeRouter()
	identity := ctx.requireNewIdentity(false)

	cfg1 := ctx.requireNewConfig("host.v1", map[string]any{
		"address":  "localhost",
		"port":     8080,
		"protocol": "tcp",
	})

	service := ctx.requireNewService(cfg1.Id)
	service.RoleAttributes = []string{eid.New()}
	ctx.NoError(ctx.managers.EdgeService.Update(service, nil, change.New()))

	ctx.requireNewServicePolicy(db.PolicyTypeDialName, ss("#all"), ss("#all"))
	ctx.requireNewServicePolicy(db.PolicyTypeBindName, ss("#all"), ss("#all"))
	ctx.requireNewEdgeRouterPolicy(ss("#all"), ss("#all"))
	ctx.requireNewServiceNewEdgeRouterPolicy(ss("#all"), ss("#all"))

	cfg2 := ctx.requireNewConfig("host.v1", map[string]any{
		"address":  "localhost",
		"port":     8080,
		"protocol": "tcp",
	})

	err := ctx.managers.Identity.AssignServiceConfigs(identity.Id, []ServiceConfig{
		{
			Service: service.Id,
			Config:  cfg2.Id,
		},
	}, change.New())
	ctx.NoError(err)

	query, err := ast.Parse(ctx.GetStores().EdgeService, "true")
	ctx.NoError(err)

	result, err := ctx.managers.EdgeService.PublicQueryForIdentity(identity, map[string]struct{}{"all": {}}, query)
	ctx.NoError(err)
	ctx.Equal(1, len(result.Services))

	err = ctx.managers.Config.Delete(cfg1.Id, change.New())
	ctx.Nil(err)

	err = ctx.managers.Config.Delete(cfg2.Id, change.New())
	ctx.NotNil(err)

	err = ctx.managers.Identity.Delete(identity.Id, change.New())
	ctx.NoError(err)

	err = ctx.managers.Config.Delete(cfg2.Id, change.New())
	ctx.NoError(err)

	err = ctx.managers.EdgeService.Delete(service.Id, change.New())
	ctx.NoError(err)

	boltztest.ValidateDeleted(ctx, service.Id)
	boltztest.ValidateDeleted(ctx, identity.Id)
	boltztest.ValidateDeleted(ctx, cfg1.Id)
	boltztest.ValidateDeleted(ctx, cfg2.Id)
}
