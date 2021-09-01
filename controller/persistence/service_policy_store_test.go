package persistence

import (
	"fmt"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/foundation/util/stringz"
	"go.etcd.io/bbolt"
	"sort"
	"testing"
)

func Test_ServicePolicyStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create service policies", ctx.testCreateServicePolicy)
	t.Run("test create/update service policies with invalid entity refs", ctx.testServicePolicyInvalidValues)
	t.Run("test service policy evaluation", ctx.testServicePolicyRoleEvaluation)
	t.Run("test update/delete referenced entities", ctx.testServicePolicyUpdateDeleteRefs)
}

func (ctx *TestContext) testCreateServicePolicy(_ *testing.T) {
	ctx.CleanupAll()

	policy := newServicePolicy(eid.New())
	ctx.RequireCreate(policy)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ctx.ValidateBaseline(policy)
		ctx.Equal(0, len(ctx.stores.ServicePolicy.GetRelatedEntitiesIdList(tx, policy.Id, db.EntityTypeServices)))
		ctx.Equal(0, len(ctx.stores.ServicePolicy.GetRelatedEntitiesIdList(tx, policy.Id, EntityTypeIdentities)))

		testPolicy, err := ctx.stores.ServicePolicy.LoadOneByName(tx, policy.Name)
		ctx.NoError(err)
		ctx.NotNil(testPolicy)
		ctx.Equal(policy.Name, testPolicy.Name)

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testServicePolicyInvalidValues(_ *testing.T) {
	ctx.CleanupAll()

	// test identity roles
	policy := newServicePolicy(eid.New())
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
	ctx.validateServicePolicyIdentities([]*Identity{identity}, []*ServicePolicy{policy})

	policy.IdentityRoles = append(policy.IdentityRoles, entityRef(invalidId))
	err = ctx.Update(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'identityRoles' is invalid: no identities found with the given ids", invalidId))
	ctx.RequireDelete(policy)

	// test service roles
	policy.IdentityRoles = nil
	policy.ServiceRoles = []string{entityRef(invalidId)}
	err = ctx.Create(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'serviceRoles' is invalid: no services found with the given ids", invalidId))

	policy.ServiceRoles = []string{AllRole, roleRef("other")}
	err = ctx.Create(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v %v]' for 'serviceRoles' is invalid: if using %v, it should be the only role specified", AllRole, roleRef("other"), AllRole))

	service := newEdgeService(eid.New())
	ctx.RequireCreate(service)

	policy.ServiceRoles = []string{entityRef(service.Id), entityRef(invalidId)}
	err = ctx.Create(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'serviceRoles' is invalid: no services found with the given ids", invalidId))

	policy.ServiceRoles = []string{entityRef(service.Id)}
	ctx.RequireCreate(policy)
	ctx.validateServicePolicyServices([]*EdgeService{service}, []*ServicePolicy{policy})

	policy.ServiceRoles = append(policy.ServiceRoles, entityRef(invalidId))
	err = ctx.Update(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'serviceRoles' is invalid: no services found with the given ids", invalidId))
	ctx.RequireDelete(policy)
}

func (ctx *TestContext) testServicePolicyUpdateDeleteRefs(_ *testing.T) {
	ctx.CleanupAll()

	// test identity roles
	policy := newServicePolicy(eid.New())
	identityTypeId := ctx.getIdentityTypeId()
	identity := newIdentity(eid.New(), identityTypeId)
	ctx.RequireCreate(identity)

	policy.IdentityRoles = []string{entityRef(identity.Id)}
	ctx.RequireCreate(policy)
	ctx.validateServicePolicyIdentities([]*Identity{identity}, []*ServicePolicy{policy})
	ctx.RequireDelete(identity)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.IdentityRoles), "identity id should have been removed from identity roles")

	identity = newIdentity(eid.New(), identityTypeId)
	ctx.RequireCreate(identity)

	policy.IdentityRoles = []string{entityRef(identity.Id)}
	ctx.RequireUpdate(policy)
	ctx.validateServicePolicyIdentities([]*Identity{identity}, []*ServicePolicy{policy})

	identity.Name = eid.New()
	ctx.RequireUpdate(identity)
	ctx.RequireReload(policy)
	ctx.True(stringz.Contains(policy.IdentityRoles, entityRef(identity.Id)))
	ctx.validateServicePolicyIdentities([]*Identity{identity}, []*ServicePolicy{policy})

	ctx.RequireDelete(identity)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.IdentityRoles), "identity name should have been removed from identity roles")

	// test service roles
	service := newEdgeService(eid.New())
	ctx.RequireCreate(service)

	policy.ServiceRoles = []string{entityRef(service.Id)}
	ctx.RequireUpdate(policy)
	ctx.validateServicePolicyServices([]*EdgeService{service}, []*ServicePolicy{policy})
	ctx.RequireDelete(service)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.ServiceRoles), "service id should have been removed from service roles")

	service = newEdgeService(eid.New())
	ctx.RequireCreate(service)

	policy.ServiceRoles = []string{entityRef(service.Id)}
	ctx.RequireUpdate(policy)
	ctx.validateServicePolicyServices([]*EdgeService{service}, []*ServicePolicy{policy})

	service.Name = eid.New()
	ctx.RequireUpdate(service)
	ctx.RequireReload(policy)
	ctx.True(stringz.Contains(policy.ServiceRoles, entityRef(service.Id)))
	ctx.validateServicePolicyServices([]*EdgeService{service}, []*ServicePolicy{policy})

	ctx.RequireDelete(service)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.ServiceRoles), "service name should have been removed from service roles")
}

