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
	"github.com/openziti/ziti/v2/common/eid"
)

func Test_ServerConnClosePropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	ctx.Req.NoError(err)

	defer func() {
		ctx.Req.NoError(listener.Close())
	}()

	errC := make(chan error, 1)

	go func() {
		defer func() {
			val := recover()
			if val != nil {
				err := val.(error)
				errC <- err
			}
			close(errC)
		}()

		conn := ctx.WrapNetConn(listener.AcceptEdge())
		name := conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		conn.RequireClose()
	}()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	conn := ctx.WrapConn(clientContext.Dial(service.Name))
	defer conn.Close()

	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	select {
	case err := <-errC:
		ctx.Req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.Req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := conn.Read(make([]byte, 1024))
	ctx.Req.Equal(0, n)
	ctx.Req.Equal(err, io.EOF)
}

func Test_ServerContextClosePropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	defer listener.Close()

	ctx.Req.NoError(err)

	errC := make(chan error, 1)

	go func() {
		defer func() {
			val := recover()
			if val != nil {
				if err, ok := val.(error); ok {
					errC <- err
				} else if str, ok := val.(string); ok {
					errC <- errors.New(str)
				} else {
					errC <- errors.New(fmt.Sprintf("%v", val))
				}
			}
			close(errC)
		}()

		conn := ctx.WrapNetConn(listener.AcceptEdge())
		name := conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		context.Close()
	}()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	conn := ctx.WrapConn(clientContext.Dial(service.Name))
	defer conn.Close()

	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	select {
	case err := <-errC:
		ctx.Req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.Req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := conn.Read(make([]byte, 1024))
	ctx.Req.Equal(0, n)
	ctx.Req.Equal(err, io.EOF)
}

// closing the listener should _not_ close open connections
func Test_ServerCloseListenerPropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	defer listener.Close()

	ctx.Req.NoError(err)

	errC := make(chan error, 1)

	go func() {
		defer func() {
			val := recover()
			if val != nil {
				if err, ok := val.(error); ok {
					errC <- err
				} else if str, ok := val.(string); ok {
					errC <- errors.New(str)
				} else {
					errC <- errors.New(fmt.Sprintf("%v", val))
				}
			}
			close(errC)
		}()

		conn := ctx.WrapNetConn(listener.AcceptEdge())
		name := conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		ctx.Req.NoError(listener.Close())
		name = conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
	}()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	conn := ctx.WrapConn(clientContext.Dial(service.Name))
	defer conn.Close()

	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)
	name = eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	// Wait for the hosting side to finish before the deferred teardown runs. Reading the last
	// reply only proves the data reached the wire, not that the hosting write has returned:
	// the SDK writes with SendAndWaitForWire, so the write is still waiting to be told its
	// buffer went out. Closing the context under it resolves that wait as "channel closed"
	// and fails a write whose data the client already has.
	select {
	case err := <-errC:
		ctx.Req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}
}

func Test_ClientConnClosePropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener.Close()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	errC := make(chan error, 1)

	go func() {
		defer func() {
			val := recover()
			if val != nil {
				if err, ok := val.(error); ok {
					errC <- err
				} else if str, ok := val.(string); ok {
					errC <- errors.New(str)
				} else {
					errC <- errors.New(fmt.Sprintf("%v", val))
				}
			}
			close(errC)
		}()

		conn := ctx.WrapConn(clientContext.Dial(service.Name))
		name := conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		conn.RequireClose()
	}()

	conn := ctx.WrapNetConn(listener.AcceptEdge())
	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	select {
	case err := <-errC:
		ctx.Req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.Req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := conn.Read(make([]byte, 1024))
	ctx.Req.Equal(0, n)
	ctx.Req.Equal(err, io.EOF)
}

