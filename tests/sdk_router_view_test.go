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

	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// routerViewCheckPassing walks a RouterView's router -> service -> policy -> check chain and
// returns the pass/fail state for the given service and check id. The second return is false
// until the view carries the service with pushed state for that check.
func routerViewCheckPassing(view ziti.RouterView, serviceName, checkId string) (bool, bool) {
	for _, svc := range view.Services {
		if svc.Name != serviceName {
			continue
		}
		for _, policy := range svc.Policies {
			for _, check := range policy.PostureChecks {
				if check.Id == checkId && check.IsPassing != nil {
					return *check.IsPassing, true
				}
			}
		}
	}
	return false, false
}

// Test_SDK_RouterViews verifies the public per-router view surface end to end, using only
// Context interface and Eventer calls: GetRouterViews reports the connected, subscribed router
// with its pushed services and the posture pass/fail overlaid into PostureQueries, and
// EventRouterViewChanged delivers the updated view when the router's posture evaluation flips.
func Test_SDK_RouterViews(t *testing.T) {
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

	// Collect view-change events through the public listener before subscribing.
	viewEvents := make(chan ziti.RouterView, 64)
	removeListener := ztx.Events().AddRouterViewChangedListener(func(_ ziti.Context, view ziti.RouterView) {
		viewEvents <- view
	})
	defer removeListener()

	// Dial with a valid MAC so posture data is on the router, then subscribe.
	clientConn := ctx.WrapConn(ztx.Dial(service.Name))
	defer func() { _ = clientConn.Close() }()

	ctx.Req.NoError(ztx.SubscribeToServiceUpdatesFromRouter())

	// GetRouterViews shows the connected, subscribed router with the service and the check
	// passing, walked entirely through the public view.
	ctx.Req.Eventually(func() bool {
		for _, view := range ztx.GetRouterViews() {
			if !view.Connected || !view.SupportsPush || !view.Subscribed || view.Id == "" {
				continue
			}
			if passing, found := routerViewCheckPassing(view, service.Name, checkId); found && passing {
				return true
			}
		}
		return false
	}, 15*time.Second, 250*time.Millisecond,
		"GetRouterViews should show the subscribed router, with its id, with the service's MAC check passing")

	// Flip the reported MAC; the router re-evaluates and the app sees the flip arrive as a
	// view-change event with the updated overlay.
	currentMac = invalidMac
	postureCache.Evaluate()

	ctx.Req.Eventually(func() bool {
		select {
		case view := <-viewEvents:
			passing, found := routerViewCheckPassing(view, service.Name, checkId)
			return found && !passing
		default:
			return false
		}
	}, 15*time.Second, 100*time.Millisecond,
		"the posture flip should arrive as an EventRouterViewChanged carrying the failing check")
}
