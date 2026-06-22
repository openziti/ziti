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
	"testing"
	"time"

	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// Test_SDK_PostureStatePush verifies the router-pushed PostureStateChange path end to end: a
// subscribed SDK receives the full posture state, and when its posture data changes the router
// re-evaluates and pushes an updated state with an incremented seq. Posture data is submitted via a
// real dial (the OIDC posture flow), mirroring the existing posture-check SDK tests. Asserted
// through gray-box accessors (GetPostureCheckPassing / GetLastPostureSeq); a public posture callback
// is a deferred follow-up.
func Test_SDK_PostureStatePush(t *testing.T) {
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
	clientCtx.CtrlClt.SetAllowOidcDynamicallyEnabled(true) // posture push is the OIDC (router-evaluated) path
	ctx.Req.NoError(clientCtx.Authenticate())

	postureCache := clientCtx.CtrlClt.PostureCache
	currentMac := validMac
	postureCache.SetMacProviderFunc(func() []string {
		return []string{currentMac}
	})

	// Dial with a valid MAC. The dial submits posture data to the router, so by the time we
	// subscribe the router can evaluate the MAC check as passing.
	clientConn := ctx.WrapConn(clientCtx.Dial(service.Name))
	defer func() { _ = clientConn.Close() }()

	ctx.Req.NoError(clientCtx.SubscribeToServiceUpdatesFromRouter(-1))

	// The pushed posture state should show the MAC check passing.
	ctx.Req.Eventually(func() bool {
		passing, found := clientCtx.GetPostureCheckPassing(checkId)
		return found && passing
	}, 15*time.Second, 250*time.Millisecond, "MAC posture check should be pushed as passing with valid posture data")

	seqBeforeChange := clientCtx.GetLastPostureSeq()

	// Report an invalid MAC and re-evaluate; the router re-evaluates and pushes an updated state.
	currentMac = invalidMac
	postureCache.Evaluate()

	ctx.Req.Eventually(func() bool {
		passing, found := clientCtx.GetPostureCheckPassing(checkId)
		return found && !passing && clientCtx.GetLastPostureSeq() > seqBeforeChange
	}, 15*time.Second, 250*time.Millisecond, "MAC posture check should flip to failing via a pushed posture state update with a higher seq")
}
