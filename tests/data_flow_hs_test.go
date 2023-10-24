//go:build dataflow
// +build dataflow

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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/common/eid"
	"sync/atomic"
	"testing"
	"time"
)

func Test_HSDataflow(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll("weighted")

	ctx.CreateEnrollAndStartEdgeRouter()

	watcher := ctx.AdminManagementSession.newTerminatorWatcher()
	defer watcher.Close()

	_, hostContext1 := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer hostContext1.Close()

	listener1, err := hostContext1.Listen(service.Name)
	ctx.Req.NoError(err)

	_, hostContext2 := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer hostContext2.Close()

	listener2, err := hostContext2.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener2.Close()

	watcher.waitForTerminators(service.Id, 2, 2*time.Second)

	serverHandler := func(conn *testServerConn) error {
		for {
			name, eof := conn.ReadString(1024, time.Minute)
			if eof {
				return nil
			}

			pfxlog.Logger().Tracef("%v-%v: received '%v' from client\n", conn.server.idx, conn.id, name)

			result := "hello, " + name
			pfxlog.Logger().Tracef("%v-%v: returning '%v' to client\n", conn.server.idx, conn.id, result)
			conn.WriteString(result, time.Second)
			atomic.AddUint32(&conn.server.msgCount, 1)
		}
	}

	server1 := newTestServer(listener1, serverHandler)
	server2 := newTestServer(listener2, serverHandler)
	server1.start()
	server2.start()

	clientIdentity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)

	clientContext, err := ziti.NewContext(clientConfig)
	ctx.Req.NoError(err)

	for i := 0; i < 100; i++ {
		conn := ctx.WrapConn(clientContext.Dial(service.Name))

		name := eid.New()
		conn.WriteString(name, time.Second)
		conn.ReadExpected("hello, "+name, time.Second)
		conn.RequireClose()
	}

	ctx.Req.NoError(listener1.Close())
	server1.waitForDone(ctx, 5*time.Second)
	ctx.Req.True(atomic.LoadUint32(&server1.msgCount) > 25)

	ctx.Req.NoError(listener2.Close())
	server2.waitForDone(ctx, 5*time.Second)
	ctx.Req.True(atomic.LoadUint32(&server2.msgCount) > 25)
}
