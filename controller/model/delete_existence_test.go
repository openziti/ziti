/*
	Copyright NetFoundry Inc.

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

package model

import (
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
)

// Test_DeleteExistence covers the presence check that ApplyDelete runs inside the transaction: a
// delete only removes a record the deleting manager's own store can see, and under
// DeleteIfExists an absent target is a no-op rather than an error.
func Test_DeleteExistence(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("edge router delete with a transit router id is not found", ctx.testEdgeRouterDeleteOfTransitRouter)
	t.Run("transit router delete with an edge router id is not found", ctx.testTransitRouterDeleteOfEdgeRouter)
	t.Run("edge service delete of a fabric-only service is refused at apply", ctx.testEdgeServiceDeleteOfFabricOnlyRefusedAtApply)
	t.Run("DeleteIfExists skips an absent target", ctx.testDeleteIfExistsAbsent)
	t.Run("DeleteIfExists deletes a present target", ctx.testDeleteIfExistsPresent)
	t.Run("DeleteIfExists skips a fabric-only service through the edge manager", ctx.testDeleteIfExistsFabricOnly)
	t.Run("DeleteIfExists skips a transit router id through the edge router manager", ctx.testDeleteIfExistsTransitRouterId)
	t.Run("transit router delete of a fabric-created router succeeds", ctx.testTransitRouterDeleteOfBaseRouter)
	t.Run("DeleteIfExists deletes a fabric-created router through the transit router manager", ctx.testDeleteIfExistsBaseRouter)
	t.Run("DeleteIfExists on managers that bypass the command path", ctx.testDeleteIfExistsLocalOnlyManagers)
	t.Run("DeleteIfExists on identities keeps the admin precondition", ctx.testDeleteIfExistsIdentity)
}

// Identity's ApplyDelete adds the admin precondition and must still go through the base presence
// check, so an absent identity is a no-op while a protected target stays refused.
func (ctx *TestContext) testDeleteIfExistsIdentity(t *testing.T) {
	ctx.NoError(ctx.managers.Identity.DeleteIfExists(eid.New(), change.New()))

	identity := ctx.requireNewIdentity(false)
	ctx.NoError(ctx.managers.Identity.DeleteIfExists(identity.Id, change.New()))
	_, err := ctx.managers.Identity.Read(identity.Id)
	ctx.True(boltz.IsErrNotFoundErr(err), "identity should be deleted, got %v", err)

	nonAdmin := ctx.requireNewIdentity(false)
	admin := ctx.requireNewIdentity(true)
	requireUnauthorized(t, ctx.managers.Identity.DeleteIfExists(admin.Id, asIdentity(nonAdmin.Id)))
	_, err = ctx.managers.Identity.Read(admin.Id)
	ctx.NoError(err, "the refused delete left the target in place")
}

// requireNewBaseRouter creates a router the way the fabric does, with no transit router record.
func (ctx *TestContext) requireNewBaseRouter() *Router {
	router := &Router{Name: eid.New()}
	ctx.NoError(ctx.managers.Router.Create(router, change.New()))
	return router
}

// A router created through the fabric has no transit router record, but the transit router store
// is extended over the router store, so the transit router manager reads it as a base router and
// must delete it too.
func (ctx *TestContext) testTransitRouterDeleteOfBaseRouter(t *testing.T) {
	router := ctx.requireNewBaseRouter()

	read, err := ctx.managers.TransitRouter.Read(router.Id)
	ctx.NoError(err)
	ctx.True(read.IsBase)

	ctx.NoError(ctx.managers.TransitRouter.Delete(router.Id, change.New()))
	_, err = ctx.managers.Router.Read(router.Id)
	ctx.True(boltz.IsErrNotFoundErr(err), "the base router should be deleted, got %v", err)
}

func (ctx *TestContext) testDeleteIfExistsBaseRouter(t *testing.T) {
	router := ctx.requireNewBaseRouter()

	ctx.NoError(ctx.managers.TransitRouter.DeleteIfExists(router.Id, change.New()))
	_, err := ctx.managers.Router.Read(router.Id)
	ctx.True(boltz.IsErrNotFoundErr(err), "the base router should be deleted, got %v", err)
}

// API sessions, sessions and API session certificates are written straight to the database and
// register no delete decoder, so DeleteIfExists must not dispatch a command for them. The test
// context's dispatcher encodes and decodes, which is where a dispatched delete would fail.
func (ctx *TestContext) testDeleteIfExistsLocalOnlyManagers(t *testing.T) {
	ctx.requireNewEdgeRouter()
	identity := ctx.requireNewIdentity(false)
	service := ctx.requireNewService()
	ctx.requireNewServicePolicy(db.PolicyTypeDialName, ss("#all"), ss("#all"))
	ctx.requireNewEdgeRouterPolicy(ss("#all"), ss("#all"))
	ctx.requireNewServiceNewEdgeRouterPolicy(ss("#all"), ss("#all"))

	apiSession := ctx.requireNewApiSession(identity)
	session := ctx.requireNewSession(apiSession, service.Id, db.SessionTypeDial)
	certId, err := ctx.managers.ApiSessionCertificate.Create(&ApiSessionCertificate{
		ApiSessionId: apiSession.Id,
		Subject:      eid.New(),
		Fingerprint:  eid.New(),
		PEM:          eid.New(),
	}, change.New())
	ctx.NoError(err)

	ctx.NoError(ctx.managers.ApiSessionCertificate.DeleteIfExists(eid.New(), change.New()))
	ctx.NoError(ctx.managers.ApiSessionCertificate.DeleteIfExists(certId, change.New()))
	_, err = ctx.managers.ApiSessionCertificate.Read(certId)
	ctx.True(boltz.IsErrNotFoundErr(err), "api session certificate should be deleted, got %v", err)

	ctx.NoError(ctx.managers.Session.DeleteIfExists(eid.New(), change.New()))
	ctx.NoError(ctx.managers.Session.DeleteIfExists(session.Id, change.New()))
	_, err = ctx.managers.Session.Read(session.Id)
	ctx.True(boltz.IsErrNotFoundErr(err), "session should be deleted, got %v", err)

	ctx.NoError(ctx.managers.ApiSession.DeleteIfExists(eid.New(), change.New()))
	ctx.NoError(ctx.managers.ApiSession.DeleteIfExists(apiSession.Id, change.New()))
	_, err = ctx.managers.ApiSession.Read(apiSession.Id)
	ctx.True(boltz.IsErrNotFoundErr(err), "api session should be deleted, got %v", err)
}

func (ctx *TestContext) testDeleteIfExistsAbsent(t *testing.T) {
	ctx.NoError(ctx.managers.EdgeService.DeleteIfExists(eid.New(), change.New()))
	ctx.NoError(ctx.managers.EdgeRouter.DeleteIfExists(eid.New(), change.New()))
}

func (ctx *TestContext) testDeleteIfExistsPresent(t *testing.T) {
	service := ctx.requireNewService()
	edgeRouter := ctx.requireNewEdgeRouter()

	ctx.NoError(ctx.managers.EdgeService.DeleteIfExists(service.Id, change.New()))
	_, err := ctx.managers.EdgeService.Read(service.Id)
	ctx.True(boltz.IsErrNotFoundErr(err), "service should be deleted, got %v", err)

	ctx.NoError(ctx.managers.EdgeRouter.DeleteIfExists(edgeRouter.Id, change.New()))
	_, err = ctx.managers.Router.Read(edgeRouter.Id)
	ctx.True(boltz.IsErrNotFoundErr(err), "edge router should be deleted, got %v", err)
}

func (ctx *TestContext) testDeleteIfExistsFabricOnly(t *testing.T) {
	fabricSvc := ctx.requireNewFabricService()

	ctx.NoError(ctx.managers.EdgeService.DeleteIfExists(fabricSvc.Id, change.New()))

	svc, err := ctx.managers.Service.Read(fabricSvc.Id)
	ctx.NoError(err, "the fabric service must survive an edge DeleteIfExists")
	ctx.NotNil(svc)
}

func (ctx *TestContext) testDeleteIfExistsTransitRouterId(t *testing.T) {
	transitRouter := ctx.requireNewTransitRouter()

	ctx.NoError(ctx.managers.EdgeRouter.DeleteIfExists(transitRouter.Id, change.New()))

	_, err := ctx.managers.TransitRouter.Read(transitRouter.Id)
	ctx.NoError(err, "the transit router must survive an edge router DeleteIfExists of its id")
}

func (ctx *TestContext) requireNewTransitRouter() *TransitRouter {
	transitRouter := &TransitRouter{Name: eid.New()}
	ctx.NoError(ctx.managers.TransitRouter.Create(transitRouter, change.New()))
	return transitRouter
}

// Edge routers and transit routers are both child stores of the router store, and a child
// store's DeleteById delegates to the parent. Without a typed presence check first, an edge
// router delete given a transit router's id removes the transit router.
func (ctx *TestContext) testEdgeRouterDeleteOfTransitRouter(t *testing.T) {
	transitRouter := ctx.requireNewTransitRouter()

	err := ctx.managers.EdgeRouter.Delete(transitRouter.Id, change.New())
	ctx.True(boltz.IsErrNotFoundErr(err), "edge router delete of a transit router id should be NotFound, got %v", err)

	_, err = ctx.managers.TransitRouter.Read(transitRouter.Id)
	ctx.NoError(err, "the transit router must survive an edge router delete of its id")
	_, err = ctx.managers.Router.Read(transitRouter.Id)
	ctx.NoError(err)
}

func (ctx *TestContext) testTransitRouterDeleteOfEdgeRouter(t *testing.T) {
	edgeRouter := ctx.requireNewEdgeRouter()

	err := ctx.managers.TransitRouter.Delete(edgeRouter.Id, change.New())
	ctx.True(boltz.IsErrNotFoundErr(err), "transit router delete of an edge router id should be NotFound, got %v", err)

	_, err = ctx.managers.EdgeRouter.Read(edgeRouter.Id)
	ctx.NoError(err, "the edge router must survive a transit router delete of its id")
	_, err = ctx.managers.Router.Read(edgeRouter.Id)
	ctx.NoError(err)
}

// A delete forwarded from another controller arrives as a decoded command and skips every check
// the serving controller's Delete method runs before dispatch, so the fabric-only guard has to
// hold at apply time. Dispatching the command directly is the same path.
func (ctx *TestContext) testEdgeServiceDeleteOfFabricOnlyRefusedAtApply(t *testing.T) {
	fabricSvc := ctx.requireNewFabricService()

	cmd := &command.DeleteEntityCommand{
		Context: change.New(),
		Deleter: ctx.managers.EdgeService,
		Id:      fabricSvc.Id,
	}
	err := ctx.GetCommandDispatcher().Dispatch(cmd)
	ctx.True(boltz.IsErrNotFoundErr(err), "applied edge delete of a fabric service should be NotFound, got %v", err)

	svc, err := ctx.managers.Service.Read(fabricSvc.Id)
	ctx.NoError(err)
	ctx.NotNil(svc)
}
