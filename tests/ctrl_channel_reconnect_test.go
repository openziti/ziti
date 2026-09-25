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
	"net"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	routerEnv "github.com/openziti/ziti/v2/router/env"
)

const (
	// reconnectHeartbeatCloseTimeout is short enough that a controller outage of a few seconds exceeds it,
	// so the router's heartbeat check acts during the test rather than after the default 30s.
	reconnectHeartbeatCloseTimeout = 2 * time.Second
	// reconnectWait allows for the dial policy's exponential backoff, which accrues failures while the
	// controller is down and can leave the next attempt several seconds after it returns.
	reconnectWait = 45 * time.Second
)

// fastHeartbeats configures control channel heartbeats fast enough for the heartbeat check to act within
// a test, and points the endpoints file into the test's temp dir, where nothing exists unless the test
// writes it.
func (ctx *TestContext) fastHeartbeats(cfg *routerEnv.Config) {
	cfg.Ctrl.EndpointsFile = ctx.T().TempDir() + "/endpoints.yml"
	cfg.Ctrl.Heartbeats.SendInterval = 500 * time.Millisecond
	cfg.Ctrl.Heartbeats.CheckInterval = 100 * time.Millisecond
	cfg.Ctrl.Heartbeats.CloseUnresponsiveTimeout = reconnectHeartbeatCloseTimeout
}

// startEdgeRouterWithFastHeartbeats starts the test edge router with no endpoints file, so the router
// knows the controller only by the endpoint in its config, as a router on a standalone network does.
func (ctx *TestContext) startEdgeRouterWithFastHeartbeats() *EdgeRouterHelper {
	return ctx.startEdgeRouter(ctx.fastHeartbeats)
}

// startEdgeRouterWithStaleRecordedController starts the test edge router with an endpoints file that
// records the controller's address under an id the controller does not have, which is what a file left
// behind by a rebuilt cluster or a move from HA to standalone looks like.
func (ctx *TestContext) startEdgeRouterWithStaleRecordedController() *EdgeRouterHelper {
	return ctx.startEdgeRouter(func(cfg *routerEnv.Config) {
		ctx.fastHeartbeats(cfg)
		ctx.Req.NoError(cfg.SaveControllerDetails([]*ctrl_pb.CtrlDetail{{
			Id:        "stale-controller-id",
			Endpoints: []*ctrl_pb.CtrlEndpoint{{Address: ctx.ControllerConfig.Ctrl.Listener.String()}},
		}}))
	})
}

// requireRouterConnected waits until the controller reports the test edge router's control channel as
// connected.
func (ctx *TestContext) requireRouterConnected(msg string) {
	ctx.Req.Eventually(func() bool {
		return ctx.fabricController.GetNetwork().GetConnectedRouter(ctx.edgeRouterEntity.id) != nil
	}, reconnectWait, 100*time.Millisecond, msg)
}

// waitForPortClosed blocks until nothing accepts connections on address, so a controller can be
// restarted on the same ports without racing the previous instance's listeners.
func (ctx *TestContext) waitForPortClosed(address string, timeout time.Duration) {
	ctx.Req.Eventually(func() bool {
		conn, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return false
		}
		return true
	}, timeout, 50*time.Millisecond, "port %s should be released", address)
}

