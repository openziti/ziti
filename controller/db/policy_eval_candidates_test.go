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
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
)

// Test_PolicyEvaluationCandidates walks the link transitions that candidate selection has to
// cover: entities selected by attribute, by id and by #all; entities gaining and losing
// attributes; policies changing roles and semantic; entities created without attributes; and
// deletes on both sides. Every step is validated in both link directions and against the
// denormalized tables.
func Test_PolicyEvaluationCandidates(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("service policies", ctx.testServicePolicyCandidateTransitions)
	t.Run("edge router policies", ctx.testEdgeRouterPolicyCandidateTransitions)
	t.Run("service edge router policies", ctx.testServiceEdgeRouterPolicyCandidateTransitions)
}

func (ctx *TestContext) testServicePolicyCandidateTransitions(_ *testing.T) {
	ctx.CleanupAll()

	identityTypeId := ctx.getIdentityTypeId()
	i1 := newIdentity(eid.New(), identityTypeId, "a")
	i2 := newIdentity(eid.New(), identityTypeId, "a", "b")
	i3 := newIdentity(eid.New(), identityTypeId)
	identities := []*Identity{i1, i2, i3}
	for _, identity := range identities {
		boltztest.RequireCreate(ctx, identity)
	}

	s1 := newEdgeService(eid.New(), "x")
	s2 := newEdgeService(eid.New())
	s3 := newEdgeService(eid.New(), "x", "y")
	services := []*Service{s1, s2, s3}
	for _, service := range services {
		boltztest.RequireCreate(ctx, service)
	}

	p1 := &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		PolicyType:    PolicyTypeDial,
		Semantic:      SemanticAllOf,
		IdentityRoles: []string{roleRef("a"), roleRef("b")},
		ServiceRoles:  []string{roleRef("x")},
	}
	p2 := &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		PolicyType:    PolicyTypeBind,
		Semantic:      SemanticAnyOf,
		IdentityRoles: []string{entityRef(i3.Id)},
		ServiceRoles:  []string{AllRole},
	}
	p3 := &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          eid.New(),
		PolicyType:    PolicyTypeDial,
		Semantic:      SemanticAllOf,
		IdentityRoles: []string{AllRole},
		ServiceRoles:  []string{entityRef(s2.Id)},
	}
	policies := []*ServicePolicy{p1, p2, p3}
	for _, policy := range policies {
		boltztest.RequireCreate(ctx, policy)
	}
	ctx.validateServicePolicies(identities, services, policies)

	// an entity that gains an attribute is found through the policy roles index
	i1.RoleAttributes = []string{"a", "b"}
	boltztest.RequireUpdate(ctx, i1)
	ctx.validateServicePolicies(identities, services, policies)

	// an entity that loses an attribute is found through its own policy links
	i2.RoleAttributes = []string{"a"}
	boltztest.RequireUpdate(ctx, i2)
	ctx.validateServicePolicies(identities, services, policies)

	s2.RoleAttributes = []string{"x"}
	boltztest.RequireUpdate(ctx, s2)
	ctx.validateServicePolicies(identities, services, policies)

	// an entity named by id keeps its link while its attributes change
	i3.RoleAttributes = []string{"c"}
	boltztest.RequireUpdate(ctx, i3)
	ctx.validateServicePolicies(identities, services, policies)

	// a policy whose roles change unlinks through its links and links through the attribute index
	p1.IdentityRoles = []string{roleRef("a")}
	p1.ServiceRoles = []string{roleRef("y")}
	boltztest.RequireUpdate(ctx, p1)
	ctx.validateServicePolicies(identities, services, policies)

	// #all to an attribute, and a mix of id and attribute under any-of
	p2.ServiceRoles = []string{roleRef("x")}
	p2.IdentityRoles = []string{entityRef(i3.Id), roleRef("c")}
	boltztest.RequireUpdate(ctx, p2)
	ctx.validateServicePolicies(identities, services, policies)

	// entities created without attributes are still picked up by #all policies
	i4 := newIdentity(eid.New(), identityTypeId)
	boltztest.RequireCreate(ctx, i4)
	identities = append(identities, i4)
	s4 := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, s4)
	services = append(services, s4)
	ctx.validateServicePolicies(identities, services, policies)

	p1.Semantic = SemanticAnyOf
	p1.IdentityRoles = []string{roleRef("a"), roleRef("c")}
	boltztest.RequireUpdate(ctx, p1)
	ctx.validateServicePolicies(identities, services, policies)

	boltztest.RequireDelete(ctx, p1)
	policies = []*ServicePolicy{p2, p3}
	ctx.validateServicePolicies(identities, services, policies)

	boltztest.RequireDelete(ctx, i2)
	identities = []*Identity{i1, i3, i4}
	ctx.validateServicePolicies(identities, services, policies)

	boltztest.RequireDelete(ctx, s3)
	services = []*Service{s1, s2, s4}
	ctx.validateServicePolicies(identities, services, policies)
}

