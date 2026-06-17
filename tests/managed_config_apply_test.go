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
	"net"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/event"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/link"
	"github.com/openziti/ziti/v2/router/managedconfig"
)

// pickFreePort opens a TCP listener on an OS-assigned port, captures the
// port number, then closes the listener. The returned port is "probably"
// free for the test's brief window. Used to feed a deterministic port
// into a controller-pushed config (port 0 is not currently usable in the
// router.link.v1 transport.bind shape).
func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pickFreePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// createRouterLinkConfig pushes a router.link.v1 Config through the
// management API and returns the new config's ID.
func createRouterLinkConfig(t *testing.T, ctx *TestContext, data map[string]interface{}) string {
	t.Helper()
	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	configTypeName := "router.link.v1"
	resp, err := mgmtClient.API.Config.CreateConfig(&config.CreateConfigParams{
		Config: &rest_model.ConfigCreate{
			Name:         util.Ptr(eid.New()),
			ConfigTypeID: &configTypeName,
			Data:         data,
		},
	}, nil)
	ctx.Req.NoError(err)
	return resp.Payload.Data.ID
}

// assignConfigsToRouter PATCHes the edge router's Configs field.
func assignConfigsToRouter(t *testing.T, ctx *TestContext, routerId string, configIds []string) {
	t.Helper()
	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	_, err = mgmtClient.API.EdgeRouter.PatchEdgeRouter(&edge_router.PatchEdgeRouterParams{
		ID: routerId,
		EdgeRouter: &rest_model.EdgeRouterPatch{
			Configs: configIds,
		},
	}, nil)
	ctx.Req.NoError(err)
}

// portIsOpen returns true when a TCP connect to addr succeeds. Used to
// verify the router actually bound a listener for a controller-pushed
// link config.
func portIsOpen(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Test_ManagedConfig_ControllerAppliesListener pushes a router.link.v1 config
// containing a listener through the management API and verifies (1) the
// router applied the controller-source config (registry.Applied source =
// Controller) and (2) the listener's bind port is actually open.
func Test_ManagedConfig_ControllerAppliesListener(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	port := pickFreePort(t)
	bindAddr := fmt.Sprintf("tls:127.0.0.1:%d", port)

	r := ctx.CreateEnrollAndStartEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		cfg.ManagedConfig.Allow = []string{"router.link"}
	})
	registry := r.Router.GetRouterConfigRegistry()

	// Push a router.link.v1 config with a listener.
	configId := createRouterLinkConfig(t, ctx, map[string]interface{}{
		"listeners": []interface{}{
			map[string]interface{}{
				"binding": "transport",
				"bind":    bindAddr,
			},
		},
	})
	assignConfigsToRouter(t, ctx, ctx.edgeRouterEntity.id, []string{configId})

	// Wait for the controller config to apply on the router.
	ctx.Req.Eventually(func() bool {
		src, _, found := registry.Applied(link.ConfigTypeV1)
		return found && src == managedconfig.SourceController
	}, 10*time.Second, 50*time.Millisecond, "controller config should be applied (Applied source = Controller)")

	// Verify the listener is actually bound.
	ctx.Req.Eventually(func() bool {
		return portIsOpen(fmt.Sprintf("127.0.0.1:%d", port))
	}, 5*time.Second, 50*time.Millisecond, "listener port %d should be open", port)

	// Sanity: the router's linkSubsystem reports the listener.
	listeners := r.Router.GetXlinkListeners()
	ctx.Req.Len(listeners, 1, "router should have one active listener from controller config")
	ctx.Req.Equal(bindAddr, listeners[0].GetAdvertisement())
}

