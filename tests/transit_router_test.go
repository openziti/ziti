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
	"testing"
)

func Test_TransitRouters(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	t.Run("transit routers can be created and enrolled", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.createAndEnrollTransitRouter()
	})

	t.Run("create transit router, then delete", func(t *testing.T) {
		ctx.testContextChanged(t)
		router := ctx.AdminSession.requireNewTransitRouter()
		ctx.AdminSession.requireDeleteEntity(router)
	})

	t.Run("create & enroll transit router, then delete", func(t *testing.T) {
		ctx.testContextChanged(t)
		router := ctx.createAndEnrollTransitRouter()
		ctx.AdminSession.requireDeleteEntity(router)
	})
}