// Test_RouterReconnectsAfterControllerOutageExceedingHeartbeatTimeout covers a standalone controller
// going away for longer than the router's heartbeat close timeout, which is what a controller upgrade
// looks like from the router. The router must be connected again once the controller is back. It fails
// when the heartbeat check closes the whole control channel instead of its underlays: the close handler
// then looks for a controller detail, a standalone controller never advertises one, and the router never
// dials again.
func Test_RouterReconnectsAfterControllerOutageExceedingHeartbeatTimeout(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.createAndEnrollEdgeRouter(false)
	ctx.startEdgeRouterWithFastHeartbeats()
	ctx.requireRouterConnected("router should connect to the controller")

	ctrlAddress := ctx.ControllerConfig.Ctrl.Listener.String()
	ctrlHostPort := ctrlAddress[len("tls:"):]

	// Take the controller down and hold it there past the heartbeat close timeout. Shutdown closes the
	// listeners but not the channels already accepted, and an in-process controller's goroutines would keep
	// answering heartbeats on one, which no controller that has exited can do. Close the router's channel
	// from the controller side, as the process exit would.
	connected := ctx.fabricController.GetNetwork().GetConnectedRouter(ctx.edgeRouterEntity.id)
	ctx.Req.NotNil(connected)
	ctx.Req.NoError(connected.Control.Close())
	ctx.EdgeController.Shutdown()
	ctx.EdgeController = nil
	ctx.fabricController.Shutdown()
	ctx.fabricController = nil
	ctx.waitForPortClosed(ctrlHostPort, 10*time.Second)
	ctx.waitForPortClosed(ctx.ApiHost, 10*time.Second)

	time.Sleep(reconnectHeartbeatCloseTimeout * 3)

	// same config, same database, so the router's enrollment is still valid
	ctx.StartServerFor("testdata/default.db", false)

	ctx.requireRouterConnected("router should reconnect once the controller is back")
}

// Test_RouterRedialsStandaloneControllerAfterCtrlChannelClose forces the control channel itself to close,
// the path the close handler takes when a channel is displaced or torn down, and requires the router to
// dial the controller again. It fails when the router has no controller detail to redial from: a
// standalone controller advertises none, so the detail has to be learned from the endpoint that was dialed.
func Test_RouterRedialsStandaloneControllerAfterCtrlChannelClose(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.createAndEnrollEdgeRouter(false)
	edgeRouter := ctx.startEdgeRouterWithFastHeartbeats()
	ctx.requireRouterConnected("router should connect to the controller")

	ctrls := edgeRouter.Router.GetNetworkControllers()
	registered := ctrls.GetAll()
	ctx.Req.Len(registered, 1, "the router should have exactly one controller registered")

	var ctrlId string
	var original routerEnv.NetworkController
	for id, ctrl := range registered {
		ctrlId = id
		original = ctrl
	}

	ctx.Req.NoError(original.Channel().Close())

	ctx.Req.Eventually(func() bool {
		current := ctrls.GetNetworkController(ctrlId)
		return current != nil && current != original && current.IsConnected()
	}, reconnectWait, 100*time.Millisecond, "router should register a new control channel to the controller")

	ctx.requireRouterConnected("controller should see the router connected again")
}

// Test_RouterRedialsStandaloneControllerRecordedUnderStaleId: the router dials from a recorded detail
// whose id is stale, registers the controller under the id it actually reports, and must be able to
// redial it after a close. A standalone controller sends no cluster update that would correct the
// recorded id, so the redial depends on the router having recorded the controller it reached.
func Test_RouterRedialsStandaloneControllerRecordedUnderStaleId(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	ctx.createAndEnrollEdgeRouter(false)
	edgeRouter := ctx.startEdgeRouterWithStaleRecordedController()
	ctx.requireRouterConnected("router should connect to the controller from the stale recorded detail")

	ctrls := edgeRouter.Router.GetNetworkControllers()
	registered := ctrls.GetAll()
	ctx.Req.Len(registered, 1, "the router should have exactly one controller registered")

	var ctrlId string
	var original routerEnv.NetworkController
	for id, ctrl := range registered {
		ctrlId = id
		original = ctrl
	}
	ctx.Req.NotEqual("stale-controller-id", ctrlId, "the controller is registered under the id it reports")

	ctx.Req.NoError(original.Channel().Close())

	ctx.Req.Eventually(func() bool {
		current := ctrls.GetNetworkController(ctrlId)
		return current != nil && current != original && current.IsConnected()
	}, reconnectWait, 100*time.Millisecond, "router should redial the controller it reached, not the id it recorded")

	ctx.requireRouterConnected("controller should see the router connected again")
}
