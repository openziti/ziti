//go:build apitests

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

package tests

import (
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/openziti/edge-api/rest_management_api_client/role_attributes"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/common/eid"
)

// usageFilter returns a filter restricting role-attribute usage results to
// attributes carrying the given unique test prefix, sorted by id so result
// order is deterministic.
func usageFilter(prefix string) *string {
	return util.Ptr(fmt.Sprintf(`id contains "%s" sort by id`, prefix))
}

// usageByAttr returns the usage detail for the given role attribute, or nil
// if the list doesn't contain it.
func usageByAttr(items rest_model.RoleAttributeUsageList, attr string) *rest_model.RoleAttributeUsageDetail {
	for _, item := range items {
		if item.RoleAttribute != nil && *item.RoleAttribute == attr {
			return item
		}
	}
	return nil
}

// usageFor returns the usage entry for one source on a detail, failing the
// test with a diagnostic if the detail, source entry, or required count is
// missing, so callers can dereference Count without nil checks.
func (ctx *TestContext) usageFor(detail *rest_model.RoleAttributeUsageDetail, source string) rest_model.RoleAttributeSourceUsage {
	ctx.T().Helper()
	ctx.Req.NotNil(detail, "expected a usage detail for source %s", source)
	entry, ok := detail.Usage[source]
	ctx.Req.True(ok, "usage map should contain source %s", source)
	ctx.Req.NotNil(entry.Count, "count is required on source %s", source)
	return entry
}

// sortedIds returns a sorted copy of ids so assertions don't depend on
// server-side ordering.
func sortedIds(ids []string) []string {
	result := append([]string(nil), ids...)
	sort.Strings(result)
	return result
}

