//go:build apitests
// +build apitests

/*
	Copyright NetFoundry, Inc.

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
	"fmt"
	"github.com/openziti/edge/eid"
	"testing"
)

func Test_ServiceEdgeRouterPolicy(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	edgeRouterRole1 := eid.New()
	edgeRouterRole2 := eid.New()
	serviceRole1 := eid.New()
	serviceRole2 := eid.New()

	edgeRouter1 := ctx.AdminManagementSession.requireNewEdgeRouter(edgeRouterRole1)
	edgeRouter2 := ctx.AdminManagementSession.requireNewEdgeRouter(edgeRouterRole1, edgeRouterRole2)
	edgeRouter3 := ctx.AdminManagementSession.requireNewEdgeRouter()

	service1 := ctx.AdminManagementSession.requireNewService(s(serviceRole1), nil)
	service2 := ctx.AdminManagementSession.requireNewService(s(serviceRole1, serviceRole2), nil)
	service3 := ctx.AdminManagementSession.requireNewService(s(serviceRole2), nil)

	policy1 := ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#"+edgeRouterRole1), s("#"+serviceRole1))
	policy2 := ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#"+edgeRouterRole1, "@"+edgeRouter3.id), s("#"+serviceRole1, "@"+service3.Id))
	policy3 := ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("@"+edgeRouter2.id, "@"+edgeRouter3.id), s("@"+service2.Id, "@"+service3.Id))
	policy4 := ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))
	policy5 := ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicyWithSemantic("AllOf", s("#"+edgeRouterRole1, "#"+edgeRouterRole2), s("#"+serviceRole1, "#"+serviceRole2))
	policy6 := ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicyWithSemantic("AnyOf", s("#"+edgeRouterRole1, "#"+edgeRouterRole2), s("#"+serviceRole1, "#"+serviceRole2))

	ctx.AdminManagementSession.validateEntityWithQuery(policy1)
	ctx.AdminManagementSession.validateEntityWithLookup(policy2)
	ctx.AdminManagementSession.validateEntityWithLookup(policy3)
	ctx.AdminManagementSession.validateEntityWithLookup(policy4)
	ctx.AdminManagementSession.validateEntityWithLookup(policy5)
	ctx.AdminManagementSession.validateEntityWithLookup(policy6)

	ctx.AdminManagementSession.validateAssociations(policy1, "edge-routers", edgeRouter1, edgeRouter2)
	ctx.AdminManagementSession.validateAssociations(policy2, "edge-routers", edgeRouter1, edgeRouter2, edgeRouter3)
	ctx.AdminManagementSession.validateAssociations(policy3, "edge-routers", edgeRouter2, edgeRouter3)
	ctx.AdminManagementSession.validateAssociationContains(policy4, "edge-routers", edgeRouter1, edgeRouter2, edgeRouter3)
	ctx.AdminManagementSession.validateAssociations(policy5, "edge-routers", edgeRouter2)
	ctx.AdminManagementSession.validateAssociations(policy6, "edge-routers", edgeRouter1, edgeRouter2)

	ctx.AdminManagementSession.validateAssociations(policy1, "services", service1, service2)
	ctx.AdminManagementSession.validateAssociations(policy2, "services", service1, service2, service3)

	t.Run("policy 2 has display information", func(t *testing.T) {
		ctx.testContextChanged(t)
		url := fmt.Sprintf("%v/%v", policy2.getEntityType(), policy2.getId())
		result := ctx.AdminManagementSession.requireQuery(url)

		t.Run("for service 3 and service role 1", func(t *testing.T) {
			ctx.testContextChanged(t)

			displayValuesContainer := ctx.RequireGetNonNilPathValue(result, "data.serviceRolesDisplay")
			displayValuesChildren, err := displayValuesContainer.Children()
			ctx.Req.NoError(err)

			hasRole1DisplayInfo := false
			hasService3DisplayInfo := false

			for _, child := range displayValuesChildren {
				roleVal := ctx.RequireGetNonNilPathValue(child, "role")
				nameVal := ctx.RequireGetNonNilPathValue(child, "name")

				role := roleVal.Data().(string)
				name := nameVal.Data().(string)

				if role == "#"+serviceRole1 && name == "#"+serviceRole1 {
					hasRole1DisplayInfo = true
				}

				if role == "@"+service3.Id && name == "@"+service3.Name {
					hasService3DisplayInfo = true
				}
			}

			ctx.Req.True(hasRole1DisplayInfo, "expected to find role display info for role [%s]", serviceRole1, serviceRole1)
			ctx.Req.True(hasService3DisplayInfo, "expected to find service display info for id/role [@%s] and name [%s]", service3.Id, service3.Name)
		})

		t.Run("for edge router 3 and edge router role 1", func(t *testing.T) {
			ctx.testContextChanged(t)

			displayValuesContainer := ctx.RequireGetNonNilPathValue(result, "data.edgeRouterRolesDisplay")
			displayValuesChildren, err := displayValuesContainer.Children()
			ctx.Req.NoError(err)

			hasRole1DisplayInfo := false
			hasRouter3DisplayInfo := false

			for _, child := range displayValuesChildren {
				roleVal := ctx.RequireGetNonNilPathValue(child, "role")
				nameVal := ctx.RequireGetNonNilPathValue(child, "name")

				role := roleVal.Data().(string)
				name := nameVal.Data().(string)

				if role == "#"+edgeRouterRole1 && name == "#"+edgeRouterRole1 {
					hasRole1DisplayInfo = true
				}

				if role == "@"+edgeRouter3.id && name == "@"+edgeRouter3.name {
					hasRouter3DisplayInfo = true
				}
			}

			ctx.Req.True(hasRole1DisplayInfo, "expected to find role display info for role [%s]", edgeRouterRole1, edgeRouterRole1)
			ctx.Req.True(hasRouter3DisplayInfo, "expected to find edge router display info for id/role [@%s] and name [%s]", edgeRouter3.id, edgeRouter3.name)
		})
	})

	ctx.AdminManagementSession.validateAssociations(policy3, "services", service2, service3)
	ctx.AdminManagementSession.validateAssociationContains(policy4, "services", service1, service2, service3)
	ctx.AdminManagementSession.validateAssociations(policy5, "services", service2)
	ctx.AdminManagementSession.validateAssociations(policy6, "services", service1, service2, service3)

	ctx.AdminManagementSession.validateAssociations(edgeRouter1, "service-edge-router-policies", policy1, policy2, policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(edgeRouter2, "service-edge-router-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(edgeRouter3, "service-edge-router-policies", policy2, policy3, policy4)

	ctx.AdminManagementSession.validateAssociations(service1, "service-edge-router-policies", policy1, policy2, policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(service2, "service-edge-router-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(service3, "service-edge-router-policies", policy2, policy3, policy4, policy6)

	policy1.edgeRouterRoles = append(policy1.edgeRouterRoles, "#"+edgeRouterRole2)
	policy1.serviceRoles = s("#" + serviceRole2)
	ctx.AdminManagementSession.requireUpdateEntity(policy1)

	ctx.AdminManagementSession.validateAssociations(policy1, "edge-routers", edgeRouter2)
	ctx.AdminManagementSession.validateAssociations(policy1, "services", service2, service3)

	ctx.AdminManagementSession.validateAssociations(edgeRouter1, "service-edge-router-policies", policy2, policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(edgeRouter2, "service-edge-router-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(edgeRouter3, "service-edge-router-policies", policy2, policy3, policy4)

	ctx.AdminManagementSession.validateAssociations(service1, "service-edge-router-policies", policy2, policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(service2, "service-edge-router-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(service3, "service-edge-router-policies", policy1, policy2, policy3, policy4, policy6)

	ctx.AdminManagementSession.requireDeleteEntity(policy2)

	ctx.AdminManagementSession.validateAssociations(edgeRouter1, "service-edge-router-policies", policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(edgeRouter2, "service-edge-router-policies", policy1, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(edgeRouter3, "service-edge-router-policies", policy3, policy4)

	ctx.AdminManagementSession.validateAssociations(service1, "service-edge-router-policies", policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(service2, "service-edge-router-policies", policy1, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(service3, "service-edge-router-policies", policy1, policy3, policy4, policy6)
}
