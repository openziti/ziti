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
	"testing"
	"time"

	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

func Test_DialerIdentityInfo(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)

	ctx.CreateEnrollAndStartEdgeRouter()

	// Create hosting identity and listen on the service
	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer hostContext.Close()

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	type dialerInfo struct {
		id   string
		name string
	}

	// Channel to collect dialer identity info from the server side
	serverInfoC := make(chan dialerInfo, 10)

	server := newTestServer(listener, func(conn *testServerConn) error {
		serviceConn := conn.Conn.(edge.ServiceConn)
		serverInfoC <- dialerInfo{
			id:   serviceConn.GetDialerIdentityId(),
			name: serviceConn.GetDialerIdentityName(),
		}
		return nil
	})
	server.start()

	// Create two dialing identities
	dialIdentity1 := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	dialConfig1 := ctx.EnrollIdentity(dialIdentity1.Id)
	dialContext1, err := ziti.NewContext(dialConfig1)
	ctx.Req.NoError(err)
	defer dialContext1.Close()

	dialIdentity2 := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
	dialConfig2 := ctx.EnrollIdentity(dialIdentity2.Id)
	dialContext2, err := ziti.NewContext(dialConfig2)
	ctx.Req.NoError(err)
	defer dialContext2.Close()

	// First identity dials
	conn1 := ctx.WrapConn(dialContext1.Dial(service.Name))
	conn1.RequireClose()

	select {
	case info := <-serverInfoC:
		ctx.Req.Equal(dialIdentity1.Id, info.id, "dialer identity ID should match first dialing identity")
		ctx.Req.Equal(dialIdentity1.name, info.name, "dialer identity name should match first dialing identity")
	case <-time.After(5 * time.Second):
		ctx.Req.Fail("timed out waiting for server to receive first connection")
	}

	// Second identity dials
	conn2 := ctx.WrapConn(dialContext2.Dial(service.Name))
	conn2.RequireClose()

	select {
	case info := <-serverInfoC:
		ctx.Req.Equal(dialIdentity2.Id, info.id, "dialer identity ID should match second dialing identity")
		ctx.Req.Equal(dialIdentity2.name, info.name, "dialer identity name should match second dialing identity")
	case <-time.After(5 * time.Second):
		ctx.Req.Fail("timed out waiting for server to receive second connection")
	}

	// Dial again with first identity to verify consistency
	conn3 := ctx.WrapConn(dialContext1.Dial(service.Name))
	conn3.RequireClose()

	select {
	case info := <-serverInfoC:
		ctx.Req.Equal(dialIdentity1.Id, info.id, "dialer identity ID should still match first dialing identity on re-dial")
		ctx.Req.Equal(dialIdentity1.name, info.name, "dialer identity name should still match first dialing identity on re-dial")
	case <-time.After(5 * time.Second):
		ctx.Req.Fail("timed out waiting for server to receive third connection")
	}

	ctx.Req.NoError(listener.Close())
	server.waitForDone(ctx, 5*time.Second)
}
