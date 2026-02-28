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

package db

import (
	"sort"
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

// Test_MigrateCollapseEdgeServices_RoundTrip proves the collapse migration preserves FK sub-buckets
// (M3) and refcounted denorm link counts (M4). It builds a representative graph through the real
// (post-collapse) store (so the FK and refcount structures are authentic), captures it, pushes every
// service back into the pre-collapse on-disk layout (edge fields/buckets nested under an "edge" child
// bucket, no isFabricOnly), re-runs the migration, and asserts the captured state is reproduced
// exactly. The down-transform's field placement logic is written independently of the migration's
// (both lean on bbolt's MoveBucket for sub-bucket relocation), so up(down(x)) == x is a real check,
// not a tautology.
func Test_MigrateCollapseEdgeServices_RoundTrip(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	// --- build a representative graph via the real (post-collapse) store ---
	configType := &ConfigType{BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()}, Name: eid.New(), Target: ConfigTypeTargetService}
	boltztest.RequireCreate(ctx, configType)

	cfgA := &Config{BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()}, Name: eid.New(), TypeId: configType.Id, Data: map[string]interface{}{"a": float64(1)}}
	cfgB := &Config{BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()}, Name: eid.New(), TypeId: configType.Id, Data: map[string]interface{}{"b": float64(2)}}
	boltztest.RequireCreate(ctx, cfgA)
	boltztest.RequireCreate(ctx, cfgB)

	edgeSvc := &Service{BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()}, Name: eid.New(), RoleAttributes: []string{"web"}, Configs: []string{cfgA.Id}, EncryptionRequired: true}
	boltztest.RequireCreate(ctx, edgeSvc)
	// EncryptionRequired matches what the fabric create path persists, so the migration's
	// rewrite of the fabric service reproduces the captured state
	fabricSvc := &Service{BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()}, Name: eid.New(), IsFabricOnly: true, EncryptionRequired: true}
	boltztest.RequireCreate(ctx, fabricSvc)

	edgeRouter := newEdgeRouter(eid.New(), "pub")
	boltztest.RequireCreate(ctx, edgeRouter)

	binder := ctx.RequireNewIdentity(eid.New(), false)
	// dialer carries a per-identity service-config override, which populates edgeSvc's
	// identityServices back-link set (one of the FK sub-buckets the migration must relocate).
	dialer := &Identity{
		BaseExtEntity:  *boltz.NewExtEntity(eid.New(), nil),
		Name:           eid.New(),
		ServiceConfigs: map[string]map[string]string{edgeSvc.Id: {configType.Id: cfgB.Id}},
	}
	boltztest.RequireCreate(ctx, dialer)

	// two dial policies both granting dialer -> edgeSvc => dial refcount 2 (the multi-policy case M4
	// must preserve); one bind policy => bind refcount 1.
	sp1 := &ServicePolicy{BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()}, Name: eid.New(), PolicyType: PolicyTypeDial, Semantic: SemanticAnyOf, IdentityRoles: []string{entityRef(dialer.Id)}, ServiceRoles: []string{entityRef(edgeSvc.Id)}}
	sp2 := &ServicePolicy{BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()}, Name: eid.New(), PolicyType: PolicyTypeDial, Semantic: SemanticAnyOf, IdentityRoles: []string{entityRef(dialer.Id)}, ServiceRoles: []string{roleRef("web")}}
	spBind := &ServicePolicy{BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()}, Name: eid.New(), PolicyType: PolicyTypeBind, Semantic: SemanticAnyOf, IdentityRoles: []string{entityRef(binder.Id)}, ServiceRoles: []string{entityRef(edgeSvc.Id)}}
	boltztest.RequireCreate(ctx, sp1)
	boltztest.RequireCreate(ctx, sp2)
	boltztest.RequireCreate(ctx, spBind)

	serp := &ServiceEdgeRouterPolicy{BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()}, Name: eid.New(), Semantic: SemanticAnyOf, ServiceRoles: []string{roleRef("web")}, EdgeRouterRoles: []string{entityRef(edgeRouter.Id)}}
	boltztest.RequireCreate(ctx, serp)

	// --- capture the authentic post-collapse state ---
	var before map[string]serviceLinks
	var beforeDialer, beforeBinder identityLinks
	require.NoError(t, ctx.GetDb().View(func(tx *bbolt.Tx) error {
		before = map[string]serviceLinks{
			edgeSvc.Id:   captureServiceLinks(tx, ctx.stores, edgeSvc.Id),
			fabricSvc.Id: captureServiceLinks(tx, ctx.stores, fabricSvc.Id),
		}
		beforeDialer = captureIdentityLinks(tx, ctx.stores, dialer.Id)
		beforeBinder = captureIdentityLinks(tx, ctx.stores, binder.Id)
		return nil
	}))

	// sanity: the multi-policy dial refcount really is 2, else the test proves nothing about counts
	require.Equal(t, int32(2), before[edgeSvc.Id].dialIdentities[dialer.Id], "expected dial refcount 2 for the two-policy case")

	// --- push every service back to the pre-collapse on-disk layout, then re-run the migration ---
	require.NoError(t, ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
		tx := mc.Tx()
		unmigrateServiceToPreCollapse(t, tx, edgeSvc.Id, false)
		unmigrateServiceToPreCollapse(t, tx, fabricSvc.Id, true)
		// The role-attributes index needs no down-transform: the old edge child store inherited
		// its parent's entity type, so the index lives at the unified service path
		// (ziti/indexes/services/roleAttributes) both pre- and post-collapse (verified against a
		// real pre-collapse db). The migration must leave it intact (M5).
		versions := boltz.GetOrCreatePath(tx, RootBucket, "versions")
		versions.SetInt64("edge", 44, nil)
		return versions.GetError()
	}))

	// The migration must report no anomaly events: nothing for the integrity check to fix, and
	// no pre-existing destinations to replace.
	var collapseEvents []string
	ServiceCollapseEventListener = func(event string) {
		collapseEvents = append(collapseEvents, event)
	}
	defer func() { ServiceCollapseEventListener = nil }()

	require.NoError(t, RunMigrations(ctx.GetDb(), ctx.stores, nil))
	require.Empty(t, collapseEvents, "migration should report no anomaly events")

	// --- re-capture and assert the migration reproduced the original state ---
	require.NoError(t, ctx.GetDb().View(func(tx *bbolt.Tx) error {
		require.Equal(t, before[edgeSvc.Id], captureServiceLinks(tx, ctx.stores, edgeSvc.Id), "edge service links/refcounts not preserved")
		require.Equal(t, before[fabricSvc.Id], captureServiceLinks(tx, ctx.stores, fabricSvc.Id), "fabric service not preserved")
		require.Equal(t, beforeDialer, captureIdentityLinks(tx, ctx.stores, dialer.Id), "dialer reverse links/refcounts not preserved")
		require.Equal(t, beforeBinder, captureIdentityLinks(tx, ctx.stores, binder.Id), "binder reverse links/refcounts not preserved")
		// M6: no edge index bucket may exist (it never did pre-collapse, and the migration must
		// not have created one)
		require.Nil(t, boltz.Path(tx, RootBucket, boltz.IndexesBucket, EntityTypeServices, EdgeBucket), "edge index bucket should not exist")
		// M5: the roleAttributes index must be left intact by the migration. Read the index
		// directly -- the field-based comparison above would not catch a damaged index.
		var webByIndex []string
		ctx.stores.Service.GetRoleAttributesIndex().Read(tx, []byte("web"), func(v []byte) {
			webByIndex = append(webByIndex, string(v))
		})
		require.Contains(t, webByIndex, edgeSvc.Id, "roleAttributes index not intact for 'web'")
		return nil
	}))
}

