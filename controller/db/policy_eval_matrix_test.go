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

	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
)

// Test_PolicyEvaluationMatrix drives every ordered pair of role-set shapes through one roles field
// of each policy type, and every ordered pair of attribute sets through each kind of policy target,
// validating links in both directions and the denormalized tables after every step. Background
// policies overlap the policy under test so that shared pairs exercise the reference counts.
func Test_PolicyEvaluationMatrix(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("service policy identity roles", ctx.testServicePolicyIdentityRolesMatrix)
	t.Run("service policy service roles", ctx.testServicePolicyServiceRolesMatrix)
	t.Run("service policy posture check roles", ctx.testServicePolicyPostureCheckRolesMatrix)
	t.Run("edge router policy identity roles", ctx.testEdgeRouterPolicyIdentityRolesMatrix)
	t.Run("edge router policy edge router roles", ctx.testEdgeRouterPolicyEdgeRouterRolesMatrix)
	t.Run("service edge router policy service roles", ctx.testServiceEdgeRouterPolicyServiceRolesMatrix)
	t.Run("service edge router policy edge router roles", ctx.testServiceEdgeRouterPolicyEdgeRouterRolesMatrix)
}

// roleSetState is one shape a policy roles field takes in the matrix.
type roleSetState struct {
	name     string
	roles    []string
	semantic string
}

// roleSetStates returns the shapes a roles field cycles through, naming e1 and e2 by id. The
// semantic is varied only where it can change the outcome.
func roleSetStates(e1, e2 string) []roleSetState {
	return []roleSetState{
		{"empty", nil, SemanticAllOf},
		{"id", []string{entityRef(e1)}, SemanticAllOf},
		{"ids", []string{entityRef(e1), entityRef(e2)}, SemanticAllOf},
		{"attr", []string{roleRef("a")}, SemanticAllOf},
		{"attrs-all-of", []string{roleRef("a"), roleRef("b")}, SemanticAllOf},
		{"attrs-any-of", []string{roleRef("a"), roleRef("b")}, SemanticAnyOf},
		{"all", []string{AllRole}, SemanticAllOf},
		{"id+attr-all-of", []string{entityRef(e1), roleRef("a")}, SemanticAllOf},
		{"id+attrs-any-of", []string{entityRef(e2), roleRef("b"), roleRef("c")}, SemanticAnyOf},
	}
}

// entityAttrStates are the attribute sets a policy target cycles through in the matrix.
var entityAttrStates = [][]string{nil, {"a"}, {"a", "b"}, {"b"}, {"c"}}

// matrixTarget adapts one roles field of one policy type, or one kind of policy target, to the
// matrix runners. create returns the entity, update changes it in place and persists it, and
// validate checks every link and denormalized table the change can affect.
type matrixTarget[S any] struct {
	create   func(state S) boltz.ExtEntity
	update   func(entity boltz.ExtEntity, state S)
	remove   func(entity boltz.ExtEntity)
	validate func()
}

// runMatrix creates an entity in every state, moves it to every other state, and removes it,
// validating after each step. The state after create depends only on from, so it is validated on
// the first pass through the inner loop only.
func runMatrix[S any](states []S, target matrixTarget[S]) {
	for _, from := range states {
		for i, to := range states {
			entity := target.create(from)
			if i == 0 {
				target.validate()
			}
			target.update(entity, to)
			target.validate()
			target.remove(entity)
			target.validate()
		}
	}
}

func newTestPostureCheck(roleAttributes ...string) *PostureCheck {
	return &PostureCheck{
		BaseExtEntity:  boltz.BaseExtEntity{Id: eid.New()},
		Name:           eid.New(),
		TypeId:         PostureCheckTypeProcess,
		RoleAttributes: roleAttributes,
		SubType: &PostureCheckProcess{
			OperatingSystem: testProcessOsType,
			Path:            testProcessPath,
		},
	}
}

func without[T comparable](list []T, item T) []T {
	var result []T
	for _, entry := range list {
		if entry != item {
			result = append(result, entry)
		}
	}
	return result
}

// matrixFixtures holds the fixed entities every matrix runs against: one of each attribute
// shape per kind, so every role-set state selects a distinct, non-empty subset.
type matrixFixtures struct {
	identities    []*Identity
	services      []*Service
	postureChecks []*PostureCheck
	edgeRouters   []*EdgeRouter
}