// Test_ManagedConfig_DelayedConfigStartsListener verifies that a router with
// no managed config applies *no* listeners until the controller pushes one.
// Tests the "the listener doesn't come up before the config arrives, and
// does come up after" promise.
func Test_ManagedConfig_DelayedConfigStartsListener(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	port := pickFreePort(t)
	bindAddr := fmt.Sprintf("tls:127.0.0.1:%d", port)

	r := ctx.CreateEnrollAndStartEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		cfg.ManagedConfig.Allow = []string{"router.link"}
	})
	registry := r.Router.GetRouterConfigRegistry()

	// Before any controller config arrives: no listeners, no Applied state,
	// port is closed.
	ctx.Req.Empty(r.Router.GetXlinkListeners(), "no listeners before any config arrives")
	_, _, found := registry.Applied(link.ConfigTypeV1)
	ctx.Req.False(found, "no applied state before any config arrives")
	ctx.Req.False(portIsOpen(fmt.Sprintf("127.0.0.1:%d", port)), "port should not be open before config arrives")

	// Push the config now.
	configId := createRouterLinkConfig(t, ctx, map[string]interface{}{
		"listeners": []interface{}{
			map[string]interface{}{
				"binding": "transport",
				"bind":    bindAddr,
			},
		},
	})
	assignConfigsToRouter(t, ctx, ctx.edgeRouterEntity.id, []string{configId})

	// After the controller config arrives, the listener comes up.
	ctx.Req.Eventually(func() bool {
		src, _, found := registry.Applied(link.ConfigTypeV1)
		return found && src == managedconfig.SourceController
	}, 10*time.Second, 50*time.Millisecond, "controller config should be applied")

	ctx.Req.Eventually(func() bool {
		return portIsOpen(fmt.Sprintf("127.0.0.1:%d", port))
	}, 5*time.Second, 50*time.Millisecond, "listener port should be open after config arrives")
}

// Test_ManagedConfig_ListenerUpdatePublishesToController verifies that
// when a router's link listener set changes mid-session (via managed
// config Apply), the router pushes UpdateLinkListeners to the controller
// and the controller's view of the router's listeners updates.
//
// Without this, peers would keep dialing the old listener address /
// using stale group memberships for link matching.
func Test_ManagedConfig_ListenerUpdatePublishesToController(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	port := pickFreePort(t)
	bindAddr := fmt.Sprintf("tls:127.0.0.1:%d", port)

	r := ctx.CreateEnrollAndStartEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		cfg.ManagedConfig.Allow = []string{"router.link"}
	})

	// Push a config with the listener in group "a".
	configId := createRouterLinkConfig(t, ctx, map[string]interface{}{
		"listeners": []interface{}{
			map[string]interface{}{"binding": "transport", "bind": bindAddr, "groups": []interface{}{"a"}},
		},
	})
	assignConfigsToRouter(t, ctx, ctx.edgeRouterEntity.id, []string{configId})

	// Wait for the listener to actually bind, then the controller's
	// view should reflect groups: [a].
	ctx.Req.Eventually(func() bool {
		return portIsOpen(fmt.Sprintf("127.0.0.1:%d", port))
	}, 10*time.Second, 50*time.Millisecond, "listener should bind from initial config")

	ctx.Req.Eventually(func() bool {
		ctrlRouter := ctx.fabricController.GetNetwork().Router.GetConnected(r.Router.GetRouterId().Token)
		if ctrlRouter == nil {
			return false
		}
		return len(ctrlRouter.Listeners) == 1 &&
			len(ctrlRouter.Listeners[0].Groups) == 1 &&
			ctrlRouter.Listeners[0].Groups[0] == "a"
	}, 10*time.Second, 50*time.Millisecond, "controller should see groups=[a] from Hello time")

	// Update the same config to change groups to ["a","b"].
	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)
	_, err = mgmtClient.API.Config.UpdateConfig(&config.UpdateConfigParams{
		ID: configId,
		Config: &rest_model.ConfigUpdate{
			Name: util.Ptr(eid.New()),
			Data: map[string]interface{}{
				"listeners": []interface{}{
					map[string]interface{}{"binding": "transport", "bind": bindAddr, "groups": []interface{}{"a", "b"}},
				},
			},
		},
	}, nil)
	ctx.Req.NoError(err)

	// The router applies the new config and pushes UpdateLinkListeners.
	// Controller's Router.Listeners should reflect the new groups.
	ctx.Req.Eventually(func() bool {
		ctrlRouter := ctx.fabricController.GetNetwork().Router.GetConnected(r.Router.GetRouterId().Token)
		if ctrlRouter == nil {
			return false
		}
		if len(ctrlRouter.Listeners) != 1 {
			return false
		}
		groups := ctrlRouter.Listeners[0].Groups
		return len(groups) == 2 && groups[0] == "a" && groups[1] == "b"
	}, 10*time.Second, 50*time.Millisecond, "controller should see updated groups=[a,b] after UpdateLinkListeners")
}

