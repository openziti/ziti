//go:build dataflow

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
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
)

func Test_CreateCircuitV3(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	// Create policies: dial and bind for "dialers" and "hosts" roles, edge router and
	// service-edge-router policies open to all.
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#dialRole"), s("#dialerRole"), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#dialRole"), s("#hostRole"), nil)
	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	svc := ctx.AdminManagementSession.requireNewService(s("dialRole"), nil)

	edgeRouter := ctx.CreateEnrollAndStartEdgeRouter()

	// Create a hosting identity and listen on the service so a terminator exists.
	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext("hostRole")
	defer hostContext.Close()

	// Start watching for the terminator before listening, so we can wait for it.
	terminatorWatcher := ctx.AdminManagementSession.newTerminatorWatcher(svc.Id, 1)
	defer terminatorWatcher.Close()

	listener, err := hostContext.Listen(svc.Name)
	ctx.Req.NoError(err)
	defer listener.Close()

	testServer := newTestServer(listener, func(conn *testServerConn) error {
		for {
			name, eof := conn.ReadString(math.MaxUint16, 1*time.Minute)
			if eof {
				return conn.server.close()
			}
			conn.WriteString("hello, "+name, time.Second)
		}
	})
	testServer.start()

	terminatorWatcher.waitForTerminators(5 * time.Second)

	// Create a dialer identity (has dial access via dialerRole).
	dialerIdentity := ctx.AdminManagementSession.requireNewIdentity(false, "dialerRole")

	// Create an identity with no dial access.
	noAccessIdentity := ctx.AdminManagementSession.requireNewIdentity(false, "noAccessRole")

	// Get the control channel from the edge router to the controller.
	ctrlCh := edgeRouter.GetNetworkControllers().AnyCtrlChannel()
	ctx.Req.NotNil(ctrlCh)

	sendV3Request := func(req *ctrl_msg.CreateCircuitV3Request) (*ctrl_msg.CreateCircuitV3Response, error) {
		msg, err := req.ToMessage().WithTimeout(5 * time.Second).SendForReply(ctrlCh.GetHighPrioritySender())
		if err != nil {
			return nil, err
		}

		if msg.ContentType == int32(edge_ctrl_pb.ContentType_ErrorType) {
			errMsg := string(msg.Body)
			if errMsg == "" {
				errMsg = "error state returned from controller with no message"
			}
			code, _ := msg.GetUint32Header(edge.ErrorCodeHeader)
			return nil, &circuitV3Error{
				msg:  errMsg,
				code: code,
			}
		}

		ctx.Req.Equal(int32(edge_ctrl_pb.ContentType_CreateCircuitV3ResponseType), msg.ContentType,
			"unexpected response content type")

		return ctrl_msg.DecodeCreateCircuitV3Response(msg)
	}

	t.Run("successful circuit creation", func(t *testing.T) {
		ctx.NextTest(t)

		req := &ctrl_msg.CreateCircuitV3Request{
			IdentityId: dialerIdentity.Id,
			ServiceId:  svc.Id,
			CircuitId:  uuid.NewString(),
			PeerData:   map[uint32][]byte{},
		}

		resp, err := sendV3Request(req)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(resp)
		ctx.Req.Equal(req.CircuitId, resp.CircuitId)
		ctx.Req.NotEmpty(resp.Address)
	})

	t.Run("invalid identity", func(t *testing.T) {
		ctx.NextTest(t)

		req := &ctrl_msg.CreateCircuitV3Request{
			IdentityId: "bogus-identity-id",
			ServiceId:  svc.Id,
			CircuitId:  uuid.NewString(),
			PeerData:   map[uint32][]byte{},
		}

		_, err := sendV3Request(req)
		ctx.Req.Error(err)
	})

	t.Run("invalid service", func(t *testing.T) {
		ctx.NextTest(t)

		req := &ctrl_msg.CreateCircuitV3Request{
			IdentityId: dialerIdentity.Id,
			ServiceId:  "bogus-service-id",
			CircuitId:  uuid.NewString(),
			PeerData:   map[uint32][]byte{},
		}

		_, err := sendV3Request(req)
		ctx.Req.Error(err)
	})

	t.Run("identity without dial access", func(t *testing.T) {
		ctx.NextTest(t)

		req := &ctrl_msg.CreateCircuitV3Request{
			IdentityId: noAccessIdentity.Id,
			ServiceId:  svc.Id,
			CircuitId:  uuid.NewString(),
			PeerData:   map[uint32][]byte{},
		}

		_, err := sendV3Request(req)
		ctx.Req.Error(err)
	})

	t.Run("duplicate circuit ID", func(t *testing.T) {
		ctx.NextTest(t)

		circuitId := uuid.NewString()

		req := &ctrl_msg.CreateCircuitV3Request{
			IdentityId: dialerIdentity.Id,
			ServiceId:  svc.Id,
			CircuitId:  circuitId,
			PeerData:   map[uint32][]byte{},
		}

		resp, err := sendV3Request(req)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(resp)
		ctx.Req.Equal(circuitId, resp.CircuitId)

		// Second request with same circuit ID should fail.
		_, err = sendV3Request(req)
		ctx.Req.Error(err)
	})
}

type circuitV3Error struct {
	msg  string
	code uint32
}

func (e *circuitV3Error) Error() string {
	return e.msg
}
