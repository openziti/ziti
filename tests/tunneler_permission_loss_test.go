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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	routerEnv "github.com/openziti/ziti/router/env"
)

const (
	ertDialLossService1 = "ert-dial-loss-svc-1"
	ertDialLossService2 = "ert-dial-loss-svc-2"
	ertDialLossPort1    = 8691
	ertDialLossPort2    = 8692
)

// tunnelerDialFixture is a running controller, an ER/T in proxy mode intercepting two services on
// fixed local ports, an SDK host echoing on both, and one established client connection per service.
// dialPolicy1 is the only dial policy granting the router access to service1.
type tunnelerDialFixture struct {
	ctx         *TestContext
	service1    *service
	service2    *service
	dialPolicy1 *servicePolicy
	addr1       string
	addr2       string
	conn1       net.Conn
	conn2       net.Conn
}

func newTunnelerDialFixture(ctx *TestContext, oidcHost bool) *tunnelerDialFixture {
	svcRole1 := eid.New()
	svcRole2 := eid.New()

	// the proxy interceptor maps fixed service names to local ports, so the names must match the
	// router config tweak below
	service1 := ctx.newService(s(svcRole1), nil)
	service1.Name = ertDialLossService1
	service1.Id = ctx.AdminManagementSession.requireCreateEntity(service1)

	service2 := ctx.newService(s(svcRole2), nil)
	service2.Name = ertDialLossService2
	service2.Id = ctx.AdminManagementSession.requireCreateEntity(service2)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#all"), s("#all"), nil)

	// one dial policy per service, so changing dialPolicy1 affects the router's access to service1 only
	dialPolicy1 := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+svcRole1), s("#all"), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+svcRole2), s("#all"), nil)

	ctx.CreateEnrollAndStartTunnelerEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		for _, l := range cfg.Listeners {
			if l.Name == "tunnel" {
				if opts, ok := l.Options["options"].(map[interface{}]interface{}); ok {
					opts["services"] = []interface{}{
						fmt.Sprintf("%s:%d", ertDialLossService1, ertDialLossPort1),
						fmt.Sprintf("%s:%d", ertDialLossService2, ertDialLossPort2),
					}
				}
			}
		}
	})

	service1Watcher := ctx.AdminManagementSession.newTerminatorWatcher()
	defer service1Watcher.Close()

	service2Watcher := ctx.AdminManagementSession.newTerminatorWatcher()
	defer service2Watcher.Close()

	_, hostZtx := ctx.AdminManagementSession.RequireCreateSdkContext()
	ctx.testing.Cleanup(hostZtx.Close)

	if oidcHost {
		hostContext := hostZtx.(*ziti.ContextImpl)
		hostContext.CtrlClt.SetAllowOidcDynamicallyEnabled(true)
		ctx.Req.NoError(hostContext.Authenticate())
	}

	listener1, err := hostZtx.Listen(service1.Name)
	ctx.Req.NoError(err)
	ctx.testing.Cleanup(func() { _ = listener1.Close() })

	listener2, err := hostZtx.Listen(service2.Name)
	ctx.Req.NoError(err)
	ctx.testing.Cleanup(func() { _ = listener2.Close() })

	service1Watcher.waitForTerminators(service1.Id, 1, 10*time.Second)
	service2Watcher.waitForTerminators(service2.Id, 1, 10*time.Second)

	newTestServer(listener1, tcpEchoHandler).start()
	newTestServer(listener2, tcpEchoHandler).start()

	f := &tunnelerDialFixture{
		ctx:         ctx,
		service1:    service1,
		service2:    service2,
		dialPolicy1: dialPolicy1,
		addr1:       fmt.Sprintf("127.0.0.1:%d", ertDialLossPort1),
		addr2:       fmt.Sprintf("127.0.0.1:%d", ertDialLossPort2),
	}

	f.conn1 = requireDialWithRetry(ctx, f.addr1, 15*time.Second)
	ctx.testing.Cleanup(func() { _ = f.conn1.Close() })

	f.conn2 = requireDialWithRetry(ctx, f.addr2, 15*time.Second)
	ctx.testing.Cleanup(func() { _ = f.conn2.Close() })

	requireTcpEcho(ctx, f.conn1)
	requireTcpEcho(ctx, f.conn2)

	return f
}

// requireService1Revoked requires conn1 to be closed, service1 to refuse new connections, and
// service2 to be unaffected on both its existing connection and a new one.
func (f *tunnelerDialFixture) requireService1Revoked() {
	ctx := f.ctx
	requireNetConnClosed(ctx, f.conn1, 15*time.Second)

	// service2 is unaffected, on the existing connection and a new one
	requireTcpEcho(ctx, f.conn2)

	conn3 := requireDialWithRetry(ctx, f.addr2, 5*time.Second)
	defer func() { _ = conn3.Close() }()
	requireTcpEcho(ctx, conn3)
}

func tcpEchoHandler(conn *testServerConn) error {
	for {
		name, eof := conn.ReadString(1024, time.Minute)
		if eof {
			return nil
		}
		conn.WriteString("hello, "+name, time.Second)
	}
}

