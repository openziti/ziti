//go:build apitests

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
	"github.com/google/uuid"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/controller/xt_sticky"
	"testing"
	"time"
)

func Test_StickyTerminators(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_sticky.Name)

	ctx.CreateEnrollAndStartEdgeRouter()
	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer hostContext.Close()

	listener1, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener1.Close()

	server1Id := uuid.NewString()
	server1 := newTestServer(listener1, func(conn *testServerConn) error {
		fmt.Println("server1 terminator called")
		conn.WriteString(server1Id, time.Second)
		conn.RequireClose()
		return nil
	})
	server1.start()

	_, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer clientContext.Close()

	conn := ctx.WrapConn(clientContext.Dial(service.Name))
	token := conn.Conn.GetStickinessToken()
	ctx.Req.NotNil(token)
	conn.ReadExpected(server1Id, time.Second)
	conn.RequireClose()

	listener2, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer listener2.Close()

	server2Id := uuid.NewString()
	server2 := newTestServer(listener2, func(conn *testServerConn) error {
		fmt.Println("server2 terminator called")
		conn.WriteString(server2Id, time.Second)
		conn.RequireClose()
		return nil
	})
	server2.start()

	// make sure we stick with the same terminator
	for range 10 {
		dialOptions := &ziti.DialOptions{
			ConnectTimeout:  time.Second,
			StickinessToken: token,
		}
		conn = ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
		nextToken := conn.Conn.GetStickinessToken()
		ctx.Req.Equal(string(token), string(nextToken))
		conn.ReadExpected(server1Id, time.Second)
		conn.RequireClose()
		token = nextToken
	}

	// bump the cost and make sure we stick with the same terminator even with the higher cost
	ctx.Req.NoError(listener1.UpdateCost(5000))
	time.Sleep(100 * time.Millisecond)

	for range 10 {
		dialOptions := &ziti.DialOptions{
			ConnectTimeout:  time.Second,
			StickinessToken: token,
		}
		conn = ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
		nextToken := conn.Conn.GetStickinessToken()
		ctx.Req.Equal(string(token), string(nextToken))
		conn.ReadExpected(server1Id, time.Second)
		conn.RequireClose()
		token = nextToken
	}

	// Fail the terminator and make sure we fail over
	ctx.Req.NoError(listener1.UpdatePrecedence(edge.PrecedenceFailed))
	time.Sleep(100 * time.Millisecond)

	dialOptions := &ziti.DialOptions{
		ConnectTimeout:  time.Second,
		StickinessToken: token,
	}
	conn = ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
	nextToken := conn.Conn.GetStickinessToken()
	ctx.Req.NotEqual(string(token), string(nextToken))
	conn.ReadExpected(server2Id, time.Second)
	conn.RequireClose()
	token = nextToken

	// Reset the initial terminator, bump the second terminator cost and make sure we stick with it
	ctx.Req.NoError(listener1.UpdateCostAndPrecedence(0, edge.PrecedenceDefault))
	ctx.Req.NoError(listener2.UpdateCost(5000))
	time.Sleep(100 * time.Millisecond)

	for range 10 {
		dialOptions = &ziti.DialOptions{
			ConnectTimeout:  time.Second,
			StickinessToken: token,
		}
		conn = ctx.WrapConn(clientContext.DialWithOptions(service.Name, dialOptions))
		nextToken = conn.Conn.GetStickinessToken()
		ctx.Req.Equal(string(token), string(nextToken))
		conn.ReadExpected(server2Id, time.Second)
		conn.RequireClose()
		token = nextToken
	}
}
