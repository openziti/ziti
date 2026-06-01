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
	"go.etcd.io/bbolt"
)

// Test_PolicyRoleAttributeIndexes verifies the derived role-attribute indexes
// installed on the three policy stores index only '#'-prefixed entries (with
// the '#' stripped) while ignoring '@'-prefixed entity refs.
func Test_PolicyRoleAttributeIndexes(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("service policy role-attribute indexes", ctx.testServicePolicyRoleAttributeIndexes)
	t.Run("edge-router policy role-attribute indexes", ctx.testEdgeRouterPolicyRoleAttributeIndexes)
	t.Run("service-edge-router policy role-attribute indexes", ctx.testServiceEdgeRouterPolicyRoleAttributeIndexes)
	t.Run("the #all wildcard is excluded from role-attribute indexes", ctx.testRoleAttributeIndexExcludesAllWildcard)
}

// testRoleAttributeIndexExcludesAllWildcard verifies the "#all" wildcard is not
// indexed as a role attribute named "all", while a normal "#attr" on the same
// policy still is.
func (ctx *TestContext) testRoleAttributeIndexExcludesAllWildcard(_ *testing.T) {
	ctx.CleanupAll()

	// "#all" must be the only entry in its field, so it gets its own field
	// while a sibling field carries a normal attribute reference.
	p := &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		PolicyType:    PolicyTypeDial,
		Semantic:      SemanticAllOf,
		IdentityRoles: []string{AllRole},
		ServiceRoles:  []string{roleRef("api")},
	}
	boltztest.RequireCreate(ctx, p)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		idxIdentity := ctx.stores.ServicePolicy.GetIdentityRoleAttributesIndex()
		idxService := ctx.stores.ServicePolicy.GetServiceRoleAttributesIndex()

		// "#all" stripped to "all" must NOT appear; the normal attr must.
		ctx.Empty(readIndexKeys(tx, idxIdentity))
		ctx.Equal([]string{"api"}, readIndexKeys(tx, idxService))
		return nil
	})
	ctx.NoError(err)
}

func readIndexKeys(tx *bbolt.Tx, idx boltz.SetReadIndex) []string {
	var keys []string
	idx.ReadKeys(tx, func(val []byte) { keys = append(keys, string(val)) })
	sort.Strings(keys)
	return keys
}

func readIndexIds(tx *bbolt.Tx, idx boltz.SetReadIndex, key string) []string {
	var ids []string
	idx.Read(tx, []byte(key), func(val []byte) { ids = append(ids, string(val)) })
	sort.Strings(ids)
	return ids
}

