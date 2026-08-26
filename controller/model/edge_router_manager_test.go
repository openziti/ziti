package model

import (
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"go.etcd.io/bbolt"
)

func TestEdgeRouterManager(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test get edge routers for service and identity", ctx.testGetEdgeRoutersForServiceAndIdentity)
}

func (ctx *TestContext) testGetEdgeRoutersForServiceAndIdentity(*testing.T) {
	edgeRouter := ctx.requireNewEdgeRouter()
	edgeRouter2 := ctx.requireNewEdgeRouter()
	identity := ctx.requireNewIdentity(false)
	service := ctx.requireNewService()
	service.RoleAttributes = []string{eid.New()}
	ctx.NoError(ctx.managers.EdgeService.Update(service, nil, change.New()))

	ctx.requireNewEdgeRouterPolicy(ss("#all"), ss("#all"))

	// test default case, with no limits on service
	ctx.False(ctx.isEdgeRouterAccessible(edgeRouter.Id, identity.Id, service.Id))
	ctx.False(ctx.isEdgeRouterAccessible(edgeRouter2.Id, identity.Id, service.Id))
	ctx.False(ctx.managers.EdgeRouter.IsSharedEdgeRouterPresent(identity.Id, service.Id))

	serp := ctx.requireNewServiceNewEdgeRouterPolicy(ss("@"+service.Id), ss("#"+eid.New()))

	// should not be accessible if we limit to a role no one has
	ctx.False(ctx.isEdgeRouterAccessible(edgeRouter.Id, identity.Id, service.Id))
	ctx.False(ctx.isEdgeRouterAccessible(edgeRouter2.Id, identity.Id, service.Id))
	ctx.False(ctx.managers.EdgeRouter.IsSharedEdgeRouterPresent(identity.Id, service.Id))

	serp.EdgeRouterRoles = []string{"@" + edgeRouter.Id}
	ctx.NoError(ctx.managers.ServiceEdgeRouterPolicy.Update(serp, nil, change.New()))

	// should be accessible if we limit to our specific router
	ctx.True(ctx.isEdgeRouterAccessible(edgeRouter.Id, identity.Id, service.Id))
	ctx.False(ctx.isEdgeRouterAccessible(edgeRouter2.Id, identity.Id, service.Id))
	ctx.True(ctx.managers.EdgeRouter.IsSharedEdgeRouterPresent(identity.Id, service.Id))

}

// TestEdgeRouterAccess uses its own context so that the policies it creates are the only ones
// in play, letting it assert on each policy link independently.
func TestEdgeRouterAccess(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	edgeRouter := ctx.requireNewEdgeRouter()
	identity := ctx.requireNewIdentity(false)
	service := ctx.requireNewService()

	requireAccess := func(identityAllowed, serviceAllowed bool) {
		access, err := ctx.managers.EdgeRouter.GetEdgeRouterAccess(identity.Id, service.Id, edgeRouter.Id)
		ctx.NoError(err)
		ctx.Equal(identityAllowed, access.IdentityAllowed)
		ctx.Equal(serviceAllowed, access.ServiceAllowed)
		ctx.Equal(identityAllowed && serviceAllowed, access.IsAllowed())
	}

	requireAccess(false, false)

	ctx.requireNewEdgeRouterPolicy(ss("#all"), ss("#all"))
	requireAccess(true, false)

	ctx.requireNewServiceNewEdgeRouterPolicy(ss("#all"), ss("#all"))
	requireAccess(true, true)
}

func (ctx *TestContext) isEdgeRouterAccessible(edgeRouterId, identityId, serviceId string) bool {
	found := false
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		result, err := ctx.managers.EdgeRouter.ListForIdentityAndServiceWithTx(tx, identityId, serviceId)
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

	access, err := ctx.managers.EdgeRouter.GetEdgeRouterAccess(identityId, serviceId, edgeRouterId)
	ctx.NoError(err)
	ctx.Equal(found, access.IsAllowed())

	return found
}
