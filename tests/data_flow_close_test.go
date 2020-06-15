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
	"fmt"
	"github.com/google/uuid"
	"github.com/openziti/sdk-golang/ziti"
	"io"
	"testing"
	"time"
)

func Test_ServerConnClosePropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	ctx.createEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.requireNewServiceAccessibleToAll("smartrouting")
	fmt.Printf("service id: %v\n", service.id)

	_, context := ctx.AdminSession.requireCreateSdkContext()
	listener, err := context.Listen(service.name)
	ctx.req.NoError(err)

	defer func() {
		ctx.req.NoError(listener.Close())
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

		conn := ctx.wrapNetConn(listener.Accept())
		name := conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		conn.RequireClose()
	}()

	clientIdentity := ctx.AdminSession.requireNewIdentityWithOtt(false)
	clientConfig := ctx.enrollIdentity(clientIdentity.id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

	conn := ctx.wrapConn(clientContext.Dial(service.name))
	name := uuid.New().String()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	select {
	case err := <-errC:
		ctx.req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := conn.Read(make([]byte, 1024))
	ctx.req.Equal(0, n)
	ctx.req.Equal(err, io.EOF)
}

func Test_ServerContextClosePropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	ctx.createEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.requireNewServiceAccessibleToAll("smartrouting")
	fmt.Printf("service id: %v\n", service.id)

	_, context := ctx.AdminSession.requireCreateSdkContext()
	listener, err := context.Listen(service.name)
	ctx.req.NoError(err)

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

		conn := ctx.wrapNetConn(listener.Accept())
		name := conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		context.Close()
	}()

	clientIdentity := ctx.AdminSession.requireNewIdentityWithOtt(false)
	clientConfig := ctx.enrollIdentity(clientIdentity.id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

	conn := ctx.wrapConn(clientContext.Dial(service.name))
	name := uuid.New().String()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	select {
	case err := <-errC:
		ctx.req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := conn.Read(make([]byte, 1024))
	ctx.req.Equal(0, n)
	ctx.req.Equal(err, io.EOF)
}

// closing the listener should _not_ close open connections
func Test_ServerCloseListenerPropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	ctx.createEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.requireNewServiceAccessibleToAll("smartrouting")
	fmt.Printf("service id: %v\n", service.id)

	_, context := ctx.AdminSession.requireCreateSdkContext()
	listener, err := context.Listen(service.name)
	ctx.req.NoError(err)

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

		conn := ctx.wrapNetConn(listener.Accept())
		name := conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		ctx.req.NoError(listener.Close())
		name = conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
	}()

	clientIdentity := ctx.AdminSession.requireNewIdentityWithOtt(false)
	clientConfig := ctx.enrollIdentity(clientIdentity.id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

	conn := ctx.wrapConn(clientContext.Dial(service.name))
	name := uuid.New().String()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)
	name = uuid.New().String()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)
}

func Test_ClientConnClosePropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	ctx.createEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.requireNewServiceAccessibleToAll("smartrouting")
	fmt.Printf("service id: %v\n", service.id)

	_, context := ctx.AdminSession.requireCreateSdkContext()
	listener, err := context.Listen(service.name)
	ctx.req.NoError(err)

	clientIdentity := ctx.AdminSession.requireNewIdentityWithOtt(false)
	clientConfig := ctx.enrollIdentity(clientIdentity.id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

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

		conn := ctx.wrapConn(clientContext.Dial(service.name))
		name := conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		conn.RequireClose()
	}()

	conn := ctx.wrapNetConn(listener.Accept())
	name := uuid.New().String()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	select {
	case err := <-errC:
		ctx.req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := conn.Read(make([]byte, 1024))
	ctx.req.Equal(0, n)
	ctx.req.Equal(err, io.EOF)
}

func Test_ClientContextClosePropagation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	ctx.createEnrollAndStartEdgeRouter()

	service := ctx.AdminSession.requireNewServiceAccessibleToAll("smartrouting")
	fmt.Printf("service id: %v\n", service.id)

	_, context := ctx.AdminSession.requireCreateSdkContext()
	listener, err := context.Listen(service.name)
	ctx.req.NoError(err)

	clientIdentity := ctx.AdminSession.requireNewIdentityWithOtt(false)
	clientConfig := ctx.enrollIdentity(clientIdentity.id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

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

		conn := ctx.wrapConn(clientContext.Dial(service.name))
		name := conn.ReadString(512, time.Second)
		conn.WriteString("hello, "+name, time.Second)
		conn.RequireClose()
		clientContext.Close()
	}()

	conn := ctx.wrapNetConn(listener.Accept())
	name := uuid.New().String()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	select {
	case err := <-errC:
		ctx.req.NoError(err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out after 2 seconds")
	}

	ctx.req.NoError(conn.SetReadDeadline(time.Now().Add(time.Second)))
	n, err := conn.Read(make([]byte, 1024))
	ctx.req.Equal(0, n)
	ctx.req.Equal(err, io.EOF)
}
