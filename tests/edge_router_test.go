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
	"github.com/netfoundry/ziti-foundation/util/stringz"
	"net/url"
	"sort"
	"testing"

	"github.com/google/uuid"
)

func Test_EdgeRouter(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	t.Run("role attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		edgeRouter := newTestEdgeRouter(role1, role2)
		edgeRouter.id = ctx.AdminSession.requireCreateEntity(edgeRouter)
		ctx.AdminSession.validateEntityWithQuery(edgeRouter)
		ctx.AdminSession.validateEntityWithLookup(edgeRouter)
	})

	t.Run("role attributes should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		edgeRouter := newTestEdgeRouter(role1, role2)
		edgeRouter.id = ctx.AdminSession.requireCreateEntity(edgeRouter)

		role3 := uuid.New().String()
		edgeRouter.roleAttributes = []string{role2, role3}
		ctx.AdminSession.requireUpdateEntity(edgeRouter)
		ctx.AdminSession.validateEntityWithLookup(edgeRouter)
	})

	t.Run("role attributes should be queryable", func(t *testing.T) {
		ctx.testContextChanged(t)
		prefix := "rol3-attribut3-qu3ry-t3st-"
		role1 := prefix + "sales"
		role2 := prefix + "support"
		role3 := prefix + "engineering"
		role4 := prefix + "field-ops"
		role5 := prefix + "executive"

		ctx.AdminSession.requireNewEdgeRouter(role1, role2)
		ctx.AdminSession.requireNewEdgeRouter(role2, role3)
		ctx.AdminSession.requireNewEdgeRouter(role3, role4)
		edgeRouter := ctx.AdminSession.requireNewEdgeRouter(role5)
		ctx.AdminSession.requireNewEdgeRouter()

		list := ctx.AdminSession.requireList("edge-router-role-attributes")
		ctx.req.True(len(list) >= 5)
		ctx.req.True(stringz.ContainsAll(list, role1, role2, role3, role4, role5))

		filter := url.QueryEscape(`id contains "e" and id contains "` + prefix + `" sort by id`)
		list = ctx.AdminSession.requireList("edge-router-role-attributes?filter=" + filter)
		ctx.req.Equal(4, len(list))

		expected := []string{role1, role3, role4, role5}
		sort.Strings(expected)
		ctx.req.Equal(expected, list)

		edgeRouter.roleAttributes = nil
		ctx.AdminSession.requireUpdateEntity(edgeRouter)
		list = ctx.AdminSession.requireList("edge-router-role-attributes")
		ctx.req.True(len(list) >= 4)
		ctx.req.True(stringz.ContainsAll(list, role1, role2, role3, role4))
		ctx.req.False(stringz.Contains(list, role5))
	})

	t.Run("newly created edge routers that is deleted", func(t *testing.T) {
		ctx.testContextChanged(t)

		edgeRouter := ctx.AdminSession.requireNewEdgeRouter()

		ctx.AdminSession.requireDeleteEntity(edgeRouter)
	})
}
