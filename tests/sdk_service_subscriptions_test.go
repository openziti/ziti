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

// Test_SDK_ServiceSubscriptions verifies that an SDK connection receives service change push
// notifications from the edge router after subscribing with a full-snapshot request (index -1).
//
// Sub-tests cover the initial snapshot (service-added event on subscribe), incremental structural
// changes (service updated / access lost), and partial-loss (dial lost but bind retained).
func Test_SDK_ServiceSubscriptions(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	edgeRouter := ctx.CreateEnrollAndStartEdgeRouter()
	ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	clientRole := eid.New()
	serviceRole := eid.New()

	service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
	dialPolicy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+clientRole), nil)
	// Bind policy keeps the service accessible after the dial policy is removed (partial-loss test).
	ctx.AdminManagementSession.requireNewServicePolicy("Bind", s("#"+serviceRole), s("#"+clientRole), nil)

	clientIdentity, clientCtxIface := ctx.AdminManagementSession.RequireCreateSdkContext(clientRole)
	defer clientCtxIface.Close()

	clientCtx, ok := clientCtxIface.(*ziti.ContextImpl)
	ctx.Req.True(ok)

	err := clientCtx.Authenticate()
	ctx.Req.NoError(err)

	// Wait for the router's RDM to propagate the identity → service-policy relationships
	// before subscribing. Without this, the initial snapshot arrives empty because the
	// controller distributes the identity and its policy memberships asynchronously.
	ctx.Req.True(edgeRouter.WaitForIdentityWithServices(clientIdentity.Id, 10*time.Second),
		"timed out waiting for identity's service policies to appear in router RDM")

	serviceAdded := make(chan *rest_model.ServiceDetail, 4)
	serviceChanged := make(chan *rest_model.ServiceDetail, 4)
	serviceRemoved := make(chan *rest_model.ServiceDetail, 4)

	clientCtxIface.Events().AddServiceAddedListener(func(_ ziti.Context, svc *rest_model.ServiceDetail) {
		serviceAdded <- svc
	})
	clientCtxIface.Events().AddServiceChangedListener(func(_ ziti.Context, svc *rest_model.ServiceDetail) {
		serviceChanged <- svc
	})
	clientCtxIface.Events().AddServiceRemovedListener(func(_ ziti.Context, svc *rest_model.ServiceDetail) {
		serviceRemoved <- svc
	})

	t.Run("subscribe and receive initial snapshot", func(t *testing.T) {
		ctx.testContextChanged(t)

		err = clientCtx.SubscribeToServiceUpdatesFromRouter(-1)
		ctx.Req.NoError(err)

		select {
		case svc := <-serviceAdded:
			ctx.Req.Equal(service.Name, *svc.Name)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout: expected ServiceAdded event from router push snapshot")
		}
	})

	t.Run("removing dial policy fires service-changed (bind retained)", func(t *testing.T) {
		ctx.testContextChanged(t)

		// Remove the dial policy — access for a bind-only identity should become "updated" not "removed".
		ctx.AdminManagementSession.requireDeleteEntity(dialPolicy)

		select {
		case svc := <-serviceChanged:
			ctx.Req.Equal(service.Name, *svc.Name)
		case <-serviceRemoved:
			t.Fatal("expected service-changed (partial loss), got service-removed")
		case <-time.After(5 * time.Second):
			t.Fatal("timeout: expected ServiceChanged event after dial policy removal")
		}
	})

	t.Run("removing service fires service-removed", func(t *testing.T) {
		ctx.testContextChanged(t)

		ctx.AdminManagementSession.requireDeleteEntity(service)

		select {
		case svc := <-serviceRemoved:
			ctx.Req.Equal(service.Name, *svc.Name)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout: expected ServiceRemoved event after service deletion")
		}
	})
}
