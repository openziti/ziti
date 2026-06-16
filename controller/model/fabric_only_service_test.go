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
	"sync"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/fields"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/storage/ast"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
)

// Test_FabricOnlyServiceBehavior verifies the durable external behavior of the fabric/edge service
// split after the store collapse: fabric-only services are invisible through the edge service API
// (read/by-name/delete/list), the fabric API can update a service without clobbering its edge
// fields, and the edge API cannot update a fabric-only service. These guards are permanent
// production behavior (not migration logic), so they belong in CI long-term.
func Test_FabricOnlyServiceBehavior(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	t.Run("edge manager hides fabric-only on read/by-name/delete", ctx.testEdgeManagerHidesFabricOnly)
	t.Run("fabric create applies secure encryption default", ctx.testFabricCreateAppliesEncryptionDefault)
	t.Run("edge query excludes fabric-only; fabric manager sees both", ctx.testEdgeQueryExcludesFabricOnly)
	t.Run("fabric update preserves edge fields", ctx.testFabricUpdatePreservesEdgeFields)
	t.Run("fabric patch preserves edge fields", ctx.testFabricPatchPreservesEdgeFields)
	t.Run("edge update of fabric-only is rejected", ctx.testEdgeUpdateOfFabricOnlyRejected)
	t.Run("edge update emits ServiceUpdated; fabric update does not", ctx.testServiceUpdateEvents)
	t.Run("identity service-config override rejected for fabric-only service", ctx.testServiceConfigOverrideRejectedForFabricOnly)
	t.Run("edge association list (terminators etc.) rejects fabric-only service", ctx.testEdgeAssociationListRejectsFabricOnly)
}

// confirming-round C1: the edge service association routes (terminators, service-policies, SERPs,
// configs) go through PreparedListAssociatedWithHandler, which must also report a fabric-only
// service as absent -- otherwise its associations leak through the edge API even though Read 404s.
func (ctx *TestContext) testEdgeAssociationListRejectsFabricOnly(t *testing.T) {
	fabricSvc := ctx.requireNewFabricService()
	edgeSvc := ctx.requireNewService()
	query, err := ast.Parse(ctx.GetStores().Service, "true limit none")
	ctx.NoError(err)
	noop := func(*bbolt.Tx, []string, *models.QueryMetaData) error { return nil }

	err = ctx.managers.EdgeService.PreparedListAssociatedWithHandler(fabricSvc.Id, db.EntityTypeTerminators, query, noop)
	ctx.True(boltz.IsErrNotFoundErr(err), "edge association list of a fabric service should be NotFound, got %v", err)

	// positive control: edge service association list succeeds
	err = ctx.managers.EdgeService.PreparedListAssociatedWithHandler(edgeSvc.Id, db.EntityTypeTerminators, query, noop)
	ctx.NoError(err)
}

// C1 (final review): assigning an identity service-config override to a fabric-only service must be
// rejected. After the store collapse, identityServicesLinks resolves against the unified service
// store, so without a guard a fabric service would gain edge-only override state.
func (ctx *TestContext) testServiceConfigOverrideRejectedForFabricOnly(t *testing.T) {
	fabricSvc := ctx.requireNewFabricService()
	edgeSvc := ctx.requireNewService()
	identity := ctx.requireNewIdentity(false)
	cfg := ctx.requireNewConfig("host.v1", map[string]any{"address": "localhost", "port": 8080, "protocol": "tcp"})

	// fabric-only target is rejected and leaves no override state on the identity
	err := ctx.managers.Identity.AssignServiceConfigs(identity.Id, []ServiceConfig{{Service: fabricSvc.Id, Config: cfg.Id}}, change.New())
	ctx.Error(err, "assigning a service-config to a fabric-only service should fail")
	reread, err := ctx.managers.Identity.Read(identity.Id)
	ctx.NoError(err)
	ctx.Empty(reread.ServiceConfigs, "no service-config override should be recorded for a fabric-only service")

	// positive control: the same assignment to an edge service succeeds
	err = ctx.managers.Identity.AssignServiceConfigs(identity.Id, []ServiceConfig{{Service: edgeSvc.Id, Config: cfg.Id}}, change.New())
	ctx.NoError(err)
}

func (ctx *TestContext) requireNewFabricService() *Service {
	service := &Service{Name: eid.New()}
	ctx.NoError(ctx.managers.Service.Create(service, change.New()))
	return service
}

