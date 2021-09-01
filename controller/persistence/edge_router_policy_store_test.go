package persistence

import (
	"fmt"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/foundation/util/stringz"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	"sort"
	"strings"
	"testing"
)

func Test_EdgeRouterPolicyStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create edge router policies", ctx.testCreateEdgeRouterPolicy)
	t.Run("test create/update edge router policies with invalid entity refs", ctx.testEdgeRouterPolicyInvalidValues)
	t.Run("test edge router policy evaluation", ctx.testEdgeRouterPolicyRoleEvaluation)
	t.Run("test update/delete referenced entities", ctx.testEdgeRouterPolicyUpdateDeleteRefs)
	t.Run("test edge router tunneler disabling", ctx.testRouterIdentityDeleteTest)
}

func (ctx *TestContext) testCreateEdgeRouterPolicy(t *testing.T) {
	ctx.NextTest(t)
	ctx.CleanupAll()

	policy := newEdgeRouterPolicy(eid.New())
	ctx.RequireCreate(policy)
	ctx.ValidateBaseline(policy)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ctx.Equal(0, len(ctx.stores.EdgeRouterPolicy.GetRelatedEntitiesIdList(tx, policy.Id, db.EntityTypeRouters)))
		ctx.Equal(0, len(ctx.stores.EdgeRouterPolicy.GetRelatedEntitiesIdList(tx, policy.Id, EntityTypeIdentities)))

		testPolicy, err := ctx.stores.EdgeRouterPolicy.LoadOneByName(tx, policy.Name)
		ctx.NoError(err)
		ctx.NotNil(testPolicy)
		ctx.Equal(policy.Name, testPolicy.Name)

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testEdgeRouterPolicyInvalidValues(t *testing.T) {
	ctx.NextTest(t)
	ctx.CleanupAll()

	// test identity roles
	policy := newEdgeRouterPolicy(eid.New())
	invalidId := eid.New()
	policy.IdentityRoles = []string{entityRef(invalidId)}
	err := ctx.Create(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'identityRoles' is invalid: no identities found with the given ids", invalidId))

	policy.IdentityRoles = []string{AllRole, roleRef("other")}
	err = ctx.Create(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v %v]' for 'identityRoles' is invalid: if using %v, it should be the only role specified", AllRole, roleRef("other"), AllRole))

	identityTypeId := ctx.getIdentityTypeId()
	identity := newIdentity(eid.New(), identityTypeId)
	ctx.RequireCreate(identity)

	policy.IdentityRoles = []string{entityRef(identity.Id), entityRef(invalidId)}
	err = ctx.Create(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'identityRoles' is invalid: no identities found with the given ids", invalidId))

	policy.IdentityRoles = []string{entityRef(identity.Id)}
	ctx.RequireCreate(policy)
	ctx.validateEdgeRouterPolicyIdentities([]*Identity{identity}, []*EdgeRouterPolicy{policy})

	policy.IdentityRoles = append(policy.IdentityRoles, entityRef(invalidId))
	err = ctx.Update(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'identityRoles' is invalid: no identities found with the given ids", invalidId))
	ctx.RequireDelete(policy)

	// test edgeRouter roles
	policy.IdentityRoles = nil
	policy.EdgeRouterRoles = []string{entityRef(invalidId)}
	err = ctx.Create(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'edgeRouterRoles' is invalid: no routers found with the given ids", invalidId))

	policy.EdgeRouterRoles = []string{AllRole, roleRef("other")}
	err = ctx.Create(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v %v]' for 'edgeRouterRoles' is invalid: if using %v, it should be the only role specified", AllRole, roleRef("other"), AllRole))

	edgeRouter := newEdgeRouter(eid.New())
	ctx.RequireCreate(edgeRouter)

	policy.EdgeRouterRoles = []string{entityRef(edgeRouter.Id), entityRef(invalidId)}
	err = ctx.Create(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'edgeRouterRoles' is invalid: no routers found with the given ids", invalidId))

	policy.EdgeRouterRoles = []string{entityRef(edgeRouter.Id)}
	ctx.RequireCreate(policy)
	ctx.validateEdgeRouterPolicyEdgeRouters([]*EdgeRouter{edgeRouter}, []*EdgeRouterPolicy{policy})

	policy.EdgeRouterRoles = append(policy.EdgeRouterRoles, entityRef(invalidId))
	err = ctx.Update(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'edgeRouterRoles' is invalid: no routers found with the given ids", invalidId))
	ctx.RequireDelete(policy)
}

func (ctx *TestContext) testEdgeRouterPolicyUpdateDeleteRefs(t *testing.T) {
	ctx.NextTest(t)
	ctx.CleanupAll()

	// test identity roles
	policy := newEdgeRouterPolicy(eid.New())
	identityTypeId := ctx.getIdentityTypeId()
	identity := newIdentity(eid.New(), identityTypeId)
	ctx.RequireCreate(identity)

	policy.IdentityRoles = []string{entityRef(identity.Id)}
	ctx.RequireCreate(policy)
	ctx.validateEdgeRouterPolicies([]*Identity{identity}, nil, []*EdgeRouterPolicy{policy})
	ctx.RequireDelete(identity)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.IdentityRoles), "identity id should have been removed from identity roles")

	identity = newIdentity(eid.New(), identityTypeId)
	ctx.RequireCreate(identity)

	policy.IdentityRoles = []string{entityRef(identity.Id)}
	ctx.RequireUpdate(policy)
	ctx.validateEdgeRouterPolicyIdentities([]*Identity{identity}, []*EdgeRouterPolicy{policy})

	identity.Name = eid.New()
	ctx.RequireUpdate(identity)
	ctx.RequireReload(policy)
	ctx.True(stringz.Contains(policy.IdentityRoles, entityRef(identity.Id)))
	ctx.validateEdgeRouterPolicyIdentities([]*Identity{identity}, []*EdgeRouterPolicy{policy})

	ctx.RequireDelete(identity)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.IdentityRoles), "identity name should have been removed from identity roles")

	// test edgeRouter roles
	edgeRouter := newEdgeRouter(eid.New())
	ctx.RequireCreate(edgeRouter)

	policy.EdgeRouterRoles = []string{entityRef(edgeRouter.Id)}
	ctx.RequireUpdate(policy)
	ctx.validateEdgeRouterPolicyEdgeRouters([]*EdgeRouter{edgeRouter}, []*EdgeRouterPolicy{policy})
	ctx.RequireDelete(edgeRouter)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.EdgeRouterRoles), "edgeRouter id should have been removed from edgeRouter roles")

	edgeRouter = newEdgeRouter(eid.New())
	ctx.RequireCreate(edgeRouter)

	policy.EdgeRouterRoles = []string{entityRef(edgeRouter.Id)}
	ctx.RequireUpdate(policy)
	ctx.validateEdgeRouterPolicyEdgeRouters([]*EdgeRouter{edgeRouter}, []*EdgeRouterPolicy{policy})

	edgeRouter.Name = eid.New()
	ctx.RequireUpdate(edgeRouter)
	ctx.RequireReload(policy)
	ctx.True(stringz.Contains(policy.EdgeRouterRoles, entityRef(edgeRouter.Id)))
	ctx.validateEdgeRouterPolicyEdgeRouters([]*EdgeRouter{edgeRouter}, []*EdgeRouterPolicy{policy})

	ctx.RequireDelete(edgeRouter)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.EdgeRouterRoles), "edgeRouter name should have been removed from edgeRouter roles")
}

