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
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/util/stringz"
	"net/url"
	"sort"
	"testing"
)

func Test_EdgeRouter(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("role attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		edgeRouter := newTestEdgeRouter(role1, role2)
		edgeRouter.id = ctx.AdminManagementSession.requireCreateEntity(edgeRouter)
		ctx.AdminManagementSession.validateEntityWithQuery(edgeRouter)
		ctx.AdminManagementSession.validateEntityWithLookup(edgeRouter)
	})

	t.Run("role attributes should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		edgeRouter := newTestEdgeRouter(role1, role2)
		edgeRouter.id = ctx.AdminManagementSession.requireCreateEntity(edgeRouter)

		role3 := eid.New()
		edgeRouter.roleAttributes = []string{role2, role3}
		ctx.AdminManagementSession.requireUpdateEntity(edgeRouter)
		ctx.AdminManagementSession.validateEntityWithLookup(edgeRouter)
	})

	t.Run("role attributes should be queryable", func(t *testing.T) {
		ctx.testContextChanged(t)
		prefix := "rol3-attribut3-qu3ry-t3st-"
		role1 := prefix + "sales"
		role2 := prefix + "support"
		role3 := prefix + "engineering"
		role4 := prefix + "field-ops"
		role5 := prefix + "executive"

		ctx.AdminManagementSession.requireNewEdgeRouter(role1, role2)
		ctx.AdminManagementSession.requireNewEdgeRouter(role2, role3)
		ctx.AdminManagementSession.requireNewEdgeRouter(role3, role4)
		edgeRouter := ctx.AdminManagementSession.requireNewEdgeRouter(role5)
		ctx.AdminManagementSession.requireNewEdgeRouter()

		list := ctx.AdminManagementSession.requireList("edge-router-role-attributes")
		ctx.Req.True(len(list) >= 5)
		ctx.Req.True(stringz.ContainsAll(list, role1, role2, role3, role4, role5))

		filter := url.QueryEscape(`id contains "e" and id contains "` + prefix + `" sort by id`)
		list = ctx.AdminManagementSession.requireList("edge-router-role-attributes?filter=" + filter)
		ctx.Req.Equal(4, len(list))

		expected := []string{role1, role3, role4, role5}
		sort.Strings(expected)
		ctx.Req.Equal(expected, list)

		edgeRouter.roleAttributes = nil
		ctx.AdminManagementSession.requireUpdateEntity(edgeRouter)
		list = ctx.AdminManagementSession.requireList("edge-router-role-attributes")
		ctx.Req.True(len(list) >= 4)
		ctx.Req.True(stringz.ContainsAll(list, role1, role2, role3, role4))
		ctx.Req.False(stringz.Contains(list, role5))
	})

	t.Run("newly created edge routers that is deleted", func(t *testing.T) {
		ctx.testContextChanged(t)

		edgeRouter := ctx.AdminManagementSession.requireNewEdgeRouter()

		ctx.AdminManagementSession.requireDeleteEntity(edgeRouter)
	})
}