func Test_ClientContextClosePropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener.Close()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	errC := make(chan error, 1)

	go func() {
		defer func() {
			val := recover()
			if val != nil {
				if err, ok := val.(error); ok {
					errC <- err
				} else if str, ok := val.(string); ok {
					errC <- errors.New(str)
				} else {
					errC <- errors.New(fmt.Sprintf("%v", val))
				}
			}
			close(errC)
		}()

		conn := ctx.WrapConn(clientContext.Dial(service.Name))
		name := conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		conn.RequireClose()
		clientContext.Close()
	}()

	conn := ctx.WrapNetConn(listener.AcceptEdge())
	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	select {
	case err := <-errC:
		ctx.Req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.Req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := conn.Read(make([]byte, 1024))
	ctx.Req.Equal(0, n)
	ctx.Req.Equal(err, io.EOF)
}

func Test_ServerConnCloseWritePropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener.Close()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	errC := make(chan error, 1)

	go func() {
		defer func() {
			val := recover()
			if val != nil {
				if err, ok := val.(error); ok {
					errC <- err
				} else if str, ok := val.(string); ok {
					errC <- errors.New(str)
				} else {
					errC <- errors.New(fmt.Sprintf("%v", val))
				}
			}
			close(errC)
		}()

		conn := ctx.WrapConn(clientContext.Dial(service.Name))
		name := conn.ReadString(512, 2*time.Second)
		n, err := conn.Read(make([]byte, 128))
		if err != io.EOF {
			errC <- fmt.Errorf("did not receive EOF err(%v) %d", err, n)
		}
		conn.WriteString("hello, "+name+"\nI got your FIN!", time.Second)
		conn.RequireClose()
	}()

	conn := ctx.WrapNetConn(listener.AcceptEdge())
	name := eid.New()
	conn.WriteString(name, time.Second)
	_ = conn.CloseWrite()

	select {
	case err := <-errC:
		ctx.Req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.Req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	conn.ReadExpected("hello, "+name+"\nI got your FIN!", time.Second)

	n, err := conn.Read(make([]byte, 1024))
	ctx.Req.Equal(0, n)
	ctx.Req.Equal(err, io.EOF)
}

