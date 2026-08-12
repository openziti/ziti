//go:build apitests

/*
	Copyright NetFoundry Inc.

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
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"google.golang.org/protobuf/proto"
)

// Test_CreateCircuitV1_InvalidTokens drives the legacy v1 create-circuit handler with tokens that fail
// validation. Token validation runs against the request context's env, so a context built without it
// takes a nil-interface method call. That panic happens on the control channel's async dispatch
// goroutine, which has no recover, and would therefore terminate the whole controller rather than
// producing an error reply.
//
// The JWT-prefixed case is the one that matters: a token without that prefix is looked up in bolt and
// fails on the missing api-session before any env call, so it never reaches the vulnerable line.
func Test_CreateCircuitV1_InvalidTokens(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	edgeRouter := ctx.CreateEnrollAndStartEdgeRouter()

	ctrlCh := edgeRouter.GetNetworkControllers().AnyCtrlChannel()
	ctx.Req.NotNil(ctrlCh)

	sendCreateCircuitV1 := func(apiSessionToken, sessionToken string) *channel.Message {
		req := &edge_ctrl_pb.CreateCircuitRequest{
			ApiSessionToken: apiSessionToken,
			SessionToken:    sessionToken,
		}

		body, err := proto.Marshal(req)
		ctx.Req.NoError(err)

		msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_CreateCircuitRequestType), body)
		reply, err := msg.WithTimeout(5 * time.Second).SendForReply(ctrlCh.GetDefaultSender())
		ctx.Req.NoError(err, "the controller must reply rather than die")

		return reply
	}

	requireErrorReply := func(reply *channel.Message) {
		ctx.Req.Equal(int32(edge_ctrl_pb.ContentType_ErrorType), reply.ContentType,
			"expected an error reply, got content type %v", reply.ContentType)
	}

	// "ey" is the OIDC access-token prefix, which routes validation through the token path.
	t.Run("jwt-prefixed api session token", func(t *testing.T) {
		ctx.NextTest(t)
		requireErrorReply(sendCreateCircuitV1("eyJhbGciOiJIUzI1NiJ9.bogus.bogus", "eyJhbGciOiJIUzI1NiJ9.bogus.bogus"))
	})

	t.Run("opaque api session token", func(t *testing.T) {
		ctx.NextTest(t)
		requireErrorReply(sendCreateCircuitV1("not-a-real-api-session-token", "not-a-real-session-token"))
	})

	t.Run("empty tokens", func(t *testing.T) {
		ctx.NextTest(t)
		requireErrorReply(sendCreateCircuitV1("", ""))
	})

	// The controller must still be serving after the above; a panic on the dispatch goroutine would
	// have taken the process down rather than let this run.
	t.Run("controller still serving", func(t *testing.T) {
		ctx.NextTest(t)

		reply := sendCreateCircuitV1("eyJhbGciOiJIUzI1NiJ9.bogus.bogus", "eyJhbGciOiJIUzI1NiJ9.bogus.bogus")
		requireErrorReply(reply)

		code, found := reply.GetUint32Header(edge.ErrorCodeHeader)
		ctx.Req.True(found, "error replies carry an edge error code")
		ctx.Req.NotZero(code)
	})
}
