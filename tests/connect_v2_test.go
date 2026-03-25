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

// Test_ConnectV2_Dataflow exercises the sessionless ConnectV2 dial path
// end-to-end. The SDK defaults to V2 whenever the router advertises the
// capability and `ForceConnectV1` is not set. The dial protocol is asserted
// explicitly via the DialEvent so a capability/auth negotiation regression
// fails directly rather than only as a hang or data failure.
func Test_ConnectV2_Dataflow(t *testing.T) {
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
			if name == "quit" {
				conn.WriteString("ok", time.Second)
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

	dialOptions := &ziti.DialOptions{
		ConnectTimeout: 5 * time.Second,
	}

	conn := ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
	defer conn.Close()

	ctx.Req.True(dialEvtSet, "expected a dial event for service %s", service.Name)
	ctx.Req.Equal(edge.DialProtocolConnectV2, dialEvt.Protocol, "expected the dial to take the ConnectV2 path")

	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	conn.WriteString("quit", time.Second)
	conn.ReadExpected("ok", time.Second)

	testServer.waitForDone(ctx, 5*time.Second)
}

// Test_ConnectV1_Fallback_Dataflow confirms that the V1 fallback path still
// works after the connect-v2 changes — important because the SDK still uses
// V1 against routers that don't advertise V2, and the ForceConnectV1 escape
// hatch is a documented supported option.
func Test_ConnectV1_Fallback_Dataflow(t *testing.T) {
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
			if name == "quit" {
				conn.WriteString("ok", time.Second)
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

	forceV1 := true
	dialOptions := &ziti.DialOptions{
		ConnectTimeout: 5 * time.Second,
		ForceConnectV1: &forceV1,
	}

	conn := ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
	defer conn.Close()

	ctx.Req.True(dialEvtSet, "expected a dial event for service %s", service.Name)
	ctx.Req.Equal(edge.DialProtocolConnectV1, dialEvt.Protocol, "expected the dial to take the ConnectV1 fallback path")
	ctx.Req.True(dialEvt.Forced, "expected the V1 dial to be flagged as forced via ForceConnectV1")

	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	conn.WriteString("quit", time.Second)
	conn.ReadExpected("ok", time.Second)

	testServer.waitForDone(ctx, 5*time.Second)
}