// Test_ServerConnCloseWritePropagationXgressTerminator covers the same half-close as
// Test_ServerConnCloseWritePropagation across a mixed-mode circuit: the host binds with SDK
// flow control, so its terminator is an xgress conn, while the client is forced onto the
// legacy ConnectV1 dial path, so the router bridges between the two. The client's read must
// still end at io.EOF while its own write side stays open, which requires the router to relay
// the terminator's xgress EOF to the client as an edge FIN.
//
// Both options matter: with a default dial the client would negotiate ConnectV2 and both ends
// would run xgress, which never crosses this seam.
func Test_ServerConnCloseWritePropagationXgressTerminator(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer context.Close()

	sdkFlowControl := true
	listenOptions := ziti.DefaultListenOptions()
	listenOptions.SdkFlowControl = &sdkFlowControl

	listener, err := context.ListenWithOptions(service.Name, listenOptions)
	ctx.Req.NoError(err)
	defer listener.Close()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	forceV1 := true
	dialOptions := &ziti.DialOptions{
		ConnectTimeout: 5 * time.Second,
		ForceConnectV1: &forceV1,
	}

	errC := make(chan error, 1)

	go func() {
		defer func() {
			val := recover()
			if val != nil {
				if err, ok := val.(error); ok {
					errC <- err
				} else if str, ok := val.(string); ok {
					errC <- errors.New(str)
				} else {
					errC <- errors.New(fmt.Sprintf("%v", val))
				}
			}
			close(errC)
		}()

		conn := ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
		name := conn.ReadString(512, 2*time.Second)
		n, err := conn.Read(make([]byte, 128))
		if err != io.EOF {
			errC <- fmt.Errorf("did not receive EOF err(%v) %d", err, n)
		}
		conn.WriteString("hello, "+name+"\nI got your FIN!", time.Second)
		conn.RequireClose()
	}()

	conn := ctx.WrapNetConn(listener.AcceptEdge())
	name := eid.New()
	conn.WriteString(name, time.Second)
	_ = conn.CloseWrite()

	select {
	case err := <-errC:
		ctx.Req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.Req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	conn.ReadExpected("hello, "+name+"\nI got your FIN!", time.Second)

	n, err := conn.Read(make([]byte, 1024))
	ctx.Req.Equal(0, n)
	ctx.Req.Equal(err, io.EOF)
}

func Test_ClientConnCloseWritePropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener.Close()

	defer func() {
		ctx.Req.NoError(listener.Close())
	}()

	errC := make(chan error, 1)

	go func() {
		defer func() {
			val := recover()
			if val != nil {
				err := val.(error)
				errC <- err
			}
			close(errC)
		}()

		conn := ctx.WrapNetConn(listener.AcceptEdge())
		name := conn.ReadString(512, time.Second)
		n, err := conn.Read(make([]byte, 128))
		if err != io.EOF {
			errC <- fmt.Errorf("did not receive EOF err(%v) %d", err, n)
		}
		conn.WriteString("hello, "+name+"\nI got your FIN!", time.Second)
		conn.RequireClose()
	}()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	conn := ctx.WrapConn(clientContext.Dial(service.Name))
	name := eid.New()
	conn.WriteString(name, time.Second)
	_ = conn.CloseWrite()
	conn.ReadExpected("hello, "+name+"\nI got your FIN!", time.Second)

	select {
	case err := <-errC:
		ctx.Req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.Req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := conn.Read(make([]byte, 1024))
	ctx.Req.Equal(0, n)
	ctx.Req.Equal(err, io.EOF)
}

// Test_ClientConnCloseWritePropagationXgressInitiator covers the same half-close as
// Test_ClientConnCloseWritePropagation across the mixed-mode circuit that
// Test_ServerConnCloseWritePropagationXgressTerminator crosses in the other direction: the
// client dials with SDK flow control, so its initiator is an xgress conn, while the host binds
// without it, so its terminator is a legacy edge conn. The host's read must end at io.EOF after
// the client's CloseWrite while its own write side stays open, which requires the router to
// relay the initiator's xgress EOF to the host as an edge FIN.
//
// ForceConnectV1 pins the legacy dial path, where SdkFlowControl is what makes the initiator an
// xgress conn. A ConnectV2 dial reaches the same seam, since V2 always uses SDK flow control.
//
// The host writes first and the client reads that before half-closing. The terminator's
// capabilities reply precedes any host data on the circuit, so by then the client's xgress
// knows its peer supports the native EOF flag and sends that. Half-closing earlier can race
// the reply, in which case the SDK falls back to an in-band edge FIN that the router forwards
// as a payload header, and the relay under test is never exercised.
func Test_ClientConnCloseWritePropagationXgressInitiator(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener.Close()

	errC := make(chan error, 1)

	go func() {
		defer func() {
			val := recover()
			if val != nil {
				if err, ok := val.(error); ok {
					errC <- err
				} else {
					errC <- fmt.Errorf("%v", val)
				}
			}
			close(errC)
		}()

		conn := ctx.WrapNetConn(listener.AcceptEdge())
		conn.WriteString("greetings", time.Second)
		name := conn.ReadString(512, time.Second)
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			errC <- err
			return
		}
		n, err := conn.Read(make([]byte, 128))
		if err != io.EOF {
			errC <- fmt.Errorf("did not receive EOF err(%v) %d", err, n)
			return
		}
		conn.WriteString("hello, "+name+"\nI got your FIN!", time.Second)
		conn.RequireClose()
	}()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	forceV1 := true
	sdkFlowControl := true
	dialOptions := &ziti.DialOptions{
		ConnectTimeout: 5 * time.Second,
		ForceConnectV1: &forceV1,
		SdkFlowControl: &sdkFlowControl,
	}

	conn := ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
	conn.ReadExpected("greetings", time.Second)

	name := eid.New()
	conn.WriteString(name, time.Second)
	_ = conn.CloseWrite()

	select {
	case err := <-errC:
		ctx.Req.NoError(err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out after 10 seconds")
	}

	conn.ReadExpected("hello, "+name+"\nI got your FIN!", time.Second)

	ctx.Req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := conn.Read(make([]byte, 1024))
	ctx.Req.Equal(0, n)
	ctx.Req.Equal(err, io.EOF)
}
