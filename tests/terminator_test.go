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

import "testing"

func Test_Terminators(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	service := ctx.AdminSession.requireNewService(nil, nil)
	edgeRouter := ctx.createAndEnrollEdgeRouter(false)
	terminator := ctx.AdminSession.requireNewTerminator(service.Id, edgeRouter.id, "transport", "tcp:localhost:2020")
	ctx.Req.NotEmpty(terminator.id)

	ctx.AdminSession.validateEntityWithQuery(terminator)
	ctx.AdminSession.validateEntityWithLookup(terminator)
	ctx.AdminSession.validateAssociations(service, terminator.getEntityType(), terminator)
}