// Test_TunnelerCircuitCloseOnDialPermissionLoss_LegacyHost verifies that circuits intercepted by an
// ER/T are closed when the router loses dial access, with the service hosted by a legacy-auth SDK.
func Test_TunnelerCircuitCloseOnDialPermissionLoss_LegacyHost(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	f := newTunnelerDialFixture(ctx, false)
	ctx.AdminManagementSession.requireDeleteEntity(f.dialPolicy1)
	f.requireService1Revoked()
}

// Test_TunnelerCircuitCloseOnDialPermissionLoss_OidcHost is the same scenario with the service
// hosted by an OIDC-auth SDK.
func Test_TunnelerCircuitCloseOnDialPermissionLoss_OidcHost(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	f := newTunnelerDialFixture(ctx, true)
	ctx.AdminManagementSession.requireDeleteEntity(f.dialPolicy1)
	f.requireService1Revoked()
}

// Test_TunnelerCircuitCloseOnIdentityDisabled verifies that disabling the router's identity closes
// every dial circuit and refuses new dials, even though the policies still list the identity.
func Test_TunnelerCircuitCloseOnIdentityDisabled(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	f := newTunnelerDialFixture(ctx, false)

	disable := &rest_model.DisableParams{DurationMinutes: ToPtr[int64](0)}
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(disable).Post("identities/" + ctx.edgeRouterEntity.id + "/disable")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

	requireNetConnClosed(ctx, f.conn1, 15*time.Second)
	requireNetConnClosed(ctx, f.conn2, 15*time.Second)

	for _, addr := range []string{f.addr1, f.addr2} {
		addr := addr
		ctx.Req.Eventually(func() bool {
			return newDialRefused(addr)
		}, 10*time.Second, 250*time.Millisecond, "expected dials to be refused while the router identity is disabled")
	}
}

// Test_TunnelerCircuitDeniedWithoutDialPolicy verifies the controller refuses a tunnel circuit for a
// service the router's identity has no dial policy for, even when the router asks for it directly.
func Test_TunnelerCircuitDeniedWithoutDialPolicy(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	svcRole := eid.New()
	hostRole := eid.New()

	service := ctx.AdminManagementSession.requireNewService(s(svcRole), nil)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#"+svcRole), s("#"+hostRole), nil)

	edgeRouter := ctx.CreateEnrollAndStartTunnelerEdgeRouter()

	watcher := ctx.AdminManagementSession.newTerminatorWatcher()
	defer watcher.Close()

	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext(hostRole)
	defer hostContext.Close()

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	watcher.waitForTerminators(service.Id, 1, 10*time.Second)

	ctrlCh := edgeRouter.GetNetworkControllers().AnyCtrlChannel()
	ctx.Req.NotNil(ctrlCh)

	request := &edge_ctrl_pb.CreateTunnelCircuitV2Request{
		ServiceName: service.Name,
	}
	reply, err := protobufs.MarshalTyped(request).WithTimeout(5 * time.Second).SendForReply(ctrlCh)
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(edge_ctrl_pb.ContentType_ErrorType), reply.ContentType, "expected an error reply, got content type %v", reply.ContentType)

	code, found := reply.GetUint32Header(edge.ErrorCodeHeader)
	ctx.Req.True(found, "error replies carry an edge error code")
	ctx.Req.Equal(uint32(edge.ErrorCodeInvalidService), code)
}

// Test_TunnelerCircuitDeniedForDisabledIdentity verifies the controller rejects a tunnel circuit
// request from a router whose identity is disabled, even though its policies still grant access.
func Test_TunnelerCircuitDeniedForDisabledIdentity(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	svcRole := eid.New()
	hostRole := eid.New()

	service := ctx.AdminManagementSession.requireNewService(s(svcRole), nil)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#"+svcRole), s("#"+hostRole), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+svcRole), s("#all"), nil)

	edgeRouter := ctx.CreateEnrollAndStartTunnelerEdgeRouter()

	watcher := ctx.AdminManagementSession.newTerminatorWatcher()
	defer watcher.Close()

	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext(hostRole)
	defer hostContext.Close()

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	watcher.waitForTerminators(service.Id, 1, 10*time.Second)

	ctrlCh := edgeRouter.GetNetworkControllers().AnyCtrlChannel()
	ctx.Req.NotNil(ctrlCh)

	sendDial := func() (*channel.Message, error) {
		request := &edge_ctrl_pb.CreateTunnelCircuitV2Request{ServiceName: service.Name}
		return protobufs.MarshalTyped(request).WithTimeout(5 * time.Second).SendForReply(ctrlCh)
	}

	// disable the router identity; its policies are unchanged
	disable := &rest_model.DisableParams{DurationMinutes: ToPtr[int64](0)}
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(disable).Post("identities/" + edgeRouter.GetRouterId().Token + "/disable")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

	// the controller must deny the dial with an access-denied error: the disabled-identity check in
	// shared identity loading runs before the dial-policy check, so this holds regardless of the
	// dialable denorm state. The router's control channel stays up (it authenticates by certificate),
	// so poll until the disable has been applied to the identity store the handler reads.
	ctx.Req.Eventually(func() bool {
		reply, err := sendDial()
		if err != nil {
			return false
		}
		if reply.ContentType != int32(edge_ctrl_pb.ContentType_ErrorType) {
			return false
		}
		code, found := reply.GetUint32Header(edge.ErrorCodeHeader)
		return found && code == edge.ErrorCodeAccessDenied
	}, 10*time.Second, 200*time.Millisecond, "controller should deny dials for a disabled router identity")
}