// Test_ManagedConfig_RemoveConfigShutsDownListener verifies that removing
// a controller-managed config from a router (by unassigning it) closes
// the listener it provisioned. Exercises the linkSubsystem.Remove path
// when the router-side subscriber sees a ConfigRemoved event because the
// per-router filter (Phase 2c) GC'd the config out of the router's view.
func Test_ManagedConfig_RemoveConfigShutsDownListener(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	port := pickFreePort(t)
	bindAddr := fmt.Sprintf("tls:127.0.0.1:%d", port)

	r := ctx.CreateEnrollAndStartEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		cfg.ManagedConfig.Allow = []string{"router.link"}
	})
	registry := r.Router.GetRouterConfigRegistry()

	configId := createRouterLinkConfig(t, ctx, map[string]interface{}{
		"listeners": []interface{}{
			map[string]interface{}{"binding": "transport", "bind": bindAddr},
		},
	})
	assignConfigsToRouter(t, ctx, ctx.edgeRouterEntity.id, []string{configId})

	// Wait for the listener to come up.
	ctx.Req.Eventually(func() bool {
		return portIsOpen(fmt.Sprintf("127.0.0.1:%d", port))
	}, 10*time.Second, 50*time.Millisecond, "listener should bind from controller config")

	// Unassign the config from the router. Phase 2c filtering + the
	// router-side RDM GC should drop the Config from rdm.Configs, which
	// dispatches OnRouterConfigRemoved through the subscriber, which
	// calls registry.RemoveController, which triggers the registry
	// reconcile path that calls handler.Remove().
	assignConfigsToRouter(t, ctx, ctx.edgeRouterEntity.id, []string{})

	// Applied state clears.
	ctx.Req.Eventually(func() bool {
		_, _, found := registry.Applied(link.ConfigTypeV1)
		return !found
	}, 10*time.Second, 50*time.Millisecond, "Applied should clear after config is removed")

	// Listener slice empties.
	ctx.Req.Eventually(func() bool {
		return len(r.Router.GetXlinkListeners()) == 0
	}, 5*time.Second, 50*time.Millisecond, "router should report no active listeners after removal")

	// Port no longer accepts connections.
	ctx.Req.Eventually(func() bool {
		return !portIsOpen(fmt.Sprintf("127.0.0.1:%d", port))
	}, 5*time.Second, 50*time.Millisecond, "listener port should close after config removal")
}

// Test_ManagedConfig_DefaultsBindingToTransport verifies that a listener
// entry with no `binding` field gets routed to the "transport" factory
// (the schema's documented default). Exercises link.defaultBinding.
func Test_ManagedConfig_DefaultsBindingToTransport(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	port := pickFreePort(t)
	bindAddr := fmt.Sprintf("tls:127.0.0.1:%d", port)

	r := ctx.CreateEnrollAndStartEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		cfg.ManagedConfig.Allow = []string{"router.link"}
	})

	// Note the omitted "binding" field — defaultBinding should fall back
	// to "transport".
	configId := createRouterLinkConfig(t, ctx, map[string]interface{}{
		"listeners": []interface{}{
			map[string]interface{}{"bind": bindAddr},
		},
	})
	assignConfigsToRouter(t, ctx, ctx.edgeRouterEntity.id, []string{configId})

	ctx.Req.Eventually(func() bool {
		return portIsOpen(fmt.Sprintf("127.0.0.1:%d", port))
	}, 10*time.Second, 50*time.Millisecond, "listener should open via the default transport factory")

	listeners := r.Router.GetXlinkListeners()
	ctx.Req.Len(listeners, 1)
	ctx.Req.Equal(bindAddr, listeners[0].GetAdvertisement())
}

