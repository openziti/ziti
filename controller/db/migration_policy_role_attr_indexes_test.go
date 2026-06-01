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
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
	"go.etcd.io/bbolt"
)

// Test_BackfillPolicyRoleAttributeIndexes simulates the upgrade path on a
// DB that already has policies. We populate the indexes via the normal
// create path, wipe the index buckets to mimic a DB from before the indexes
// existed, then run backfillPolicyRoleAttributeIndexes and verify the indexes
// recover with the same content.
func Test_BackfillPolicyRoleAttributeIndexes(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()
	ctx.CleanupAll()

	// Real entities so the policy can carry valid '@id' entity refs alongside
	// '#attr' role refs; the backfill must index only the role attributes.
	identity := ctx.RequireNewIdentity(eid.New(), false)
	service := ctx.RequireNewService(eid.New())

	sp := &ServicePolicy{
		BaseExtEntity:     boltz.BaseExtEntity{Id: eid.New()},
		Name:              eid.New(),
		PolicyType:        PolicyTypeDial,
		Semantic:          SemanticAllOf,
		IdentityRoles:     []string{roleRef("sales"), roleRef("marketing"), entityRef(identity.Id)},
		ServiceRoles:      []string{roleRef("api"), entityRef(service.Id)},
		PostureCheckRoles: []string{roleRef("mfa")},
	}
	boltztest.RequireCreate(ctx, sp)

	erp := &EdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        SemanticAllOf,
		IdentityRoles:   []string{roleRef("ops")},
		EdgeRouterRoles: []string{roleRef("us-east")},
	}
	boltztest.RequireCreate(ctx, erp)

	serp := &ServiceEdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        SemanticAllOf,
		ServiceRoles:    []string{roleRef("public")},
		EdgeRouterRoles: []string{roleRef("eu-west")},
	}
	boltztest.RequireCreate(ctx, serp)

	type indexLoc struct {
		entityType string
		field      string
	}
	indexLocs := []indexLoc{
		{EntityTypeServicePolicies, FieldIdentityRoles},
		{EntityTypeServicePolicies, FieldServiceRoles},
		{EntityTypeServicePolicies, FieldPostureCheckRoles},
		{EntityTypeEdgeRouterPolicies, FieldIdentityRoles},
		{EntityTypeEdgeRouterPolicies, FieldEdgeRouterRoles},
		{EntityTypeServiceEdgeRouterPolicies, FieldServiceRoles},
		{EntityTypeServiceEdgeRouterPolicies, FieldEdgeRouterRoles},
	}

	// Wipe the index buckets to simulate a pre-migration DB where the derived
	// role-attribute indexes did not exist. We delete and re-create the bucket
	// empty so that subsequent reads see an initialized-but-empty index.
	err := ctx.GetDb().Update(nil, func(mctx boltz.MutateContext) error {
		for _, loc := range indexLocs {
			parent := boltz.Path(mctx.Tx(), RootBucket, boltz.IndexesBucket, loc.entityType)
			if parent == nil {
				continue
			}
			if sub := parent.Bucket.Bucket([]byte(loc.field)); sub == nil {
				continue
			}
			if delErr := parent.DeleteBucket([]byte(loc.field)); delErr != nil {
				return delErr
			}
			parent.GetOrCreateBucket(loc.field)
		}
		return nil
	})
	ctx.NoError(err)

	// Sanity: indexes are empty post-wipe.
	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ctx.Empty(readIndexKeys(tx, ctx.stores.ServicePolicy.GetIdentityRoleAttributesIndex()))
		ctx.Empty(readIndexKeys(tx, ctx.stores.ServicePolicy.GetServiceRoleAttributesIndex()))
		ctx.Empty(readIndexKeys(tx, ctx.stores.ServicePolicy.GetPostureCheckRoleAttributesIndex()))
		ctx.Empty(readIndexKeys(tx, ctx.stores.EdgeRouterPolicy.GetIdentityRoleAttributesIndex()))
		ctx.Empty(readIndexKeys(tx, ctx.stores.EdgeRouterPolicy.GetEdgeRouterRoleAttributesIndex()))
		ctx.Empty(readIndexKeys(tx, ctx.stores.ServiceEdgeRouterPolicy.GetServiceRoleAttributesIndex()))
		ctx.Empty(readIndexKeys(tx, ctx.stores.ServiceEdgeRouterPolicy.GetEdgeRouterRoleAttributesIndex()))
		return nil
	})
	ctx.NoError(err)

	// Run the backfill in its own update transaction, mimicking the
	// migration manager's surrounding call.
	migrations := &Migrations{stores: ctx.stores}
	err = ctx.GetDb().Update(change.New().NewMutateContext(), func(mctx boltz.MutateContext) error {
		step := &boltz.MigrationStep{
			Component:      "edge",
			Ctx:            mctx,
			CurrentVersion: CurrentDbVersion - 1,
		}
		migrations.backfillPolicyRoleAttributeIndexes(step)
		return step.GetError()
	})
	ctx.NoError(err)

	// Verify the indexes are repopulated and match the expected content from
	// the live entities.
	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		idxIdSp := ctx.stores.ServicePolicy.GetIdentityRoleAttributesIndex()
		ctx.Equal([]string{"marketing", "sales"}, readIndexKeys(tx, idxIdSp))
		ctx.Equal([]string{sp.Id}, readIndexIds(tx, idxIdSp, "sales"))
		ctx.Equal([]string{sp.Id}, readIndexIds(tx, idxIdSp, "marketing"))

		idxSvcSp := ctx.stores.ServicePolicy.GetServiceRoleAttributesIndex()
		ctx.Equal([]string{"api"}, readIndexKeys(tx, idxSvcSp))
		ctx.Equal([]string{sp.Id}, readIndexIds(tx, idxSvcSp, "api"))

		idxPcSp := ctx.stores.ServicePolicy.GetPostureCheckRoleAttributesIndex()
		ctx.Equal([]string{"mfa"}, readIndexKeys(tx, idxPcSp))
		ctx.Equal([]string{sp.Id}, readIndexIds(tx, idxPcSp, "mfa"))

		idxIdErp := ctx.stores.EdgeRouterPolicy.GetIdentityRoleAttributesIndex()
		ctx.Equal([]string{"ops"}, readIndexKeys(tx, idxIdErp))
		ctx.Equal([]string{erp.Id}, readIndexIds(tx, idxIdErp, "ops"))

		idxRouterErp := ctx.stores.EdgeRouterPolicy.GetEdgeRouterRoleAttributesIndex()
		ctx.Equal([]string{"us-east"}, readIndexKeys(tx, idxRouterErp))
		ctx.Equal([]string{erp.Id}, readIndexIds(tx, idxRouterErp, "us-east"))

		idxSvcSerp := ctx.stores.ServiceEdgeRouterPolicy.GetServiceRoleAttributesIndex()
		ctx.Equal([]string{"public"}, readIndexKeys(tx, idxSvcSerp))
		ctx.Equal([]string{serp.Id}, readIndexIds(tx, idxSvcSerp, "public"))

		idxRouterSerp := ctx.stores.ServiceEdgeRouterPolicy.GetEdgeRouterRoleAttributesIndex()
		ctx.Equal([]string{"eu-west"}, readIndexKeys(tx, idxRouterSerp))
		ctx.Equal([]string{serp.Id}, readIndexIds(tx, idxRouterSerp, "eu-west"))
		return nil
	})
	ctx.NoError(err)

	// Running CheckIntegrity again with fix=false should report no errors —
	// confirming the rebuilt indexes are internally consistent.
	err = ctx.GetDb().Update(change.New().NewMutateContext(), func(mctx boltz.MutateContext) error {
		stores := []boltz.Store{
			ctx.stores.ServicePolicy,
			ctx.stores.EdgeRouterPolicy,
			ctx.stores.ServiceEdgeRouterPolicy,
		}
		for _, store := range stores {
			if checkErr := store.CheckIntegrity(mctx, false, func(err error, fixed bool) {
				t.Fatalf("unexpected integrity error after rebuild on %s: %v", store.GetEntityType(), err)
			}); checkErr != nil {
				return checkErr
			}
		}
		return nil
	})
	ctx.NoError(err)

	// Idempotency: running the real backfill a second time against
	// already-populated indexes must not error or alter content (no duplicate
	// ids, no phantom keys).
	err = ctx.GetDb().Update(change.New().NewMutateContext(), func(mctx boltz.MutateContext) error {
		step := &boltz.MigrationStep{
			Component:      "edge",
			Ctx:            mctx,
			CurrentVersion: CurrentDbVersion - 1,
		}
		migrations.backfillPolicyRoleAttributeIndexes(step)
		return step.GetError()
	})
	ctx.NoError(err)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		idxIdSp := ctx.stores.ServicePolicy.GetIdentityRoleAttributesIndex()
		ctx.Equal([]string{"marketing", "sales"}, readIndexKeys(tx, idxIdSp))
		ctx.Equal([]string{sp.Id}, readIndexIds(tx, idxIdSp, "sales"))

		idxSvcSp := ctx.stores.ServicePolicy.GetServiceRoleAttributesIndex()
		ctx.Equal([]string{"api"}, readIndexKeys(tx, idxSvcSp))
		ctx.Equal([]string{sp.Id}, readIndexIds(tx, idxSvcSp, "api"))
		return nil
	})
	ctx.NoError(err)
}
