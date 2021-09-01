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

func Test_ServiceEdgeRouterPolicyStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test create edge router policies", ctx.testCreateServiceEdgeRouterPolicy)
	t.Run("test create/update edge router policies with invalid entity refs", ctx.testServiceEdgeRouterPolicyInvalidValues)
	t.Run("test edge router policy evaluation", ctx.testServiceEdgeRouterPolicyRoleEvaluation)
	t.Run("test update/delete referenced entities", ctx.testServiceEdgeRouterPolicyUpdateDeleteRefs)
}

func (ctx *TestContext) testCreateServiceEdgeRouterPolicy(_ *testing.T) {
	ctx.CleanupAll()

	policy := newServiceEdgeRouterPolicy(eid.New())
	ctx.RequireCreate(policy)
	ctx.ValidateBaseline(policy)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ctx.Equal(0, len(ctx.stores.ServiceEdgeRouterPolicy.GetRelatedEntitiesIdList(tx, policy.Id, db.EntityTypeRouters)))
		ctx.Equal(0, len(ctx.stores.ServiceEdgeRouterPolicy.GetRelatedEntitiesIdList(tx, policy.Id, db.EntityTypeServices)))

		testPolicy, err := ctx.stores.ServiceEdgeRouterPolicy.LoadOneByName(tx, policy.Name)
		ctx.NoError(err)
		ctx.NotNil(testPolicy)
		ctx.Equal(policy.Name, testPolicy.Name)

		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testServiceEdgeRouterPolicyInvalidValues(_ *testing.T) {
	ctx.CleanupAll()

	// test service roles
	policy := newServiceEdgeRouterPolicy(eid.New())
	invalidId := eid.New()
	policy.ServiceRoles = []string{entityRef(invalidId)}
	err := ctx.Create(policy)
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
	ctx.validateServiceEdgeRouterPolicyServices([]*EdgeService{service}, []*ServiceEdgeRouterPolicy{policy})

	policy.ServiceRoles = append(policy.ServiceRoles, entityRef(invalidId))
	err = ctx.Update(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'serviceRoles' is invalid: no services found with the given ids", invalidId))
	ctx.RequireDelete(policy)

	// test edgeRouter roles
	policy.ServiceRoles = nil
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
	ctx.validateServiceEdgeRouterPolicyEdgeRouters([]*EdgeRouter{edgeRouter}, []*ServiceEdgeRouterPolicy{policy})

	policy.EdgeRouterRoles = append(policy.EdgeRouterRoles, entityRef(invalidId))
	err = ctx.Update(policy)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'edgeRouterRoles' is invalid: no routers found with the given ids", invalidId))
	ctx.RequireDelete(policy)
}

func (ctx *TestContext) testServiceEdgeRouterPolicyUpdateDeleteRefs(_ *testing.T) {
	ctx.CleanupAll()

	// test service roles
	policy := newServiceEdgeRouterPolicy(eid.New())
	service := newEdgeService(eid.New())
	ctx.RequireCreate(service)

	policy.ServiceRoles = []string{entityRef(service.Id)}
	ctx.RequireCreate(policy)
	ctx.validateServiceEdgeRouterPolicyServices([]*EdgeService{service}, []*ServiceEdgeRouterPolicy{policy})
	ctx.RequireDelete(service)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.ServiceRoles), "service id should have been removed from service roles")

	service = newEdgeService(eid.New())
	ctx.RequireCreate(service)

	policy.ServiceRoles = []string{entityRef(service.Id)}
	ctx.RequireUpdate(policy)
	ctx.validateServiceEdgeRouterPolicyServices([]*EdgeService{service}, []*ServiceEdgeRouterPolicy{policy})

	service.Name = eid.New()
	ctx.RequireUpdate(service)
	ctx.RequireReload(policy)
	ctx.True(stringz.Contains(policy.ServiceRoles, entityRef(service.Id)))
	ctx.validateServiceEdgeRouterPolicyServices([]*EdgeService{service}, []*ServiceEdgeRouterPolicy{policy})

	ctx.RequireDelete(service)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.ServiceRoles), "service name should have been removed from service roles")

	// test edgeRouter roles
	edgeRouter := newEdgeRouter(eid.New())
	ctx.RequireCreate(edgeRouter)

	policy.EdgeRouterRoles = []string{entityRef(edgeRouter.Id)}
	ctx.RequireUpdate(policy)
	ctx.validateServiceEdgeRouterPolicyEdgeRouters([]*EdgeRouter{edgeRouter}, []*ServiceEdgeRouterPolicy{policy})
	ctx.RequireDelete(edgeRouter)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.EdgeRouterRoles), "edgeRouter id should have been removed from edgeRouter roles")

	edgeRouter = newEdgeRouter(eid.New())
	ctx.RequireCreate(edgeRouter)

	policy.EdgeRouterRoles = []string{entityRef(edgeRouter.Id)}
	ctx.RequireUpdate(policy)
	ctx.validateServiceEdgeRouterPolicyEdgeRouters([]*EdgeRouter{edgeRouter}, []*ServiceEdgeRouterPolicy{policy})

	edgeRouter.Name = eid.New()
	ctx.RequireUpdate(edgeRouter)
	ctx.RequireReload(policy)
	ctx.True(stringz.Contains(policy.EdgeRouterRoles, entityRef(edgeRouter.Id)))
	ctx.validateServiceEdgeRouterPolicyEdgeRouters([]*EdgeRouter{edgeRouter}, []*ServiceEdgeRouterPolicy{policy})

	ctx.RequireDelete(edgeRouter)
	ctx.RequireReload(policy)
	ctx.Equal(0, len(policy.EdgeRouterRoles), "edgeRouter name should have been removed from edgeRouter roles")
}

