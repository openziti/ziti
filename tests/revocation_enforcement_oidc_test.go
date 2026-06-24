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
	"net/http"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

// Revocation enforcement is driven by the router's CheckConnections reaper, which
// runs on a 5s interval, so closure is not immediate. These helpers bound the
// wait generously (several reaper ticks) to detect the close yet still fail fast
// on a regression rather than hanging until the test timeout.

// requireConnClosedByReaper waits for the reaper to close a revoked connection. A
// read surfaces the close (a bare IsClosed poll never advances the conn state).
func requireConnClosedByReaper(ctx *TestContext, conn *TestConn) {
	deadline := time.Now().Add(20 * time.Second)
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
	ctx.Req.True(conn.IsClosed(), "expected connection to be closed by revocation enforcement")
}

// requireConnSurvivesReaper confirms a connection is NOT revoked: it waits past a
// reaper tick (so an erroneous revocation would have closed it) and then verifies
// the connection is still open and carries data.
func requireConnSurvivesReaper(ctx *TestContext, conn *TestConn) {
	time.Sleep(7 * time.Second)
	ctx.Req.False(conn.IsClosed(), "connection should not be revoked")
	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)
}

func requireSetIdentityDisabled(ctx *TestContext, identityId string, path string) {
	body := &rest_model.DisableParams{DurationMinutes: ToPtr(int64(0))}
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(body).Post("identities/" + identityId + "/" + path)
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusOK, resp.StatusCode())
}

// setupRevocationEnv creates an edge router and an SDK-hosted service, and returns
// the admin management client, the service, the dialing identity, and a
// freshly-authenticated OIDC client context for it. Cleanup of the SDK contexts
// is registered on t.
func setupRevocationEnv(t *testing.T, ctx *TestContext) (*ManagementHelperClient, *service, *identity, *ziti.ContextImpl) {
	ctx.RequireAdminManagementApiLogin()

	dialIdentityRole := eid.New()
	hostIdentityRole := eid.New()
	serviceRole := eid.New()

	adminApi := ctx.NewEdgeManagementApi(nil)
	_, err := adminApi.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	svc := ctx.AdminManagementSession.testContext.newService(s(serviceRole), nil)
	svc.terminatorStrategy = xt_smartrouting.Name
	ctx.AdminManagementSession.requireCreateEntity(svc)

	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+dialIdentityRole), nil)
	ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Bind", "AllOf", s("#"+serviceRole), s("#"+hostIdentityRole), nil)

	ctx.CreateEnrollAndStartEdgeRouter()

	// Host (OIDC) hosting the service.
	_, hostZtx := ctx.AdminManagementSession.RequireCreateSdkContext(hostIdentityRole)
	hostContext := hostZtx.(*ziti.ContextImpl)
	hostContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	ctx.Req.NoError(hostContext.Authenticate())
	t.Cleanup(hostContext.Close)

	listener, err := hostContext.Listen(svc.Name)
	ctx.Req.NoError(err)
	t.Cleanup(func() { _ = listener.Close() })

	testServer := newTestServer(listener, echoServerHandler)
	testServer.start()

	// Dialing client (OIDC).
	dialIdentity, clientZtx := ctx.AdminManagementSession.RequireCreateSdkContext(dialIdentityRole)
	clientContext := clientZtx.(*ziti.ContextImpl)
	clientContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	ctx.Req.NoError(clientContext.Authenticate())
	t.Cleanup(clientContext.Close)

	return adminApi, svc, dialIdentity, clientContext
}

func requireDataFlows(ctx *TestContext, conn *TestConn) {
	name := eid.New()
	conn.WriteString(name, time.Second)
	conn.ReadExpected("hello, "+name, time.Second)
}

// echoServerHandler echoes "hello, <name>" for each name read, closing only the
// individual connection when it ends rather than the whole listener. This keeps
// the host serving across connection closes, which the revocation tests require:
// a revocation closes one connection and the test then re-dials a fresh one.
func echoServerHandler(conn *testServerConn) error {
	for {
		name, eof := conn.ReadString(1024, time.Minute)
		if eof || name == "quit" {
			return nil
		}
		conn.WriteString("hello, "+name, time.Second)
	}
}

// freshAuthedContext creates and authenticates a new OIDC SDK context for an
// existing identity, yielding a brand-new api-session (a fresh z_asid and issue
// time). Authenticate on an already-authenticated context is a no-op, so a new
// context is required to exercise re-authentication.
func freshAuthedContext(t *testing.T, ctx *TestContext, id *identity) *ziti.ContextImpl {
	ztx, err := ziti.NewContext(id.config)
	ctx.Req.NoError(err)
	clientContext, ok := ztx.(*ziti.ContextImpl)
	ctx.Req.True(ok)
	clientContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
	t.Cleanup(clientContext.Close)
	ctx.Req.NoError(clientContext.Authenticate())
	return clientContext
}