func (ctx *TestContext) testEdgeRouterPolicyCandidateTransitions(_ *testing.T) {
	ctx.CleanupAll()

	identityTypeId := ctx.getIdentityTypeId()
	i1 := newIdentity(eid.New(), identityTypeId, "a")
	i2 := newIdentity(eid.New(), identityTypeId)
	i3 := newIdentity(eid.New(), identityTypeId, "a", "b")
	identities := []*Identity{i1, i2, i3}
	for _, identity := range identities {
		boltztest.RequireCreate(ctx, identity)
	}

	r1 := newEdgeRouter(eid.New(), "east")
	r2 := newEdgeRouter(eid.New())
	r3 := newEdgeRouter(eid.New(), "east", "west")
	edgeRouters := []*EdgeRouter{r1, r2, r3}
	for _, edgeRouter := range edgeRouters {
		boltztest.RequireCreate(ctx, edgeRouter)
	}

	e1 := &EdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        SemanticAllOf,
		IdentityRoles:   []string{roleRef("a")},
		EdgeRouterRoles: []string{roleRef("east"), roleRef("west")},
	}
	e2 := &EdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        SemanticAnyOf,
		IdentityRoles:   []string{entityRef(i2.Id)},
		EdgeRouterRoles: []string{AllRole},
	}
	policies := []*EdgeRouterPolicy{e1, e2}
	for _, policy := range policies {
		boltztest.RequireCreate(ctx, policy)
	}
	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	r2.RoleAttributes = []string{"east", "west"}
	boltztest.RequireUpdate(ctx, r2)
	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	r3.RoleAttributes = []string{"east"}
	boltztest.RequireUpdate(ctx, r3)
	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	i2.RoleAttributes = []string{"z"}
	boltztest.RequireUpdate(ctx, i2)
	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	e1.Semantic = SemanticAnyOf
	e1.EdgeRouterRoles = []string{entityRef(r1.Id), roleRef("west")}
	boltztest.RequireUpdate(ctx, e1)
	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	r4 := newEdgeRouter(eid.New())
	boltztest.RequireCreate(ctx, r4)
	edgeRouters = append(edgeRouters, r4)
	i4 := newIdentity(eid.New(), identityTypeId)
	boltztest.RequireCreate(ctx, i4)
	identities = append(identities, i4)
	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	boltztest.RequireDelete(ctx, e1)
	policies = []*EdgeRouterPolicy{e2}
	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	boltztest.RequireDelete(ctx, r3)
	edgeRouters = []*EdgeRouter{r1, r2, r4}
	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)
}

func (ctx *TestContext) testServiceEdgeRouterPolicyCandidateTransitions(_ *testing.T) {
	ctx.CleanupAll()

	s1 := newEdgeService(eid.New(), "x")
	s2 := newEdgeService(eid.New())
	s3 := newEdgeService(eid.New(), "x", "y")
	services := []*Service{s1, s2, s3}
	for _, service := range services {
		boltztest.RequireCreate(ctx, service)
	}

	r1 := newEdgeRouter(eid.New(), "east")
	r2 := newEdgeRouter(eid.New())
	r3 := newEdgeRouter(eid.New(), "east", "west")
	edgeRouters := []*EdgeRouter{r1, r2, r3}
	for _, edgeRouter := range edgeRouters {
		boltztest.RequireCreate(ctx, edgeRouter)
	}

	q1 := &ServiceEdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        SemanticAllOf,
		ServiceRoles:    []string{roleRef("x"), roleRef("y")},
		EdgeRouterRoles: []string{roleRef("east")},
	}
	q2 := &ServiceEdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: eid.New()},
		Name:            eid.New(),
		Semantic:        SemanticAnyOf,
		ServiceRoles:    []string{entityRef(s2.Id)},
		EdgeRouterRoles: []string{AllRole},
	}
	policies := []*ServiceEdgeRouterPolicy{q1, q2}
	for _, policy := range policies {
		boltztest.RequireCreate(ctx, policy)
	}
	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	s2.RoleAttributes = []string{"x", "y"}
	boltztest.RequireUpdate(ctx, s2)
	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	s3.RoleAttributes = []string{"x"}
	boltztest.RequireUpdate(ctx, s3)
	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	r2.RoleAttributes = []string{"east"}
	boltztest.RequireUpdate(ctx, r2)
	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	q1.Semantic = SemanticAnyOf
	q1.ServiceRoles = []string{AllRole}
	q1.EdgeRouterRoles = []string{entityRef(r2.Id), roleRef("west")}
	boltztest.RequireUpdate(ctx, q1)
	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	s4 := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, s4)
	services = append(services, s4)
	r4 := newEdgeRouter(eid.New())
	boltztest.RequireCreate(ctx, r4)
	edgeRouters = append(edgeRouters, r4)
	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	boltztest.RequireDelete(ctx, q2)
	policies = []*ServiceEdgeRouterPolicy{q1}
	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	boltztest.RequireDelete(ctx, s1)
	services = []*Service{s2, s3, s4}
	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	boltztest.RequireDelete(ctx, r3)
	edgeRouters = []*EdgeRouter{r1, r2, r4}
	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)
}
