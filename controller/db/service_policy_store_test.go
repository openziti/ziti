package db

import (
	"fmt"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/common/eid"
	"go.etcd.io/bbolt"
	"math/rand"
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

func newServicePolicy(name string) *ServicePolicy {
	policyType := PolicyTypeDial
	/* #nosec */
	if rand.Int()%2 == 0 {
		policyType = PolicyTypeBind
	}
	return &ServicePolicy{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          name,
		PolicyType:    policyType,
		Semantic:      SemanticAllOf,
	}
}

func (ctx *TestContext) testCreateServicePolicy(_ *testing.T) {
	ctx.CleanupAll()

	policy := newServicePolicy(eid.New())
	boltztest.RequireCreate(ctx, policy)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		boltztest.ValidateBaseline(ctx, policy)
		ctx.Equal(0, len(ctx.stores.ServicePolicy.GetRelatedEntitiesIdList(tx, policy.Id, EntityTypeServices)))
		ctx.Equal(0, len(ctx.stores.ServicePolicy.GetRelatedEntitiesIdList(tx, policy.Id, EntityTypeIdentities)))

		testPolicy, err := ctx.stores.ServicePolicy.LoadOneById(tx, policy.Id)
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
	err := boltztest.Create(ctx, policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'identityRoles' is invalid: no identities found with the given ids", invalidId))

	policy.IdentityRoles = []string{AllRole, roleRef("other")}
	err = boltztest.Create(ctx, policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v %v]' for 'identityRoles' is invalid: if using %v, it should be the only role specified", AllRole, roleRef("other"), AllRole))

	identityTypeId := ctx.getIdentityTypeId()
	identity := newIdentity(eid.New(), identityTypeId)
	boltztest.RequireCreate(ctx, identity)

	policy.IdentityRoles = []string{entityRef(identity.Id), entityRef(invalidId)}
	err = boltztest.Create(ctx, policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'identityRoles' is invalid: no identities found with the given ids", invalidId))

	policy.IdentityRoles = []string{entityRef(identity.Id)}
	boltztest.RequireCreate(ctx, policy)
	ctx.validateServicePolicyIdentities([]*Identity{identity}, []*ServicePolicy{policy})

	policy.IdentityRoles = append(policy.IdentityRoles, entityRef(invalidId))
	err = boltztest.Update(ctx, policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'identityRoles' is invalid: no identities found with the given ids", invalidId))
	boltztest.RequireDelete(ctx, policy)

	// test service roles
	policy.IdentityRoles = nil
	policy.ServiceRoles = []string{entityRef(invalidId)}
	err = boltztest.Create(ctx, policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'serviceRoles' is invalid: no services found with the given ids", invalidId))

	policy.ServiceRoles = []string{AllRole, roleRef("other")}
	err = boltztest.Create(ctx, policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v %v]' for 'serviceRoles' is invalid: if using %v, it should be the only role specified", AllRole, roleRef("other"), AllRole))

	service := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, service)

	policy.ServiceRoles = []string{entityRef(service.Id), entityRef(invalidId)}
	err = boltztest.Create(ctx, policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'serviceRoles' is invalid: no services found with the given ids", invalidId))

	policy.ServiceRoles = []string{entityRef(service.Id)}
	boltztest.RequireCreate(ctx, policy)
	ctx.validateServicePolicyServices([]*EdgeService{service}, []*ServicePolicy{policy})

	policy.ServiceRoles = append(policy.ServiceRoles, entityRef(invalidId))
	err = boltztest.Update(ctx, policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'serviceRoles' is invalid: no services found with the given ids", invalidId))
	boltztest.RequireDelete(ctx, policy)
}

func (ctx *TestContext) testServicePolicyUpdateDeleteRefs(_ *testing.T) {
	ctx.CleanupAll()

	// test identity roles
	policy := newServicePolicy(eid.New())
	identityTypeId := ctx.getIdentityTypeId()
	identity := newIdentity(eid.New(), identityTypeId)
	boltztest.RequireCreate(ctx, identity)

	policy.IdentityRoles = []string{entityRef(identity.Id)}
	boltztest.RequireCreate(ctx, policy)
	ctx.validateServicePolicyIdentities([]*Identity{identity}, []*ServicePolicy{policy})
	boltztest.RequireDelete(ctx, identity)
	boltztest.RequireReload(ctx, policy)
	ctx.Equal(0, len(policy.IdentityRoles), "identity id should have been removed from identity roles")

	identity = newIdentity(eid.New(), identityTypeId)
	boltztest.RequireCreate(ctx, identity)

	policy.IdentityRoles = []string{entityRef(identity.Id)}
	boltztest.RequireUpdate(ctx, policy)
	ctx.validateServicePolicyIdentities([]*Identity{identity}, []*ServicePolicy{policy})

	identity.Name = eid.New()
	boltztest.RequireUpdate(ctx, identity)
	boltztest.RequireReload(ctx, policy)
	ctx.True(stringz.Contains(policy.IdentityRoles, entityRef(identity.Id)))
	ctx.validateServicePolicyIdentities([]*Identity{identity}, []*ServicePolicy{policy})

	boltztest.RequireDelete(ctx, identity)
	boltztest.RequireReload(ctx, policy)
	ctx.Equal(0, len(policy.IdentityRoles), "identity name should have been removed from identity roles")

	// test service roles
	service := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, service)

	policy.ServiceRoles = []string{entityRef(service.Id)}
	boltztest.RequireUpdate(ctx, policy)
	ctx.validateServicePolicyServices([]*EdgeService{service}, []*ServicePolicy{policy})
	boltztest.RequireDelete(ctx, service)
	boltztest.RequireReload(ctx, policy)
	ctx.Equal(0, len(policy.ServiceRoles), "service id should have been removed from service roles")

	service = newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, service)

	policy.ServiceRoles = []string{entityRef(service.Id)}
	boltztest.RequireUpdate(ctx, policy)
	ctx.validateServicePolicyServices([]*EdgeService{service}, []*ServicePolicy{policy})

	service.Name = eid.New()
	boltztest.RequireUpdate(ctx, service)
	boltztest.RequireReload(ctx, policy)
	ctx.True(stringz.Contains(policy.ServiceRoles, entityRef(service.Id)))
	ctx.validateServicePolicyServices([]*EdgeService{service}, []*ServicePolicy{policy})

	boltztest.RequireDelete(ctx, service)
	boltztest.RequireReload(ctx, policy)
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
		boltztest.RequireCreate(ctx, identity)
		identities = append(identities, identity)
	}

	services := make([]*EdgeService, 0, 5)
	for i := 0; i < 5; i++ {
		service := newEdgeService(eid.New())
		boltztest.RequireCreate(ctx, service)
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
		relatedServices := ctx.getRelatedIds(policies[i], EntityTypeServices)
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
	boltztest.RequireCreate(ctx, identity)
	identities = append(identities, identity)

	stringz.Permutations(identityRolesAttrs, func(roles []string) {
		identity := newIdentity(eid.New(), identityTypeId, roles...)
		boltztest.RequireCreate(ctx, identity)
		identities = append(identities, identity)
	})

	// no roles
	service := newEdgeService(eid.New())
	boltztest.RequireCreate(ctx, service)
	services = append(services, service)

	stringz.Permutations(serviceRoleAttrs, func(roles []string) {
		service := newEdgeService(eid.New(), roles...)
		boltztest.RequireCreate(ctx, service)
		services = append(services, service)
	})

	ctx.validateServicePolicies(identities, services, policies)

	for _, identity := range identities {
		boltztest.RequireDelete(ctx, identity)
	}

	for _, service := range services {
		boltztest.RequireDelete(ctx, service)
	}

	identities = nil
	services = nil

	stringz.Permutations(identityRolesAttrs, func(roles []string) {
		identity := newIdentity(eid.New(), identityTypeId)
		boltztest.RequireCreate(ctx, identity)
		identity.RoleAttributes = roles
		boltztest.RequireUpdate(ctx, identity)
		identities = append(identities, identity)
	})

	stringz.Permutations(serviceRoleAttrs, func(roles []string) {
		service := newEdgeService(eid.New())
		boltztest.RequireCreate(ctx, service)
		service.RoleAttributes = roles
		boltztest.RequireUpdate(ctx, service)
		services = append(services, service)
	})

	ctx.validateServicePolicies(identities, services, policies)

	// ensure policies get cleaned up
	for _, policy := range policies {
		boltztest.RequireDelete(ctx, policy)
	}

	// test with policies created after identities/edge routers
	policies = ctx.createServicePolicies(identityRoles, serviceRoles, identities, services, true)

	ctx.validateServicePolicies(identities, services, policies)

	for _, policy := range policies {
		boltztest.RequireDelete(ctx, policy)
	}

	// test with policies created after identities/edge routers and roles added after created
	policies = ctx.createServicePolicies(identityRoles, serviceRoles, identities, services, false)

	ctx.validateServicePolicies(identities, services, policies)

	for _, identity := range identities {
		if len(identity.RoleAttributes) > 0 {
			identity.RoleAttributes = identity.RoleAttributes[1:]
			boltztest.RequireUpdate(ctx, identity)
		}
	}

	for _, service := range services {
		if len(service.RoleAttributes) > 0 {
			service.RoleAttributes = service.RoleAttributes[1:]
			boltztest.RequireUpdate(ctx, service)
		}
	}

	for _, policy := range policies {
		if len(policy.IdentityRoles) > 0 {
			policy.IdentityRoles = policy.IdentityRoles[1:]
		}
		if len(policy.ServiceRoles) > 0 {
			policy.ServiceRoles = policy.ServiceRoles[1:]
		}
		boltztest.RequireUpdate(ctx, policy)
	}

	ctx.validateServicePolicies(identities, services, policies)
}

func (ctx *TestContext) createServicePolicies(identityRoles, serviceRoles []string, identities []*Identity, services []*EdgeService, oncreate bool) []*ServicePolicy {
	var policies []*ServicePolicy
	for i := 0; i < 9; i++ {
		policy := newServicePolicy(eid.New())
		if !oncreate {
			boltztest.RequireCreate(ctx, policy)
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
			boltztest.RequireCreate(ctx, policy)
		} else {
			boltztest.RequireUpdate(ctx, policy)
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
		relatedServices := ctx.getRelatedIds(policy, EntityTypeServices)
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
	errorHolder.SetError(ctx.GetDb().View(func(tx *bbolt.Tx) error {
		return ctx.stores.ServicePolicy.CheckIntegrity(ctx.newViewTestCtx(tx), false, func(err error, _ bool) {
			errorHolder.SetError(err)
		})
	}))
	ctx.NoError(errorHolder.GetError())
}