func Test_RoleAttributeUsageEndpoints(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	mgmtClient := ctx.NewEdgeManagementApi(nil)
	adminCreds := ctx.NewAdminCredentials()
	_, err := mgmtClient.Authenticate(adminCreds, nil)
	ctx.Req.NoError(err)

	t.Run("identity role-attribute usage", func(t *testing.T) {
		ctx.testContextChanged(t)

		prefix := "idrau-" + eid.New() + "-"
		attrBoth := prefix + "both"
		attrIdentityOnly := prefix + "id-only"
		attrSpOnly := prefix + "sp-only"
		attrErpOnly := prefix + "erp-only"

		// Identities
		idBoth := ctx.AdminManagementSession.requireNewIdentity(false, attrBoth)
		idSolo := ctx.AdminManagementSession.requireNewIdentity(false, attrIdentityOnly)

		// Service policies referencing identity role attributes
		sp1 := ctx.AdminManagementSession.requireNewServicePolicy(
			"Dial",
			s("#all"),
			s("#"+attrBoth, "#"+attrSpOnly),
			s(),
		)
		sp2 := ctx.AdminManagementSession.requireNewServicePolicy(
			"Bind",
			s("#all"),
			s("#"+attrSpOnly),
			s(),
		)

		// Edge-router policy referencing identity role attributes
		erp := ctx.AdminManagementSession.requireNewEdgeRouterPolicy(
			s("#all"),
			s("#"+attrBoth, "#"+attrErpOnly),
		)

		t.Run("default (no withIds) returns all four attributes with accurate counts", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mgmtClient.API.RoleAttributes.ListIdentityRoleAttributeUsage(&role_attributes.ListIdentityRoleAttributeUsageParams{
				Filter: usageFilter(prefix),
			}, nil)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp.Payload.Meta, "list responses must carry a meta section")

			items := resp.Payload.Data
			ctx.Req.Len(items, 4)

			// items are sorted by roleAttribute ascending due to our filter's sort clause
			expectedOrder := []string{attrBoth, attrErpOnly, attrIdentityOnly, attrSpOnly}
			sort.Strings(expectedOrder)
			for i, want := range expectedOrder {
				ctx.Req.Equal(want, *items[i].RoleAttribute, "at index %d", i)
			}

			both := usageByAttr(items, attrBoth)
			ctx.Req.EqualValues(1, *ctx.usageFor(both, "identities").Count)
			ctx.Req.Nil(ctx.usageFor(both, "identities").Ids, "ids should be null when withIds is unset")
			ctx.Req.EqualValues(1, *ctx.usageFor(both, "servicePolicies").Count)
			ctx.Req.EqualValues(1, *ctx.usageFor(both, "edgeRouterPolicies").Count)

			solo := usageByAttr(items, attrIdentityOnly)
			ctx.Req.EqualValues(1, *ctx.usageFor(solo, "identities").Count)
			ctx.Req.EqualValues(0, *ctx.usageFor(solo, "servicePolicies").Count)
			ctx.Req.EqualValues(0, *ctx.usageFor(solo, "edgeRouterPolicies").Count)

			spOnly := usageByAttr(items, attrSpOnly)
			ctx.Req.EqualValues(0, *ctx.usageFor(spOnly, "identities").Count)
			ctx.Req.EqualValues(2, *ctx.usageFor(spOnly, "servicePolicies").Count)
			ctx.Req.EqualValues(0, *ctx.usageFor(spOnly, "edgeRouterPolicies").Count)

			erpOnly := usageByAttr(items, attrErpOnly)
			ctx.Req.EqualValues(0, *ctx.usageFor(erpOnly, "identities").Count)
			ctx.Req.EqualValues(0, *ctx.usageFor(erpOnly, "servicePolicies").Count)
			ctx.Req.EqualValues(1, *ctx.usageFor(erpOnly, "edgeRouterPolicies").Count)
		})

		t.Run("withIds=true populates entity id arrays", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mgmtClient.API.RoleAttributes.ListIdentityRoleAttributeUsage(&role_attributes.ListIdentityRoleAttributeUsageParams{
				Filter:  usageFilter(prefix),
				WithIds: util.Ptr(true),
			}, nil)
			ctx.Req.NoError(err)

			items := resp.Payload.Data
			ctx.Req.Len(items, 4)

			both := usageByAttr(items, attrBoth)
			ctx.Req.Equal([]string{idBoth.Id}, ctx.usageFor(both, "identities").Ids)
			ctx.Req.Equal([]string{sp1.id}, ctx.usageFor(both, "servicePolicies").Ids)
			ctx.Req.Equal([]string{erp.id}, ctx.usageFor(both, "edgeRouterPolicies").Ids)

			solo := usageByAttr(items, attrIdentityOnly)
			ctx.Req.Equal([]string{idSolo.Id}, ctx.usageFor(solo, "identities").Ids)
			// When withIds=true, policy sources with count=0 must still emit an
			// empty (but present) ids array so callers can distinguish
			// "not requested" (null -> nil) from "requested, no matches" ([]).
			soloSp := ctx.usageFor(solo, "servicePolicies")
			ctx.Req.NotNil(soloSp.Ids, "ids should be [] (present) when withIds=true and count=0")
			ctx.Req.Empty(soloSp.Ids)
			ctx.Req.EqualValues(0, *soloSp.Count)

			spOnly := usageByAttr(items, attrSpOnly)
			expectedSp := sortedIds([]string{sp1.id, sp2.id})
			ctx.Req.Equal(expectedSp, sortedIds(ctx.usageFor(spOnly, "servicePolicies").Ids))
		})

		t.Run("filter narrows result set", func(t *testing.T) {
			ctx.testContextChanged(t)
			filter := fmt.Sprintf(`id contains "%s" and (id contains "both" or id contains "sp-only") sort by id`, prefix)
			resp, err := mgmtClient.API.RoleAttributes.ListIdentityRoleAttributeUsage(&role_attributes.ListIdentityRoleAttributeUsageParams{
				Filter: util.Ptr(filter),
			}, nil)
			ctx.Req.NoError(err)

			items := resp.Payload.Data
			ctx.Req.Len(items, 2)
			ctx.Req.Equal(attrBoth, *items[0].RoleAttribute)
			ctx.Req.Equal(attrSpOnly, *items[1].RoleAttribute)
		})

		t.Run("limit and skip apply over the sorted attribute set", func(t *testing.T) {
			ctx.testContextChanged(t)
			filter := fmt.Sprintf(`id contains "%s" sort by id skip 1 limit 2`, prefix)
			resp, err := mgmtClient.API.RoleAttributes.ListIdentityRoleAttributeUsage(&role_attributes.ListIdentityRoleAttributeUsageParams{
				Filter: util.Ptr(filter),
			}, nil)
			ctx.Req.NoError(err)

			items := resp.Payload.Data
			ctx.Req.Len(items, 2)

			all := []string{attrBoth, attrErpOnly, attrIdentityOnly, attrSpOnly}
			sort.Strings(all)
			ctx.Req.Equal(all[1], *items[0].RoleAttribute)
			ctx.Req.Equal(all[2], *items[1].RoleAttribute)
		})
	})

	t.Run("edge router role-attribute usage", func(t *testing.T) {
		ctx.testContextChanged(t)

		prefix := "errau-" + eid.New() + "-"
		attrBoth := prefix + "both"
		attrRouterOnly := prefix + "er-only"
		attrErpOnly := prefix + "erp-only"
		attrSerpOnly := prefix + "serp-only"

		er := ctx.AdminManagementSession.requireNewEdgeRouter(attrBoth)
		erSolo := ctx.AdminManagementSession.requireNewEdgeRouter(attrRouterOnly)

		erp := ctx.AdminManagementSession.requireNewEdgeRouterPolicy(
			s("#"+attrBoth, "#"+attrErpOnly),
			s("#all"),
		)
		serp := ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(
			s("#"+attrSerpOnly),
			s("#all"),
		)

		resp, err := mgmtClient.API.RoleAttributes.ListEdgeRouterRoleAttributeUsage(&role_attributes.ListEdgeRouterRoleAttributeUsageParams{
			Filter:  usageFilter(prefix),
			WithIds: util.Ptr(true),
		}, nil)
		ctx.Req.NoError(err)

		items := resp.Payload.Data
		ctx.Req.Len(items, 4)

		both := usageByAttr(items, attrBoth)
		ctx.Req.Equal([]string{er.id}, ctx.usageFor(both, "edgeRouters").Ids)
		ctx.Req.Equal([]string{erp.id}, ctx.usageFor(both, "edgeRouterPolicies").Ids)
		ctx.Req.EqualValues(0, *ctx.usageFor(both, "serviceEdgeRouterPolicies").Count)

		erOnly := usageByAttr(items, attrRouterOnly)
		ctx.Req.Equal([]string{erSolo.id}, ctx.usageFor(erOnly, "edgeRouters").Ids)
		ctx.Req.EqualValues(0, *ctx.usageFor(erOnly, "edgeRouterPolicies").Count)
		ctx.Req.EqualValues(0, *ctx.usageFor(erOnly, "serviceEdgeRouterPolicies").Count)

		erpOnly := usageByAttr(items, attrErpOnly)
		ctx.Req.EqualValues(0, *ctx.usageFor(erpOnly, "edgeRouters").Count)
		ctx.Req.Equal([]string{erp.id}, ctx.usageFor(erpOnly, "edgeRouterPolicies").Ids)

		serpOnly := usageByAttr(items, attrSerpOnly)
		ctx.Req.Equal([]string{serp.id}, ctx.usageFor(serpOnly, "serviceEdgeRouterPolicies").Ids)
	})

	t.Run("service role-attribute usage", func(t *testing.T) {
		ctx.testContextChanged(t)

		prefix := "srau-" + eid.New() + "-"
		attrBoth := prefix + "both"
		attrSvcOnly := prefix + "svc-only"
		attrSpOnly := prefix + "sp-only"
		attrSerpOnly := prefix + "serp-only"

		svc := ctx.AdminManagementSession.requireNewService(s(attrBoth), nil)
		svcSolo := ctx.AdminManagementSession.requireNewService(s(attrSvcOnly), nil)

		sp := ctx.AdminManagementSession.requireNewServicePolicy(
			"Dial",
			s("#"+attrBoth, "#"+attrSpOnly),
			s("#all"),
			s(),
		)
		serp := ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(
			s("#all"),
			s("#"+attrSerpOnly),
		)

		resp, err := mgmtClient.API.RoleAttributes.ListServiceRoleAttributeUsage(&role_attributes.ListServiceRoleAttributeUsageParams{
			Filter:  usageFilter(prefix),
			WithIds: util.Ptr(true),
		}, nil)
		ctx.Req.NoError(err)

		items := resp.Payload.Data
		ctx.Req.Len(items, 4)

		both := usageByAttr(items, attrBoth)
		ctx.Req.Equal([]string{svc.Id}, ctx.usageFor(both, "services").Ids)
		ctx.Req.Equal([]string{sp.id}, ctx.usageFor(both, "servicePolicies").Ids)
		ctx.Req.EqualValues(0, *ctx.usageFor(both, "serviceEdgeRouterPolicies").Count)

		svcOnly := usageByAttr(items, attrSvcOnly)
		ctx.Req.Equal([]string{svcSolo.Id}, ctx.usageFor(svcOnly, "services").Ids)

		serpOnly := usageByAttr(items, attrSerpOnly)
		ctx.Req.Equal([]string{serp.id}, ctx.usageFor(serpOnly, "serviceEdgeRouterPolicies").Ids)
	})

	t.Run("posture check role-attribute usage", func(t *testing.T) {
		ctx.testContextChanged(t)

		prefix := "pcrau-" + eid.New() + "-"
		attrBoth := prefix + "both"
		attrPostureOnly := prefix + "pc-only"
		attrSpOnly := prefix + "sp-only"

		pc := ctx.AdminManagementSession.requireNewPostureCheckDomain(s("example.com"), s(attrBoth))
		pcSolo := ctx.AdminManagementSession.requireNewPostureCheckDomain(s("example.org"), s(attrPostureOnly))

		sp := ctx.AdminManagementSession.requireNewServicePolicy(
			"Dial",
			s("#all"),
			s("#all"),
			s("#"+attrBoth, "#"+attrSpOnly),
		)

		resp, err := mgmtClient.API.RoleAttributes.ListPostureCheckRoleAttributeUsage(&role_attributes.ListPostureCheckRoleAttributeUsageParams{
			Filter:  usageFilter(prefix),
			WithIds: util.Ptr(true),
		}, nil)
		ctx.Req.NoError(err)

		items := resp.Payload.Data
		ctx.Req.Len(items, 3)

		both := usageByAttr(items, attrBoth)
		ctx.Req.Equal([]string{pc.id}, ctx.usageFor(both, "postureChecks").Ids)
		ctx.Req.Equal([]string{sp.id}, ctx.usageFor(both, "servicePolicies").Ids)

		pcOnly := usageByAttr(items, attrPostureOnly)
		ctx.Req.Equal([]string{pcSolo.id}, ctx.usageFor(pcOnly, "postureChecks").Ids)
		ctx.Req.EqualValues(0, *ctx.usageFor(pcOnly, "servicePolicies").Count)

		spOnly := usageByAttr(items, attrSpOnly)
		ctx.Req.EqualValues(0, *ctx.usageFor(spOnly, "postureChecks").Count)
		ctx.Req.Equal([]string{sp.id}, ctx.usageFor(spOnly, "servicePolicies").Ids)
	})

	t.Run("usage endpoints require the same management auth as the sibling list endpoints", func(t *testing.T) {
		ctx.testContextChanged(t)
		// withIds returns cross-entity policy ids, which could tempt a future
		// change toward per-source authorization. The intended behavior is that
		// usage endpoints carry exactly the same management read permission as
		// the existing *-role-attributes list endpoints. Lock that by proving an
		// unauthenticated request is rejected identically on both, including the
		// withIds variant. If the permission model ever diverges, this fails.
		// Raw HTTP is deliberate here: status codes for anonymous requests are
		// the point.
		pairs := []struct {
			usage string
			list  string
		}{
			{"identity-role-attribute-usage", "identity-role-attributes"},
			{"edge-router-role-attribute-usage", "edge-router-role-attributes"},
			{"service-role-attribute-usage", "service-role-attributes"},
			{"posture-check-role-attribute-usage", "posture-check-role-attributes"},
		}

		for _, p := range pairs {
			listResp, err := ctx.newAnonymousManagementApiRequest().Get(p.list)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusUnauthorized, listResp.StatusCode(),
				"baseline: unauthenticated %s should be rejected", p.list)

			usageResp, err := ctx.newAnonymousManagementApiRequest().Get(p.usage)
			ctx.Req.NoError(err)
			ctx.Req.Equal(listResp.StatusCode(), usageResp.StatusCode(),
				"%s must require the same auth as %s", p.usage, p.list)

			usageIdsResp, err := ctx.newAnonymousManagementApiRequest().Get(p.usage + "?withIds=true")
			ctx.Req.NoError(err)
			ctx.Req.Equal(listResp.StatusCode(), usageIdsResp.StatusCode(),
				"%s?withIds=true must require the same auth as %s; withIds does not relax authorization", p.usage, p.list)
		}
	})
}