func (ctx *TestContext) newMatrixFixtures() *matrixFixtures {
	ctx.CleanupAll()
	identityTypeId := ctx.getIdentityTypeId()
	result := &matrixFixtures{}
	for _, attrs := range entityAttrStates {
		identity := newIdentity(eid.New(), identityTypeId, attrs...)
		boltztest.RequireCreate(ctx, identity)
		result.identities = append(result.identities, identity)

		service := newEdgeService(eid.New(), attrs...)
		boltztest.RequireCreate(ctx, service)
		result.services = append(result.services, service)

		postureCheck := newTestPostureCheck(attrs...)
		boltztest.RequireCreate(ctx, postureCheck)
		result.postureChecks = append(result.postureChecks, postureCheck)

		edgeRouter := newEdgeRouter(eid.New(), attrs...)
		boltztest.RequireCreate(ctx, edgeRouter)
		result.edgeRouters = append(result.edgeRouters, edgeRouter)
	}
	return result
}

// --- service policies

func (ctx *TestContext) validateAllServicePolicies(f *matrixFixtures, policies []*ServicePolicy) {
	ctx.validateServicePolicyIdentities(f.identities, policies)
	ctx.validateServicePolicyServices(f.services, policies)
	ctx.validateServicePolicyPostureChecks(f.postureChecks, policies)
	ctx.validateServicePolicyDenormalization()
}

func (ctx *TestContext) validateServicePolicyPostureChecks(postureChecks []*PostureCheck, policies []*ServicePolicy) {
	for _, policy := range policies {
		count := 0
		relatedPostureChecks := ctx.getRelatedIds(policy, EntityTypePostureChecks)
		for _, postureCheck := range postureChecks {
			relatedPolicies := ctx.getRelatedIds(postureCheck, EntityTypeServicePolicies)
			shouldContain := ctx.policyShouldMatch(policy.Semantic, policy.PostureCheckRoles, postureCheck, postureCheck.RoleAttributes)

			policyContains := stringz.Contains(relatedPostureChecks, postureCheck.Id)
			ctx.Equal(shouldContain, policyContains, "entity roles attr: %v. policy roles: %v", postureCheck.RoleAttributes, policy.PostureCheckRoles)
			if shouldContain {
				count++
			}

			entityContains := stringz.Contains(relatedPolicies, policy.Id)
			ctx.Equal(shouldContain, entityContains, "posture check: %v, policy: %v, entity roles attr: %v. policy roles: %v",
				postureCheck.Id, policy.Id, postureCheck.RoleAttributes, policy.PostureCheckRoles)
		}
		ctx.Equal(count, len(relatedPostureChecks))
	}
}

func (ctx *TestContext) newServicePolicy(policyType PolicyType, semantic string, identityRoles, serviceRoles, postureCheckRoles []string) *ServicePolicy {
	return &ServicePolicy{
		BaseExtEntity:     boltz.BaseExtEntity{Id: eid.New()},
		Name:              eid.New(),
		PolicyType:        policyType,
		Semantic:          semantic,
		IdentityRoles:     identityRoles,
		ServiceRoles:      serviceRoles,
		PostureCheckRoles: postureCheckRoles,
	}
}

// servicePolicyBackground creates policies that stay for the whole matrix: a wildcard, an
// attribute and an id policy, so pairs selected by the policy under test are also selected by
// another policy and the reference counts see both directions.
func (ctx *TestContext) servicePolicyBackground(f *matrixFixtures) []*ServicePolicy {
	policies := []*ServicePolicy{
		ctx.newServicePolicy(PolicyTypeDial, SemanticAllOf, []string{AllRole}, []string{AllRole}, []string{AllRole}),
		ctx.newServicePolicy(PolicyTypeBind, SemanticAllOf, []string{roleRef("a")}, []string{roleRef("b")}, []string{roleRef("a")}),
		ctx.newServicePolicy(PolicyTypeDial, SemanticAnyOf, []string{entityRef(f.identities[1].Id)}, []string{entityRef(f.services[1].Id)}, []string{entityRef(f.postureChecks[1].Id)}),
	}
	for _, policy := range policies {
		boltztest.RequireCreate(ctx, policy)
	}
	return policies
}

