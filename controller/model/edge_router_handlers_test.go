package model

import (
	"github.com/google/uuid"
	"go.etcd.io/bbolt"
	"testing"
)

func TestEdgeRouterHandler(t *testing.T) {
	ctx := newTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test get edge routers for service and identity", ctx.testGetEdgeRoutersForServiceAndIdentity)
}

func (ctx *TestContext) testGetEdgeRoutersForServiceAndIdentity(*testing.T) {
	edgeRouter := ctx.requireNewEdgeRouter()
	edgeRouter2 := ctx.requireNewEdgeRouter()
	identity := ctx.requireNewIdentity(false)
	service := ctx.requireNewService()
	service.RoleAttributes = []string{uuid.New().String()}
	ctx.NoError(ctx.handlers.Service.Update(service))

	ctx.requireNewEdgeRouterPolicy(ss("#all"), ss("#all"))

	// test default case, with no limits on service
	ctx.True(ctx.isEdgeRouterAccessible(edgeRouter.Id, identity.Id, service.Id))
	ctx.True(ctx.isEdgeRouterAccessible(edgeRouter2.Id, identity.Id, service.Id))

	service.EdgeRouterRoles = []string{"#" + uuid.New().String()}
	ctx.NoError(ctx.handlers.Service.Update(service))

	// should not be accessible if we limit to a role no one has
	ctx.False(ctx.isEdgeRouterAccessible(edgeRouter.Id, identity.Id, service.Id))
	ctx.False(ctx.isEdgeRouterAccessible(edgeRouter2.Id, identity.Id, service.Id))

	service.EdgeRouterRoles = []string{"@" + edgeRouter.Id}
	ctx.NoError(ctx.handlers.Service.Update(service))

	// should be accessible if we limit to our specific router
	ctx.True(ctx.isEdgeRouterAccessible(edgeRouter.Id, identity.Id, service.Id))
	ctx.False(ctx.isEdgeRouterAccessible(edgeRouter2.Id, identity.Id, service.Id))
}

func (ctx *TestContext) isEdgeRouterAccessible(edgeRouterId, identityId, serviceId string) bool {
	found := false
	err := ctx.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		result, err := ctx.handlers.EdgeRouter.ListForIdentityAndServiceWithTx(tx, identityId, serviceId, nil)
		if err != nil {
			return err
		}
		for _, er := range result.EdgeRouters {
			if er.Id == edgeRouterId {
				found = true
				break
			}
		}
		return nil
	})
	ctx.NoError(err)
	return found
}
