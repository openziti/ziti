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
	"errors"
	"fmt"
	"github.com/openziti/edge/eid"
	"github.com/openziti/sdk-golang/ziti"
	"io"
	"testing"
	"time"
)

func Test_ServerConnClosePropagation(t *testing.T) {
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

	clientIdentity := ctx.AdminSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

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
	ctx.RequireAdminLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminSession.RequireCreateSdkContext()
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

	clientIdentity := ctx.AdminSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

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
	ctx.RequireAdminLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminSession.RequireCreateSdkContext()
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

	clientIdentity := ctx.AdminSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

	conn := ctx.WrapConn(clientContext.Dial(service.Name))
	defer conn.Close()

	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)
	name = eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)
}

func Test_ClientConnClosePropagation(t *testing.T) {
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
	defer listener.Close()

	clientIdentity := ctx.AdminSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

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
	ctx.RequireAdminLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener.Close()

	clientIdentity := ctx.AdminSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

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
	ctx.RequireAdminLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.RequireNewServiceAccessibleToAll("smartrouting")

	_, context := ctx.AdminSession.RequireCreateSdkContext()
	defer context.Close()

	listener, err := context.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener.Close()

	clientIdentity := ctx.AdminSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

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

func Test_ClientConnCloseWritePropagation(t *testing.T) {
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

	clientIdentity := ctx.AdminSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

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
