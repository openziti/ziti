// +build apitests

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

package tests

import (
	"testing"

	"github.com/google/uuid"
)

func Test_ServiceEdgeRouterPolicy(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	edgeRouterRole1 := uuid.New().String()
	edgeRouterRole2 := uuid.New().String()
	serviceRole1 := uuid.New().String()
	serviceRole2 := uuid.New().String()

	edgeRouter1 := ctx.AdminSession.requireNewEdgeRouter(edgeRouterRole1)
	edgeRouter2 := ctx.AdminSession.requireNewEdgeRouter(edgeRouterRole1, edgeRouterRole2)
	edgeRouter3 := ctx.AdminSession.requireNewEdgeRouter()

	ctx.enabledJsonLogging = true
	service1 := ctx.AdminSession.requireNewService(s(serviceRole1), nil)
	service2 := ctx.AdminSession.requireNewService(s(serviceRole1, serviceRole2), nil)
	service3 := ctx.AdminSession.requireNewService(s(serviceRole2), nil)

	policy1 := ctx.AdminSession.requireNewServiceEdgeRouterPolicy(s("#"+edgeRouterRole1), s("#"+serviceRole1))
	policy2 := ctx.AdminSession.requireNewServiceEdgeRouterPolicy(s("#"+edgeRouterRole1, "@"+edgeRouter3.id), s("#"+serviceRole1, "@"+service3.id))
	policy3 := ctx.AdminSession.requireNewServiceEdgeRouterPolicy(s("@"+edgeRouter2.id, "@"+edgeRouter3.id), s("@"+service2.id, "@"+service3.id))
	policy4 := ctx.AdminSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))
	policy5 := ctx.AdminSession.requireNewServiceEdgeRouterPolicyWithSemantic("AllOf", s("#"+edgeRouterRole1, "#"+edgeRouterRole2), s("#"+serviceRole1, "#"+serviceRole2))
	policy6 := ctx.AdminSession.requireNewServiceEdgeRouterPolicyWithSemantic("AnyOf", s("#"+edgeRouterRole1, "#"+edgeRouterRole2), s("#"+serviceRole1, "#"+serviceRole2))

	ctx.AdminSession.validateEntityWithQuery(policy1)
	ctx.AdminSession.validateEntityWithLookup(policy2)
	ctx.AdminSession.validateEntityWithLookup(policy3)
	ctx.AdminSession.validateEntityWithLookup(policy4)
	ctx.AdminSession.validateEntityWithLookup(policy5)
	ctx.AdminSession.validateEntityWithLookup(policy6)

	ctx.AdminSession.validateAssociations(policy1, "edge-routers", edgeRouter1, edgeRouter2)
	ctx.AdminSession.validateAssociations(policy2, "edge-routers", edgeRouter1, edgeRouter2, edgeRouter3)
	ctx.AdminSession.validateAssociations(policy3, "edge-routers", edgeRouter2, edgeRouter3)
	ctx.AdminSession.validateAssociationContains(policy4, "edge-routers", edgeRouter1, edgeRouter2, edgeRouter3)
	ctx.AdminSession.validateAssociations(policy5, "edge-routers", edgeRouter2)
	ctx.AdminSession.validateAssociations(policy6, "edge-routers", edgeRouter1, edgeRouter2)

	ctx.AdminSession.validateAssociations(policy1, "services", service1, service2)
	ctx.AdminSession.validateAssociations(policy2, "services", service1, service2, service3)
	ctx.AdminSession.validateAssociations(policy3, "services", service2, service3)
	ctx.AdminSession.validateAssociationContains(policy4, "services", service1, service2, service3)
	ctx.AdminSession.validateAssociations(policy5, "services", service2)
	ctx.AdminSession.validateAssociations(policy6, "services", service1, service2, service3)

	ctx.AdminSession.validateAssociations(edgeRouter1, "service-edge-router-policies", policy1, policy2, policy4, policy6)
	ctx.AdminSession.validateAssociations(edgeRouter2, "service-edge-router-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminSession.validateAssociations(edgeRouter3, "service-edge-router-policies", policy2, policy3, policy4)

	ctx.AdminSession.validateAssociations(service1, "service-edge-router-policies", policy1, policy2, policy4, policy6)
	ctx.AdminSession.validateAssociations(service2, "service-edge-router-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminSession.validateAssociations(service3, "service-edge-router-policies", policy2, policy3, policy4, policy6)

	policy1.edgeRouterRoles = append(policy1.edgeRouterRoles, "#"+edgeRouterRole2)
	policy1.serviceRoles = s("#" + serviceRole2)
	ctx.AdminSession.requireUpdateEntity(policy1)

	ctx.AdminSession.validateAssociations(policy1, "edge-routers", edgeRouter2)
	ctx.AdminSession.validateAssociations(policy1, "services", service2, service3)

	ctx.AdminSession.validateAssociations(edgeRouter1, "service-edge-router-policies", policy2, policy4, policy6)
	ctx.AdminSession.validateAssociations(edgeRouter2, "service-edge-router-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminSession.validateAssociations(edgeRouter3, "service-edge-router-policies", policy2, policy3, policy4)

	ctx.AdminSession.validateAssociations(service1, "service-edge-router-policies", policy2, policy4, policy6)
	ctx.AdminSession.validateAssociations(service2, "service-edge-router-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminSession.validateAssociations(service3, "service-edge-router-policies", policy1, policy2, policy3, policy4, policy6)

	ctx.AdminSession.requireDeleteEntity(policy2)

	ctx.AdminSession.validateAssociations(edgeRouter1, "service-edge-router-policies", policy4, policy6)
	ctx.AdminSession.validateAssociations(edgeRouter2, "service-edge-router-policies", policy1, policy3, policy4, policy5, policy6)
	ctx.AdminSession.validateAssociations(edgeRouter3, "service-edge-router-policies", policy3, policy4)

	ctx.AdminSession.validateAssociations(service1, "service-edge-router-policies", policy4, policy6)
	ctx.AdminSession.validateAssociations(service2, "service-edge-router-policies", policy1, policy3, policy4, policy5, policy6)
	ctx.AdminSession.validateAssociations(service3, "service-edge-router-policies", policy1, policy3, policy4, policy6)
}
