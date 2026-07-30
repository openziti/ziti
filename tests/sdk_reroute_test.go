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

	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// Test_SdkReroute_SameRouterTakeover proves the Phase A walking skeleton: a
// reroutable circuit survives the loss of the SDK's ingress edge-router channel
// by reattaching the circuit via a takeover, so application data keeps flowing.
//
// It uses a single edge router, which makes the takeover a same-router
// reattachment (the design allows the new ingress to equal the old). That still
// exercises the whole E1 loop end-to-end: the SDK detects the path loss and
// holds the xgress open, selects a takeover candidate, runs the takeover
// exchange, the controller holds the circuit in Limbo and re-splices the ingress
// (X1), the SDK adds the new path and flushes buffered data. It also exercises
// endpoint-scoped ingress cleanup preserving the co-located terminator.
func Test_SdkReroute_SameRouterTakeover(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)

	ctx.CreateEnrollAndStartEdgeRouter()

	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer hostContext.Close()

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener.Close()

	testServer := newTestServer(listener, func(conn *testServerConn) error {
		for {
			name, eof := conn.ReadString(math.MaxUint16*4, time.Minute)
			if eof {
				return conn.server.close()
			}
			conn.WriteString("hello, "+name, time.Second)
		}
	})
	testServer.start()

	_, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer clientContext.Close()

	var dialEvt ziti.DialEvent
	dialEvtSet := false
	removeListener := clientContext.Events().AddDialListener(func(_ ziti.Context, evt ziti.DialEvent) {
		if evt.ServiceName == service.Name {
			dialEvt = evt
			dialEvtSet = true
		}
	})
	defer removeListener()

	conn := ctx.WrapConn(clientContext.DialWithOptions(service.Name, &ziti.DialOptions{ConnectTimeout: 5 * time.Second}))
	defer conn.Close()

	// Reroute rides the ConnectV2 xgress path; fail loudly if the dial fell back.
	ctx.Req.True(dialEvtSet, "expected a dial event for service %s", service.Name)
	ctx.Req.Equal(edge.DialProtocolConnectV2, dialEvt.Protocol, "reroute requires the ConnectV2 dial path")

	// Baseline: data flows before the rollover.
	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	// Drop the client's ingress edge-router channel. This is a transport loss,
	// not a logical close, so the reroutable conn holds its xgress open and the
	// recovery loop reattaches the circuit via a takeover.
	ctxImpl, ok := clientContext.(*ziti.ContextImpl)
	ctx.Req.True(ok, "expected *ziti.ContextImpl from ziti.NewContext")
	ctxImpl.CloseAllEdgeRouterConns()

	// Data flow survives: the write is buffered during the recovery hold and
	// flushed over the new path once the takeover completes.
	name2 := eid.New()
	conn.WriteString(name2, 10*time.Second)
	conn.ReadExpected("hello, "+name2, 10*time.Second)
}
