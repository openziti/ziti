/*
	Copyright 2019 Netfoundry, Inc.

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

package persistence

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"go.etcd.io/bbolt"
)

func Test_EdgeServiceStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test service parent child relationship", ctx.testServiceParentChild)
	t.Run("test create invalid api services", ctx.testCreateInvalidServices)
	t.Run("test create service", ctx.testCreateServices)
	t.Run("test load/query services", ctx.testLoadQueryServices)
	t.Run("test update services", ctx.testUpdateServices)
	t.Run("test delete services", ctx.testDeleteServices)
	t.Run("test edge router role with invalid entity refs", ctx.testServiceEdgeRouterRolesInvalidValues)
	t.Run("test edge router role evaluation", ctx.testServiceEdgeRouterRoleEvaluation)
	t.Run("test update/delete referenced entities", ctx.testServiceEdgeRouterRolesUpdateDeleteRefs)
}

func (ctx *TestContext) testServiceParentChild(_ *testing.T) {
	fabricService := &network.Service{
		Id:              uuid.New().String(),
		Binding:         uuid.New().String(),
		EndpointAddress: uuid.New().String(),
		Egress:          uuid.New().String(),
	}

	ctx.requireCreate(fabricService)
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ctx.False(ctx.stores.EdgeService.IsEntityPresent(tx, fabricService.GetId()))

		_, count, err := ctx.stores.EdgeService.QueryIds(tx, "true")
		ctx.NoError(err)
		ctx.Equal(int64(0), count)
		return nil
	})
	ctx.NoError(err)

	edgeService := &EdgeService{
		Service:     *fabricService,
		Name:        uuid.New().String(),
		DnsHostname: uuid.New().String(),
		DnsPort:     0,
	}

	ctx.requireCreate(edgeService)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		query := fmt.Sprintf(`binding = "%v" and name = "%v"`, fabricService.Binding, edgeService.Name)
		ids, _, err := ctx.stores.EdgeService.QueryIds(tx, query)
		if err != nil {
			return err
		}
		ctx.Equal(1, len(ids))
		ctx.Equal(fabricService.Id, ids[0])
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testCreateInvalidServices(_ *testing.T) {
	defer ctx.cleanupAll()

	identity := ctx.requireNewIdentity("test-user", false)
	apiSession := NewApiSession(identity.Id)
	ctx.requireCreate(apiSession)

	edgeService := &EdgeService{
		Service: network.Service{
			Id:              uuid.New().String(),
			Binding:         uuid.New().String(),
			EndpointAddress: uuid.New().String(),
			Egress:          uuid.New().String(),
		},
		Name:        uuid.New().String(),
		DnsHostname: uuid.New().String(),
		DnsPort:     uint16(rand.Uint32()),
	}

	ctx.requireCreate(edgeService)
	err := ctx.create(edgeService)
	ctx.EqualError(err, fmt.Sprintf("an entity of type services already exists with id %v", edgeService.Id))
}

func (ctx *TestContext) testCreateServices(_ *testing.T) {
	defer ctx.cleanupAll()

	edgeService := &EdgeService{
		Service: network.Service{
			Id:              uuid.New().String(),
			Binding:         uuid.New().String(),
			EndpointAddress: uuid.New().String(),
			Egress:          uuid.New().String(),
		},
		Name:        uuid.New().String(),
		DnsHostname: uuid.New().String(),
		DnsPort:     uint16(rand.Uint32()),
	}
	ctx.requireCreate(edgeService)
	ctx.validateBaseline(edgeService)
}

type serviceTestEntities struct {
	servicePolicy *ServicePolicy
	identity1     *Identity
	apiSession1   *ApiSession
	service1      *EdgeService
	service2      *EdgeService
	session1      *Session
	session2      *Session
}

func (ctx *TestContext) createServiceTestEntities() *serviceTestEntities {
	identity1 := ctx.requireNewIdentity("admin1", true)

	apiSession1 := NewApiSession(identity1.Id)

	role := uuid.New().String()

	ctx.requireCreate(apiSession1)
	servicePolicy := ctx.requireNewServicePolicy(PolicyTypeDial, ss(), ss(roleRef(role)))

	service1 := &EdgeService{
		Service: network.Service{
			Id:              uuid.New().String(),
			Binding:         uuid.New().String(),
			EndpointAddress: uuid.New().String(),
			Egress:          uuid.New().String(),
		},
		Name:           uuid.New().String(),
		DnsHostname:    uuid.New().String(),
		DnsPort:        uint16(rand.Uint32()),
		RoleAttributes: []string{role},
	}

	ctx.requireCreate(service1)
	service2 := ctx.requireNewService(uuid.New().String())

	session1 := NewSession(apiSession1.Id, service1.Id)
	ctx.requireCreate(session1)

	session2 := NewSession(apiSession1.Id, service2.Id)
	ctx.requireCreate(session2)

	return &serviceTestEntities{
		servicePolicy: servicePolicy,
		identity1:     identity1,
		apiSession1:   apiSession1,
		service1:      service1,
		service2:      service2,
		session1:      session1,
		session2:      session2,
	}
}

func (ctx *TestContext) testLoadQueryServices(_ *testing.T) {
	ctx.cleanupAll()

	entities := ctx.createServiceTestEntities()

	err := ctx.db.View(func(tx *bbolt.Tx) error {
		service, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		service, err = ctx.stores.EdgeService.LoadOneByName(tx, entities.service1.Name)
		ctx.NoError(err)
		ctx.NotNil(service)
		ctx.EqualValues(entities.service1.Id, service.Id)

		query := fmt.Sprintf(`anyOf(sessions) = "%v"`, entities.session1.Id)
		ids, _, err := ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(1, len(ids))
		ctx.Equal(entities.service1.Id, ids[0])

		query = fmt.Sprintf(`anyOf(servicePolicies) = "%v"`, entities.servicePolicy.Id)
		ids, _, err = ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.True(stringz.Contains(ids, entities.service1.Id))

		query = fmt.Sprintf(`anyOf(sessions.apiSession) = "%v"`, entities.apiSession1.Id)
		ids, _, err = ctx.stores.EdgeService.QueryIds(tx, query)
		ctx.NoError(err)
		ctx.EqualValues(2, len(ids))
		ctx.True(stringz.Contains(ids, entities.service1.Id))
		ctx.True(stringz.Contains(ids, entities.service2.Id))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testUpdateServices(_ *testing.T) {
	ctx.cleanupAll()
	entities := ctx.createServiceTestEntities()
	earlier := time.Now()
	time.Sleep(time.Millisecond * 50)

	err := ctx.db.Update(func(tx *bbolt.Tx) error {
		original, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(original)

		service, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(service)

		tags := ctx.createTags()
		now := time.Now()
		service.Name = uuid.New().String()
		service.Binding = uuid.New().String()
		service.EndpointAddress = uuid.New().String()
		service.Egress = uuid.New().String()
		service.UpdatedAt = earlier
		service.CreatedAt = now
		service.Tags = tags

		err = ctx.stores.EdgeService.Update(boltz.NewMutateContext(tx), service, nil)
		ctx.NoError(err)
		loaded, err := ctx.stores.EdgeService.LoadOneById(tx, entities.service1.Id)
		ctx.NoError(err)
		ctx.NotNil(loaded)
		ctx.EqualValues(original.CreatedAt, loaded.CreatedAt)
		ctx.True(loaded.UpdatedAt.Equal(now) || loaded.UpdatedAt.After(now))
		service.CreatedAt = loaded.CreatedAt
		service.UpdatedAt = loaded.UpdatedAt
		ctx.True(cmp.Equal(service, loaded), cmp.Diff(service, loaded))
		return nil
	})
	ctx.NoError(err)
}

func (ctx *TestContext) testDeleteServices(_ *testing.T) {
	ctx.cleanupAll()
	entities := ctx.createServiceTestEntities()
	ctx.requireDelete(entities.service1)
	ctx.requireDelete(entities.service2)
}

func (ctx *TestContext) testServiceEdgeRouterRolesInvalidValues(_ *testing.T) {
	ctx.cleanupAll()

	service := newEdgeService(uuid.New().String())
	invalidId := uuid.New().String()
	service.EdgeRouterRoles = []string{entityRef(invalidId)}
	err := ctx.create(service)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'edgeRouterRoles' is invalid: no edgeRouters found with the given names/ids", invalidId))

	edgeRouter := newEdgeRouter(uuid.New().String())
	ctx.requireCreate(edgeRouter)

	service.EdgeRouterRoles = []string{entityRef(edgeRouter.Id), entityRef(invalidId)}
	err = ctx.create(service)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'edgeRouterRoles' is invalid: no edgeRouters found with the given names/ids", invalidId))

	service.EdgeRouterRoles = []string{entityRef(edgeRouter.Id)}
	ctx.requireCreate(service)
	ctx.validateServiceEdgeRouters([]*EdgeRouter{edgeRouter}, []*EdgeService{service})
	ctx.requireDelete(service)

	service.EdgeRouterRoles = []string{entityRef(edgeRouter.Name)}
	ctx.requireCreate(service)
	ctx.validateServiceEdgeRouters([]*EdgeRouter{edgeRouter}, []*EdgeService{service})

	service.EdgeRouterRoles = append(service.EdgeRouterRoles, entityRef(invalidId))
	err = ctx.update(service)
	ctx.EqualError(err, fmt.Sprintf("the value '[%v]' for 'edgeRouterRoles' is invalid: no edgeRouters found with the given names/ids", invalidId))
	ctx.requireDelete(service)
}

func (ctx *TestContext) testServiceEdgeRouterRolesUpdateDeleteRefs(_ *testing.T) {
	ctx.cleanupAll()

	// test edgeRouter roles
	service := newEdgeService(uuid.New().String())
	ctx.requireCreate(service)

	edgeRouter := newEdgeRouter(uuid.New().String())
	ctx.requireCreate(edgeRouter)

	service.EdgeRouterRoles = []string{entityRef(edgeRouter.Id)}
	ctx.requireUpdate(service)
	ctx.validateServiceEdgeRouters([]*EdgeRouter{edgeRouter}, []*EdgeService{service})
	ctx.requireDelete(edgeRouter)
	ctx.requireReload(service)
	ctx.Equal(0, len(service.EdgeRouterRoles), "edgeRouter id should have been removed from edgeRouter roles")

	edgeRouter = newEdgeRouter(uuid.New().String())
	ctx.requireCreate(edgeRouter)

	service.EdgeRouterRoles = []string{entityRef(edgeRouter.Name)}
	ctx.requireUpdate(service)
	ctx.validateServiceEdgeRouters([]*EdgeRouter{edgeRouter}, []*EdgeService{service})

	edgeRouter.Name = uuid.New().String()
	ctx.requireUpdate(edgeRouter)
	ctx.requireReload(service)
	ctx.True(stringz.Contains(service.EdgeRouterRoles, entityRef(edgeRouter.Name)))
	ctx.validateServiceEdgeRouters([]*EdgeRouter{edgeRouter}, []*EdgeService{service})

	ctx.requireDelete(edgeRouter)
	ctx.requireReload(service)
	ctx.Equal(0, len(service.EdgeRouterRoles), "edgeRouter name should have been removed from edgeRouter roles")
}

func (ctx *TestContext) testServiceEdgeRouterRoleEvaluation(_ *testing.T) {
	ctx.cleanupAll()

	// create some identities, edge routers for reference by id
	// create initial policies, check state
	// create edge routers/identities with roles on create, check state
	// delete all er/identities, check state
	// create edge routers/identities with roles added after create, check state
	// add 5 new policies, check
	// modify polices, add roles, check
	// modify policies, remove roles, check

	var edgeRouters []*EdgeRouter
	for i := 0; i < 5; i++ {
		edgeRouter := newEdgeRouter(uuid.New().String())
		ctx.requireCreate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	}

	edgeRouterRoleAttrs := []string{uuid.New().String(), "another-role", "parsley, sage, rosemary and don't forget thyme", uuid.New().String(), "blop", "asdf"}
	var edgeRouterRoles []string
	for _, role := range edgeRouterRoleAttrs {
		edgeRouterRoles = append(edgeRouterRoles, roleRef(role))
	}

	multipleEdgeRouterList := []string{edgeRouters[1].Id, edgeRouters[2].Id, edgeRouters[3].Id}

	services := ctx.createServiceWithEdgeRouterLimits(edgeRouterRoles, edgeRouters, true)

	for i := 0; i < 7; i++ {
		relatedEdgeRouters := ctx.getRelatedIds(services[i], EntityTypeEdgeRouters)
		if i == 3 {
			ctx.Equal([]string{edgeRouters[0].Id}, relatedEdgeRouters)
		} else if i == 4 || i == 5 {
			sort.Strings(multipleEdgeRouterList)
			ctx.Equal(multipleEdgeRouterList, relatedEdgeRouters)
		} else if i == 6 {
			ctx.Equal(5, len(relatedEdgeRouters))
		} else {
			ctx.Equal(0, len(relatedEdgeRouters))
		}
	}

	stringz.Permutations(edgeRouterRoleAttrs, func(roles []string) {
		edgeRouter := newEdgeRouter(uuid.New().String(), roles...)
		ctx.requireCreate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	})

	ctx.validateServiceEdgeRouters(edgeRouters, services)

	for _, edgeRouter := range edgeRouters {
		ctx.requireDelete(edgeRouter)
	}

	edgeRouters = nil

	stringz.Permutations(edgeRouterRoleAttrs, func(roles []string) {
		edgeRouter := newEdgeRouter(uuid.New().String())
		ctx.requireCreate(edgeRouter)
		edgeRouter.RoleAttributes = roles
		ctx.requireUpdate(edgeRouter)
		edgeRouters = append(edgeRouters, edgeRouter)
	})

	ctx.validateServiceEdgeRouters(edgeRouters, services)

	// ensure policies get cleaned up
	for _, service := range services {
		ctx.requireDelete(service)
	}

	// test with policies created after identities/edge routers
	services = ctx.createServiceWithEdgeRouterLimits(edgeRouterRoles, edgeRouters, true)

	ctx.validateServiceEdgeRouters(edgeRouters, services)

	for _, service := range services {
		ctx.requireDelete(service)
	}

	// test with policies created after identities/edge routers and roles added after created
	services = ctx.createServiceWithEdgeRouterLimits(edgeRouterRoles, edgeRouters, false)

	ctx.validateServiceEdgeRouters(edgeRouters, services)

	for _, edgeRouter := range edgeRouters {
		if len(edgeRouter.RoleAttributes) > 0 {
			edgeRouter.RoleAttributes = edgeRouter.RoleAttributes[1:]
			ctx.requireUpdate(edgeRouter)
		}
	}

	for _, service := range services {
		if len(service.EdgeRouterRoles) > 0 {
			service.EdgeRouterRoles = service.EdgeRouterRoles[1:]
		}
		ctx.requireUpdate(service)
	}

	ctx.validateServiceEdgeRouters(edgeRouters, services)
}

func (ctx *TestContext) createServiceWithEdgeRouterLimits(edgeRouterRoles []string, edgeRouters []*EdgeRouter, oncreate bool) []*EdgeService {
	var services []*EdgeService
	for i := 0; i < 7; i++ {
		service := newEdgeService(uuid.New().String())
		if !oncreate {
			ctx.requireCreate(service)
		}
		if i == 1 {
			service.EdgeRouterRoles = []string{edgeRouterRoles[0]}
		}
		if i == 2 {
			service.EdgeRouterRoles = []string{edgeRouterRoles[1], edgeRouterRoles[2], edgeRouterRoles[3]}
		}
		if i == 3 {
			service.EdgeRouterRoles = []string{entityRef(edgeRouters[0].Id)}
		}
		if i == 4 {
			service.EdgeRouterRoles = []string{entityRef(edgeRouters[1].Id), entityRef(edgeRouters[2].Id), entityRef(edgeRouters[3].Id)}
		}
		if i == 5 {
			service.EdgeRouterRoles = []string{edgeRouterRoles[4], entityRef(edgeRouters[1].Id), entityRef(edgeRouters[2].Id), entityRef(edgeRouters[3].Id)}
		}
		if i == 6 {
			service.EdgeRouterRoles = []string{AllRole}
		}

		services = append(services, service)
		if oncreate {
			ctx.requireCreate(service)
		} else {
			ctx.requireUpdate(service)
		}
	}
	return services
}

func (ctx *TestContext) validateServiceEdgeRouters(edgeRouters []*EdgeRouter, services []*EdgeService) {
	for _, service := range services {
		count := 0
		relatedEdgeRouters := ctx.getRelatedIds(service, EntityTypeEdgeRouters)
		for _, edgeRouter := range edgeRouters {
			relatedServices := ctx.getRelatedIds(edgeRouter, EntityTypeServices)
			shouldContain := ctx.policyShouldMatch(service.EdgeRouterRoles, edgeRouter, edgeRouter.RoleAttributes)
			policyContains := stringz.Contains(relatedEdgeRouters, edgeRouter.Id)
			ctx.Equal(shouldContain, policyContains, "entity roles attr: %v. service roles: %v, service edge routers %+v, edge router services: %+v",
				edgeRouter.RoleAttributes, service.EdgeRouterRoles, relatedEdgeRouters, relatedServices)
			if shouldContain {
				count++
			}

			entityContains := stringz.Contains(relatedServices, service.Id)
			ctx.Equal(shouldContain, entityContains, "identity: %v, service: %v, entity roles attr: %v. service roles: %v",
				edgeRouter.Id, service.Id, edgeRouter.RoleAttributes, service.EdgeRouterRoles)
		}
		ctx.Equal(count, len(relatedEdgeRouters))
	}
}
