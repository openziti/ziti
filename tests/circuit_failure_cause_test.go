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
	"encoding/json"
	"testing"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/pkg/errors"
)

// circuitFailureCollector captures circuit failed events from the event dispatcher.
type circuitFailureCollector struct {
	events chan *event.CircuitEvent
}

func newCircuitFailureCollector() *circuitFailureCollector {
	return &circuitFailureCollector{
		events: make(chan *event.CircuitEvent, 50),
	}
}

func (self *circuitFailureCollector) AcceptCircuitEvent(evt *event.CircuitEvent) {
	if evt.EventType == event.CircuitFailed {
		self.events <- evt
	}
}

// waitForFailedCircuitForService waits for a circuit failed event matching the given service ID.
// Events for other services are discarded.
func (self *circuitFailureCollector) waitForFailedCircuitForService(ctx *TestContext, serviceId string, timeout time.Duration) *event.CircuitEvent {
	deadline := time.After(timeout)
	for {
		select {
		case evt := <-self.events:
			if evt.ServiceId == serviceId {
				return evt
			}
		case <-deadline:
			ctx.Fail("timed out waiting for circuit failed event for service " + serviceId)
			return nil
		}
	}
}

func Test_CircuitFailureCauses(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	// blanket policies
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#all"), s("#all"), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#all"), s("#all"), nil)
	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	// register circuit failure event collector
	fc := newCircuitFailureCollector()
	dispatcher := ctx.fabricController.GetEventDispatcher()
	dispatcher.AddCircuitEventHandler(fc)
	defer dispatcher.RemoveCircuitEventHandler(fc)

	// start ER/T in host mode
	ctx.CreateEnrollAndStartTunnelerEdgeRouterWithCfgTweaks(func(cfg *routerEnv.Config) {
		for _, l := range cfg.Listeners {
			if l.Name == "tunnel" {
				if opts, ok := l.Options["options"].(map[interface{}]interface{}); ok {
					opts["mode"] = "host"
					delete(opts, "services")
				}
			}
		}
	})

	t.Run("rejected_by_application", func(t *testing.T) {
		ctx.testContextChanged(t)

		service := ctx.AdminManagementSession.testContext.newService(nil, nil)
		service.terminatorStrategy = xt_smartrouting.Name
		service.Id = ctx.AdminManagementSession.requireCreateEntity(service)

		_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext()
		defer hostContext.Close()

		terminatorWatcher := ctx.AdminManagementSession.newTerminatorWatcher(service.Id, 1)
		defer terminatorWatcher.Close()

		listener, err := hostContext.ListenWithOptions(service.Name, &ziti.ListenOptions{
			ManualStart: true,
		})
		ctx.Req.NoError(err)
		defer listener.Close()

		terminatorWatcher.waitForTerminators(15 * time.Second)

		// host goroutine: accept and reject
		go func() {
			conn, err := listener.AcceptEdge()
			if err != nil {
				pfxlog.Logger().WithError(err).Error("accept failed")
				return
			}
			conn.CompleteAcceptFailed(errors.New("rejected by application"))
			_ = conn.Close()
		}()

		_, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
		defer clientContext.Close()

		_, err = clientContext.Dial(service.Name)
		ctx.Req.Error(err)

		evt := fc.waitForFailedCircuitForService(ctx, service.Id, 10*time.Second)
		ctx.Req.NotNil(evt.FailureCause, "expected FailureCause to be set")
		ctx.Req.Equal(string(network.CircuitFailureRouterErrRejectedByApp), *evt.FailureCause)
	})

	t.Run("dns_resolution_failed", func(t *testing.T) {
		ctx.testContextChanged(t)

		hostConfig := ctx.newConfig("NH5p4FpGR", map[string]interface{}{
			"protocol": "tcp",
			"address":  "nonexistent.invalid.host.test",
			"port":     float64(8080),
		})
		ctx.AdminManagementSession.requireCreateEntity(hostConfig)

		service := ctx.AdminManagementSession.testContext.newService(nil, s(hostConfig.Id))
		service.terminatorStrategy = xt_smartrouting.Name
		ctx.AdminManagementSession.requireCreateEntity(service)

		terminatorWatcher := ctx.AdminManagementSession.newTerminatorWatcher(service.Id, 1)
		defer terminatorWatcher.Close()
		terminatorWatcher.waitForTerminators(30 * time.Second)

		_, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
		defer clientContext.Close()

		_, err := clientContext.Dial(service.Name)
		ctx.Req.Error(err)

		evt := fc.waitForFailedCircuitForService(ctx, service.Id, 10*time.Second)
		ctx.Req.NotNil(evt.FailureCause, "expected FailureCause to be set")
		ctx.Req.Equal(string(network.CircuitFailureRouterErrDnsResolutionFailed), *evt.FailureCause)
	})

	t.Run("connection_refused", func(t *testing.T) {
		ctx.testContextChanged(t)

		hostConfig := ctx.newConfig("NH5p4FpGR", map[string]interface{}{
			"protocol": "tcp",
			"address":  "127.0.0.1",
			"port":     float64(54321),
		})
		ctx.AdminManagementSession.requireCreateEntity(hostConfig)

		service := ctx.AdminManagementSession.testContext.newService(nil, s(hostConfig.Id))
		service.terminatorStrategy = xt_smartrouting.Name
		ctx.AdminManagementSession.requireCreateEntity(service)

		terminatorWatcher := ctx.AdminManagementSession.newTerminatorWatcher(service.Id, 1)
		defer terminatorWatcher.Close()
		terminatorWatcher.waitForTerminators(30 * time.Second)

		_, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
		defer clientContext.Close()

		_, err := clientContext.Dial(service.Name)
		ctx.Req.Error(err)

		evt := fc.waitForFailedCircuitForService(ctx, service.Id, 10*time.Second)
		ctx.Req.NotNil(evt.FailureCause, "expected FailureCause to be set")
		ctx.Req.Equal(string(network.CircuitFailureRouterErrDialConnRefused), *evt.FailureCause)
	})

	t.Run("port_not_allowed", func(t *testing.T) {
		ctx.testContextChanged(t)

		hostConfig := ctx.newConfig("NH5p4FpGR", map[string]interface{}{
			"forwardProtocol":  true,
			"allowedProtocols": []interface{}{"tcp"},
			"address":          "127.0.0.1",
			"forwardPort":      true,
			"allowedPortRanges": []interface{}{
				map[string]interface{}{"low": float64(8000), "high": float64(8100)},
			},
		})
		ctx.AdminManagementSession.requireCreateEntity(hostConfig)

		service := ctx.AdminManagementSession.testContext.newService(nil, s(hostConfig.Id))
		service.terminatorStrategy = xt_smartrouting.Name
		ctx.AdminManagementSession.requireCreateEntity(service)

		terminatorWatcher := ctx.AdminManagementSession.newTerminatorWatcher(service.Id, 1)
		defer terminatorWatcher.Close()
		terminatorWatcher.waitForTerminators(30 * time.Second)

		_, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
		defer clientContext.Close()

		appData, err := json.Marshal(map[string]interface{}{
			"dst_protocol": "tcp",
			"dst_ip":       "127.0.0.1",
			"dst_port":     "9999",
		})
		ctx.Req.NoError(err)

		_, err = clientContext.DialWithOptions(service.Name, &ziti.DialOptions{
			AppData: appData,
		})
		ctx.Req.Error(err)

		evt := fc.waitForFailedCircuitForService(ctx, service.Id, 10*time.Second)
		ctx.Req.NotNil(evt.FailureCause, "expected FailureCause to be set")
		ctx.Req.Equal(string(network.CircuitFailureRouterErrPortNotAllowed), *evt.FailureCause)
	})
}
