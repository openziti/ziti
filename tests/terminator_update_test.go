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
	"github.com/openziti/sdk-golang/ziti/edge"
	"testing"
	"time"
)

func Test_UpdateTerminators(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	ctx.Req.NoError(err)
	ctx.requireNListener(1, listener, time.Second)
	defer func() { _ = listener.Close() }()

	terminators := ctx.AdminSession.listTerminators(`binding="edge"`)
	ctx.Req.Equal(1, len(terminators))
	term := terminators[0]
	ctx.Req.Equal(0, term.cost)
	ctx.Req.Equal("default", term.precedence)

	err = listener.UpdateCost(999)
	ctx.Req.NoError(err)

	time.Sleep(25 * time.Millisecond) // update is async, so need to give a little time to process

	term.cost = 999
	ctx.AdminSession.validateEntityWithLookup(term)

	err = listener.UpdatePrecedence(edge.PrecedenceRequired)
	ctx.Req.NoError(err)

	time.Sleep(25 * time.Millisecond) // update is async, so need to give a little time to process

	term.precedence = "required"
	ctx.AdminSession.validateEntityWithLookup(term)

	err = listener.UpdateCostAndPrecedence(585, edge.PrecedenceFailed)
	ctx.Req.NoError(err)

	time.Sleep(25 * time.Millisecond) // update is async, so need to give a little time to process

	term.cost = 585
	term.precedence = "failed"
	ctx.AdminSession.validateEntityWithLookup(term)

}