func (ctx *TestContext) testRouterIdentityDeleteTest(t *testing.T) {
	ctx.NextTest(t)
	ctx.CleanupAll()

	policy := newEdgeRouterPolicy(eid.New())
	policy.IdentityRoles = []string{"#all"}
	policy.EdgeRouterRoles = []string{"#all"}
	ctx.RequireCreate(policy)

	edgeRouterOther := newEdgeRouter(eid.New())
	ctx.RequireCreate(edgeRouterOther)
	logrus.Infof("router 1 id: %v", edgeRouterOther.Id)

	edgeRouter := newEdgeRouter(eid.New())
	edgeRouter.IsTunnelerEnabled = true
	ctx.RequireCreate(edgeRouter)
	logrus.Infof("router 2 id: %v", edgeRouter.Id)

	addErrors := func(err error, fixed bool) {
		ctx.NoError(err)
	}

	err := ctx.db.View(func(tx *bbolt.Tx) error {
		if err := ctx.stores.EdgeRouter.CheckIntegrity(tx, false, addErrors); err != nil {
			return nil
		}

		if err := ctx.stores.EdgeRouterPolicy.CheckIntegrity(tx, false, addErrors); err != nil {
			return nil
		}

		c := ctx.stores.Identity.GetRefCountedLinkCollection(db.EntityTypeRouters)
		count := c.GetLinkCount(tx, []byte(edgeRouter.Id), []byte(edgeRouter.Id))
		ctx.NotNil(count)
		ctx.Equal(int32(2), *count)

		count = c.GetLinkCount(tx, []byte(edgeRouter.Id), []byte(edgeRouterOther.Id))
		ctx.NotNil(count)
		ctx.Equal(int32(1), *count)

		c = ctx.stores.EdgeRouter.GetRefCountedLinkCollection(EntityTypeIdentities)
		count = c.GetLinkCount(tx, []byte(edgeRouter.Id), []byte(edgeRouter.Id))
		ctx.NotNil(count)
		ctx.Equal(int32(2), *count)

		count = c.GetLinkCount(tx, []byte(edgeRouterOther.Id), []byte(edgeRouter.Id))
		ctx.NotNil(count)
		ctx.Equal(int32(1), *count)

		return nil
	})
	ctx.NoError(err)

	edgeRouter.IsTunnelerEnabled = false
	edgeRouter.Name = eid.New()
	ctx.RequireUpdate(edgeRouter)

	err = ctx.db.View(func(tx *bbolt.Tx) error {
		if err := ctx.stores.EdgeRouter.CheckIntegrity(tx, false, addErrors); err != nil {
			return nil
		}

		if err := ctx.stores.EdgeRouterPolicy.CheckIntegrity(tx, false, addErrors); err != nil {
			return nil
		}

		c := ctx.stores.EdgeRouter.GetRefCountedLinkCollection(EntityTypeIdentities)
		count := c.GetLinkCount(tx, []byte(edgeRouter.Id), []byte(edgeRouter.Id))
		ctx.Nil(count)

		c = ctx.stores.EdgeRouter.GetRefCountedLinkCollection(EntityTypeIdentities)
		count = c.GetLinkCount(tx, []byte(edgeRouterOther.Id), []byte(edgeRouter.Id))
		ctx.Nil(count)

		return nil
	})
	ctx.NoError(err)

	edgeRouter.IsTunnelerEnabled = true
	ctx.RequireUpdate(edgeRouter)
}