func (ctx *TestContext) testServiceEdgeRouterPolicyRoleEvaluation(_ *testing.T) {
	ctx.CleanupAll()

	// create some services, edge routers for reference by id
	// create initial policies, check state
	// create edge routers/services with roles on create, check state
	// delete all er/services, check state
	// create edge routers/services with roles added after create, check state
	// add 5 new policies, check
	// modify polices, add roles, check
	// modify policies, remove roles, check

	var services []*EdgeService
	for i := 0; i < 5; i++ {
		service := newEdgeService(eid.New())
		ctx.RequireCreate(service)
		services = append(services, service)
	}

	var edgeRouters []*EdgeRouter
	for i := 0; i < 5; i++ {
		edgeRouter := newEdgeRouter(eid.New())
		ctx.RequireCreate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	}

	serviceRolesAttrs := []string{"foo", "bar", eid.New(), "baz", eid.New(), "quux"}
	var serviceRoles []string
	for _, role := range serviceRolesAttrs {
		serviceRoles = append(serviceRoles, roleRef(role))
	}

	edgeRouterRoleAttrs := []string{eid.New(), "another-role", "parsley, sage, rosemary and don't forget thyme", eid.New(), "blop", "asdf"}
	var edgeRouterRoles []string
	for _, role := range edgeRouterRoleAttrs {
		edgeRouterRoles = append(edgeRouterRoles, roleRef(role))
	}

	multipleServiceList := []string{services[1].Id, services[2].Id, services[3].Id}
	multipleEdgeRouterList := []string{edgeRouters[1].Id, edgeRouters[2].Id, edgeRouters[3].Id}

	policies := ctx.createServiceEdgeRouterPolicies(serviceRoles, edgeRouterRoles, services, edgeRouters, true)

	for i := 0; i < 9; i++ {
		relatedEdgeRouters := ctx.getRelatedIds(policies[i], db.EntityTypeRouters)
		relatedServices := ctx.getRelatedIds(policies[i], db.EntityTypeServices)
		if i == 3 {
			ctx.Equal([]string{edgeRouters[0].Id}, relatedEdgeRouters)
			ctx.Equal([]string{services[0].Id}, relatedServices)
		} else if i == 4 || i == 5 {
			sort.Strings(multipleEdgeRouterList)
			sort.Strings(multipleServiceList)
			ctx.Equal(multipleEdgeRouterList, relatedEdgeRouters)
			ctx.Equal(multipleServiceList, relatedServices)
		} else if i == 6 {
			ctx.Equal(5, len(relatedEdgeRouters))
			ctx.Equal(5, len(relatedServices))
		} else {
			ctx.Equal(0, len(relatedServices))
			ctx.Equal(0, len(relatedEdgeRouters))
		}
	}

	// no roles
	service := newEdgeService(eid.New())
	ctx.RequireCreate(service)
	services = append(services, service)

	stringz.Permutations(serviceRolesAttrs, func(roles []string) {
		service := newEdgeService(eid.New(), roles...)
		ctx.RequireCreate(service)
		services = append(services, service)
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

	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	for _, service := range services {
		ctx.RequireDelete(service)
	}

	for _, edgeRouter := range edgeRouters {
		ctx.RequireDelete(edgeRouter)
	}

	services = nil
	edgeRouters = nil

	stringz.Permutations(serviceRolesAttrs, func(roles []string) {
		service := newEdgeService(eid.New())
		ctx.RequireCreate(service)
		service.RoleAttributes = roles
		ctx.RequireUpdate(service)
		services = append(services, service)
	})

	stringz.Permutations(edgeRouterRoleAttrs, func(roles []string) {
		edgeRouter := newEdgeRouter(eid.New())
		ctx.RequireCreate(edgeRouter)
		edgeRouter.RoleAttributes = roles
		ctx.RequireUpdate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	})

	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	// ensure policies get cleaned up
	for _, policy := range policies {
		ctx.RequireDelete(policy)
	}

	// test with policies created after services/edge routers
	policies = ctx.createServiceEdgeRouterPolicies(serviceRoles, edgeRouterRoles, services, edgeRouters, true)

	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	for _, policy := range policies {
		ctx.RequireDelete(policy)
	}

	// test with policies created after services/edge routers and roles added after created
	policies = ctx.createServiceEdgeRouterPolicies(serviceRoles, edgeRouterRoles, services, edgeRouters, false)

	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)

	for _, service := range services {
		if len(service.RoleAttributes) > 0 {
			service.RoleAttributes = service.RoleAttributes[1:]
			ctx.RequireUpdate(service)
		}
	}

	for _, edgeRouter := range edgeRouters {
		if len(edgeRouter.RoleAttributes) > 0 {
			edgeRouter.RoleAttributes = edgeRouter.RoleAttributes[1:]
			ctx.RequireUpdate(edgeRouter)
		}
	}

	for _, policy := range policies {
		if len(policy.ServiceRoles) > 0 {
			policy.ServiceRoles = policy.ServiceRoles[1:]
		}
		if len(policy.EdgeRouterRoles) > 0 {
			policy.EdgeRouterRoles = policy.EdgeRouterRoles[1:]
		}
		ctx.RequireUpdate(policy)
	}

	ctx.validateServiceEdgeRouterPolicies(services, edgeRouters, policies)
}

func (ctx *TestContext) createServiceEdgeRouterPolicies(serviceRoles, edgeRouterRoles []string, services []*EdgeService, edgeRouters []*EdgeRouter, oncreate bool) []*ServiceEdgeRouterPolicy {
	var policies []*ServiceEdgeRouterPolicy
	for i := 0; i < 9; i++ {
		policy := newServiceEdgeRouterPolicy(eid.New())
		policy.Semantic = SemanticAllOf

		if !oncreate {
			ctx.RequireCreate(policy)
		}
		if i == 1 {
			policy.ServiceRoles = []string{serviceRoles[0]}
			policy.EdgeRouterRoles = []string{edgeRouterRoles[0]}
		}
		if i == 2 {
			policy.ServiceRoles = []string{serviceRoles[1], serviceRoles[2], serviceRoles[3]}
			policy.EdgeRouterRoles = []string{edgeRouterRoles[1], edgeRouterRoles[2], edgeRouterRoles[3]}
		}
		if i == 3 {
			policy.ServiceRoles = []string{entityRef(services[0].Id)}
			policy.EdgeRouterRoles = []string{entityRef(edgeRouters[0].Id)}
		}
		if i == 4 {
			policy.ServiceRoles = []string{entityRef(services[1].Id), entityRef(services[2].Id), entityRef(services[3].Id)}
			policy.EdgeRouterRoles = []string{entityRef(edgeRouters[1].Id), entityRef(edgeRouters[2].Id), entityRef(edgeRouters[3].Id)}
		}
		if i == 5 {
			policy.ServiceRoles = []string{serviceRoles[4], entityRef(services[1].Id), entityRef(services[2].Id), entityRef(services[3].Id)}
			policy.EdgeRouterRoles = []string{edgeRouterRoles[4], entityRef(edgeRouters[1].Id), entityRef(edgeRouters[2].Id), entityRef(edgeRouters[3].Id)}
		}
		if i == 6 {
			policy.ServiceRoles = []string{AllRole}
			policy.EdgeRouterRoles = []string{AllRole}
		}
		if i == 7 {
			policy.Semantic = SemanticAnyOf
			policy.ServiceRoles = []string{serviceRoles[0]}
			policy.EdgeRouterRoles = []string{edgeRouterRoles[0]}
		}
		if i == 8 {
			policy.Semantic = SemanticAnyOf
			policy.ServiceRoles = []string{serviceRoles[1], serviceRoles[2], serviceRoles[3]}
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

func (ctx *TestContext) validateServiceEdgeRouterPolicies(services []*EdgeService, edgeRouters []*EdgeRouter, policies []*ServiceEdgeRouterPolicy) {
	ctx.validateServiceEdgeRouterPolicyServices(services, policies)
	ctx.validateServiceEdgeRouterPolicyEdgeRouters(edgeRouters, policies)
	ctx.validateServiceEdgeRouterPolicyDenormalization()
}

func (ctx *TestContext) validateServiceEdgeRouterPolicyServices(services []*EdgeService, policies []*ServiceEdgeRouterPolicy) {
	for _, policy := range policies {
		count := 0
		relatedServices := ctx.getRelatedIds(policy, db.EntityTypeServices)
		for _, service := range services {
			relatedPolicies := ctx.getRelatedIds(service, EntityTypeServiceEdgeRouterPolicies)
			shouldContain := ctx.policyShouldMatch(policy.Semantic, policy.ServiceRoles, service, service.RoleAttributes)

			policyContains := stringz.Contains(relatedServices, service.Id)
			ctx.Equal(shouldContain, policyContains, "entity roles attr: %v. policy roles: %v", service.RoleAttributes, policy.ServiceRoles)
			if shouldContain {
				count++
			}

			entityContains := stringz.Contains(relatedPolicies, policy.Id)
			ctx.Equal(shouldContain, entityContains, "service: %v, policy: %v, entity roles attr: %v. policy roles: %v",
				service.Id, policy.Id, service.RoleAttributes, policy.ServiceRoles)
		}
		ctx.Equal(count, len(relatedServices))
	}
}

func (ctx *TestContext) validateServiceEdgeRouterPolicyEdgeRouters(edgeRouters []*EdgeRouter, policies []*ServiceEdgeRouterPolicy) {
	for _, policy := range policies {
		count := 0
		relatedEdgeRouters := ctx.getRelatedIds(policy, db.EntityTypeRouters)
		for _, edgeRouter := range edgeRouters {
			relatedPolicies := ctx.getRelatedIds(edgeRouter, EntityTypeServiceEdgeRouterPolicies)
			shouldContain := ctx.policyShouldMatch(policy.Semantic, policy.EdgeRouterRoles, edgeRouter, edgeRouter.RoleAttributes)
			policyContains := stringz.Contains(relatedEdgeRouters, edgeRouter.Id)
			ctx.Equal(shouldContain, policyContains, "entity roles attr: %v. policy roles: %v", edgeRouter.RoleAttributes, policy.EdgeRouterRoles)
			if shouldContain {
				count++
			}

			entityContains := stringz.Contains(relatedPolicies, policy.Id)
			ctx.Equal(shouldContain, entityContains, "service: %v, policy: %v, entity roles attr: %v. policy roles: %v",
				edgeRouter.Id, policy.Id, edgeRouter.RoleAttributes, policy.EdgeRouterRoles)
		}
		ctx.Equal(count, len(relatedEdgeRouters))
	}
}

func (ctx *TestContext) validateServiceEdgeRouterPolicyDenormalization() {
	errorHolder := &errorz.ErrorHolderImpl{}
	errorHolder.SetError(ctx.db.View(func(tx *bbolt.Tx) error {
		return ctx.stores.ServiceEdgeRouterPolicy.CheckIntegrity(tx, false, func(err error, _ bool) {
			errorHolder.SetError(err)
		})
	}))
	ctx.NoError(errorHolder.GetError())
}
