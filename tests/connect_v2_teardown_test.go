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
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// Test_ConnectV2_TeardownPropagation verifies that connection close propagates
// across the circuit in both directions on the ConnectV2 dial path, and that
// the V1 fallback still behaves identically.
//
// The interesting case is client-initiated close: when the dialing (initiator)
// side closes, the hosting (terminator) side's blocked Read must return io.EOF
// promptly, not hang until the channel is torn down. ConnectV2 dials the
// initiator over SDK xgress while the host stays on the legacy edge conn, so
// the initiator's end-of-circuit has to cross that boundary and surface as EOF
// on the host. The matching server-initiated direction already works via the
// data-flow tests; it's included here as a guard.
func Test_ConnectV2_TeardownPropagation(t *testing.T) {
	t.Run("connect-v2", func(t *testing.T) {
		testTeardownPropagation(t, false)
	})
	t.Run("connect-v1-fallback", func(t *testing.T) {
		testTeardownPropagation(t, true)
	})
}

func testTeardownPropagation(t *testing.T, forceV1 bool) {
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

	dialOptions := &ziti.DialOptions{ConnectTimeout: 5 * time.Second}
	if forceV1 {
		dialOptions.ForceConnectV1 = &forceV1
	}

	expectedProtocol := edge.DialProtocolConnectV2
	if forceV1 {
		expectedProtocol = edge.DialProtocolConnectV1
	}

	t.Run("client-initiated close surfaces EOF on the host", func(t *testing.T) {
		errC := make(chan error, 1)

		go func() {
			defer func() {
				if val := recover(); val != nil {
					if err, ok := val.(error); ok {
						errC <- err
					} else {
						errC <- errors.New(fmt.Sprintf("%v", val))
					}
				}
				close(errC)
			}()

			conn := ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
			ctx.Req.True(dialEvtSet, "expected a dial event for service %s", service.Name)
			ctx.Req.Equal(expectedProtocol, dialEvt.Protocol, "unexpected dial protocol")

			name := conn.ReadString(512, time.Second)
			conn.WriteString("hello, "+name, time.Second)
			conn.RequireClose()
		}()

		hostConn := ctx.WrapNetConn(listener.AcceptEdge())
		name := eid.New()
		hostConn.WriteString(name, time.Second)
		hostConn.ReadExpected("hello, "+name, time.Second)

		select {
		case err := <-errC:
			ctx.Req.NoError(err)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for client goroutine to finish")
		}

		// The client has closed. The host's Read must observe EOF promptly,
		// driven by the propagated end-of-circuit, well before any channel
		// teardown.
		ctx.Req.NoError(hostConn.SetReadDeadline(time.Now().Add(2 * time.Second)))
		n, err := hostConn.Read(make([]byte, 1024))
		ctx.Req.Equal(0, n)
		ctx.Req.Equal(io.EOF, err, "host should observe EOF after client close, got %v", err)
	})

	t.Run("host-initiated close surfaces EOF on the client", func(t *testing.T) {
		errC := make(chan error, 1)

		var clientConn *TestConn
		go func() {
			defer func() {
				if val := recover(); val != nil {
					if err, ok := val.(error); ok {
						errC <- err
					} else {
						errC <- errors.New(fmt.Sprintf("%v", val))
					}
				}
				close(errC)
			}()

			hostConn := ctx.WrapNetConn(listener.AcceptEdge())
			name := hostConn.ReadString(512, time.Second)
			hostConn.WriteString("hello, "+name, time.Second)
			hostConn.RequireClose()
		}()

		clientConn = ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
		name := eid.New()
		clientConn.WriteString(name, time.Second)
		clientConn.ReadExpected("hello, "+name, time.Second)

		select {
		case err := <-errC:
			ctx.Req.NoError(err)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for host goroutine to finish")
		}

		ctx.Req.NoError(clientConn.SetReadDeadline(time.Now().Add(2 * time.Second)))
		n, err := clientConn.Read(make([]byte, 1024))
		ctx.Req.Equal(0, n)
		ctx.Req.Equal(io.EOF, err, "client should observe EOF after host close, got %v", err)
	})
}