func (ctx *TestContext) testEdgeRouterPolicyRoleEvaluation(t *testing.T) {
	ctx.NextTest(t)
	ctx.CleanupAll()

	// create some identities, edge routers for reference by id
	// create initial policies, check state
	// create edge routers/identities with roles on create, check state
	// delete all er/identities, check state
	// create edge routers/identities with roles added after create, check state
	// add 5 new policies, check
	// modify polices, add roles, check
	// modify policies, remove roles, check

	identityTypeId := ctx.getIdentityTypeId()

	identities := make([]*Identity, 0) // goland complains of potential nil panic if we use var identities []*Identities
	for i := 0; i < 5; i++ {
		identity := newIdentity(eid.New(), identityTypeId)
		ctx.RequireCreate(identity)
		identities = append(identities, identity)
	}

	edgeRouters := make([]*EdgeRouter, 0)
	for i := 0; i < 5; i++ {
		edgeRouter := newEdgeRouter(eid.New())
		ctx.RequireCreate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	}

	identityRolesAttrs := []string{"foo", "bar", eid.New(), "baz", eid.New(), "quux"}
	var identityRoles []string
	for _, role := range identityRolesAttrs {
		identityRoles = append(identityRoles, roleRef(role))
	}

	edgeRouterRoleAttrs := []string{eid.New(), "another-role", "parsley, sage, rosemary and don't forget thyme", eid.New(), "blop", "asdf"}
	var edgeRouterRoles []string
	for _, role := range edgeRouterRoleAttrs {
		edgeRouterRoles = append(edgeRouterRoles, roleRef(role))
	}

	multipleIdentityList := []string{identities[1].Id, identities[2].Id, identities[3].Id}
	multipleEdgeRouterList := []string{edgeRouters[1].Id, edgeRouters[2].Id, edgeRouters[3].Id}

	policies := ctx.createEdgeRouterPolicies(identityRoles, edgeRouterRoles, identities, edgeRouters, true)

	for i := 0; i < 9; i++ {
		relatedEdgeRouters := ctx.getRelatedIds(policies[i], db.EntityTypeRouters)
		relatedIdentities := ctx.getRelatedIds(policies[i], EntityTypeIdentities)
		if i == 3 {
			ctx.Equal([]string{edgeRouters[0].Id}, relatedEdgeRouters)
			ctx.Equal([]string{identities[0].Id}, relatedIdentities)
		} else if i == 4 || i == 5 {
			sort.Strings(multipleEdgeRouterList)
			sort.Strings(multipleIdentityList)
			ctx.Equal(multipleEdgeRouterList, relatedEdgeRouters)
			ctx.Equal(multipleIdentityList, relatedIdentities)
		} else if i == 6 {
			ctx.Equal(5, len(relatedEdgeRouters))
			ctx.Equal(5, len(relatedIdentities))
		} else {
			ctx.Equal(0, len(relatedIdentities))
			ctx.Equal(0, len(relatedEdgeRouters))
		}
	}

	// no roles
	identity := newIdentity(eid.New(), identityTypeId)
	ctx.RequireCreate(identity)
	identities = append(identities, identity)

	stringz.Permutations(identityRolesAttrs, func(roles []string) {
		identity := newIdentity(eid.New(), identityTypeId, roles...)
		ctx.RequireCreate(identity)
		identities = append(identities, identity)
	})

	// no roles
	edgeRouter := newEdgeRouter(eid.New())
	ctx.RequireCreate(edgeRouter)
	edgeRouters = append(edgeRouters, edgeRouter)

	stringz.Permutations(edgeRouterRoleAttrs, func(roles []string) {
		edgeRouter := newEdgeRouter(eid.New(), roles...)
		ctx.RequireCreate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	})

	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	for _, identity := range identities {
		ctx.RequireDelete(identity)
	}

	for _, edgeRouter := range edgeRouters {
		ctx.RequireDelete(edgeRouter)
	}

	identities = nil
	edgeRouters = nil

	stringz.Permutations(identityRolesAttrs, func(roles []string) {
		identity := newIdentity(eid.New(), identityTypeId)
		ctx.RequireCreate(identity)
		identity.RoleAttributes = roles
		ctx.RequireUpdate(identity)
		identities = append(identities, identity)
	})

	stringz.Permutations(edgeRouterRoleAttrs, func(roles []string) {
		edgeRouter := newEdgeRouter(eid.New())
		ctx.RequireCreate(edgeRouter)
		edgeRouter.RoleAttributes = roles
		ctx.RequireUpdate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	})

	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	// ensure policies get cleaned up
	for _, policy := range policies {
		ctx.RequireDelete(policy)
	}

	// test with policies created after identities/edge routers
	policies = ctx.createEdgeRouterPolicies(identityRoles, edgeRouterRoles, identities, edgeRouters, true)

	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	for _, policy := range policies {
		ctx.RequireDelete(policy)
	}

	// test with policies created after identities/edge routers and roles added after created
	policies = ctx.createEdgeRouterPolicies(identityRoles, edgeRouterRoles, identities, edgeRouters, false)

	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)

	for _, identity := range identities {
		if len(identity.RoleAttributes) > 0 {
			identity.RoleAttributes = identity.RoleAttributes[1:]
			ctx.RequireUpdate(identity)
		}
	}

	for _, edgeRouter := range edgeRouters {
		if len(edgeRouter.RoleAttributes) > 0 {
			edgeRouter.RoleAttributes = edgeRouter.RoleAttributes[1:]
			ctx.RequireUpdate(edgeRouter)
		}
	}

	for _, policy := range policies {
		if len(policy.IdentityRoles) > 0 {
			policy.IdentityRoles = policy.IdentityRoles[1:]
		}
		if len(policy.EdgeRouterRoles) > 0 {
			policy.EdgeRouterRoles = policy.EdgeRouterRoles[1:]
		}
		ctx.RequireUpdate(policy)
	}

	ctx.validateEdgeRouterPolicies(identities, edgeRouters, policies)
}

