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
	"github.com/netfoundry/ziti-fabric/controller/xt_smartrouting"
	"testing"
	"time"
)

func Test_Dataflow(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	service := ctx.AdminSession.requireNewServiceAccessibleToAll(xt_smartrouting.Name)
	fmt.Printf("service id: %v\n", service.id)

	ctx.createEnrollAndStartEdgeRouter()
	_, hostContext := ctx.AdminSession.requireCreateSdkContext()
	listener, err := hostContext.Listen(service.name)
	ctx.req.NoError(err)

	testServer := newTestServer(listener, func(conn *testServerConn) error {
		for {
			name, eof := conn.ReadString(1024, 1*time.Minute)
			if eof {
				fmt.Print("got eof, closing")
				return conn.server.close()
			}

			fmt.Printf("received '%v' from client\n", name)

			if name == "quit" {
				conn.WriteString("ok", time.Second)
				fmt.Print("quitting")
				return conn.server.close()
			}

			result := "hello, " + name
			fmt.Printf("returning '%v' to client\n", result)
			conn.WriteString(result, time.Second)
		}
	})
	testServer.start()

	_, clientContext := ctx.AdminSession.requireCreateSdkContext()
	conn := ctx.wrapConn(clientContext.Dial(service.name))

	name := uuid.New().String()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)
	conn.WriteString("quit", time.Second)
	conn.ReadExpected("ok", time.Second)

	testServer.waitForDone(ctx, 5*time.Second)
}