// servicePolicyMatrixTarget runs the policy-side matrix over one roles field of service policies.
// set assigns the field under test; the other fields select every fixture so each link under test
// creates denormalized pairs.
func (ctx *TestContext) servicePolicyMatrixTarget(f *matrixFixtures, policyType PolicyType, set func(policy *ServicePolicy, state roleSetState)) matrixTarget[roleSetState] {
	policies := ctx.servicePolicyBackground(f)
	return matrixTarget[roleSetState]{
		create: func(state roleSetState) boltz.ExtEntity {
			policy := ctx.newServicePolicy(policyType, state.semantic, []string{AllRole}, []string{AllRole}, []string{AllRole})
			set(policy, state)
			boltztest.RequireCreate(ctx, policy)
			policies = append(policies, policy)
			return policy
		},
		update: func(entity boltz.ExtEntity, state roleSetState) {
			policy := entity.(*ServicePolicy)
			policy.Semantic = state.semantic
			set(policy, state)
			boltztest.RequireUpdate(ctx, policy)
		},
		remove: func(entity boltz.ExtEntity) {
			policy := entity.(*ServicePolicy)
			boltztest.RequireDelete(ctx, policy)
			policies = without(policies, policy)
		},
		validate: func() {
			ctx.validateAllServicePolicies(f, policies)
		},
	}
}

func (ctx *TestContext) testServicePolicyIdentityRolesMatrix(_ *testing.T) {
	f := ctx.newMatrixFixtures()
	target := ctx.servicePolicyMatrixTarget(f, PolicyTypeDial, func(policy *ServicePolicy, state roleSetState) {
		policy.IdentityRoles = state.roles
	})
	runMatrix(roleSetStates(f.identities[0].Id, f.identities[4].Id), target)
	ctx.runIdentityAttributeMatrix(f, func() { target.validate() })
}

func (ctx *TestContext) testServicePolicyServiceRolesMatrix(_ *testing.T) {
	f := ctx.newMatrixFixtures()
	target := ctx.servicePolicyMatrixTarget(f, PolicyTypeBind, func(policy *ServicePolicy, state roleSetState) {
		policy.ServiceRoles = state.roles
	})
	runMatrix(roleSetStates(f.services[0].Id, f.services[4].Id), target)
	ctx.runServiceAttributeMatrix(f, func() { target.validate() })
}

func (ctx *TestContext) testServicePolicyPostureCheckRolesMatrix(_ *testing.T) {
	f := ctx.newMatrixFixtures()
	target := ctx.servicePolicyMatrixTarget(f, PolicyTypeDial, func(policy *ServicePolicy, state roleSetState) {
		policy.PostureCheckRoles = state.roles
	})
	runMatrix(roleSetStates(f.postureChecks[0].Id, f.postureChecks[4].Id), target)

	postureCheckTarget := matrixTarget[[]string]{
		create: func(attrs []string) boltz.ExtEntity {
			postureCheck := newTestPostureCheck(attrs...)
			boltztest.RequireCreate(ctx, postureCheck)
			f.postureChecks = append(f.postureChecks, postureCheck)
			return postureCheck
		},
		update: func(entity boltz.ExtEntity, attrs []string) {
			postureCheck := entity.(*PostureCheck)
			postureCheck.RoleAttributes = attrs
			boltztest.RequireUpdate(ctx, postureCheck)
		},
		remove: func(entity boltz.ExtEntity) {
			postureCheck := entity.(*PostureCheck)
			boltztest.RequireDelete(ctx, postureCheck)
			f.postureChecks = without(f.postureChecks, postureCheck)
		},
		validate: target.validate,
	}
	runMatrix(entityAttrStates, postureCheckTarget)
}

// --- entity-side matrices shared by the policy types

func (ctx *TestContext) runIdentityAttributeMatrix(f *matrixFixtures, validate func()) {
	identityTypeId := ctx.getIdentityTypeId()
	runMatrix(entityAttrStates, matrixTarget[[]string]{
		create: func(attrs []string) boltz.ExtEntity {
			identity := newIdentity(eid.New(), identityTypeId, attrs...)
			boltztest.RequireCreate(ctx, identity)
			f.identities = append(f.identities, identity)
			return identity
		},
		update: func(entity boltz.ExtEntity, attrs []string) {
			identity := entity.(*Identity)
			identity.RoleAttributes = attrs
			boltztest.RequireUpdate(ctx, identity)
		},
		remove: func(entity boltz.ExtEntity) {
			identity := entity.(*Identity)
			boltztest.RequireDelete(ctx, identity)
			f.identities = without(f.identities, identity)
		},
		validate: validate,
	})
}

