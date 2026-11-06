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
	"sort"
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/db"
)

// TestQueryRoleAttributeUsage_Identity verifies the identity-kind aggregator
// covers three distinct cases: an attribute only declared on an identity, an
// attribute only referenced by policies, and an attribute that appears in
// both. Also verifies the withIds branch populates entity id arrays.
func TestQueryRoleAttributeUsage_Identity(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	idBoth := ctx.requireNewIdentity(false)
	idBoth.RoleAttributes = []string{"sales"}
	ctx.NoError(ctx.managers.Identity.Update(idBoth, nil, change.New()))

	idEntityOnly := ctx.requireNewIdentity(false)
	idEntityOnly.RoleAttributes = []string{"engineering"}
	ctx.NoError(ctx.managers.Identity.Update(idEntityOnly, nil, change.New()))

	// "sales" is referenced by both a service policy and an edge-router policy.
	sp1 := ctx.requireNewServicePolicy(db.PolicyTypeDialName, ss("#sales"), ss("#public"))
	sp2 := ctx.requireNewServicePolicy(db.PolicyTypeDialName, ss("#marketing"), ss("#public"))
	erp := ctx.requireNewEdgeRouterPolicy(ss("#sales"), ss("#any-router"))

	// Use unique per-test identity attribute names to avoid contamination from
	// identities created by other test setup paths.
	results, qmd, err := QueryRoleAttributeUsage(ctx, RoleAttributeKindIdentity, `id contains "sale" or id contains "engineer" or id contains "market"`, true)
	ctx.NoError(err)
	ctx.NotNil(qmd)

	byAttr := make(map[string]*RoleAttributeUsage)
	for _, r := range results {
		byAttr[r.RoleAttribute] = r
	}

	// "sales" — both sources
	sales := byAttr["sales"]
	ctx.NotNil(sales)
	ctx.Equal(int64(1), sales.Usage[RoleAttributeSourceIdentities].Count)
	ctx.Equal([]string{idBoth.Id}, sales.Usage[RoleAttributeSourceIdentities].Ids)
	ctx.Equal(int64(1), sales.Usage[RoleAttributeSourceServicePolicies].Count)
	ctx.Equal([]string{sp1.Id}, sales.Usage[RoleAttributeSourceServicePolicies].Ids)
	ctx.Equal(int64(1), sales.Usage[RoleAttributeSourceEdgeRouterPolicies].Count)
	ctx.Equal([]string{erp.Id}, sales.Usage[RoleAttributeSourceEdgeRouterPolicies].Ids)

	// "engineering" — identity only
	engineering := byAttr["engineering"]
	ctx.NotNil(engineering)
	ctx.Equal(int64(1), engineering.Usage[RoleAttributeSourceIdentities].Count)
	ctx.Equal([]string{idEntityOnly.Id}, engineering.Usage[RoleAttributeSourceIdentities].Ids)
	ctx.Equal(int64(0), engineering.Usage[RoleAttributeSourceServicePolicies].Count)
	ctx.Empty(engineering.Usage[RoleAttributeSourceServicePolicies].Ids)
	ctx.Equal(int64(0), engineering.Usage[RoleAttributeSourceEdgeRouterPolicies].Count)

	// "marketing" — policy only (no identity declares it)
	marketing := byAttr["marketing"]
	ctx.NotNil(marketing)
	ctx.Equal(int64(0), marketing.Usage[RoleAttributeSourceIdentities].Count)
	ctx.Empty(marketing.Usage[RoleAttributeSourceIdentities].Ids)
	ctx.Equal(int64(1), marketing.Usage[RoleAttributeSourceServicePolicies].Count)
	ctx.Equal([]string{sp2.Id}, marketing.Usage[RoleAttributeSourceServicePolicies].Ids)
	ctx.Equal(int64(0), marketing.Usage[RoleAttributeSourceEdgeRouterPolicies].Count)
}

// TestQueryRoleAttributeUsage_WithoutIds verifies that includeIds=false omits
// entity id arrays but still reports accurate counts.
func TestQueryRoleAttributeUsage_WithoutIds(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	id := ctx.requireNewIdentity(false)
	id.RoleAttributes = []string{"ops"}
	ctx.NoError(ctx.managers.Identity.Update(id, nil, change.New()))

	ctx.requireNewServicePolicy(db.PolicyTypeDialName, ss("#ops"), ss("#whatever"))
	ctx.requireNewServicePolicy(db.PolicyTypeDialName, ss("#ops"), ss("#whatever"))

	results, _, err := QueryRoleAttributeUsage(ctx, RoleAttributeKindIdentity, `id = "ops"`, false)
	ctx.NoError(err)
	ctx.Len(results, 1)
	ops := results[0]
	ctx.Equal("ops", ops.RoleAttribute)
	ctx.Equal(int64(1), ops.Usage[RoleAttributeSourceIdentities].Count)
	ctx.Nil(ops.Usage[RoleAttributeSourceIdentities].Ids)
	ctx.Equal(int64(2), ops.Usage[RoleAttributeSourceServicePolicies].Count)
	ctx.Nil(ops.Usage[RoleAttributeSourceServicePolicies].Ids)
}

