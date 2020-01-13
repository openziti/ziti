package persistence

import (
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"go.etcd.io/bbolt"
	"sort"
	"testing"
)

func Test_EdgeRouterPolicyStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create edge router policies", ctx.testCreateEdgeRouterPolicy)
	t.Run("test edge router policy evaluation", ctx.testEdgeRouterPolicyRoleEvaluation)
}

func (ctx *TestContext) testCreateEdgeRouterPolicy(_ *testing.T) {
	ctx.cleanupAll()

	policy := NewEdgeRouterPolicy(uuid.New().String())
	ctx.requireCreate(policy)
	ctx.validateBaseline(policy)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ctx.Equal(0, len(ctx.stores.EdgeRouterPolicy.GetRelatedEntitiesIdList(tx, policy.Id, EntityTypeEdgeRouters)))
		ctx.Equal(0, len(ctx.stores.EdgeRouterPolicy.GetRelatedEntitiesIdList(tx, policy.Id, EntityTypeIdentities)))

		testPolicy, err := ctx.stores.EdgeRouterPolicy.LoadOneByName(tx, policy.Name)
		ctx.NoError(err)
		ctx.NotNil(testPolicy)
		ctx.Equal(policy.Name, testPolicy.Name)

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testEdgeRouterPolicyRoleEvaluation(_ *testing.T) {
	ctx.cleanupAll()

	// create some identities, edge routers for reference by id
	// create initial policies, check state
	// create edge routers/identities with roles on create, check state
	// delete all er/identities, check state
	// create edge routers/identities with roles added after create, check state
	// add 5 new policies, check
	// modify polices, add roles, check
	// modify policies, remove roles, check

	identityTypeId := ctx.getIdentityTypeId()

	var identities []*Identity
	for i := 0; i < 5; i++ {
		identity := NewIdentity(uuid.New().String(), identityTypeId)
		ctx.requireCreate(identity)
		identities = append(identities, identity)
	}

	var edgeRouters []*EdgeRouter
	for i := 0; i < 5; i++ {
		edgeRouter := newEdgeRouter(uuid.New().String())
		ctx.requireCreate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	}

	identityRolesAttrs := []string{"foo", "bar", uuid.New().String(), "baz", uuid.New().String(), "quux"}
	var identityRoles []string
	for _, role := range identityRolesAttrs {
		identityRoles = append(identityRoles, "@"+role)
	}

	edgeRouterRoleAttrs := []string{uuid.New().String(), "another-role", "parsley, sage, rosemary and don't forget thyme", uuid.New().String(), "blop", "asdf"}
	var edgeRouterRoles []string
	for _, role := range edgeRouterRoleAttrs {
		edgeRouterRoles = append(edgeRouterRoles, "@"+role)
	}

	multipleIdentityList := []string{identities[1].Id, identities[2].Id, identities[3].Id}
	multipleEdgeRouterList := []string{edgeRouters[1].Id, edgeRouters[2].Id, edgeRouters[3].Id}

	policies := ctx.createEdgeRouterPolicies(identityRoles, edgeRouterRoles, identities, edgeRouters, true)

	for i := 0; i < 9; i++ {
		relatedEdgeRouters := ctx.getRelatedIds(policies[i], EntityTypeEdgeRouters)
		relatedIdentities := ctx.getRelatedIds(policies[i], EntityTypeIdentities)
		if i == 3 {
			ctx.Equal([]string{edgeRouters[0].Id}, relatedEdgeRouters)
			ctx.Equal([]string{identities[0].Id}, relatedIdentities)
		} else if i == 4 || i == 5 {
			sort.Strings(multipleEdgeRouterList)
			sort.Strings(multipleIdentityList)
			ctx.Equal(multipleEdgeRouterList, relatedEdgeRouters)
			ctx.Equal(multipleIdentityList, relatedIdentities)
		} else if i == 7 {
			ctx.Equal([]string{edgeRouters[4].Id}, relatedEdgeRouters)
			ctx.Equal([]string{identities[4].Id}, relatedIdentities)
		} else if i == 9 {
			ctx.Equal(6, len(relatedEdgeRouters))
			ctx.Equal(6, len(relatedIdentities))
		} else {
			ctx.Equal(0, len(relatedIdentities))
			ctx.Equal(0, len(relatedEdgeRouters))
		}
	}

	// no roles
	identity := NewIdentity(uuid.New().String(), identityTypeId)
	ctx.requireCreate(identity)
	identities = append(identities, identity)

	stringz.Permutations(identityRolesAttrs, func(roles []string) {
		identity := NewIdentity(uuid.New().String(), identityTypeId, roles...)
		ctx.requireCreate(identity)
		identities = append(identities, identity)
	})

	// no roles
	edgeRouter := newEdgeRouter(uuid.New().String())
	ctx.requireCreate(edgeRouter)
	edgeRouters = append(edgeRouters, edgeRouter)

	stringz.Permutations(edgeRouterRoleAttrs, func(roles []string) {
		edgeRouter := newEdgeRouter(uuid.New().String(), roles...)
		ctx.requireCreate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	})

	ctx.validateEdgeRouterPolicyIdentities(identities, policies)
	ctx.validateEdgeRouterPolicyEdgeRouters(edgeRouters, policies)

	for _, identity := range identities {
		ctx.requireDelete(identity)
	}

	for _, edgeRouter := range edgeRouters {
		ctx.requireDelete(edgeRouter)
	}

	identities = nil
	edgeRouters = nil

	stringz.Permutations(identityRolesAttrs, func(roles []string) {
		identity := NewIdentity(uuid.New().String(), identityTypeId)
		ctx.requireCreate(identity)
		identity.RoleAttributes = roles
		ctx.requireUpdate(identity)
		identities = append(identities, identity)
	})

	stringz.Permutations(edgeRouterRoleAttrs, func(roles []string) {
		edgeRouter := newEdgeRouter(uuid.New().String())
		ctx.requireCreate(edgeRouter)
		edgeRouter.RoleAttributes = roles
		ctx.requireUpdate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	})

	ctx.validateEdgeRouterPolicyIdentities(identities, policies)
	ctx.validateEdgeRouterPolicyEdgeRouters(edgeRouters, policies)

	// ensure policies get cleaned up
	for _, policy := range policies {
		ctx.requireDelete(policy)
	}

	// test with policies created after identities/edge routers
	policies = ctx.createEdgeRouterPolicies(identityRoles, edgeRouterRoles, identities, edgeRouters, true)

	ctx.validateEdgeRouterPolicyIdentities(identities, policies)
	ctx.validateEdgeRouterPolicyEdgeRouters(edgeRouters, policies)

	for _, policy := range policies {
		ctx.requireDelete(policy)
	}

	// test with policies created after identities/edge routers and roles added after created
	policies = ctx.createEdgeRouterPolicies(identityRoles, edgeRouterRoles, identities, edgeRouters, false)

	ctx.validateEdgeRouterPolicyIdentities(identities, policies)
	ctx.validateEdgeRouterPolicyEdgeRouters(edgeRouters, policies)

	for _, identity := range identities {
		if len(identity.RoleAttributes) > 0 {
			identity.RoleAttributes = identity.RoleAttributes[1:]
			ctx.requireUpdate(identity)
		}
	}

	for _, edgeRouter := range edgeRouters {
		if len(edgeRouter.RoleAttributes) > 0 {
			edgeRouter.RoleAttributes = edgeRouter.RoleAttributes[1:]
			ctx.requireUpdate(edgeRouter)
		}
	}

	for _, policy := range policies {
		if len(policy.IdentityRoles) > 0 {
			policy.IdentityRoles = policy.IdentityRoles[1:]
		}
		if len(policy.EdgeRouterRoles) > 0 {
			policy.EdgeRouterRoles = policy.EdgeRouterRoles[1:]
		}
		ctx.requireUpdate(policy)
	}

	ctx.validateEdgeRouterPolicyIdentities(identities, policies)
	ctx.validateEdgeRouterPolicyEdgeRouters(edgeRouters, policies)
}

func (ctx *TestContext) createEdgeRouterPolicies(identityRoles, edgeRouterRoles []string, identities []*Identity, edgeRouters []*EdgeRouter, oncreate bool) []*EdgeRouterPolicy {
	var policies []*EdgeRouterPolicy
	for i := 0; i < 10; i++ {
		policy := NewEdgeRouterPolicy(uuid.New().String())
		if !oncreate {
			ctx.requireCreate(policy)
		}
		if i == 1 {
			policy.IdentityRoles = []string{identityRoles[0]}
			policy.EdgeRouterRoles = []string{edgeRouterRoles[0]}
		}
		if i == 2 {
			policy.IdentityRoles = []string{identityRoles[1], identityRoles[2], identityRoles[3]}
			policy.EdgeRouterRoles = []string{edgeRouterRoles[1], edgeRouterRoles[2], edgeRouterRoles[3]}
		}
		if i == 3 {
			policy.IdentityRoles = []string{identities[0].Id}
			policy.EdgeRouterRoles = []string{edgeRouters[0].Id}
		}
		if i == 4 {
			policy.IdentityRoles = []string{identities[1].Id, identities[2].Id, identities[3].Id}
			policy.EdgeRouterRoles = []string{edgeRouters[1].Id, edgeRouters[2].Id, edgeRouters[3].Id}
		}
		if i == 5 {
			policy.IdentityRoles = []string{identityRoles[4], identities[1].Id, identities[2].Id, identities[3].Id}
			policy.EdgeRouterRoles = []string{edgeRouterRoles[4], edgeRouters[1].Id, edgeRouters[2].Id, edgeRouters[3].Id}
		}
		if i == 6 {
			policy.IdentityRoles = []string{uuid.New().String()}
			policy.EdgeRouterRoles = []string{uuid.New().String()}
		}
		if i == 7 {
			policy.IdentityRoles = []string{uuid.New().String(), identities[4].Id}
			policy.EdgeRouterRoles = []string{uuid.New().String(), edgeRouters[4].Id}
		}
		if i == 8 {
			policy.IdentityRoles = []string{uuid.New().String(), identityRoles[5]}
			policy.EdgeRouterRoles = []string{uuid.New().String(), edgeRouterRoles[5]}
		}
		if i == 9 {
			policy.IdentityRoles = []string{"@all"}
			policy.EdgeRouterRoles = []string{"@all"}
		}

		policies = append(policies, policy)
		if oncreate {
			ctx.requireCreate(policy)
		} else {
			ctx.requireUpdate(policy)
		}
	}
	return policies
}

func (ctx *TestContext) validateEdgeRouterPolicyIdentities(identities []*Identity, policies []*EdgeRouterPolicy) {
	for _, policy := range policies {
		count := 0
		relatedIdentities := ctx.getRelatedIds(policy, EntityTypeIdentities)
		for _, identity := range identities {
			relatedPolicies := ctx.getRelatedIds(identity, EntityTypeEdgeRouterPolicies)
			shouldContain := ctx.policyShouldMatch(policy.IdentityRoles, identity.Id, identity.RoleAttributes)

			policyContains := stringz.Contains(relatedIdentities, identity.Id)
			ctx.Equal(shouldContain, policyContains, "entity roles attr: %v. policy roles: %v", identity.RoleAttributes, policy.IdentityRoles)
			if shouldContain {
				count++
			}

			entityContains := stringz.Contains(relatedPolicies, policy.Id)
			ctx.Equal(shouldContain, entityContains, "identity: %v, policy: %v, entity roles attr: %v. policy roles: %v",
				identity.Id, policy.Id, identity.RoleAttributes, policy.IdentityRoles)
		}
		ctx.Equal(count, len(relatedIdentities))
	}
}

func (ctx *TestContext) validateEdgeRouterPolicyEdgeRouters(edgeRouters []*EdgeRouter, policies []*EdgeRouterPolicy) {
	for _, policy := range policies {
		count := 0
		relatedEdgeRouters := ctx.getRelatedIds(policy, EntityTypeEdgeRouters)
		for _, edgeRouter := range edgeRouters {
			relatedPolicies := ctx.getRelatedIds(edgeRouter, EntityTypeEdgeRouterPolicies)
			shouldContain := ctx.policyShouldMatch(policy.EdgeRouterRoles, edgeRouter.Id, edgeRouter.RoleAttributes)
			policyContains := stringz.Contains(relatedEdgeRouters, edgeRouter.Id)
			ctx.Equal(shouldContain, policyContains, "entity roles attr: %v. policy roles: %v", edgeRouter.RoleAttributes, policy.EdgeRouterRoles)
			if shouldContain {
				count++
			}

			entityContains := stringz.Contains(relatedPolicies, policy.Id)
			ctx.Equal(shouldContain, entityContains, "identity: %v, policy: %v, entity roles attr: %v. policy roles: %v",
				edgeRouter.Id, policy.Id, edgeRouter.RoleAttributes, policy.EdgeRouterRoles)
		}
		ctx.Equal(count, len(relatedEdgeRouters))
	}
}

func (ctx *TestContext) policyShouldMatch(roleSet []string, entityId string, roleAttribute []string) bool {
	roles, ids := splitRolesAndIds(roleSet)
	isIdMatch := stringz.Contains(ids, entityId)
	isAllMatch := stringz.Contains(roles, "all")
	IsRoleMatch := len(roles) > 0 && stringz.ContainsAll(roleAttribute, roles...)
	return isIdMatch || isAllMatch || IsRoleMatch
}