func (ctx *TestContext) createEdgeRouterPolicies(identityRoles, edgeRouterRoles []string, identities []*Identity, edgeRouters []*EdgeRouter, oncreate bool) []*EdgeRouterPolicy {
	var policies []*EdgeRouterPolicy
	for i := 0; i < 9; i++ {
		policy := newEdgeRouterPolicy(eid.New())
		policy.Semantic = SemanticAllOf

		if !oncreate {
			ctx.RequireCreate(policy)
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
			policy.IdentityRoles = []string{entityRef(identities[0].Id)}
			policy.EdgeRouterRoles = []string{entityRef(edgeRouters[0].Id)}
		}
		if i == 4 {
			policy.IdentityRoles = []string{entityRef(identities[1].Id), entityRef(identities[2].Id), entityRef(identities[3].Id)}
			policy.EdgeRouterRoles = []string{entityRef(edgeRouters[1].Id), entityRef(edgeRouters[2].Id), entityRef(edgeRouters[3].Id)}
		}
		if i == 5 {
			policy.IdentityRoles = []string{identityRoles[4], entityRef(identities[1].Id), entityRef(identities[2].Id), entityRef(identities[3].Id)}
			policy.EdgeRouterRoles = []string{edgeRouterRoles[4], entityRef(edgeRouters[1].Id), entityRef(edgeRouters[2].Id), entityRef(edgeRouters[3].Id)}
		}
		if i == 6 {
			policy.IdentityRoles = []string{AllRole}
			policy.EdgeRouterRoles = []string{AllRole}
		}
		if i == 7 {
			policy.Semantic = SemanticAnyOf
			policy.IdentityRoles = []string{identityRoles[0]}
			policy.EdgeRouterRoles = []string{edgeRouterRoles[0]}
		}
		if i == 8 {
			policy.Semantic = SemanticAnyOf
			policy.IdentityRoles = []string{identityRoles[1], identityRoles[2], identityRoles[3]}
			policy.EdgeRouterRoles = []string{edgeRouterRoles[1], edgeRouterRoles[2], edgeRouterRoles[3]}
		}

		policies = append(policies, policy)
		if oncreate {
			ctx.RequireCreate(policy)
		} else {
			ctx.RequireUpdate(policy)
		}
	}
	return policies
}

