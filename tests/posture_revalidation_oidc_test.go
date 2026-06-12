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

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge/posture"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// echoPostureTestServer is the hosted-service handler used by the posture
// revalidation tests: it echoes "hello, <name>" for each name it reads.
func echoPostureTestServer(conn *testServerConn) error {
	for {
		name, eof := conn.ReadString(1024, 1*time.Minute)
		if eof {
			return conn.server.close()
		}
		if name == "quit" {
			conn.WriteString("ok", time.Second)
			return conn.server.close()
		}
		conn.WriteString("hello, "+name, time.Second)
	}
}

// requireConnClosed polls until conn reports closed, failing the test if it
// stays open past the timeout. Posture revalidation is asynchronous (the router
// re-evaluates and tears the circuit out of band), so a poll is required.
func requireConnClosed(ctx *TestContext, conn *TestConn) {
	// A read is what surfaces the close on these conns (a bare IsClosed poll
	// never advances the conn state). Loop with a short read deadline so we detect
	// the close promptly yet bound the total wait, failing fast on a regression
	// instead of blocking until the test timeout.
	deadline := time.Now().Add(10 * time.Second)
	buf := make([]byte, 1)
	for time.Now().Before(deadline) {
		if conn.IsClosed() {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		if _, err := conn.Read(buf); err != nil && conn.IsClosed() {
			return
		}
	}
	ctx.Req.True(conn.IsClosed(), "expected connection to be closed by posture revalidation")
}

// requireConnUsable asserts a connection survives a posture change: revocation,
// if it were going to happen, fires within ~100ms of the posture update, so this
// waits well past that and then confirms the connection is still open and still
// carries data end to end.
func requireConnUsable(ctx *TestContext, conn *TestConn) {
	time.Sleep(1 * time.Second)
	ctx.Req.False(conn.IsClosed(), "connection should remain open after a still-compliant posture change")
	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)
}

// Test_PostureRevalidation_HostTerminator_OnPostureData_OIDC covers the gap where
// a posture-data change revoked only dial conns and missed hosted bind
// terminators. The host's OS posture goes invalid while a client circuit is
// active; the router must revalidate the host's bind access and revoke its
// terminator, tearing down the active circuit.
func Test_PostureRevalidation_HostTerminator_OnPostureData_OIDC(t *testing.T) {
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

	targetOs := &rest_model.OperatingSystem{
		Type:     ToPtr(rest_model.OsTypeWindows),
		Versions: []string{"1.0.0"},
	}
	validOsInfo := posture.OsInfo{Type: string(*targetOs.Type), Version: targetOs.Versions[0]}
	invalidOsInfo := posture.OsInfo{Type: string(*targetOs.Type), Version: "2.0.0"}

	_, err = adminManagementApi.CreatePostureCheckOs([]*rest_model.OperatingSystem{targetOs}, []string{postureCheckRoleAttr})
	ctx.Req.NoError(err)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	service := ctx.AdminManagementSession.testContext.newService(s(serviceRole), nil)
	service.terminatorStrategy = xt_smartrouting.Name
	ctx.AdminManagementSession.requireCreateEntity(service)

	// Bind requires the OS posture check (host must satisfy it); Dial has none, so
	// the client's access never changes and any revocation is driven by the host.
	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+dialIdentityRole), nil)
	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Bind", "AllOf", s("#"+serviceRole), s("#"+hostIdentityRole), s("#"+postureCheckRoleAttr))

	ctx.CreateEnrollAndStartEdgeRouter()

	// Host (OIDC) reports valid posture and hosts the service.
	_, hostZtx := ctx.AdminManagementSession.RequireCreateSdkContext(hostIdentityRole)
	hostContext := hostZtx.(*ziti.ContextImpl)
	hostContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	defer hostContext.Close()
	ctx.Req.NoError(hostContext.Authenticate())

	currentHostOsInfo := validOsInfo
	hostContext.CtrlClt.PostureCache.SetOsProviderFunc(func() posture.OsInfo {
		return currentHostOsInfo
	})

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	testServer := newTestServer(listener, echoPostureTestServer)
	testServer.start()

	// Client (OIDC) dials successfully while the host's posture is valid.
	_, clientZtx := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)
	clientContext := clientZtx.(*ziti.ContextImpl)
	clientContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	defer clientContext.Close()
	ctx.Req.NoError(clientContext.Authenticate())

	clientConn := ctx.WrapConn(clientContext.Dial(service.Name))
	defer func() { _ = clientConn.Close() }()

	name := eid.New()
	clientConn.WriteString(name, time.Second)
	clientConn.ReadExpected("hello, "+name, time.Second)

	t.Run("host posture data goes invalid", func(t *testing.T) {
		ctx.testContextChanged(t)

		currentHostOsInfo = invalidOsInfo
		hostContext.CtrlClt.PostureCache.Evaluate()

		t.Run("revokes the host terminator, tearing down the active circuit", func(t *testing.T) {
			ctx.testContextChanged(t)
			requireConnClosed(ctx, clientConn)
		})

		t.Run("a new dial fails with the terminator revoked", func(t *testing.T) {
			ctx.testContextChanged(t)

			var dialErr error
			for i := 0; i < 50; i++ {
				newConn, err := clientContext.Dial(service.Name)
				if err != nil {
					dialErr = err
					break
				}
				_ = newConn.Close()
				time.Sleep(100 * time.Millisecond)
			}
			ctx.Req.Error(dialErr)
		})
	})
}