// fabric creates persist encryptionRequired=true, the secure default. The field is unreachable
// for fabric-only services today, but when fabric and edge services fully merge, fabric services
// should require encryption unless explicitly opted out.
func (ctx *TestContext) testFabricCreateAppliesEncryptionDefault(t *testing.T) {
	fabricSvc := ctx.requireNewFabricService()
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		svc, err := ctx.GetStores().Service.LoadById(tx, fabricSvc.Id)
		ctx.NoError(err)
		ctx.True(svc.EncryptionRequired)
		return nil
	})
	ctx.NoError(err)
}

// V1: fabric-only services must be reported as absent through the edge service by-id entry points.
func (ctx *TestContext) testEdgeManagerHidesFabricOnly(t *testing.T) {
	fabricSvc := ctx.requireNewFabricService()
	edgeSvc := ctx.requireNewService()

	_, err := ctx.managers.EdgeService.Read(fabricSvc.Id)
	ctx.True(boltz.IsErrNotFoundErr(err), "edge Read of a fabric service should be NotFound, got %v", err)

	_, err = ctx.managers.EdgeService.ReadByName(fabricSvc.Name)
	ctx.True(boltz.IsErrNotFoundErr(err), "edge ReadByName of a fabric service should be NotFound, got %v", err)

	err = ctx.managers.EdgeService.Delete(fabricSvc.Id, change.New())
	ctx.True(boltz.IsErrNotFoundErr(err), "edge Delete of a fabric service should be NotFound, got %v", err)

	// the fabric service must still exist (the edge delete was rejected, not silently applied)
	svc, err := ctx.managers.Service.Read(fabricSvc.Id)
	ctx.NoError(err)
	ctx.NotNil(svc)

	// positive control: the edge service is reachable through the same paths
	read, err := ctx.managers.EdgeService.Read(edgeSvc.Id)
	ctx.NoError(err)
	ctx.Equal(edgeSvc.Id, read.Id)
	byName, err := ctx.managers.EdgeService.ReadByName(edgeSvc.Name)
	ctx.NoError(err)
	ctx.Equal(edgeSvc.Id, byName.Id)
}

// B6: edge service listing excludes fabric-only services; the fabric service manager still sees both.
func (ctx *TestContext) testEdgeQueryExcludesFabricOnly(t *testing.T) {
	fabricSvc := ctx.requireNewFabricService()
	edgeSvc := ctx.requireNewService()
	admin := ctx.requireNewIdentity(true)

	query, err := ast.Parse(ctx.GetStores().Service, "true limit none")
	ctx.NoError(err)
	result, err := ctx.managers.EdgeService.PublicQueryForMgmtAccess(admin, nil, query)
	ctx.NoError(err)

	listed := map[string]bool{}
	for _, svc := range result.Services {
		listed[svc.Id] = true
	}
	ctx.True(listed[edgeSvc.Id], "edge service should appear in the edge service listing")
	ctx.False(listed[fabricSvc.Id], "fabric-only service must not appear in the edge service listing")

	// the fabric manager operates on the unified store and sees both classes
	fb, err := ctx.managers.Service.Read(fabricSvc.Id)
	ctx.NoError(err)
	ctx.NotNil(fb)
	eb, err := ctx.managers.Service.Read(edgeSvc.Id)
	ctx.NoError(err)
	ctx.NotNil(eb)
}

// B1: a fabric-surface update (even a full PUT with a nil checker) must not clobber the edge fields
// of an edge service.
func (ctx *TestContext) testFabricUpdatePreservesEdgeFields(t *testing.T) {
	cfg := ctx.requireNewConfig("host.v1", map[string]any{"address": "localhost", "port": 8080, "protocol": "tcp"})

	edgeSvc := ctx.requireNewService(cfg.Id)
	edgeSvc.RoleAttributes = []string{"web"}
	edgeSvc.EncryptionRequired = true
	ctx.NoError(ctx.managers.EdgeService.Update(edgeSvc, nil, change.New()))

	// full fabric-surface update (nil checker) changing only the name
	newName := eid.New()
	ctx.NoError(ctx.managers.Service.Update(&Service{
		BaseEntity: models.BaseEntity{Id: edgeSvc.Id},
		Name:       newName,
	}, nil, change.New()))

	// edge fields survive; Read succeeding at all confirms it is still an edge service
	reread, err := ctx.managers.EdgeService.Read(edgeSvc.Id)
	ctx.NoError(err)
	ctx.Equal(newName, reread.Name)
	ctx.Equal([]string{"web"}, reread.RoleAttributes)
	ctx.Equal([]string{cfg.Id}, reread.Configs)
	ctx.True(reread.EncryptionRequired)
}