func (ctx *TestContext) runServiceAttributeMatrix(f *matrixFixtures, validate func()) {
	runMatrix(entityAttrStates, matrixTarget[[]string]{
		create: func(attrs []string) boltz.ExtEntity {
			service := newEdgeService(eid.New(), attrs...)
			boltztest.RequireCreate(ctx, service)
			f.services = append(f.services, service)
			return service
		},
		update: func(entity boltz.ExtEntity, attrs []string) {
			service := entity.(*Service)
			service.RoleAttributes = attrs
			boltztest.RequireUpdate(ctx, service)
		},
		remove: func(entity boltz.ExtEntity) {
			service := entity.(*Service)
			boltztest.RequireDelete(ctx, service)
			f.services = without(f.services, service)
		},
		validate: validate,
	})
}

func (ctx *TestContext) runEdgeRouterAttributeMatrix(f *matrixFixtures, validate func()) {
	runMatrix(entityAttrStates, matrixTarget[[]string]{
		create: func(attrs []string) boltz.ExtEntity {
			edgeRouter := newEdgeRouter(eid.New(), attrs...)
			boltztest.RequireCreate(ctx, edgeRouter)
			f.edgeRouters = append(f.edgeRouters, edgeRouter)
			return edgeRouter
		},
		update: func(entity boltz.ExtEntity, attrs []string) {
			edgeRouter := entity.(*EdgeRouter)
			edgeRouter.RoleAttributes = attrs
			boltztest.RequireUpdate(ctx, edgeRouter)
		},
		remove: func(entity boltz.ExtEntity) {
			edgeRouter := entity.(*EdgeRouter)
			boltztest.RequireDelete(ctx, edgeRouter)
			f.edgeRouters = without(f.edgeRouters, edgeRouter)
		},
		validate: validate,
	})
}

// --- edge router policies

func (ctx *TestContext) newEdgeRouterPolicy(semantic string, identityRoles, edgeRouterRoles []string) *EdgeRouterPolicy {
	return &EdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        semantic,
		IdentityRoles:   identityRoles,
		EdgeRouterRoles: edgeRouterRoles,
	}
}

func (ctx *TestContext) edgeRouterPolicyMatrixTarget(f *matrixFixtures, set func(policy *EdgeRouterPolicy, state roleSetState)) matrixTarget[roleSetState] {
	policies := []*EdgeRouterPolicy{
		ctx.newEdgeRouterPolicy(SemanticAllOf, []string{AllRole}, []string{AllRole}),
		ctx.newEdgeRouterPolicy(SemanticAllOf, []string{roleRef("a")}, []string{roleRef("b")}),
		ctx.newEdgeRouterPolicy(SemanticAnyOf, []string{entityRef(f.identities[1].Id)}, []string{entityRef(f.edgeRouters[1].Id)}),
	}
	for _, policy := range policies {
		boltztest.RequireCreate(ctx, policy)
	}
	return matrixTarget[roleSetState]{
		create: func(state roleSetState) boltz.ExtEntity {
			policy := ctx.newEdgeRouterPolicy(state.semantic, []string{AllRole}, []string{AllRole})
			set(policy, state)
			boltztest.RequireCreate(ctx, policy)
			policies = append(policies, policy)
			return policy
		},
		update: func(entity boltz.ExtEntity, state roleSetState) {
			policy := entity.(*EdgeRouterPolicy)
			policy.Semantic = state.semantic
			set(policy, state)
			boltztest.RequireUpdate(ctx, policy)
		},
		remove: func(entity boltz.ExtEntity) {
			policy := entity.(*EdgeRouterPolicy)
			boltztest.RequireDelete(ctx, policy)
			policies = without(policies, policy)
		},
		validate: func() {
			ctx.validateEdgeRouterPolicies(f.identities, f.edgeRouters, policies)
		},
	}
}