// Test_PostureRevalidation_DialCircuit_OnRequirementChange_OIDC covers the gap
// where tightening a service policy's posture requirements (here, adding a
// posture check to the dial policy) did not re-evaluate active circuits. After
// the client establishes a circuit, the dial policy gains an OS posture check the
// client cannot satisfy; the router must revalidate and revoke the active circuit
// without waiting for a fresh posture-data report or dial.
func Test_PostureRevalidation_DialCircuit_OnRequirementChange_OIDC(t *testing.T) {
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

	// An OS posture check the client will not satisfy; not attached to any policy
	// yet, so the initial dial succeeds.
	targetOs := &rest_model.OperatingSystem{
		Type:     ToPtr(rest_model.OsTypeWindows),
		Versions: []string{"1.0.0"},
	}
	clientOsInfo := posture.OsInfo{Type: string(*targetOs.Type), Version: "2.0.0"}

	_, err = adminManagementApi.CreatePostureCheckOs([]*rest_model.OperatingSystem{targetOs}, []string{postureCheckRoleAttr})
	ctx.Req.NoError(err)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	service := ctx.AdminManagementSession.testContext.newService(s(serviceRole), nil)
	service.terminatorStrategy = xt_smartrouting.Name
	ctx.AdminManagementSession.requireCreateEntity(service)

	// Dial policy starts with no posture requirement; the host has none.
	dialPolicy := ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+dialIdentityRole), nil)
	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Bind", "AllOf", s("#"+serviceRole), s("#"+hostIdentityRole), nil)

	ctx.CreateEnrollAndStartEdgeRouter()

	// Host (OIDC) hosts the service; it has no posture requirement.
	_, hostZtx := ctx.AdminManagementSession.RequireCreateSdkContext(hostIdentityRole)
	hostContext := hostZtx.(*ziti.ContextImpl)
	hostContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	defer hostContext.Close()
	ctx.Req.NoError(hostContext.Authenticate())

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	testServer := newTestServer(listener, echoPostureTestServer)
	testServer.start()

	// Client (OIDC) reports an OS that won't satisfy the (not-yet-attached) check,
	// then dials successfully since the dial policy currently has no posture.
	_, clientZtx := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)
	clientContext := clientZtx.(*ziti.ContextImpl)
	clientContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	defer clientContext.Close()
	ctx.Req.NoError(clientContext.Authenticate())

	clientContext.CtrlClt.PostureCache.SetOsProviderFunc(func() posture.OsInfo {
		return clientOsInfo
	})

	clientConn := ctx.WrapConn(clientContext.Dial(service.Name))
	defer func() { _ = clientConn.Close() }()

	name := eid.New()
	clientConn.WriteString(name, time.Second)
	clientConn.ReadExpected("hello, "+name, time.Second)

	t.Run("adding a posture check to the dial policy", func(t *testing.T) {
		ctx.testContextChanged(t)

		dialPolicy.postureCheckRoles = s("#" + postureCheckRoleAttr)
		ctx.AdminManagementSession.requireUpdateEntity(dialPolicy)

		t.Run("revalidates and revokes the active circuit", func(t *testing.T) {
			ctx.testContextChanged(t)
			requireConnClosed(ctx, clientConn)
		})
	})
}