// B1 (patch variant): a fabric-surface PATCH (field-checker update) must also leave the edge
// fields of an edge service undisturbed. The patch path differs from PUT: the field checker
// limits which fields the store persists, so it exercises a separate code path.
func (ctx *TestContext) testFabricPatchPreservesEdgeFields(t *testing.T) {
	cfg := ctx.requireNewConfig("host.v1", map[string]any{"address": "localhost", "port": 8080, "protocol": "tcp"})

	edgeSvc := ctx.requireNewService(cfg.Id)
	edgeSvc.RoleAttributes = []string{"web"}
	edgeSvc.EncryptionRequired = true
	ctx.NoError(ctx.managers.EdgeService.Update(edgeSvc, nil, change.New()))

	// fabric-surface patch updating only the name, with a field checker as the fabric PATCH
	// handler builds it
	newName := eid.New()
	ctx.NoError(ctx.managers.Service.Update(&Service{
		BaseEntity: models.BaseEntity{Id: edgeSvc.Id},
		Name:       newName,
	}, fields.UpdatedFieldsMap{db.FieldName: struct{}{}}, change.New()))

	// edge fields survive; Read succeeding at all confirms it is still an edge service
	reread, err := ctx.managers.EdgeService.Read(edgeSvc.Id)
	ctx.NoError(err)
	ctx.Equal(newName, reread.Name)
	ctx.Equal([]string{"web"}, reread.RoleAttributes)
	ctx.Equal([]string{cfg.Id}, reread.Configs)
	ctx.True(reread.EncryptionRequired)
}

// B2: an edge-surface update of a fabric-only service must be rejected, leaving it a fabric service.
func (ctx *TestContext) testEdgeUpdateOfFabricOnlyRejected(t *testing.T) {
	fabricSvc := ctx.requireNewFabricService()

	err := ctx.managers.EdgeService.Update(&EdgeService{
		BaseEntity: models.BaseEntity{Id: fabricSvc.Id},
		Name:       fabricSvc.Name,
	}, nil, change.New())
	ctx.True(boltz.IsErrNotFoundErr(err), "edge update of a fabric-only service should be NotFound, got %v", err)

	// still a fabric service: invisible to edge read, present via the fabric manager, unchanged
	_, err = ctx.managers.EdgeService.Read(fabricSvc.Id)
	ctx.True(boltz.IsErrNotFoundErr(err))
	svc, err := ctx.managers.Service.Read(fabricSvc.Id)
	ctx.NoError(err)
	ctx.Equal(fabricSvc.Name, svc.Name)
}

// B3: updating an edge service notifies connected identities (ServiceUpdated, so SDKs refresh);
// updating a fabric-only service dispatches no such event. Events fire async on commit.
func (ctx *TestContext) testServiceUpdateEvents(t *testing.T) {
	ctx.requireNewIdentity(false)
	edgeSvc := ctx.requireNewService()
	fabricSvc := ctx.requireNewFabricService()
	// #all dial policy links every identity to every edge service (fabric-only is filtered out),
	// so the edge service has a dial identity to notify on update.
	ctx.requireNewServicePolicy(db.PolicyTypeDialName, ss("#all"), ss("#all"))

	// Capture ServiceUpdated events. We use an "active" guard rather than RemoveServiceEventHandler
	// because the registry deletes handlers by equality and funcs are uncomparable (it would panic);
	// the handler is left registered but goes inert after the test.
	var mu sync.Mutex
	active := true
	updatedFor := map[string]bool{}
	db.ServiceEvents.AddServiceEventHandler(func(e *db.ServiceEvent) {
		mu.Lock()
		defer mu.Unlock()
		if active && e.Type == db.ServiceUpdated {
			updatedFor[e.ServiceId] = true
		}
	})
	defer func() {
		mu.Lock()
		active = false
		mu.Unlock()
	}()

	// edge service update -> ServiceUpdated for the connected identity
	edgeSvc.RoleAttributes = []string{eid.New()}
	ctx.NoError(ctx.managers.EdgeService.Update(edgeSvc, nil, change.New()))
	ctx.Eventually(func() bool {
		mu.Lock()
		defer mu.Unlock()
		return updatedFor[edgeSvc.Id]
	}, 2*time.Second, 10*time.Millisecond, "edge service update should dispatch a ServiceUpdated event")

	// fabric service update -> no ServiceUpdated (no edge identities; gated on IsFabricOnly)
	ctx.NoError(ctx.managers.Service.Update(&Service{BaseEntity: models.BaseEntity{Id: fabricSvc.Id}, Name: eid.New()}, nil, change.New()))
	time.Sleep(250 * time.Millisecond) // allow any async dispatch to settle
	mu.Lock()
	sawFabric := updatedFor[fabricSvc.Id]
	mu.Unlock()
	ctx.False(sawFabric, "fabric service update must not dispatch a ServiceUpdated event")
}