type serviceLinks struct {
	roleAttrs        []string
	configs          []string
	encReq           bool
	isFabricOnly     bool
	servicePolicies  []string
	serps            []string
	identityServices []string
	dialIdentities   map[string]int32
	bindIdentities   map[string]int32
	edgeRouters      map[string]int32
}

type identityLinks struct {
	dialServices map[string]int32
	bindServices map[string]int32
}

func captureServiceLinks(tx *bbolt.Tx, stores *Stores, id string) serviceLinks {
	svc, _, err := stores.Service.FindById(tx, id)
	if err != nil || svc == nil {
		return serviceLinks{}
	}
	dialIds := stores.Service.GetRelatedEntitiesIdList(tx, id, FieldEdgeServiceDialIdentities)
	bindIds := stores.Service.GetRelatedEntitiesIdList(tx, id, FieldEdgeServiceBindIdentities)
	erIds := stores.Service.GetRelatedEntitiesIdList(tx, id, FieldEdgeRouters)
	return serviceLinks{
		roleAttrs:        sortedCopy(svc.RoleAttributes),
		configs:          sortedCopy(svc.Configs),
		encReq:           svc.EncryptionRequired,
		isFabricOnly:     svc.IsFabricOnly,
		servicePolicies:  sortedCopy(stores.Service.GetRelatedEntitiesIdList(tx, id, EntityTypeServicePolicies)),
		serps:            sortedCopy(stores.Service.GetRelatedEntitiesIdList(tx, id, EntityTypeServiceEdgeRouterPolicies)),
		identityServices: sortedCopy(stores.Service.GetRelatedEntitiesIdList(tx, id, FieldServiceIdentityService)),
		dialIdentities:   linkCountMap(tx, EntityTypeServices, id, FieldEdgeServiceDialIdentities, dialIds),
		bindIdentities:   linkCountMap(tx, EntityTypeServices, id, FieldEdgeServiceBindIdentities, bindIds),
		edgeRouters:      linkCountMap(tx, EntityTypeServices, id, FieldEdgeRouters, erIds),
	}
}