// Test_PostureRevalidation_NoRevocation_HostPostureStaysCompliant_OIDC is the
// negative control for the hosted-terminator path: when the host's posture data
// changes but still satisfies the bind policy's posture check, revalidation must
// run but retain the terminator and the active circuit.
func Test_PostureRevalidation_NoRevocation_HostPostureStaysCompliant_OIDC(t *testing.T) {
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

	// The OS check accepts two versions; the host moves between them, staying
	// compliant throughout.
	targetOs := &rest_model.OperatingSystem{
		Type:     ToPtr(rest_model.OsTypeWindows),
		Versions: []string{"1.0.0", "2.0.0"},
	}
	firstOsInfo := posture.OsInfo{Type: string(*targetOs.Type), Version: "1.0.0"}
	secondOsInfo := posture.OsInfo{Type: string(*targetOs.Type), Version: "2.0.0"}

	_, err = adminManagementApi.CreatePostureCheckOs([]*rest_model.OperatingSystem{targetOs}, []string{postureCheckRoleAttr})
	ctx.Req.NoError(err)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	service := ctx.AdminManagementSession.testContext.newService(s(serviceRole), nil)
	service.terminatorStrategy = xt_smartrouting.Name
	ctx.AdminManagementSession.requireCreateEntity(service)

	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+dialIdentityRole), nil)
	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Bind", "AllOf", s("#"+serviceRole), s("#"+hostIdentityRole), s("#"+postureCheckRoleAttr))

	ctx.CreateEnrollAndStartEdgeRouter()

	_, hostZtx := ctx.AdminManagementSession.RequireCreateSdkContext(hostIdentityRole)
	hostContext := hostZtx.(*ziti.ContextImpl)
	hostContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	defer hostContext.Close()
	ctx.Req.NoError(hostContext.Authenticate())

	currentHostOsInfo := firstOsInfo
	hostContext.CtrlClt.PostureCache.SetOsProviderFunc(func() posture.OsInfo {
		return currentHostOsInfo
	})

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	testServer := newTestServer(listener, echoPostureTestServer)
	testServer.start()

	_, clientZtx := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)
	clientContext := clientZtx.(*ziti.ContextImpl)
	clientContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	defer clientContext.Close()
	ctx.Req.NoError(clientContext.Authenticate())

	clientConn := ctx.WrapConn(clientContext.Dial(service.Name))
	defer func() { _ = clientConn.Close() }()

	name := eid.New()
	clientConn.WriteString(name, time.Second)
	clientConn.ReadExpected("hello, "+name, time.Second)

	t.Run("host posture data changes but stays compliant", func(t *testing.T) {
		ctx.testContextChanged(t)

		currentHostOsInfo = secondOsInfo
		hostContext.CtrlClt.PostureCache.Evaluate()

		t.Run("retains the terminator and the active circuit", func(t *testing.T) {
			ctx.testContextChanged(t)
			requireConnUsable(ctx, clientConn)
		})

		t.Run("still accepts new dials", func(t *testing.T) {
			ctx.testContextChanged(t)
			newConn := ctx.WrapConn(clientContext.Dial(service.Name))
			defer func() { _ = newConn.Close() }()
			newName := eid.New()
			newConn.WriteString(newName, time.Second)
			newConn.ReadExpected("hello, "+newName, time.Second)
		})
	})
}

// Test_PostureRevalidation_NoRevocation_DialPostureStaysCompliant_OIDC is the
// negative control for the dial path: when the client's posture data changes but
// still satisfies the dial policy's posture check, revalidation must run but
// retain the active circuit.
func Test_PostureRevalidation_NoRevocation_DialPostureStaysCompliant_OIDC(t *testing.T) {
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

	targetOs := &rest_model.OperatingSystem{
		Type:     ToPtr(rest_model.OsTypeWindows),
		Versions: []string{"1.0.0", "2.0.0"},
	}
	firstOsInfo := posture.OsInfo{Type: string(*targetOs.Type), Version: "1.0.0"}
	secondOsInfo := posture.OsInfo{Type: string(*targetOs.Type), Version: "2.0.0"}

	_, err = adminManagementApi.CreatePostureCheckOs([]*rest_model.OperatingSystem{targetOs}, []string{postureCheckRoleAttr})
	ctx.Req.NoError(err)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	service := ctx.AdminManagementSession.testContext.newService(s(serviceRole), nil)
	service.terminatorStrategy = xt_smartrouting.Name
	ctx.AdminManagementSession.requireCreateEntity(service)

	// Dial requires the OS posture check (client must satisfy it); Bind has none.
	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+dialIdentityRole), s("#"+postureCheckRoleAttr))
	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Bind", "AllOf", s("#"+serviceRole), s("#"+hostIdentityRole), nil)

	ctx.CreateEnrollAndStartEdgeRouter()

	_, hostZtx := ctx.AdminManagementSession.RequireCreateSdkContext(hostIdentityRole)
	hostContext := hostZtx.(*ziti.ContextImpl)
	hostContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	defer hostContext.Close()
	ctx.Req.NoError(hostContext.Authenticate())

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	testServer := newTestServer(listener, echoPostureTestServer)
	testServer.start()

	_, clientZtx := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)
	clientContext := clientZtx.(*ziti.ContextImpl)
	clientContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	defer clientContext.Close()
	ctx.Req.NoError(clientContext.Authenticate())

	currentClientOsInfo := firstOsInfo
	clientContext.CtrlClt.PostureCache.SetOsProviderFunc(func() posture.OsInfo {
		return currentClientOsInfo
	})

	clientConn := ctx.WrapConn(clientContext.Dial(service.Name))
	defer func() { _ = clientConn.Close() }()

	name := eid.New()
	clientConn.WriteString(name, time.Second)
	clientConn.ReadExpected("hello, "+name, time.Second)

	t.Run("client posture data changes but stays compliant", func(t *testing.T) {
		ctx.testContextChanged(t)

		currentClientOsInfo = secondOsInfo
		clientContext.CtrlClt.PostureCache.Evaluate()

		t.Run("retains the active circuit", func(t *testing.T) {
			ctx.testContextChanged(t)
			requireConnUsable(ctx, clientConn)
		})
	})
}
