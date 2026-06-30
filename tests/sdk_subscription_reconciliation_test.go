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

// Test_SDK_SubscriptionReconciliation locks in the dynamic push/poll behavior as the only
// service-subscription-capable router comes and goes:
//   - capable router online  -> SDK uses push (polling paused)
//   - capable router offline  -> SDK falls back to controller polling (so it does not go blind)
//   - capable router returns  -> SDK resubscribes and uses push again
//
// The capable/incapable distinction is not exercised here (all in-process test routers advertise
// the same capabilities); this test targets the harder transition: losing and regaining the push
// source. The push-vs-poll MODE is asserted via ContextImpl.IsPushSubscriptionActive so the test
// pins the transition itself, not just eventual consistency.
func Test_SDK_SubscriptionReconciliation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	clientRole := eid.New()
	serviceRole := eid.New()

	initialService := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
	ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+clientRole), nil)

	// Short refresh interval so the controller-polling fallback is observable quickly.
	identity := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false, clientRole)
	cfg := ctx.EnrollIdentity(identity.Id)
	opts := *ziti.DefaultOptions
	opts.RefreshInterval = time.Second
	opts.SessionRefreshInterval = time.Second

	clientCtxIface, err := ziti.NewContextWithOpts(cfg, &opts)
	ctx.Req.NoError(err)
	defer clientCtxIface.Close()

	clientCtx, ok := clientCtxIface.(*ziti.ContextImpl)
	ctx.Req.True(ok)
	ctx.Req.NoError(clientCtx.Authenticate())

	serviceAdded := make(chan *rest_model.ServiceDetail, 16)
	clientCtxIface.Events().AddServiceAddedListener(func(_ ziti.Context, svc *rest_model.ServiceDetail) {
		serviceAdded <- svc
	})

	waitForServiceAdded := func(name string, timeout time.Duration) bool {
		deadline := time.After(timeout)
		for {
			select {
			case svc := <-serviceAdded:
				if svc.Name != nil && *svc.Name == name {
					return true
				}
			case <-deadline:
				return false
			}
		}
	}

	// Wait for the RDM to propagate the identity's service-policy relationships before subscribing,
	// so the snapshot is not empty.
	er := &EdgeRouterHelper{Router: ctx.routers[0]}
	ctx.Req.True(er.WaitForIdentityWithServices(identity.Id, 10*time.Second),
		"identity service policies should reach the router RDM")

	t.Run("subscribe with a capable router uses push", func(t *testing.T) {
		ctx.testContextChanged(t)

		ctx.Req.NoError(clientCtx.SubscribeToServiceUpdatesFromRouter(-1))

		ctx.Req.True(waitForServiceAdded(initialService.Name, 10*time.Second),
			"initial service should arrive via the push snapshot")
		ctx.Req.True(clientCtx.IsPushSubscriptionActive(), "push should be active with a capable router connected")
	})

	t.Run("losing the only capable router falls back to polling", func(t *testing.T) {
		ctx.testContextChanged(t)

		ctx.shutdownRouters()

		ctx.Req.Eventually(func() bool {
			return !clientCtx.IsPushSubscriptionActive()
		}, 15*time.Second, 250*time.Millisecond, "push must deactivate once the last capable router is gone")

		// A service created while push is down must still be observed, via controller polling.
		pollService := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		ctx.Req.True(waitForServiceAdded(pollService.Name, 15*time.Second),
			"service created while offline should arrive via controller polling fallback")
	})

	t.Run("capable router returning resubscribes to push", func(t *testing.T) {
		ctx.testContextChanged(t)

		ctx.startEdgeRouter(nil)

		ctx.Req.Eventually(func() bool {
			return clientCtx.IsPushSubscriptionActive()
		}, 20*time.Second, 250*time.Millisecond, "push must reactivate once a capable router returns")

		pushService := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		ctx.Req.True(waitForServiceAdded(pushService.Name, 15*time.Second),
			"service created after the router returns should arrive via push")
	})
}
