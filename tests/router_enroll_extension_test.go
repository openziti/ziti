//go:build apitests
// +build apitests

/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/edge/edge_common"
	"github.com/openziti/edge/router/fabric"
	"github.com/openziti/edge/router/xgress_edge"
	"github.com/openziti/edge/router/xgress_edge_tunnel"
	"github.com/openziti/fabric/router"
	"github.com/openziti/fabric/router/xgress"
	"testing"
)

func Test_EdgeRouter_Enrollment_Extension(t *testing.T) {

	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	//ctx.CreateEnrollAndStartEdgeRouter()
	ctx.shutdownRouter()
	ctx.createAndEnrollEdgeRouter(false)

	configFile := EdgeRouterConfFile
	if ctx.edgeRouterEntity.isTunnelerEnabled {
		configFile = TunnelerEdgeRouterConfFile
	}
	config, err := router.LoadConfig(configFile)
	config.Ctrl.Options.PeekHandlers =
	ctx.Req.NoError(err)
	router := router.Create(config, NewVersionProviderTest())

	stateManager := fabric.NewStateManager()
	xgressEdgeFactory := xgress_edge.NewFactory(config, NewVersionProviderTest(), stateManager, router.MetricsRegistry())
	xgress.GlobalRegistry().Register(edge_common.EdgeBinding, xgressEdgeFactory)

	xgressEdgeTunnelFactory := xgress_edge_tunnel.NewFactory(config, stateManager)
	xgress.GlobalRegistry().Register(edge_common.TunnelBinding, xgressEdgeTunnelFactory)

	ctx.Req.NoError(router.RegisterXctrl(xgressEdgeFactory))
	ctx.Req.NoError(router.RegisterXctrl(xgressEdgeTunnelFactory))
	ctx.Req.NoError(router.Start())
}

func NewTestRouter(peekHandler []xgress.PeekHandler)
