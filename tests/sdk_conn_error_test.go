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
	"errors"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// Test_SDK_TypedDialErrors verifies dial failures return typed ConnErrors an app can branch on:
// a posture denial matches errors.Is(err, ziti.ErrPostureFailed) and carries the failing check
// ids plus the service and router ids, and dialing an unknown service matches
// ziti.ErrServiceNotAvailable — no message-text parsing anywhere.
func Test_SDK_TypedDialErrors(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	dialIdentityRole := eid.New()
	hostIdentityRole := eid.New()
	serviceRole := eid.New()
	postureCheckRoleAttr := eid.New()

	adminManagementApi := ctx.NewEdgeManagementApi(nil)
	_, err := adminManagementApi.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	validMac := "00:1B:21:AE:43:97"
	invalidMac := "3A:7C:92:4F:11:B6"

	postureCheck, err := adminManagementApi.CreatePostureCheckMac([]string{validMac}, []string{postureCheckRoleAttr})
	ctx.Req.NoError(err)
	checkId := *postureCheck.ID()

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	service := ctx.AdminManagementSession.testContext.newService(s(serviceRole), nil)
	service.terminatorStrategy = xt_smartrouting.Name
	ctx.AdminManagementSession.requireCreateEntity(service)

	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+dialIdentityRole), s("#"+postureCheckRoleAttr))
	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Bind", "AllOf", s("#"+serviceRole), s("#"+hostIdentityRole), nil)

	ctx.CreateEnrollAndStartEdgeRouter()

	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext(hostIdentityRole)
	defer hostContext.Close()

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	testServer := newTestServer(listener, func(conn *testServerConn) error {
		for {
			name, eof := conn.ReadString(1024, time.Minute)
			if eof {
				return conn.server.close()
			}
			conn.WriteString("hello, "+name, time.Second)
		}
	})
	testServer.start()

	_, ztx := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)
	defer ztx.Close()

	clientCtx, ok := ztx.(*ziti.ContextImpl)
	ctx.Req.True(ok)
	clientCtx.CtrlClt.SetAllowOidcDynamicallyEnabled(true) // posture is router-evaluated on the OIDC path
	ctx.Req.NoError(clientCtx.Authenticate())

	// Report a MAC the check does not allow, so the router denies the dial on posture.
	clientCtx.CtrlClt.PostureCache.SetMacProviderFunc(func() []string {
		return []string{invalidMac}
	})

	_, err = ztx.Dial(service.Name)
	ctx.Req.Error(err, "dial must fail: the MAC posture check cannot pass")

	ctx.Req.True(errors.Is(err, ziti.ErrPostureFailed),
		"the dial failure must match ziti.ErrPostureFailed, got: %v", err)

	var connErr *ziti.ConnError
	ctx.Req.True(errors.As(err, &connErr), "the dial failure must expose a ConnError, got: %v", err)
	ctx.Req.Equal(ziti.CausePostureFailed, connErr.Cause)
	ctx.Req.Contains(connErr.FailingChecks, checkId, "the failing posture check id must ride the error")
	ctx.Req.Equal(service.Id, connErr.ServiceId)
	ctx.Req.NotEmpty(connErr.RouterId, "the refusing router's id must ride the error")

	// An unknown service classifies locally, before any router is involved.
	_, err = ztx.Dial("no-such-service-" + eid.New())
	ctx.Req.Error(err)
	ctx.Req.True(errors.Is(err, ziti.ErrServiceNotAvailable),
		"an unknown service must match ziti.ErrServiceNotAvailable, got: %v", err)
}
