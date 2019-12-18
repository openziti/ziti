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

func Test_EdgeRouterPolicy(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	edgeRouterRole1 := uuid.New().String()
	edgeRouterRole2 := uuid.New().String()
	identityRole1 := uuid.New().String()
	identityRole2 := uuid.New().String()

	edgeRouter1 := ctx.requireNewEdgeRouter(edgeRouterRole1)
	edgeRouter2 := ctx.requireNewEdgeRouter(edgeRouterRole1, edgeRouterRole2)
	edgeRouter3 := ctx.requireNewEdgeRouter()

	identity1 := ctx.requireNewIdentity(false, identityRole1)
	identity2 := ctx.requireNewIdentity(false, identityRole1, identityRole2)
	identity3 := ctx.requireNewIdentity(false, identityRole2)

	policy1 := ctx.requireNewEdgeRouterPolicy(s("@"+edgeRouterRole1), s("@"+identityRole1))
	policy2 := ctx.requireNewEdgeRouterPolicy(s("@"+edgeRouterRole1, edgeRouter3.id), s("@"+identityRole1, identity3.id))
	policy3 := ctx.requireNewEdgeRouterPolicy(s(edgeRouter2.id, edgeRouter3.id), s(identity2.id, identity3.id))
	policy4 := ctx.requireNewEdgeRouterPolicy(s("@all"), s("@all"))

	ctx.validateEntityWithQuery(policy1)
	ctx.validateEntityWithLookup(policy2)
	ctx.validateEntityWithLookup(policy3)
	ctx.validateEntityWithLookup(policy4)

	ctx.validateAssociations(policy1, "edge-routers", edgeRouter1, edgeRouter2)
	ctx.validateAssociations(policy2, "edge-routers", edgeRouter1, edgeRouter2, edgeRouter3)
	ctx.validateAssociations(policy3, "edge-routers", edgeRouter2, edgeRouter3)
	ctx.validateAssociationContains(policy4, "edge-routers", edgeRouter1, edgeRouter2, edgeRouter3)

	ctx.validateAssociations(policy1, "identities", identity1, identity2)
	ctx.validateAssociations(policy2, "identities", identity1, identity2, identity3)
	ctx.validateAssociations(policy3, "identities", identity2, identity3)
	ctx.validateAssociationContains(policy4, "identities", identity1, identity2, identity3)

	ctx.validateAssociations(edgeRouter1, "edge-router-policies", policy1, policy2, policy4)
	ctx.validateAssociations(edgeRouter2, "edge-router-policies", policy1, policy2, policy3, policy4)
	ctx.validateAssociations(edgeRouter3, "edge-router-policies", policy2, policy3, policy4)

	ctx.validateAssociations(identity1, "edge-router-policies", policy1, policy2, policy4)
	ctx.validateAssociations(identity2, "edge-router-policies", policy1, policy2, policy3, policy4)
	ctx.validateAssociations(identity3, "edge-router-policies", policy2, policy3, policy4)

	policy1.edgeRouterRoles = append(policy1.edgeRouterRoles, "@"+edgeRouterRole2)
	policy1.identityRoles = s("@" + identityRole2)
	ctx.requireUpdateEntity(policy1)

	ctx.validateAssociations(policy1, "edge-routers", edgeRouter2)
	ctx.validateAssociations(policy1, "identities", identity2, identity3)

	ctx.validateAssociations(edgeRouter1, "edge-router-policies", policy2, policy4)
	ctx.validateAssociations(edgeRouter2, "edge-router-policies", policy1, policy2, policy3, policy4)
	ctx.validateAssociations(edgeRouter3, "edge-router-policies", policy2, policy3, policy4)

	ctx.validateAssociations(identity1, "edge-router-policies", policy2, policy4)
	ctx.validateAssociations(identity2, "edge-router-policies", policy1, policy2, policy3, policy4)
	ctx.validateAssociations(identity3, "edge-router-policies", policy1, policy2, policy3, policy4)

	ctx.requireDeleteEntity(policy2)

	ctx.validateAssociations(edgeRouter1, "edge-router-policies", policy4)
	ctx.validateAssociations(edgeRouter2, "edge-router-policies", policy1, policy3, policy4)
	ctx.validateAssociations(edgeRouter3, "edge-router-policies", policy3, policy4)

	ctx.validateAssociations(identity1, "edge-router-policies", policy4)
	ctx.validateAssociations(identity2, "edge-router-policies", policy1, policy3, policy4)
	ctx.validateAssociations(identity3, "edge-router-policies", policy1, policy3, policy4)

}
