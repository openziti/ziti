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

	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// Test_ConnectV2_Dataflow exercises the sessionless ConnectV2 dial path
// end-to-end. The SDK defaults to V2 whenever the router advertises the
// capability and `ForceConnectV1` is not set, so the assertion is implicit:
// if any part of the V2 path is broken (decoder, access check, ordering,
// liveness tracking) the dial hangs or data fails to flow.
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

	dialOptions := &ziti.DialOptions{
		ConnectTimeout: 5 * time.Second,
	}

	conn := ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
	defer conn.Close()

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

	forceV1 := true
	dialOptions := &ziti.DialOptions{
		ConnectTimeout: 5 * time.Second,
		ForceConnectV1: &forceV1,
	}

	conn := ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
	defer conn.Close()

	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	conn.WriteString("quit", time.Second)
	conn.ReadExpected("ok", time.Second)

	testServer.waitForDone(ctx, 5*time.Second)
}
