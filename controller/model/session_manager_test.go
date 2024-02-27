package model

import (
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestSessionManager(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test get edge routers for service and identity", ctx.testSessionIdempotency)
}

func (ctx *TestContext) testSessionIdempotency(t *testing.T) {
	ctx.requireNewEdgeRouter()
	identity := ctx.requireNewIdentity(false)
	service := ctx.requireNewService()
	service2 := ctx.requireNewService()
	service.RoleAttributes = []string{eid.New()}
	ctx.NoError(ctx.managers.EdgeService.Update(service, nil, change.New()))

	ctx.requireNewServicePolicy(db.PolicyTypeDialName, ss("#all"), ss("#all"))
	ctx.requireNewServicePolicy(db.PolicyTypeBindName, ss("#all"), ss("#all"))
	ctx.requireNewEdgeRouterPolicy(ss("#all"), ss("#all"))
	ctx.requireNewServiceNewEdgeRouterPolicy(ss("#all"), ss("#all"))

	apiSession := ctx.requireNewApiSession(identity)
	sessSvc1Dial := ctx.requireNewSession(apiSession, service.Id, db.SessionTypeDial)
	sessSvc1Bind := ctx.requireNewSession(apiSession, service.Id, db.SessionTypeBind)

	req := require.New(t)
	req.NotEqual(sessSvc1Dial.Id, sessSvc1Bind.Id)

	sessSvc1Dial2 := ctx.requireNewSession(apiSession, service.Id, db.SessionTypeDial)
	sessSvc1Bind2 := ctx.requireNewSession(apiSession, service.Id, db.SessionTypeBind)

	req.Equal(sessSvc1Dial.Id, sessSvc1Dial2.Id)
	req.Equal(sessSvc1Bind.Id, sessSvc1Bind2.Id)

	sessSvc2Dial1 := ctx.requireNewSession(apiSession, service2.Id, db.SessionTypeDial)
	sessSvc2Bind1 := ctx.requireNewSession(apiSession, service2.Id, db.SessionTypeBind)

	req.NotEqual(sessSvc1Dial2.Id, sessSvc1Bind2.Id)
	req.NotEqual(sessSvc1Dial.Id, sessSvc2Dial1.Id)
	req.NotEqual(sessSvc1Dial.Id, sessSvc2Bind1.Id)
	req.NotEqual(sessSvc1Dial2.Id, sessSvc2Dial1.Id)
	req.NotEqual(sessSvc1Dial2.Id, sessSvc2Bind1.Id)
	req.NotEqual(sessSvc1Bind.Id, sessSvc2Dial1.Id)
	req.NotEqual(sessSvc1Bind.Id, sessSvc2Bind1.Id)
	req.NotEqual(sessSvc1Bind2.Id, sessSvc2Dial1.Id)
	req.NotEqual(sessSvc1Bind2.Id, sessSvc2Bind1.Id)

	sessSvc2Dial2 := ctx.requireNewSession(apiSession, service2.Id, db.SessionTypeDial)
	sessSvc2Bind2 := ctx.requireNewSession(apiSession, service2.Id, db.SessionTypeBind)

	req.Equal(sessSvc2Dial1.Id, sessSvc2Dial2.Id)
	req.Equal(sessSvc2Bind1.Id, sessSvc2Bind2.Id)

	sessSvc1Dial3 := ctx.requireNewSession(apiSession, service.Id, db.SessionTypeDial)
	sessSvc1Bind3 := ctx.requireNewSession(apiSession, service.Id, db.SessionTypeBind)

	req.Equal(sessSvc1Dial.Id, sessSvc1Dial3.Id)
	req.Equal(sessSvc1Bind.Id, sessSvc1Bind3.Id)

	req.NoError(ctx.managers.ApiSession.Delete(apiSession.Id, change.New()))
	done, err := ctx.GetStores().EventualEventer.Trigger()
	ctx.NoError(err)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		ctx.Fail("did not receive done notification from eventual eventer")
	}

	boltztest.ValidateDeleted(ctx, apiSession.Id)
	boltztest.ValidateDeleted(ctx, sessSvc1Dial.Id)
	boltztest.ValidateDeleted(ctx, sessSvc1Dial2.Id)
	boltztest.ValidateDeleted(ctx, sessSvc1Bind.Id)
	boltztest.ValidateDeleted(ctx, sessSvc1Bind2.Id)
}
