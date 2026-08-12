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
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
	"google.golang.org/protobuf/proto"
)

// Test_TerminatorOwnership drives the fabric control channel directly to confirm that terminator
// operations are scoped to the requesting router: a router may only remove or update terminators it
// owns. Without that scoping any enrolled router can delete another router's terminators (taking
// down services hosted elsewhere) or re-weight them to steer traffic.
func Test_TerminatorOwnership(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	svc := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)

	// The requesting router: real, enrolled, and connected, so we can send on its control channel.
	requestingRouter := ctx.CreateEnrollAndStartEdgeRouter()

	// The victim router only needs to exist in the model to own a terminator; it is never started.
	victimRouter := ctx.AdminManagementSession.requireNewEdgeRouter()

	ctrlCh := requestingRouter.GetNetworkControllers().AnyCtrlChannel()
	ctx.Req.NotNil(ctrlCh)

	// sendForResult sends a fabric ctrl_pb request and returns the controller's Result.
	sendForResult := func(contentType int32, body proto.Message) *channel.Result {
		bodyBytes, err := proto.Marshal(body)
		ctx.Req.NoError(err)

		msg := channel.NewMessage(contentType, bodyBytes)
		reply, err := msg.WithTimeout(5 * time.Second).SendForReply(ctrlCh.GetDefaultSender())
		ctx.Req.NoError(err)

		return channel.UnmarshalResult(reply)
	}

	terminatorExists := func(id string) bool {
		return ctx.AdminManagementSession.requireQuery("terminators/"+id) != nil
	}

	terminatorPrecedence := func(id string) string {
		entity := ctx.AdminManagementSession.requireQuery("terminators/" + id)
		return entity.Path("data.precedence").Data().(string)
	}

	newVictimTerminator := func() string {
		term := ctx.AdminManagementSession.requireNewTerminator(svc.Id, victimRouter.id, "transport", "tcp:localhost:1234")
		return term.id
	}

	t.Run("cannot remove a terminator owned by another router", func(t *testing.T) {
		ctx.NextTest(t)

		terminatorId := newVictimTerminator()

		result := sendForResult(int32(ctrl_pb.ContentType_RemoveTerminatorRequestType),
			&ctrl_pb.RemoveTerminatorRequest{TerminatorId: terminatorId})

		ctx.Req.False(result.Success, "removing another router's terminator must be rejected")
		ctx.Req.True(terminatorExists(terminatorId), "the victim's terminator must survive the rejected removal")
	})

	t.Run("cannot remove another router's terminator in a batch", func(t *testing.T) {
		ctx.NextTest(t)

		terminatorId := newVictimTerminator()

		// The batch handler drops ids it does not own rather than failing the whole request, so the
		// assertion that matters is that the terminator survives.
		sendForResult(int32(ctrl_pb.ContentType_RemoveTerminatorsRequestType),
			&ctrl_pb.RemoveTerminatorsRequest{TerminatorIds: []string{terminatorId}})

		ctx.Req.True(terminatorExists(terminatorId), "the victim's terminator must survive the batch removal")
	})

	t.Run("cannot re-weight a terminator owned by another router", func(t *testing.T) {
		ctx.NextTest(t)

		terminatorId := newVictimTerminator()
		ctx.Req.Equal("default", terminatorPrecedence(terminatorId))

		result := sendForResult(int32(ctrl_pb.ContentType_UpdateTerminatorRequestType),
			&ctrl_pb.UpdateTerminatorRequest{
				TerminatorId:     terminatorId,
				UpdatePrecedence: true,
				Precedence:       ctrl_pb.TerminatorPrecedence_Failed,
			})

		ctx.Req.False(result.Success, "updating another router's terminator must be rejected")
		ctx.Req.Equal("default", terminatorPrecedence(terminatorId),
			"the victim's terminator precedence must be unchanged")
	})

	t.Run("can remove a terminator it owns", func(t *testing.T) {
		ctx.NextTest(t)

		term := ctx.AdminManagementSession.requireNewTerminator(svc.Id, requestingRouter.GetRouterId().Token, "transport", "tcp:localhost:1234")

		result := sendForResult(int32(ctrl_pb.ContentType_RemoveTerminatorRequestType),
			&ctrl_pb.RemoveTerminatorRequest{TerminatorId: term.id})

		ctx.Req.True(result.Success, "a router must still be able to remove its own terminator: %v", result.Message)
	})
}