func (ctx *TestContext) testServicePolicyRoleAttributeIndexes(_ *testing.T) {
	ctx.CleanupAll()

	p1 := &ServicePolicy{
		BaseExtEntity:     boltz.BaseExtEntity{Id: eid.New()},
		Name:              eid.New(),
		PolicyType:        PolicyTypeDial,
		Semantic:          SemanticAllOf,
		IdentityRoles:     []string{roleRef("sales")},
		ServiceRoles:      []string{roleRef("api"), roleRef("public")},
		PostureCheckRoles: []string{roleRef("mfa")},
	}
	boltztest.RequireCreate(ctx, p1)

	p2 := &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		PolicyType:    PolicyTypeBind,
		Semantic:      SemanticAllOf,
		IdentityRoles: []string{roleRef("sales"), roleRef("marketing")},
		ServiceRoles:  []string{roleRef("internal")},
	}
	boltztest.RequireCreate(ctx, p2)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		idxIdentity := ctx.stores.ServicePolicy.GetIdentityRoleAttributesIndex()
		idxService := ctx.stores.ServicePolicy.GetServiceRoleAttributesIndex()
		idxPosture := ctx.stores.ServicePolicy.GetPostureCheckRoleAttributesIndex()

		ctx.Equal([]string{"marketing", "sales"}, readIndexKeys(tx, idxIdentity))
		ctx.Equal([]string{"api", "internal", "public"}, readIndexKeys(tx, idxService))
		ctx.Equal([]string{"mfa"}, readIndexKeys(tx, idxPosture))

		salesIds := readIndexIds(tx, idxIdentity, "sales")
		expected := []string{p1.Id, p2.Id}
		sort.Strings(expected)
		ctx.Equal(expected, salesIds)

		ctx.Equal([]string{p2.Id}, readIndexIds(tx, idxIdentity, "marketing"))
		ctx.Equal([]string{p1.Id}, readIndexIds(tx, idxService, "api"))
		ctx.Equal([]string{p1.Id}, readIndexIds(tx, idxService, "public"))
		ctx.Equal([]string{p2.Id}, readIndexIds(tx, idxService, "internal"))
		ctx.Equal([]string{p1.Id}, readIndexIds(tx, idxPosture, "mfa"))
		return nil
	})
	ctx.NoError(err)

	// Mutate roles and verify the indexes update correctly.
	p1.IdentityRoles = []string{roleRef("engineering")}
	p1.ServiceRoles = nil
	p1.PostureCheckRoles = nil
	boltztest.RequireUpdate(ctx, p1)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		idxIdentity := ctx.stores.ServicePolicy.GetIdentityRoleAttributesIndex()
		idxService := ctx.stores.ServicePolicy.GetServiceRoleAttributesIndex()
		idxPosture := ctx.stores.ServicePolicy.GetPostureCheckRoleAttributesIndex()

		ctx.Equal([]string{"engineering", "marketing", "sales"}, readIndexKeys(tx, idxIdentity))
		ctx.Equal([]string{p1.Id}, readIndexIds(tx, idxIdentity, "engineering"))
		ctx.Equal([]string{p2.Id}, readIndexIds(tx, idxIdentity, "sales"))

		// Only p2 still has service roles; p1 has none.
		ctx.Equal([]string{"internal"}, readIndexKeys(tx, idxService))
		ctx.Empty(readIndexKeys(tx, idxPosture))
		return nil
	})
	ctx.NoError(err)

	// Deleting p2 should clear the remaining keys that only p2 held.
	boltztest.RequireDelete(ctx, p2)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		idxIdentity := ctx.stores.ServicePolicy.GetIdentityRoleAttributesIndex()
		ctx.Equal([]string{"engineering"}, readIndexKeys(tx, idxIdentity))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testEdgeRouterPolicyRoleAttributeIndexes(_ *testing.T) {
	ctx.CleanupAll()

	p := &EdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        SemanticAllOf,
		IdentityRoles:   []string{roleRef("ops")},
		EdgeRouterRoles: []string{roleRef("us-east"), roleRef("us-west")},
	}
	boltztest.RequireCreate(ctx, p)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		idxIdentity := ctx.stores.EdgeRouterPolicy.GetIdentityRoleAttributesIndex()
		idxRouter := ctx.stores.EdgeRouterPolicy.GetEdgeRouterRoleAttributesIndex()

		ctx.Equal([]string{"ops"}, readIndexKeys(tx, idxIdentity))
		ctx.Equal([]string{"us-east", "us-west"}, readIndexKeys(tx, idxRouter))
		ctx.Equal([]string{p.Id}, readIndexIds(tx, idxIdentity, "ops"))
		ctx.Equal([]string{p.Id}, readIndexIds(tx, idxRouter, "us-west"))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testServiceEdgeRouterPolicyRoleAttributeIndexes(_ *testing.T) {
	ctx.CleanupAll()

	p := &ServiceEdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        SemanticAllOf,
		ServiceRoles:    []string{roleRef("public")},
		EdgeRouterRoles: []string{roleRef("eu-west")},
	}
	boltztest.RequireCreate(ctx, p)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		idxService := ctx.stores.ServiceEdgeRouterPolicy.GetServiceRoleAttributesIndex()
		idxRouter := ctx.stores.ServiceEdgeRouterPolicy.GetEdgeRouterRoleAttributesIndex()

		ctx.Equal([]string{"public"}, readIndexKeys(tx, idxService))
		ctx.Equal([]string{"eu-west"}, readIndexKeys(tx, idxRouter))
		ctx.Equal([]string{p.Id}, readIndexIds(tx, idxService, "public"))
		ctx.Equal([]string{p.Id}, readIndexIds(tx, idxRouter, "eu-west"))
		return nil
	})
	ctx.NoError(err)
}