// newDialRefused reports whether a fresh TCP dial to addr is refused, either because the connect
// fails (the intercept was removed) or the router closes the connection without answering (the dial
// was denied before a circuit formed). A read timeout means the connection is alive and hanging,
// which is not a refusal, so it returns false and lets the caller keep polling.
func newDialRefused(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return true
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = conn.Write([]byte(eid.New())); err != nil {
		return true
	}
	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}
	return true
}

// Test_TunnelerV1CircuitDeniedForDisabledIdentity verifies the controller rejects a legacy
// (CreateCircuitForService) tunnel circuit request from a router whose identity is disabled. The
// disabled check lives in shared identity loading, so it must apply to this older content type too.
func Test_TunnelerV1CircuitDeniedForDisabledIdentity(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	svcRole := eid.New()
	hostRole := eid.New()

	service := ctx.AdminManagementSession.requireNewService(s(svcRole), nil)

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#"+svcRole), s("#"+hostRole), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+svcRole), s("#all"), nil)

	edgeRouter := ctx.CreateEnrollAndStartTunnelerEdgeRouter()

	watcher := ctx.AdminManagementSession.newTerminatorWatcher()
	defer watcher.Close()

	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext(hostRole)
	defer hostContext.Close()

	listener, err := hostContext.Listen(service.Name)
	ctx.Req.NoError(err)
	defer func() { _ = listener.Close() }()

	watcher.waitForTerminators(service.Id, 1, 10*time.Second)

	ctrlCh := edgeRouter.GetNetworkControllers().AnyCtrlChannel()
	ctx.Req.NotNil(ctrlCh)

	sendV1Dial := func() (*channel.Message, error) {
		request := &edge_ctrl_pb.CreateCircuitForServiceRequest{ServiceName: service.Name}
		return protobufs.MarshalTyped(request).WithTimeout(5 * time.Second).SendForReply(ctrlCh)
	}

	// the legacy dial succeeds while the identity is enabled
	reply, err := sendV1Dial()
	ctx.Req.NoError(err)
	ctx.Req.Equal(int32(edge_ctrl_pb.ContentType_CreateCircuitForServiceResponseType), reply.ContentType, "legacy dial should succeed while enabled")

	disable := &rest_model.DisableParams{DurationMinutes: ToPtr[int64](0)}
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(disable).Post("identities/" + edgeRouter.GetRouterId().Token + "/disable")
	ctx.Req.NoError(err)
	ctx.Req.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))

	ctx.Req.Eventually(func() bool {
		reply, err := sendV1Dial()
		if err != nil || reply.ContentType != int32(edge_ctrl_pb.ContentType_ErrorType) {
			return false
		}
		code, found := reply.GetUint32Header(edge.ErrorCodeHeader)
		return found && code == edge.ErrorCodeAccessDenied
	}, 10*time.Second, 200*time.Millisecond, "controller should deny legacy dials for a disabled router identity")
}

// requireDialWithRetry dials addr until it connects or timeout elapses, failing the test on timeout.
func requireDialWithRetry(ctx *TestContext, addr string, timeout time.Duration) net.Conn {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			return conn
		}
		if time.Now().After(deadline) {
			ctx.Req.NoError(err, "timed out waiting to connect to %s", addr)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// requireTcpEcho sends a unique token over conn and requires the echo server's greeting in reply.
func requireTcpEcho(ctx *TestContext, conn net.Conn) {
	name := eid.New()
	ctx.Req.NoError(conn.SetDeadline(time.Now().Add(5 * time.Second)))
	_, err := conn.Write([]byte(name))
	ctx.Req.NoError(err)

	expected := "hello, " + name
	buf := make([]byte, len(expected))
	_, err = io.ReadFull(conn, buf)
	ctx.Req.NoError(err)
	ctx.Req.Equal(expected, string(buf))
	ctx.Req.NoError(conn.SetDeadline(time.Time{}))
}

// requireNetConnClosed requires conn to be closed by its peer within timeout. Unsolicited data is
// tolerated, since only a non-timeout read error indicates closure.
func requireNetConnClosed(ctx *TestContext, conn net.Conn, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 64)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		_, err := conn.Read(buf)
		if err == nil {
			continue
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			continue
		}
		return
	}
	ctx.Req.Fail("timed out waiting for connection to be closed")
}
