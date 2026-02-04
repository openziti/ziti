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
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/common/eid"
)

func Test_TerminatorCloseOnBindPermissionLoss(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	// Create shared role attributes
	hostRole := eid.New()
	svcRole1 := eid.New()
	svcRole2 := eid.New()

	// Create two services with distinct role attributes
	service1 := ctx.AdminManagementSession.requireNewService(s(svcRole1), nil)
	service2 := ctx.AdminManagementSession.requireNewService(s(svcRole2), nil)

	// Create edge router policies and service edge router policies so everything can connect
	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	// Create dial policies for all identities and both services
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#all"), s("#all"), nil)

	// Create bind policies: one per service, both grant to the host role
	bindPolicy1 := ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#"+svcRole1), s("#"+hostRole), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#"+svcRole2), s("#"+hostRole), nil)

	service1Watcher := ctx.AdminManagementSession.newTerminatorWatcher(service1.Id, 1)
	defer service1Watcher.Close()

	service2Watcher := ctx.AdminManagementSession.newTerminatorWatcher(service2.Id, 1)
	defer service2Watcher.Close()

	// Create a hosting identity with the host role and listen on both services
	hostIdentity, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext(hostRole)
	defer hostContext.Close()
	_ = hostIdentity

	listener1, err := hostContext.Listen(service1.Name)
	ctx.Req.NoError(err)

	listener2, err := hostContext.Listen(service2.Name)
	ctx.Req.NoError(err)

	service1Watcher.waitForTerminators(5 * time.Second)
	service2Watcher.waitForTerminators(5 * time.Second)

	serverHandler := func(conn *testServerConn) error {
		for {
			name, eof := conn.ReadString(1024, time.Minute)
			if eof {
				return nil
			}
			conn.WriteString("hello, "+name, time.Second)
			atomic.AddUint32(&conn.server.msgCount, 1)
		}
	}

	server1 := newTestServer(listener1, serverHandler)
	server2 := newTestServer(listener2, serverHandler)
	server1.start()
	server2.start()

	// Verify both services are working
	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)
	defer clientContext.Close()

	conn1 := ctx.WrapConn(clientContext.Dial(service1.Name))
	name := eid.New()
	conn1.WriteString(name, time.Second)
	conn1.ReadExpected("hello, "+name, time.Second)
	conn1.RequireClose()

	conn2 := ctx.WrapConn(clientContext.Dial(service2.Name))
	name = eid.New()
	conn2.WriteString(name, time.Second)
	conn2.ReadExpected("hello, "+name, time.Second)
	conn2.RequireClose()

	// Remove bind permission for service1 by deleting its bind policy
	ctx.AdminManagementSession.requireDeleteEntity(bindPolicy1)

	// Wait for the terminator to be removed and listener to be closed
	timeout := time.After(10 * time.Second)
	for {
		if listener1.IsClosed() {
			break
		}
		select {
		case <-timeout:
			ctx.Req.Fail("timed out waiting for listener1 to close after bind permission loss")
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Verify listener2 is still open
	ctx.Req.False(listener2.IsClosed(), "listener2 should still be open")

	// Verify service2 still works
	conn2 = ctx.WrapConn(clientContext.Dial(service2.Name))
	name = eid.New()
	conn2.WriteString(name, time.Second)
	conn2.ReadExpected("hello, "+name, time.Second)
	conn2.RequireClose()
}

func Test_CircuitCloseOnDialPermissionLoss(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.CreateEnrollAndStartEdgeRouter()

	log := pfxlog.Logger()

	// Create role attributes
	svcRole1 := eid.New()
	svcRole2 := eid.New()
	clientRole1 := eid.New()
	clientRole2 := eid.New()

	// Create two services
	service1 := ctx.AdminManagementSession.requireNewService(s(svcRole1), nil)
	service2 := ctx.AdminManagementSession.requireNewService(s(svcRole2), nil)

	// Create edge router and service edge router policies
	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	// Create bind policies for all
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#all"), s("#all"), nil)

	// Create dial policies: both client roles can dial both services
	// We'll remove access for clientRole1 to service1
	dialPolicy1 := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+svcRole1), s("#"+clientRole1), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+svcRole1), s("#"+clientRole2), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+svcRole2), s("#"+clientRole1), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+svcRole2), s("#"+clientRole2), nil)

	service1Watcher := ctx.AdminManagementSession.newTerminatorWatcher(service1.Id, 1)
	defer service1Watcher.Close()

	service2Watcher := ctx.AdminManagementSession.newTerminatorWatcher(service2.Id, 1)
	defer service2Watcher.Close()

	// Host both services
	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer hostContext.Close()

	listener1, err := hostContext.Listen(service1.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener1.Close() }()

	listener2, err := hostContext.Listen(service2.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener2.Close() }()

	service1Watcher.waitForTerminators(5 * time.Second)
	service2Watcher.waitForTerminators(5 * time.Second)

	// Echo server handler
	serverHandler := func(conn *testServerConn) error {
		for {
			name, eof := conn.ReadString(1024, time.Minute)
			if eof {
				return nil
			}
			conn.WriteString("hello, "+name, time.Second)
		}
	}

	server1 := newTestServer(listener1, serverHandler)
	server2 := newTestServer(listener2, serverHandler)
	server1.start()
	server2.start()

	// Create two client identities with distinct roles
	client1Identity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false, clientRole1)
	client1Config := ctx.EnrollIdentity(client1Identity.Id)
	client1Context, err := ziti.NewContext(client1Config)
	ctx.Req.NoError(err)
	defer client1Context.Close()

	client2Identity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false, clientRole2)
	client2Config := ctx.EnrollIdentity(client2Identity.Id)
	client2Context, err := ziti.NewContext(client2Config)
	ctx.Req.NoError(err)
	defer client2Context.Close()

	// Establish 4 persistent connections: 2 clients x 2 services
	type circuitState struct {
		label   string
		conn    *TestConn
		failed  atomic.Bool
		stopped atomic.Bool
	}

	// client1 -> service1 (this is the one that will lose access)
	c1s1Conn := ctx.WrapConn(client1Context.Dial(service1.Name))
	c1s1 := &circuitState{label: "client1-service1", conn: c1s1Conn}

	// client1 -> service2
	c1s2Conn := ctx.WrapConn(client1Context.Dial(service2.Name))
	c1s2 := &circuitState{label: "client1-service2", conn: c1s2Conn}

	// client2 -> service1
	c2s1Conn := ctx.WrapConn(client2Context.Dial(service1.Name))
	c2s1 := &circuitState{label: "client2-service1", conn: c2s1Conn}

	// client2 -> service2
	c2s2Conn := ctx.WrapConn(client2Context.Dial(service2.Name))
	c2s2 := &circuitState{label: "client2-service2", conn: c2s2Conn}

	circuits := []*circuitState{c1s1, c1s2, c2s1, c2s2}

	// Verify all circuits work before policy change
	for _, cs := range circuits {
		name := eid.New()
		cs.conn.WriteString(name, time.Second)
		cs.conn.ReadExpected("hello, "+name, time.Second)
		log.Infof("%s: initial message exchange succeeded", cs.label)
	}

	// Start goroutines that send/receive every 250ms
	for _, cs := range circuits {
		cs := cs
		go func() {
			ticker := time.NewTicker(250 * time.Millisecond)
			defer ticker.Stop()

			for range ticker.C {
				if cs.stopped.Load() {
					return
				}
				name := eid.New()
				err := cs.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
				if err != nil {
					log.Infof("%s: write deadline error: %v", cs.label, err)
					cs.failed.Store(true)
					return
				}
				_, err = cs.conn.Write([]byte(name))
				if err != nil {
					log.Infof("%s: write failed: %v", cs.label, err)
					cs.failed.Store(true)
					return
				}

				err = cs.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
				if err != nil {
					log.Infof("%s: read deadline error: %v", cs.label, err)
					cs.failed.Store(true)
					return
				}
				buf := make([]byte, len("hello, ")+len(name)+1)
				n, err := cs.conn.Read(buf)
				if err != nil {
					log.Infof("%s: read failed: %v", cs.label, err)
					cs.failed.Store(true)
					return
				}
				expected := "hello, " + name
				if string(buf[:n]) != expected {
					log.Infof("%s: unexpected response: got %q, expected %q", cs.label, string(buf[:n]), expected)
					cs.failed.Store(true)
					return
				}
			}
		}()
	}

	// Let traffic flow for a bit
	time.Sleep(time.Second)

	// Verify all circuits are still working
	for _, cs := range circuits {
		ctx.Req.False(cs.failed.Load(), fmt.Sprintf("%s should be working before policy change", cs.label))
	}

	// Remove dial permission for client1 to service1
	log.Info("deleting dial policy for client1 -> service1")
	ctx.AdminManagementSession.requireDeleteEntity(dialPolicy1)

	// Wait for the targeted circuit to fail
	timeout := time.After(15 * time.Second)
	for {
		if c1s1.failed.Load() {
			log.Info("client1-service1 circuit closed as expected")
			break
		}
		select {
		case <-timeout:
			ctx.Req.Fail("timed out waiting for client1-service1 circuit to close")
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Give a bit more time to make sure the other circuits don't fail
	time.Sleep(10 * time.Second)

	// Stop all goroutines
	for _, cs := range circuits {
		cs.stopped.Store(true)
	}
	time.Sleep(500 * time.Millisecond)

	// Verify only the targeted circuit failed
	ctx.Req.True(c1s1.failed.Load(), "client1-service1 should have failed")
	ctx.Req.False(c1s2.failed.Load(), "client1-service2 should still be working")
	ctx.Req.False(c2s1.failed.Load(), "client2-service1 should still be working")
	ctx.Req.False(c2s2.failed.Load(), "client2-service2 should still be working")
}
