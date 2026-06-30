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
	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/ziti/v2/common/eid"
)

// Test_SDK_ServiceConfigPush verifies that router-pushed service definitions carry the
// identity-resolved config bodies, and that config-related changes push updated bodies:
//   - on subscribe, a service that references a config carries that config's body; a service with
//     no config carries none
//   - adding the config to the second service pushes a service-changed with the body
//   - editing the config body pushes a service-changed to every service that references it
func Test_SDK_ServiceConfigPush(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	clientRole := eid.New()
	serviceRole := eid.New()

	configType := ctx.AdminManagementSession.requireCreateNewConfigType()
	config1 := ctx.AdminManagementSession.requireCreateNewConfig(configType.Id, map[string]interface{}{"hostname": "v1.example.com"})

	serviceA := ctx.AdminManagementSession.requireNewService(s(serviceRole), s(config1.Id))
	serviceB := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+clientRole), nil)

	identity, clientCtxIface := ctx.AdminManagementSession.RequireCreateSdkContext(clientRole)
	defer clientCtxIface.Close()

	clientCtx, ok := clientCtxIface.(*ziti.ContextImpl)
	ctx.Req.True(ok)
	ctx.Req.NoError(clientCtx.Authenticate())

	serviceAdded := make(chan *rest_model.ServiceDetail, 16)
	serviceChanged := make(chan *rest_model.ServiceDetail, 16)
	clientCtxIface.Events().AddServiceAddedListener(func(_ ziti.Context, svc *rest_model.ServiceDetail) {
		serviceAdded <- svc
	})
	clientCtxIface.Events().AddServiceChangedListener(func(_ ziti.Context, svc *rest_model.ServiceDetail) {
		serviceChanged <- svc
	})

	er := &EdgeRouterHelper{Router: ctx.routers[0]}
	ctx.Req.True(er.WaitForIdentityWithServices(identity.Id, 10*time.Second),
		"identity service policies should reach the router RDM")

	ctx.Req.NoError(clientCtx.SubscribeToServiceUpdatesFromRouter(-1))

	t.Run("snapshot carries the config body only for the referencing service", func(t *testing.T) {
		ctx.testContextChanged(t)

		// Both services arrive as added events in arbitrary order on one channel; collect both
		// rather than draining for one (which would discard the other).
		var a, b *rest_model.ServiceDetail
		deadline := time.After(10 * time.Second)
		for a == nil || b == nil {
			select {
			case svc := <-serviceAdded:
				if svc.Name == nil {
					continue
				}
				switch *svc.Name {
				case serviceA.Name:
					a = svc
				case serviceB.Name:
					b = svc
				}
			case <-deadline:
				ctx.Req.Fail("timed out waiting for both services via the push snapshot",
					"serviceA=%v serviceB=%v", a != nil, b != nil)
				return
			}
		}

		ctx.Req.Contains(a.Config, configType.Name, "serviceA references config1, so it should carry the body")
		ctx.Req.Equal("v1.example.com", configHostname(a, configType.Name))
		ctx.Req.NotContains(b.Config, configType.Name, "serviceB references no config")

		// Confirm the config is stored and retrievable through the SDK's own lookup, not just
		// present on the emitted event.
		storedA, found := clientCtx.GetService(serviceA.Name)
		ctx.Req.True(found, "serviceA should be retrievable via GetService")
		ctx.Req.Equal("v1.example.com", configHostname(storedA, configType.Name))
		storedB, found := clientCtx.GetService(serviceB.Name)
		ctx.Req.True(found, "serviceB should be retrievable via GetService")
		ctx.Req.NotContains(storedB.Config, configType.Name)

		t.Run("adding the config to serviceB pushes the body", func(t *testing.T) {
			ctx.testContextChanged(t)

			serviceB.configs = []string{config1.Id}
			ctx.AdminManagementSession.requireUpdateEntity(serviceB)

			changed := awaitService(serviceChanged, serviceB.Name, 10*time.Second)
			ctx.Req.NotNil(changed, "serviceB change should be pushed")
			ctx.Req.Equal("v1.example.com", configHostname(changed, configType.Name),
				"serviceB should now carry the config body")

			storedB, found := clientCtx.GetService(serviceB.Name)
			ctx.Req.True(found)
			ctx.Req.Equal("v1.example.com", configHostname(storedB, configType.Name),
				"GetService should return serviceB with the newly added config body")

			t.Run("editing the config pushes updates to every referencing service", func(t *testing.T) {
				ctx.testContextChanged(t)

				config1.Data = map[string]interface{}{"hostname": "v2.example.com"}
				ctx.AdminManagementSession.requirePatchEntity(config1, "data")

				seenA, seenB := false, false
				deadline := time.After(15 * time.Second)
				for !(seenA && seenB) {
					select {
					case svc := <-serviceChanged:
						if svc.Name == nil || configHostname(svc, configType.Name) != "v2.example.com" {
							continue
						}
						if *svc.Name == serviceA.Name {
							seenA = true
						}
						if *svc.Name == serviceB.Name {
							seenB = true
						}
					case <-deadline:
						ctx.Req.Fail("timed out waiting for both services to receive the updated config body",
							"seenA=%v seenB=%v", seenA, seenB)
						return
					}
				}

				for _, name := range []string{serviceA.Name, serviceB.Name} {
					stored, found := clientCtx.GetService(name)
					ctx.Req.True(found, "service %s should be retrievable via GetService", name)
					ctx.Req.Equal("v2.example.com", configHostname(stored, configType.Name),
						"GetService should return %s with the edited config body", name)
				}
			})
		})
	})
}

// awaitService drains a service-event channel until an event for the named service arrives, or
// returns nil on timeout.
func awaitService(ch chan *rest_model.ServiceDetail, name string, timeout time.Duration) *rest_model.ServiceDetail {
	deadline := time.After(timeout)
	for {
		select {
		case svc := <-ch:
			if svc.Name != nil && *svc.Name == name {
				return svc
			}
		case <-deadline:
			return nil
		}
	}
}

// configHostname returns the "hostname" string from a service's resolved config of the given type,
// or empty string if absent.
func configHostname(svc *rest_model.ServiceDetail, typeName string) string {
	cfg, ok := svc.Config[typeName]
	if !ok {
		return ""
	}
	hostname, _ := cfg["hostname"].(string)
	return hostname
}
