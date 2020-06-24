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
	"github.com/openziti/edge/eid"
	"github.com/openziti/sdk-golang/ziti"
	"sync/atomic"
	"testing"
	"time"
)

func Test_HSDataflow(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	service := ctx.AdminSession.RequireNewServiceAccessibleToAll("weighted")
	fmt.Printf("service id: %v\n", service.Id)

	ctx.CreateEnrollAndStartEdgeRouter()

	_, hostContext1 := ctx.AdminSession.RequireCreateSdkContext()
	listener1, err := hostContext1.Listen(service.Name)
	ctx.Req.NoError(err)

	_, hostContext2 := ctx.AdminSession.RequireCreateSdkContext()
	listener2, err := hostContext2.Listen(service.Name)
	ctx.Req.NoError(err)

	serverHandler := func(conn *testServerConn) error {
		for {
			name, eof := conn.ReadString(1024, time.Minute)
			if eof {
				return nil
			}

			fmt.Printf("%v-%v: received '%v' from client\n", conn.server.idx, conn.id, name)

			result := "hello, " + name
			fmt.Printf("%v-%v: returning '%v' to client\n", conn.server.idx, conn.id, result)
			conn.WriteString(result, time.Second)
			atomic.AddUint32(&conn.server.msgCount, 1)
		}
	}

	server1 := newTestServer(listener1, serverHandler)
	server2 := newTestServer(listener2, serverHandler)
	server1.start()
	server2.start()

	clientIdentity := ctx.AdminSession.RequireNewIdentityWithOtt(false)
	clientConfig := ctx.EnrollIdentity(clientIdentity.Id)
	clientContext := ziti.NewContextWithConfig(clientConfig)

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