// TestQueryRoleAttributeUsage_Empty verifies that a kind with no role
// attributes anywhere returns an empty result rather than panicking. When no
// source index has any key the merged TreeSet has a nil root, which
// TreeSet.ToCursor would dereference; the aggregator must return an empty
// cursor instead. A freshly-initialized context has no posture checks and no
// posture-check role refs, so both posture-check sources are empty.
func TestQueryRoleAttributeUsage_Empty(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	results, qmd, err := QueryRoleAttributeUsage(ctx, RoleAttributeKindPostureCheck, "true", true)
	ctx.NoError(err)
	ctx.NotNil(qmd)
	ctx.Empty(results)
	ctx.Equal(int64(0), qmd.Count)
}

// TestQueryRoleAttributeUsage_EdgeRouter exercises the edge-router kind with
// both home-entity and policy sources.
func TestQueryRoleAttributeUsage_EdgeRouter(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	er := ctx.requireNewEdgeRouter()
	er.RoleAttributes = []string{"edge-" + eid.New()}
	ctx.NoError(ctx.managers.EdgeRouter.Update(er, false, nil, change.New()))

	tag := er.RoleAttributes[0]
	ctx.requireNewEdgeRouterPolicy(ss("#ignored"), ss("#"+tag))
	ctx.requireNewServiceNewEdgeRouterPolicy(ss("#irrelevant"), ss("#"+tag))

	results, _, err := QueryRoleAttributeUsage(ctx, RoleAttributeKindEdgeRouter, `id = "`+tag+`"`, true)
	ctx.NoError(err)
	ctx.Len(results, 1)
	r := results[0]
	ctx.Equal(tag, r.RoleAttribute)
	ctx.Equal(int64(1), r.Usage[RoleAttributeSourceEdgeRouters].Count)
	ctx.Equal([]string{er.Id}, r.Usage[RoleAttributeSourceEdgeRouters].Ids)
	ctx.Equal(int64(1), r.Usage[RoleAttributeSourceEdgeRouterPolicies].Count)
	ctx.Equal(int64(1), r.Usage[RoleAttributeSourceServiceEdgeRouterPolicies].Count)
}

// TestQueryRoleAttributeUsage_Filter_Paging confirms the shared filter/sort/
// paging pipeline still applies: a filter restricts the result set, and
// limit/offset bound it.
func TestQueryRoleAttributeUsage_Filter_Paging(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	// Install a fixed set of identity attributes spanning multiple "groups"
	// so we can exercise filter + sort + limit/offset together.
	tagPrefix := "grp-" + eid.New() + "-"
	var expected []string
	for _, suffix := range []string{"alpha", "beta", "gamma", "delta"} {
		identity := ctx.requireNewIdentity(false)
		identity.RoleAttributes = []string{tagPrefix + suffix}
		ctx.NoError(ctx.managers.Identity.Update(identity, nil, change.New()))
		expected = append(expected, tagPrefix+suffix)
	}
	sort.Strings(expected)

	// Broad filter: should match all four.
	results, _, err := QueryRoleAttributeUsage(ctx, RoleAttributeKindIdentity, `id contains "`+tagPrefix+`" sort by id`, false)
	ctx.NoError(err)
	ctx.Len(results, 4)
	got := make([]string, 0, len(results))
	for _, r := range results {
		got = append(got, r.RoleAttribute)
	}
	ctx.Equal(expected, got)

	// Narrow filter: should match just alpha & gamma via contains pattern.
	results, _, err = QueryRoleAttributeUsage(ctx, RoleAttributeKindIdentity, `id contains "`+tagPrefix+`" and (id contains "alpha" or id contains "gamma") sort by id`, false)
	ctx.NoError(err)
	ctx.Len(results, 2)
	ctx.Equal(tagPrefix+"alpha", results[0].RoleAttribute)
	ctx.Equal(tagPrefix+"gamma", results[1].RoleAttribute)

	// Limit + offset.
	results, _, err = QueryRoleAttributeUsage(ctx, RoleAttributeKindIdentity, `id contains "`+tagPrefix+`" sort by id skip 1 limit 2`, false)
	ctx.NoError(err)
	ctx.Len(results, 2)
	ctx.Equal(expected[1], results[0].RoleAttribute)
	ctx.Equal(expected[2], results[1].RoleAttribute)
}