func (ctx *TestContext) validateEdgeRouterPolicies(identities []*Identity, edgeRouters []*EdgeRouter, policies []*EdgeRouterPolicy) {
	ctx.validateEdgeRouterPolicyIdentities(identities, policies)
	ctx.validateEdgeRouterPolicyEdgeRouters(edgeRouters, policies)
	ctx.validateEdgeRouterPolicyDenormalization()
}

func (ctx *TestContext) validateEdgeRouterPolicyIdentities(identities []*Identity, policies []*EdgeRouterPolicy) {
	for _, policy := range policies {
		count := 0
		relatedIdentities := ctx.getRelatedIds(policy, EntityTypeIdentities)
		for _, identity := range identities {
			relatedPolicies := ctx.getRelatedIds(identity, EntityTypeEdgeRouterPolicies)
			shouldContain := ctx.policyShouldMatch(policy.Semantic, policy.IdentityRoles, identity, identity.RoleAttributes)

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

func (ctx *TestContext) validateEdgeRouterPolicyDenormalization() {
	errorHolder := &errorz.ErrorHolderImpl{}
	errorHolder.SetError(ctx.db.View(func(tx *bbolt.Tx) error {
		return ctx.stores.EdgeRouterPolicy.CheckIntegrity(tx, false, func(err error, _ bool) {
			errorHolder.SetError(err)
		})
	}))
	ctx.NoError(errorHolder.GetError())
}

func (ctx *TestContext) validateEdgeRouterPolicyEdgeRouters(edgeRouters []*EdgeRouter, policies []*EdgeRouterPolicy) {
	for _, policy := range policies {
		count := 0
		relatedEdgeRouters := ctx.getRelatedIds(policy, db.EntityTypeRouters)
		for _, edgeRouter := range edgeRouters {
			relatedPolicies := ctx.getRelatedIds(edgeRouter, EntityTypeEdgeRouterPolicies)
			shouldContain := ctx.policyShouldMatch(policy.Semantic, policy.EdgeRouterRoles, edgeRouter, edgeRouter.RoleAttributes)
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

func (ctx *TestContext) policyShouldMatch(semantic string, roleSet []string, entity boltz.ExtEntity, roleAttribute []string) bool {
	roles, ids, err := splitRolesAndIds(roleSet)
	ctx.NoError(err)
	isIdMatch := stringz.Contains(ids, entity.GetId())
	isAllMatch := stringz.Contains(roles, "all")
	isRoleMatch := false
	if strings.EqualFold(semantic, SemanticAllOf) {
		isRoleMatch = len(roles) > 0 && stringz.ContainsAll(roleAttribute, roles...)
	} else if strings.EqualFold(semantic, SemanticAnyOf) {
		isRoleMatch = stringz.ContainsAny(roleAttribute, roles...)
	}
	return isIdMatch || isAllMatch || isRoleMatch
}