func captureIdentityLinks(tx *bbolt.Tx, stores *Stores, id string) identityLinks {
	dialIds := stores.Identity.GetRelatedEntitiesIdList(tx, id, FieldIdentityDialServices)
	bindIds := stores.Identity.GetRelatedEntitiesIdList(tx, id, FieldIdentityBindServices)
	return identityLinks{
		dialServices: linkCountMap(tx, EntityTypeIdentities, id, FieldIdentityDialServices, dialIds),
		bindServices: linkCountMap(tx, EntityTypeIdentities, id, FieldIdentityBindServices, bindIds),
	}
}

// linkCountMap reads refcounted denorm link counts directly from the bbolt buckets.
func linkCountMap(tx *bbolt.Tx, entityType, id, field string, relatedIds []string) map[string]int32 {
	result := map[string]int32{}
	bucket := boltz.Path(tx, RootBucket, entityType, id, field)
	if bucket == nil {
		return result
	}
	for _, relatedId := range relatedIds {
		if c := bucket.GetLinkCount(boltz.TypeString, []byte(relatedId)); c != nil {
			result[relatedId] = *c
		}
	}
	return result
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// unmigrateServiceToPreCollapse rewrites a post-collapse service entity bucket into the pre-collapse
// (edge child-bucket) on-disk layout: for an edge service it nests the edge-owned scalar and
// sub-buckets under an "edge" child bucket (duplicating name, as the old format did) and removes the
// isFabricOnly discriminator; for a fabric service it drops the isFabricOnly and encryptionRequired
// fields that pre-collapse fabric services never had. This is the inverse of the migration,
// with its own field placement logic.
func unmigrateServiceToPreCollapse(t *testing.T, tx *bbolt.Tx, id string, isFabricOnly bool) {
	t.Helper()
	services := tx.Bucket([]byte(RootBucket)).Bucket([]byte(EntityTypeServices))
	entity := services.Bucket([]byte(id))
	require.NotNil(t, entity)

	require.NoError(t, entity.Delete([]byte(FieldServiceIsFabricOnly)))

	if isFabricOnly {
		require.NoError(t, entity.Delete([]byte(FieldServiceEncryptionRequired)))
		return
	}

	edge, err := entity.CreateBucket([]byte(EdgeBucket))
	require.NoError(t, err)

	if name := entity.Get([]byte(FieldName)); name != nil {
		require.NoError(t, edge.Put([]byte(FieldName), name))
	}

	if v := entity.Get([]byte(FieldServiceEncryptionRequired)); v != nil {
		require.NoError(t, edge.Put([]byte(FieldServiceEncryptionRequired), v))
		require.NoError(t, entity.Delete([]byte(FieldServiceEncryptionRequired)))
	}

	for _, field := range []string{
		FieldRoleAttributes,
		EntityTypeConfigs,
		EntityTypeServicePolicies,
		EntityTypeServiceEdgeRouterPolicies,
		FieldEdgeServiceDialIdentities,
		FieldEdgeServiceBindIdentities,
		FieldEdgeRouters,
		FieldServiceIdentityService,
	} {
		if entity.Bucket([]byte(field)) != nil {
			require.NoError(t, entity.MoveBucket([]byte(field), edge))
		}
	}
}