// Test_Revocation_ApiSession_OIDC covers api-session revocation enforcement: when
// the specific api-session (z_asid) backing a live connection is revoked, the
// router closes that connection; a re-authenticated session (a fresh z_asid) is
// unaffected.
func Test_Revocation_ApiSession_OIDC(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminApi, svc, dialIdentity, clientContext := setupRevocationEnv(t, ctx)

	clientConn := ctx.WrapConn(clientContext.Dial(svc.Name))
	requireDataFlows(ctx, clientConn)

	apiSessionId := clientContext.CtrlClt.GetCurrentApiSession().GetId()
	ctx.Req.NotEmpty(apiSessionId)

	t.Run("revoking the api-session closes its connection", func(t *testing.T) {
		ctx.testContextChanged(t)

		_, err := adminApi.CreateRevocation(apiSessionId, rest_model.RevocationTypeEnumAPISESSION)
		ctx.Req.NoError(err)

		requireConnClosedByReaper(ctx, clientConn)
	})

	t.Run("a fresh session for the same identity is unaffected", func(t *testing.T) {
		ctx.testContextChanged(t)

		freshContext := freshAuthedContext(t, ctx, dialIdentity)
		newApiSessionId := freshContext.CtrlClt.GetCurrentApiSession().GetId()
		ctx.Req.NotEqual(apiSessionId, newApiSessionId, "a fresh authentication should produce a new api-session id")

		newConn := ctx.WrapConn(freshContext.Dial(svc.Name))
		requireConnSurvivesReaper(ctx, newConn)
	})
}

// Test_Revocation_IdentityDisable_OIDC covers identity revocation on disable:
// disabling an identity produces an identity-scoped revocation, and the router
// closes that identity's live connections.
func Test_Revocation_IdentityDisable_OIDC(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	_, svc, dialIdentity, clientContext := setupRevocationEnv(t, ctx)

	clientConn := ctx.WrapConn(clientContext.Dial(svc.Name))
	requireDataFlows(ctx, clientConn)

	t.Run("disabling the identity closes its connection", func(t *testing.T) {
		ctx.testContextChanged(t)

		requireSetIdentityDisabled(ctx, dialIdentity.Id, "disable")

		requireConnClosedByReaper(ctx, clientConn)
	})
}

// Test_Revocation_IdentityDelete_OIDC covers identity revocation on delete:
// deleting an identity produces an identity-scoped revocation (its self-contained
// OIDC JWTs aren't otherwise reachable), and the router closes its live
// connections.
func Test_Revocation_IdentityDelete_OIDC(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	_, svc, dialIdentity, clientContext := setupRevocationEnv(t, ctx)

	clientConn := ctx.WrapConn(clientContext.Dial(svc.Name))
	requireDataFlows(ctx, clientConn)

	t.Run("deleting the identity closes its connection", func(t *testing.T) {
		ctx.testContextChanged(t)

		ctx.AdminManagementSession.requireDeleteEntity(dialIdentity)

		requireConnClosedByReaper(ctx, clientConn)
	})
}

// Test_Revocation_IdentityCutoff_PostCutoffSessionSurvives_OIDC covers the
// IssuedBefore cutoff: an identity revocation invalidates only sessions issued
// before it, so a session re-authenticated after a re-enable survives even though
// the revocation still lingers.
func Test_Revocation_IdentityCutoff_PostCutoffSessionSurvives_OIDC(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminApi, svc, dialIdentity, clientContext := setupRevocationEnv(t, ctx)

	clientConn := ctx.WrapConn(clientContext.Dial(svc.Name))
	requireDataFlows(ctx, clientConn)

	// An identity revocation's cutoff is set to its creation time, which is after
	// this session was issued, so this pre-cutoff session is revoked. (Created via
	// the management API rather than a disable so the identity stays enabled and
	// can still authenticate a new session below.)
	_, err := adminApi.CreateRevocation(dialIdentity.Id, rest_model.RevocationTypeEnumIDENTITY)
	ctx.Req.NoError(err)
	requireConnClosedByReaper(ctx, clientConn)

	t.Run("a session authenticated after the cutoff survives", func(t *testing.T) {
		ctx.testContextChanged(t)

		// A fresh session is issued after the cutoff, so it must not be revoked
		// even though the identity revocation still lingers.
		freshContext := freshAuthedContext(t, ctx, dialIdentity)
		newConn := ctx.WrapConn(freshContext.Dial(svc.Name))
		requireConnSurvivesReaper(ctx, newConn)
	})
}