// Test_ManagedConfig_UpdateRebindsListener verifies that updating a
// router.link.v1 config to a different bind address closes the original
// port and opens the new one. Tests the "rebuild on Apply" semantics in
// link.FactoryRegistry — the old listener slice is captured and closed
// after the new one is built.
func Test_ManagedConfig_UpdateRebindsListener(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	portA := pickFreePort(t)
	portB := pickFreePort(t)
	ctx.Req.NotEqual(portA, portB, "free-port picker handed out the same port twice")
	bindA := fmt.Sprintf("tls:127.0.0.1:%d", portA)
	bindB := fmt.Sprintf("tls:127.0.0.1:%d", portB)

	r := ctx.CreateEnrollAndStartEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		cfg.ManagedConfig.Allow = []string{"router.link"}
	})

	// Push config with port A → port A opens.
	configId := createRouterLinkConfig(t, ctx, map[string]interface{}{
		"listeners": []interface{}{
			map[string]interface{}{"binding": "transport", "bind": bindA},
		},
	})
	assignConfigsToRouter(t, ctx, ctx.edgeRouterEntity.id, []string{configId})

	ctx.Req.Eventually(func() bool {
		return portIsOpen(fmt.Sprintf("127.0.0.1:%d", portA))
	}, 10*time.Second, 50*time.Millisecond, "port A should open after first config")

	// Update config to port B.
	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)
	_, err = mgmtClient.API.Config.UpdateConfig(&config.UpdateConfigParams{
		ID: configId,
		Config: &rest_model.ConfigUpdate{
			Name: util.Ptr(eid.New()),
			Data: map[string]interface{}{
				"listeners": []interface{}{
					map[string]interface{}{"binding": "transport", "bind": bindB},
				},
			},
		},
	}, nil)
	ctx.Req.NoError(err)

	// Port B opens.
	ctx.Req.Eventually(func() bool {
		return portIsOpen(fmt.Sprintf("127.0.0.1:%d", portB))
	}, 10*time.Second, 50*time.Millisecond, "port B should open after rebind")

	// Port A closes (the old listener was Close()d during the rebuild).
	ctx.Req.Eventually(func() bool {
		return !portIsOpen(fmt.Sprintf("127.0.0.1:%d", portA))
	}, 5*time.Second, 50*time.Millisecond, "port A should close after rebind")

	// The router reports exactly the new listener.
	listeners := r.Router.GetXlinkListeners()
	ctx.Req.Len(listeners, 1)
	ctx.Req.Equal(bindB, listeners[0].GetAdvertisement())
}

// Test_ManagedConfig_BadUpdateRollsBack pushes a valid router.link.v1 config,
// waits for the listener to come up, then pushes a malformed update that
// fails handler.Apply. The registry must roll back to the previous-good
// state and emit an alert. The original listener stays bound throughout.
func Test_ManagedConfig_BadUpdateRollsBack(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	port := pickFreePort(t)
	bindAddr := fmt.Sprintf("tls:127.0.0.1:%d", port)

	r := ctx.CreateEnrollAndStartEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		cfg.ManagedConfig.Allow = []string{"router.link"}
	})
	registry := r.Router.GetRouterConfigRegistry()

	alertHandler := newRecordingAlertHandler(16)
	dispatcher := ctx.fabricController.GetNetwork().GetEventDispatcher()
	dispatcher.AddAlertEventHandler(alertHandler)
	defer dispatcher.RemoveAlertEventHandler(alertHandler)

	// Push good config → listener comes up.
	configId := createRouterLinkConfig(t, ctx, map[string]interface{}{
		"listeners": []interface{}{
			map[string]interface{}{
				"binding": "transport",
				"bind":    bindAddr,
			},
		},
	})
	assignConfigsToRouter(t, ctx, ctx.edgeRouterEntity.id, []string{configId})

	ctx.Req.Eventually(func() bool {
		return portIsOpen(fmt.Sprintf("127.0.0.1:%d", port))
	}, 10*time.Second, 50*time.Millisecond, "good config listener should bind")

	// Update the same Config to use an unknown binding. JSON parses fine
	// and the controller's schema accepts it (binding only requires
	// minLength: 1); handler.Apply fails when it can't find a factory.
	mgmtClient := ctx.NewEdgeManagementApi(nil)
	_, err := mgmtClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)
	_, err = mgmtClient.API.Config.UpdateConfig(&config.UpdateConfigParams{
		ID: configId,
		Config: &rest_model.ConfigUpdate{
			Name: util.Ptr(eid.New()),
			Data: map[string]interface{}{
				"listeners": []interface{}{
					map[string]interface{}{
						"binding": "made-up",
						"bind":    bindAddr,
					},
				},
			},
		},
	}, nil)
	ctx.Req.NoError(err)

	// An alert must fire for the failed apply.
	alert := alertHandler.waitFor(t, 10*time.Second, func(e *event.AlertEvent) bool {
		return e.RelatedEntities["configBaseType"] == "router.link"
	})
	ctx.Req.Equal("error", alert.Severity)

	// The original listener must still be bound after rollback — the
	// previous-good config's listener wasn't disturbed.
	ctx.Req.True(portIsOpen(fmt.Sprintf("127.0.0.1:%d", port)),
		"port should remain open after rollback to previous-good config")

	// Registry's Applied should reflect the rolled-back state: still
	// Controller v1 active.
	src, ver, found := registry.Applied(link.ConfigTypeV1)
	ctx.Req.True(found)
	ctx.Req.Equal(managedconfig.SourceController, src)
	ctx.Req.Equal(1, ver)
}
