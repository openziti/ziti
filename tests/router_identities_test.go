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
	"github.com/google/uuid"
	"net/http"
	"net/url"
	"testing"
)

func TestEdgeRouterIdentities(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	edgeRouter1 := ctx.AdminSession.requireNewTunnelerEnabledEdgeRouter()

	// identity should have been created
	identities := ctx.AdminSession.listIdentities("filter=" + url.QueryEscape(`type="Router"`))
	ctx.Req.Equal(1, len(identities))

	identity1 := identities[0]
	ctx.Req.Equal(edgeRouter1.name, identity1.name)

	resp := ctx.AdminSession.deleteEntityOfType("identities", identity1.Id)
	ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())

	identity1.name = edgeRouter1.name
	identity1.roleAttributes = []string{"test", "hello"}
	ctx.AdminSession.requireUpdateEntity(identity1)
	ctx.AdminSession.validateEntityWithLookup(identity1)

	identity1.name = edgeRouter1.name
	identity1.roleAttributes = []string{"foo", "bar"}
	ctx.AdminSession.requirePatchEntity(identity1, "roleAttributes")
	ctx.AdminSession.validateEntityWithLookup(identity1)

	identity1.name = uuid.NewString()
	resp = ctx.AdminSession.patchEntity(identity1, "name")
	ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode()) // should not be allowed to change name

	resp = ctx.AdminSession.updateEntity(identity1)
	ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode()) // should not be allowed to change name

	// edge router policy should have been created
	edgeRouterPolicies := ctx.AdminSession.listEdgeRouterPolicies("")
	ctx.Req.Equal(1, len(edgeRouterPolicies))

	edgeRouterPolicy1 := edgeRouterPolicies[0]
	ctx.Req.Equal("edge-router-"+edgeRouter1.id+"-system", edgeRouterPolicy1.name)
	ctx.Req.Equal(1, len(edgeRouterPolicy1.edgeRouterRoles))
	ctx.Req.Equal("@"+edgeRouter1.id, edgeRouterPolicy1.edgeRouterRoles[0])
	ctx.Req.Equal(1, len(edgeRouterPolicy1.identityRoles))
	ctx.Req.Equal("@"+identity1.Id, edgeRouterPolicy1.identityRoles[0])

	resp = ctx.AdminSession.deleteEntityOfType("edge-router-policies", edgeRouterPolicy1.id)
	ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())

	// deleting edge router should remove all three entitie
	ctx.AdminSession.requireDeleteEntity(edgeRouter1)
	ctx.RequireNotFoundError(ctx.AdminSession.query("edge-routers/" + edgeRouter1.id))
	ctx.RequireNotFoundError(ctx.AdminSession.query("identities/" + identity1.Id))
	ctx.RequireNotFoundError(ctx.AdminSession.query("edge-router-policies/" + edgeRouterPolicy1.id))
}

func TestEdgeRouterIdentitiesNotEnabled(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	edgeRouter1 := ctx.AdminSession.requireNewEdgeRouter()

	identities := ctx.AdminSession.listIdentities("filter=" + url.QueryEscape(`type="Router"`))
	ctx.Req.Equal(0, len(identities))

	edgeRouterPolicies := ctx.AdminSession.listEdgeRouterPolicies("")
	ctx.Req.Equal(0, len(edgeRouterPolicies))

	edgeRouter1.isTunnelerEnabled = true
	ctx.AdminSession.requireUpdateEntity(edgeRouter1)

	// identity should have been created
	identities = ctx.AdminSession.listIdentities("filter=" + url.QueryEscape(`type="Router"`))
	ctx.Req.Equal(1, len(identities))

	identity1 := identities[0]
	ctx.Req.Equal(edgeRouter1.name, identity1.name)

	resp := ctx.AdminSession.deleteEntityOfType("identities", identity1.Id)
	ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())

	identity1.name = edgeRouter1.name
	identity1.roleAttributes = []string{"test", "hello"}
	ctx.AdminSession.requireUpdateEntity(identity1)
	ctx.AdminSession.validateEntityWithLookup(identity1)

	identity1.name = edgeRouter1.name
	identity1.roleAttributes = []string{"foo", "bar"}
	ctx.AdminSession.requirePatchEntity(identity1, "roleAttributes")
	ctx.AdminSession.validateEntityWithLookup(identity1)

	identity1.name = uuid.NewString()
	resp = ctx.AdminSession.patchEntity(identity1, "name")
	ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode()) // should not be allowed to change name

	resp = ctx.AdminSession.updateEntity(identity1)
	ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode()) // should not be allowed to change name

	// edge router policy should have been created
	edgeRouterPolicies = ctx.AdminSession.listEdgeRouterPolicies("")
	ctx.Req.Equal(1, len(edgeRouterPolicies))

	edgeRouterPolicy1 := edgeRouterPolicies[0]
	ctx.Req.Equal("edge-router-"+edgeRouter1.id+"-system", edgeRouterPolicy1.name)
	ctx.Req.Equal(1, len(edgeRouterPolicy1.edgeRouterRoles))
	ctx.Req.Equal("@"+edgeRouter1.id, edgeRouterPolicy1.edgeRouterRoles[0])
	ctx.Req.Equal(1, len(edgeRouterPolicy1.identityRoles))
	ctx.Req.Equal("@"+identity1.Id, edgeRouterPolicy1.identityRoles[0])

	edgeRouter1.isTunnelerEnabled = false
	ctx.AdminSession.requireUpdateEntity(edgeRouter1)

	// setting flag to false should remove associated entities
	ctx.RequireNotFoundError(ctx.AdminSession.query("identities/" + identity1.Id))
	ctx.RequireNotFoundError(ctx.AdminSession.query("edge-router-policies/" + edgeRouterPolicy1.id))
}
