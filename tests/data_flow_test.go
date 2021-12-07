//go:build dataflow
// +build dataflow

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
	"bytes"
	"github.com/google/uuid"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/xt_smartrouting"
	"math"
	"testing"
	"time"
)

func Test_Dataflow(t *testing.T) {
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
			name, eof := conn.ReadString(math.MaxUint16*4, 1*time.Minute)
			if eof {
				return conn.server.close()
			}

			if name == "quit" {
				conn.WriteString("ok", time.Second)
				return conn.server.close()
			}

			result := "hello, " + name
			conn.WriteString(result, time.Second)
		}
	})
	testServer.start()

	_, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer clientContext.Close()

	conn := ctx.WrapConn(clientContext.Dial(service.Name))
	defer conn.Close()

	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)

	longStr := &bytes.Buffer{}
	for longStr.Len() < math.MaxUint16*2 {
		longStr.WriteString(uuid.NewString())
	}
	conn.WriteString(longStr.String(), time.Second)
	conn.ReadExpected("hello, "+longStr.String(), time.Second)
	conn.WriteString("quit", time.Second)
	conn.ReadExpected("ok", time.Second)

	testServer.waitForDone(ctx, 5*time.Second)
}