func (ctx *TestContext) testServicePolicyRoleEvaluation(_ *testing.T) {
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

	identities := make([]*Identity, 0, 5)
	for i := 0; i < 5; i++ {
		identity := newIdentity(eid.New(), identityTypeId)
		ctx.RequireCreate(identity)
		identities = append(identities, identity)
	}

	services := make([]*EdgeService, 0, 5)
	for i := 0; i < 5; i++ {
		service := newEdgeService(eid.New())
		ctx.RequireCreate(service)
		services = append(services, service)
	}

	identityRolesAttrs := []string{"foo", "bar", eid.New(), "baz", eid.New(), "quux"}
	var identityRoles []string
	for _, role := range identityRolesAttrs {
		identityRoles = append(identityRoles, roleRef(role))
	}

	serviceRoleAttrs := []string{eid.New(), "another-role", "parsley, sage, rosemary and don't forget thyme", eid.New(), "blop", "asdf"}
	var serviceRoles []string
	for _, role := range serviceRoleAttrs {
		serviceRoles = append(serviceRoles, roleRef(role))
	}

	multipleIdentityList := []string{identities[1].Id, identities[2].Id, identities[3].Id}
	multipleServiceList := []string{services[1].Id, services[2].Id, services[3].Id}

	policies := ctx.createServicePolicies(identityRoles, serviceRoles, identities, services, true)

	for i := 0; i < 9; i++ {
		relatedServices := ctx.getRelatedIds(policies[i], db.EntityTypeServices)
		relatedIdentities := ctx.getRelatedIds(policies[i], EntityTypeIdentities)
		if i == 3 {
			ctx.Equal([]string{services[0].Id}, relatedServices)
			ctx.Equal([]string{identities[0].Id}, relatedIdentities)
		} else if i == 4 || i == 5 {
			sort.Strings(multipleServiceList)
			sort.Strings(multipleIdentityList)
			ctx.Equal(multipleServiceList, relatedServices)
			ctx.Equal(multipleIdentityList, relatedIdentities)
		} else if i == 6 {
			ctx.Equal(5, len(relatedServices))
			ctx.Equal(5, len(relatedIdentities))
		} else {
			ctx.Equal(0, len(relatedIdentities))
			ctx.Equal(0, len(relatedServices))
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
	service := newEdgeService(eid.New())
	ctx.RequireCreate(service)
	services = append(services, service)

	stringz.Permutations(serviceRoleAttrs, func(roles []string) {
		service := newEdgeService(eid.New(), roles...)
		ctx.RequireCreate(service)
		services = append(services, service)
	})

	ctx.validateServicePolicies(identities, services, policies)

	for _, identity := range identities {
		ctx.RequireDelete(identity)
	}

	for _, service := range services {
		ctx.RequireDelete(service)
	}

	identities = nil
	services = nil

	stringz.Permutations(identityRolesAttrs, func(roles []string) {
		identity := newIdentity(eid.New(), identityTypeId)
		ctx.RequireCreate(identity)
		identity.RoleAttributes = roles
		ctx.RequireUpdate(identity)
		identities = append(identities, identity)
	})

	stringz.Permutations(serviceRoleAttrs, func(roles []string) {
		service := newEdgeService(eid.New())
		ctx.RequireCreate(service)
		service.RoleAttributes = roles
		ctx.RequireUpdate(service)
		services = append(services, service)
	})

	ctx.validateServicePolicies(identities, services, policies)

	// ensure policies get cleaned up
	for _, policy := range policies {
		ctx.RequireDelete(policy)
	}

	// test with policies created after identities/edge routers
	policies = ctx.createServicePolicies(identityRoles, serviceRoles, identities, services, true)

	ctx.validateServicePolicies(identities, services, policies)

	for _, policy := range policies {
		ctx.RequireDelete(policy)
	}

	// test with policies created after identities/edge routers and roles added after created
	policies = ctx.createServicePolicies(identityRoles, serviceRoles, identities, services, false)

	ctx.validateServicePolicies(identities, services, policies)

	for _, identity := range identities {
		if len(identity.RoleAttributes) > 0 {
			identity.RoleAttributes = identity.RoleAttributes[1:]
			ctx.RequireUpdate(identity)
		}
	}

	for _, service := range services {
		if len(service.RoleAttributes) > 0 {
			service.RoleAttributes = service.RoleAttributes[1:]
			ctx.RequireUpdate(service)
		}
	}

	for _, policy := range policies {
		if len(policy.IdentityRoles) > 0 {
			policy.IdentityRoles = policy.IdentityRoles[1:]
		}
		if len(policy.ServiceRoles) > 0 {
			policy.ServiceRoles = policy.ServiceRoles[1:]
		}
		ctx.RequireUpdate(policy)
	}

	ctx.validateServicePolicies(identities, services, policies)
}

func (ctx *TestContext) createServicePolicies(identityRoles, serviceRoles []string, identities []*Identity, services []*EdgeService, oncreate bool) []*ServicePolicy {
	var policies []*ServicePolicy
	for i := 0; i < 9; i++ {
		policy := newServicePolicy(eid.New())
		if !oncreate {
			ctx.RequireCreate(policy)
		}
		if i == 1 {
			policy.IdentityRoles = []string{identityRoles[0]}
			policy.ServiceRoles = []string{serviceRoles[0]}
		}
		if i == 2 {
			policy.IdentityRoles = []string{identityRoles[1], identityRoles[2], identityRoles[3]}
			policy.ServiceRoles = []string{serviceRoles[1], serviceRoles[2], serviceRoles[3]}
		}
		if i == 3 {
			policy.IdentityRoles = []string{entityRef(identities[0].Id)}
			policy.ServiceRoles = []string{entityRef(services[0].Id)}
		}
		if i == 4 {
			policy.IdentityRoles = []string{entityRef(identities[1].Id), entityRef(identities[2].Id), entityRef(identities[3].Id)}
			policy.ServiceRoles = []string{entityRef(services[1].Id), entityRef(services[2].Id), entityRef(services[3].Id)}
		}
		if i == 5 {
			policy.IdentityRoles = []string{identityRoles[4], entityRef(identities[1].Id), entityRef(identities[2].Id), entityRef(identities[3].Id)}
			policy.ServiceRoles = []string{serviceRoles[4], entityRef(services[1].Id), entityRef(services[2].Id), entityRef(services[3].Id)}
		}
		if i == 6 {
			policy.IdentityRoles = []string{AllRole}
			policy.ServiceRoles = []string{AllRole}
		}
		if i == 7 {
			policy.Semantic = SemanticAnyOf
			policy.IdentityRoles = []string{identityRoles[0]}
			policy.ServiceRoles = []string{serviceRoles[0]}
		}
		if i == 8 {
			policy.Semantic = SemanticAnyOf
			policy.IdentityRoles = []string{identityRoles[1], identityRoles[2], identityRoles[3]}
			policy.ServiceRoles = []string{serviceRoles[1], serviceRoles[2], serviceRoles[3]}
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

func (ctx *TestContext) validateServicePolicies(identities []*Identity, services []*EdgeService, policies []*ServicePolicy) {
	ctx.validateServicePolicyIdentities(identities, policies)
	ctx.validateServicePolicyServices(services, policies)
	ctx.validateServicePolicyDenormalization()
}

func (ctx *TestContext) validateServicePolicyIdentities(identities []*Identity, policies []*ServicePolicy) {
	for _, policy := range policies {
		count := 0
		relatedIdentities := ctx.getRelatedIds(policy, EntityTypeIdentities)
		for _, identity := range identities {
			relatedPolicies := ctx.getRelatedIds(identity, EntityTypeServicePolicies)
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

func (ctx *TestContext) validateServicePolicyServices(services []*EdgeService, policies []*ServicePolicy) {
	for _, policy := range policies {
		count := 0
		relatedServices := ctx.getRelatedIds(policy, db.EntityTypeServices)
		for _, service := range services {
			relatedPolicies := ctx.getRelatedIds(service, EntityTypeServicePolicies)
			shouldContain := ctx.policyShouldMatch(policy.Semantic, policy.ServiceRoles, service, service.RoleAttributes)
			policyContains := stringz.Contains(relatedServices, service.Id)
			ctx.Equal(shouldContain, policyContains, "entity roles attr: %v. policy roles: %v", service.RoleAttributes, policy.ServiceRoles)
			if shouldContain {
				count++
			}

			entityContains := stringz.Contains(relatedPolicies, policy.Id)
			ctx.Equal(shouldContain, entityContains, "identity: %v, policy: %v, entity roles attr: %v. policy roles: %v",
				service.Id, policy.Id, service.RoleAttributes, policy.ServiceRoles)
		}
		ctx.Equal(count, len(relatedServices))
	}
}

func (ctx *TestContext) validateServicePolicyDenormalization() {
	errorHolder := &errorz.ErrorHolderImpl{}
	errorHolder.SetError(ctx.db.View(func(tx *bbolt.Tx) error {
		return ctx.stores.ServicePolicy.CheckIntegrity(tx, false, func(err error, _ bool) {
			errorHolder.SetError(err)
		})
	}))
	ctx.NoError(errorHolder.GetError())
}
