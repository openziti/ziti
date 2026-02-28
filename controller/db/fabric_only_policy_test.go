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
	"fmt"
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
	"go.etcd.io/bbolt"
)

// Test_FabricOnlyPolicyExclusion verifies that fabric-only services are never linked into edge
// service policies or service edge router policies, including via the #all role which matches every
// other service. This must hold regardless of creation order: the policy-side guard
// (EvaluatePolicy's entityFilter) covers policies created after the services, and the entity-side
// guard (the rolesChanged-on-create skip for fabric-only services) covers services created after the
// policies. Explicit @id references to fabric-only services must be rejected outright.
func Test_FabricOnlyPolicyExclusion(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("#all service policy created after services excludes fabric-only", ctx.testAllServicePolicyExcludesFabricOnly_policyLast)
	t.Run("#all service policy excludes fabric-only service created later", ctx.testAllServicePolicyExcludesFabricOnly_serviceLast)
	t.Run("#all SERP created after services excludes fabric-only", ctx.testAllSerpExcludesFabricOnly_policyLast)
	t.Run("#all SERP excludes fabric-only service created later", ctx.testAllSerpExcludesFabricOnly_serviceLast)
	t.Run("explicit @fabric-id service role is rejected", ctx.testExplicitFabricServiceRoleRejected)
}

// newFabricOnlyService builds an IsFabricOnly service for tests.
func newFabricOnlyService(name string) *Service {
	return &Service{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
		IsFabricOnly:  true,
	}
}

// requirePolicyServices asserts the exact set of services linked to a service policy.
func (ctx *TestContext) requirePolicyServices(policyId string, expectedServiceIds ...string) {
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		linked := ctx.stores.ServicePolicy.GetRelatedEntitiesIdList(tx, policyId, EntityTypeServices)
		ctx.ElementsMatch(expectedServiceIds, linked, "service policy %v linked services mismatch", policyId)
		return nil
	})
	ctx.NoError(err)
}

// requireSerpServices asserts the exact set of services linked to a service edge router policy.
func (ctx *TestContext) requireSerpServices(policyId string, expectedServiceIds ...string) {
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		linked := ctx.stores.ServiceEdgeRouterPolicy.GetRelatedEntitiesIdList(tx, policyId, EntityTypeServices)
		ctx.ElementsMatch(expectedServiceIds, linked, "SERP %v linked services mismatch", policyId)
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) newAllServicePolicy(policyType PolicyType) *ServicePolicy {
	return &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		PolicyType:    policyType,
		Semantic:      SemanticAnyOf,
		ServiceRoles:  []string{AllRole},
	}
}

func (ctx *TestContext) newAllSerp() *ServiceEdgeRouterPolicy {
	return &ServiceEdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        SemanticAnyOf,
		ServiceRoles:    []string{AllRole},
		EdgeRouterRoles: []string{AllRole},
	}
}

// testAllServicePolicyExcludesFabricOnly_policyLast: services exist first, then the #all policy is
// created (policy-side EvaluatePolicy path).
func (ctx *TestContext) testAllServicePolicyExcludesFabricOnly_policyLast(_ *testing.T) {
	ctx.CleanupAll()

	edgeService := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, edgeService)
	fabricService := newFabricOnlyService(eid.New())
	boltztest.RequireCreate(ctx, fabricService)

	dialPolicy := ctx.newAllServicePolicy(PolicyTypeDial)
	boltztest.RequireCreate(ctx, dialPolicy)
	bindPolicy := ctx.newAllServicePolicy(PolicyTypeBind)
	boltztest.RequireCreate(ctx, bindPolicy)

	ctx.requirePolicyServices(dialPolicy.Id, edgeService.Id)
	ctx.requirePolicyServices(bindPolicy.Id, edgeService.Id)
}

// testAllServicePolicyExcludesFabricOnly_serviceLast: the #all policy exists first, then services are
// created (entity-side rolesChanged path).
func (ctx *TestContext) testAllServicePolicyExcludesFabricOnly_serviceLast(_ *testing.T) {
	ctx.CleanupAll()

	dialPolicy := ctx.newAllServicePolicy(PolicyTypeDial)
	boltztest.RequireCreate(ctx, dialPolicy)

	fabricService := newFabricOnlyService(eid.New())
	boltztest.RequireCreate(ctx, fabricService)
	edgeService := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, edgeService)

	ctx.requirePolicyServices(dialPolicy.Id, edgeService.Id)
}

func (ctx *TestContext) testAllSerpExcludesFabricOnly_policyLast(_ *testing.T) {
	ctx.CleanupAll()

	edgeService := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, edgeService)
	fabricService := newFabricOnlyService(eid.New())
	boltztest.RequireCreate(ctx, fabricService)

	serp := ctx.newAllSerp()
	boltztest.RequireCreate(ctx, serp)

	ctx.requireSerpServices(serp.Id, edgeService.Id)
}

func (ctx *TestContext) testAllSerpExcludesFabricOnly_serviceLast(_ *testing.T) {
	ctx.CleanupAll()

	serp := ctx.newAllSerp()
	boltztest.RequireCreate(ctx, serp)

	fabricService := newFabricOnlyService(eid.New())
	boltztest.RequireCreate(ctx, fabricService)
	edgeService := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, edgeService)

	ctx.requireSerpServices(serp.Id, edgeService.Id)
}

// testExplicitFabricServiceRoleRejected: an explicit @<fabric-id> in serviceRoles must be rejected
// rather than silently dropped (the C3 fix), on both create and update, for service policies and
// SERPs. An explicit @<edge-id> still works.
func (ctx *TestContext) testExplicitFabricServiceRoleRejected(_ *testing.T) {
	ctx.CleanupAll()

	fabricService := newFabricOnlyService(eid.New())
	boltztest.RequireCreate(ctx, fabricService)
	edgeService := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, edgeService)

	expectedErr := fmt.Sprintf("the value '[%v]' for 'serviceRoles' is invalid: no services found with the given ids", fabricService.Id)

	// --- service policy ---
	// create referencing a fabric-only service is rejected
	policy := ctx.newAllServicePolicy(PolicyTypeDial)
	policy.ServiceRoles = []string{entityRef(fabricService.Id)}
	ctx.EqualError(boltztest.Create(ctx, policy), expectedErr)

	// create referencing an edge service succeeds
	policy.ServiceRoles = []string{entityRef(edgeService.Id)}
	boltztest.RequireCreate(ctx, policy)
	ctx.requirePolicyServices(policy.Id, edgeService.Id)

	// updating it to reference a fabric-only service is rejected
	policy.ServiceRoles = []string{entityRef(fabricService.Id)}
	ctx.EqualError(boltztest.Update(ctx, policy), expectedErr)

	// --- service edge router policy ---
	// create referencing a fabric-only service is rejected
	serp := ctx.newAllSerp()
	serp.ServiceRoles = []string{entityRef(fabricService.Id)}
	ctx.EqualError(boltztest.Create(ctx, serp), expectedErr)

	// create referencing an edge service succeeds
	serp.ServiceRoles = []string{entityRef(edgeService.Id)}
	boltztest.RequireCreate(ctx, serp)
	ctx.requireSerpServices(serp.Id, edgeService.Id)

	// updating it to reference a fabric-only service is rejected
	serp.ServiceRoles = []string{entityRef(fabricService.Id)}
	ctx.EqualError(boltztest.Update(ctx, serp), expectedErr)
}