func (ctx *TestContext) testEdgeRouterPolicyIdentityRolesMatrix(_ *testing.T) {
	f := ctx.newMatrixFixtures()
	target := ctx.edgeRouterPolicyMatrixTarget(f, func(policy *EdgeRouterPolicy, state roleSetState) {
		policy.IdentityRoles = state.roles
	})
	runMatrix(roleSetStates(f.identities[0].Id, f.identities[4].Id), target)
	ctx.runIdentityAttributeMatrix(f, target.validate)
}

func (ctx *TestContext) testEdgeRouterPolicyEdgeRouterRolesMatrix(_ *testing.T) {
	f := ctx.newMatrixFixtures()
	target := ctx.edgeRouterPolicyMatrixTarget(f, func(policy *EdgeRouterPolicy, state roleSetState) {
		policy.EdgeRouterRoles = state.roles
	})
	runMatrix(roleSetStates(f.edgeRouters[0].Id, f.edgeRouters[4].Id), target)
	ctx.runEdgeRouterAttributeMatrix(f, target.validate)
}

// --- service edge router policies

func (ctx *TestContext) newServiceEdgeRouterPolicy(semantic string, serviceRoles, edgeRouterRoles []string) *ServiceEdgeRouterPolicy {
	return &ServiceEdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        semantic,
		ServiceRoles:    serviceRoles,
		EdgeRouterRoles: edgeRouterRoles,
	}
}

func (ctx *TestContext) serviceEdgeRouterPolicyMatrixTarget(f *matrixFixtures, set func(policy *ServiceEdgeRouterPolicy, state roleSetState)) matrixTarget[roleSetState] {
	policies := []*ServiceEdgeRouterPolicy{
		ctx.newServiceEdgeRouterPolicy(SemanticAllOf, []string{AllRole}, []string{AllRole}),
		ctx.newServiceEdgeRouterPolicy(SemanticAllOf, []string{roleRef("a")}, []string{roleRef("b")}),
		ctx.newServiceEdgeRouterPolicy(SemanticAnyOf, []string{entityRef(f.services[1].Id)}, []string{entityRef(f.edgeRouters[1].Id)}),
	}
	for _, policy := range policies {
		boltztest.RequireCreate(ctx, policy)
	}
	return matrixTarget[roleSetState]{
		create: func(state roleSetState) boltz.ExtEntity {
			policy := ctx.newServiceEdgeRouterPolicy(state.semantic, []string{AllRole}, []string{AllRole})
			set(policy, state)
			boltztest.RequireCreate(ctx, policy)
			policies = append(policies, policy)
			return policy
		},
		update: func(entity boltz.ExtEntity, state roleSetState) {
			policy := entity.(*ServiceEdgeRouterPolicy)
			policy.Semantic = state.semantic
			set(policy, state)
			boltztest.RequireUpdate(ctx, policy)
		},
		remove: func(entity boltz.ExtEntity) {
			policy := entity.(*ServiceEdgeRouterPolicy)
			boltztest.RequireDelete(ctx, policy)
			policies = without(policies, policy)
		},
		validate: func() {
			ctx.validateServiceEdgeRouterPolicies(f.services, f.edgeRouters, policies)
		},
	}
}

func (ctx *TestContext) testServiceEdgeRouterPolicyServiceRolesMatrix(_ *testing.T) {
	f := ctx.newMatrixFixtures()
	target := ctx.serviceEdgeRouterPolicyMatrixTarget(f, func(policy *ServiceEdgeRouterPolicy, state roleSetState) {
		policy.ServiceRoles = state.roles
	})
	runMatrix(roleSetStates(f.services[0].Id, f.services[4].Id), target)
	ctx.runServiceAttributeMatrix(f, target.validate)
}

func (ctx *TestContext) testServiceEdgeRouterPolicyEdgeRouterRolesMatrix(_ *testing.T) {
	f := ctx.newMatrixFixtures()
	target := ctx.serviceEdgeRouterPolicyMatrixTarget(f, func(policy *ServiceEdgeRouterPolicy, state roleSetState) {
		policy.EdgeRouterRoles = state.roles
	})
	runMatrix(roleSetStates(f.edgeRouters[0].Id, f.edgeRouters[4].Id), target)
	ctx.runEdgeRouterAttributeMatrix(f, target.validate)
}
