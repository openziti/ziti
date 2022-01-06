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

func Test_ServicePolicy(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	serviceRole1 := eid.New()
	serviceRole2 := eid.New()
	identityRole1 := eid.New()
	identityRole2 := eid.New()

	service1 := ctx.AdminManagementSession.requireNewService(s(serviceRole1), nil)
	service2 := ctx.AdminManagementSession.requireNewService(s(serviceRole1, serviceRole2), nil)
	service3 := ctx.AdminManagementSession.requireNewService(nil, nil)

	identity1 := ctx.AdminManagementSession.requireNewIdentity(false, identityRole1)
	identity2 := ctx.AdminManagementSession.requireNewIdentity(false, identityRole1, identityRole2)
	identity3 := ctx.AdminManagementSession.requireNewIdentity(false, identityRole2)

	policy1 := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole1), s("#"+identityRole1), s())
	policy2 := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole1, "@"+service3.Id), s("#"+identityRole1, "@"+identity3.Id), s())
	policy3 := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("@"+service2.Id, "@"+service3.Id), s("@"+identity2.Id, "@"+identity3.Id), s())
	policy4 := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#all"), s("#all"), nil)
	policy5 := ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole1, "#"+serviceRole2), s("#"+identityRole1, "#"+identityRole2), s())
	policy6 := ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AnyOf", s("#"+serviceRole1, "#"+serviceRole2), s("#"+identityRole1, "#"+identityRole2), s())

	ctx.AdminManagementSession.validateEntityWithQuery(policy1)
	ctx.AdminManagementSession.validateEntityWithLookup(policy2)
	ctx.AdminManagementSession.validateEntityWithLookup(policy3)
	ctx.AdminManagementSession.validateEntityWithLookup(policy4)
	ctx.AdminManagementSession.validateEntityWithLookup(policy5)
	ctx.AdminManagementSession.validateEntityWithLookup(policy6)

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

		t.Run("for identity 3 and identity role 1", func(t *testing.T) {
			ctx.testContextChanged(t)

			displayValuesContainer := ctx.RequireGetNonNilPathValue(result, "data.identityRolesDisplay")
			displayValuesChildren, err := displayValuesContainer.Children()
			ctx.Req.NoError(err)

			hasRole1DisplayInfo := false
			hasIdentity3DisplayInfo := false

			for _, child := range displayValuesChildren {
				roleVal := ctx.RequireGetNonNilPathValue(child, "role")
				nameVal := ctx.RequireGetNonNilPathValue(child, "name")

				role := roleVal.Data().(string)
				name := nameVal.Data().(string)

				if role == "#"+identityRole1 && name == "#"+identityRole1 {
					hasRole1DisplayInfo = true
				}

				if role == "@"+identity3.Id && name == "@"+identity3.name {
					hasIdentity3DisplayInfo = true
				}
			}

			ctx.Req.True(hasRole1DisplayInfo, "expected to find role display info for role [%s]", identityRole1)
			ctx.Req.True(hasIdentity3DisplayInfo, "expected to find identity display info for id/role [@%s] and name [%s]", identity3.Id, identity3.name)
		})
	})

	ctx.AdminManagementSession.validateAssociations(policy3, "services", service2, service3)
	ctx.AdminManagementSession.validateAssociationContains(policy4, "services", service1, service2, service3)
	ctx.AdminManagementSession.validateAssociationContains(policy5, "services", service2)
	ctx.AdminManagementSession.validateAssociationContains(policy6, "services", service1, service2)

	ctx.AdminManagementSession.validateAssociations(policy1, "identities", identity1, identity2)
	ctx.AdminManagementSession.validateAssociations(policy2, "identities", identity1, identity2, identity3)
	ctx.AdminManagementSession.validateAssociations(policy3, "identities", identity2, identity3)
	ctx.AdminManagementSession.validateAssociationContains(policy4, "identities", identity1, identity2, identity3)
	ctx.AdminManagementSession.validateAssociations(policy5, "identities", identity2)
	ctx.AdminManagementSession.validateAssociations(policy6, "identities", identity1, identity2, identity3)

	ctx.AdminManagementSession.validateAssociations(service1, "service-policies", policy1, policy2, policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(service2, "service-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(service3, "service-policies", policy2, policy3, policy4)

	ctx.AdminManagementSession.validateAssociations(identity1, "service-policies", policy1, policy2, policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(identity2, "service-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(identity3, "service-policies", policy2, policy3, policy4, policy6)

	policy1.serviceRoles = append(policy1.serviceRoles, "#"+serviceRole2)
	policy1.identityRoles = s("#" + identityRole2)
	ctx.AdminManagementSession.requireUpdateEntity(policy1)

	ctx.AdminManagementSession.validateAssociations(policy1, "services", service2)
	ctx.AdminManagementSession.validateAssociations(policy1, "identities", identity2, identity3)

	ctx.AdminManagementSession.validateAssociations(service1, "service-policies", policy2, policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(service2, "service-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(service3, "service-policies", policy2, policy3, policy4)

	ctx.AdminManagementSession.validateAssociations(identity1, "service-policies", policy2, policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(identity2, "service-policies", policy1, policy2, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(identity3, "service-policies", policy1, policy2, policy3, policy4, policy6)

	ctx.AdminManagementSession.requireDeleteEntity(policy2)

	ctx.AdminManagementSession.validateAssociations(service1, "service-policies", policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(service2, "service-policies", policy1, policy3, policy4, policy5, policy6)

	ctx.AdminManagementSession.validateAssociations(service3, "service-policies", policy3, policy4)

	ctx.AdminManagementSession.validateAssociations(identity1, "service-policies", policy4, policy6)
	ctx.AdminManagementSession.validateAssociations(identity2, "service-policies", policy1, policy3, policy4, policy5, policy6)
	ctx.AdminManagementSession.validateAssociations(identity3, "service-policies", policy1, policy3, policy4, policy6)
}
