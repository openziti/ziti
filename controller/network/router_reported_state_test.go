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

package network

import (
	"sync/atomic"
	"testing"

	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/apierror"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/stretchr/testify/require"
)

// reportedStateRefusingLimiter refuses the first refusals writes it runs as rate limited and runs the rest.
type reportedStateRefusingLimiter struct {
	refusals int32
	attempts atomic.Int32
}

func (self *reportedStateRefusingLimiter) RunRateLimited(f func() error) error {
	if self.attempts.Add(1) <= self.refusals {
		return apierror.NewTooManyUpdatesError()
	}
	return f()
}

func (self *reportedStateRefusingLimiter) GetQueueFillPct() float64 {
	return 0
}

func newReportedStateTestNetwork(t *testing.T) (*model.TestContext, *Network) {
	mctx, network, _ := newConnectTestNetwork(t)
	return mctx, network
}

func newReportedStateEdgeRouter(t *testing.T, network *Network) string {
	t.Helper()
	edgeRouter := &model.EdgeRouter{Name: eid.New()}
	require.NoError(t, network.EdgeRouter.Create(edgeRouter, change.New()))
	return edgeRouter.Id
}

// connectReportingRouter registers a connection for routerId that reports version and caps, as the accept
// path would from the connection's headers.
func connectReportingRouter(network *Network, routerId, version string, caps ...capabilities.RouterCapability) {
	network.Router.MarkConnected(&model.Router{
		BaseEntity:   models.BaseEntity{Id: routerId},
		VersionInfo:  &versions.VersionInfo{Version: version},
		Capabilities: capabilities.NewMask(caps...),
	})
}

func receiveReportingCanary(network *Network, routerId, version string, caps ...capabilities.RouterCapability) {
	value := &CanaryValue{Seq: 1, Version: version, Capabilities: capabilities.NewMask(caps...).Bytes()}
	network.canaryListener.EntryChanged(routerId, value, 1, routerId, false, 0)
}

func requireStoredReport(t *testing.T, network *Network, routerId, version string, caps ...capabilities.RouterCapability) {
	t.Helper()
	edgeRouter, err := network.EdgeRouter.Read(routerId)
	require.NoError(t, err)
	require.Equal(t, version, edgeRouter.Version)
	require.Equal(t, persistedCapabilitiesMask(routerId, capabilities.NewMask(caps...)), edgeRouter.CapabilitiesMask)
}

// Test_routerReportedStates_olderRouterIsJudgedByItsConnection covers a router that has gone back to a version
// without link gossip. The canary its gossiping lifetime left behind stays in the store, since canaries have no
// tombstones, so the router's connection to this controller has to win over it.
func Test_routerReportedStates_olderRouterIsJudgedByItsConnection(t *testing.T) {
	_, network := newReportedStateTestNetwork(t)
	routerId := newReportedStateEdgeRouter(t, network)

	receiveReportingCanary(network, routerId, "v2.0.0", capabilities.RouterLinkGossip)
	connectReportingRouter(network, routerId, "v1.6.0", capabilities.RouterMultiChannel)

	network.routerReportedStates.pass()

	requireStoredReport(t, network, routerId, "v1.6.0", capabilities.RouterMultiChannel)
}

// Test_routerReportedStates_gossipingRouterIsJudgedByItsCanary covers a gossiping router whose connection to
// this controller disagrees with its canary, as a connection from a lifetime that has ended does until this
// controller notices it is gone. The canary is ordered by the router itself, so it wins.
func Test_routerReportedStates_gossipingRouterIsJudgedByItsCanary(t *testing.T) {
	_, network := newReportedStateTestNetwork(t)
	routerId := newReportedStateEdgeRouter(t, network)

	connectReportingRouter(network, routerId, "v2.0.0", capabilities.RouterLinkGossip)
	receiveReportingCanary(network, routerId, "v2.1.0", capabilities.RouterLinkGossip, capabilities.RouterMultiChannel)

	network.routerReportedStates.pass()

	requireStoredReport(t, network, routerId, "v2.1.0", capabilities.RouterLinkGossip, capabilities.RouterMultiChannel)
}

// Test_routerReportedStates_routerNotConnectedToLeaderIsJudgedByItsCanary covers a gossiping router that only
// reaches other controllers. Its canary still reaches this one through gossip.
func Test_routerReportedStates_routerNotConnectedToLeaderIsJudgedByItsCanary(t *testing.T) {
	_, network := newReportedStateTestNetwork(t)
	routerId := newReportedStateEdgeRouter(t, network)

	receiveReportingCanary(network, routerId, "v2.1.0", capabilities.RouterLinkGossip)

	network.routerReportedStates.pass()

	requireStoredReport(t, network, routerId, "v2.1.0", capabilities.RouterLinkGossip)
}

// Test_routerReportedStates_connectionsAreUsedWhileCanariesAreUnusable covers a cluster with a member that
// cannot carry gossip, so even a gossiping router is judged by its connection. A controller still takes
// canaries from the routers connected to it while the type is unusable, but they are not shared, so the leader
// cannot rely on holding one for every router.
func Test_routerReportedStates_connectionsAreUsedWhileCanariesAreUnusable(t *testing.T) {
	_, network := newReportedStateTestNetwork(t)
	routerId := newReportedStateEdgeRouter(t, network)

	network.GossipStore.SetClusterCapabilities(capabilities.NewMask[capabilities.ControllerCapability]())
	connectReportingRouter(network, routerId, "v2.0.0", capabilities.RouterLinkGossip)
	receiveReportingCanary(network, routerId, "v2.1.0", capabilities.RouterLinkGossip)

	network.routerReportedStates.pass()

	requireStoredReport(t, network, routerId, "v2.0.0", capabilities.RouterLinkGossip)
}

// Test_routerReportedStates_onlyCaughtUpLeaderWrites covers a follower, which writes nothing, becoming leader,
// which re-checks every router whether or not it was marked.
func Test_routerReportedStates_onlyCaughtUpLeaderWrites(t *testing.T) {
	_, network := newReportedStateTestNetwork(t)
	routerId := newReportedStateEdgeRouter(t, network)
	states := network.routerReportedStates

	leader := false
	states.isLeader = func() bool { return leader }

	connectReportingRouter(network, routerId, "v1.6.0")
	states.mark(routerId)
	states.pass()
	requireStoredReport(t, network, routerId, "")

	states.takePending()
	leader = true
	states.pass()
	requireStoredReport(t, network, routerId, "v1.6.0")
}

func Test_routerReportedStates_retriesRefusedWrite(t *testing.T) {
	mctx, network := newReportedStateTestNetwork(t)
	routerId := newReportedStateEdgeRouter(t, network)
	limiter := &reportedStateRefusingLimiter{refusals: 1}
	mctx.GetCommandDispatcher().(*command.LocalDispatcher).Limiter = limiter

	connectReportingRouter(network, routerId, "v1.6.0")

	network.routerReportedStates.pass()
	requireStoredReport(t, network, routerId, "")

	network.routerReportedStates.pass()
	require.Equal(t, int32(2), limiter.attempts.Load())
	requireStoredReport(t, network, routerId, "v1.6.0")
}

// Test_routerReportedStates_skipsRouterThatIsNotAnEdgeRouter covers a router with no edge router entity to
// write to. It is not an error, so it is not retried.
func Test_routerReportedStates_skipsRouterThatIsNotAnEdgeRouter(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)
	router := newPersistedRouter(t, network, addr, "r1")

	connectReportingRouter(network, router.Id, "v1.6.0")

	network.routerReportedStates.pass()
	require.Empty(t, network.routerReportedStates.takePending())
}

func Test_routerReportedStates_canaryMarksOnlyAChangedReport(t *testing.T) {
	_, network := newReportedStateTestNetwork(t)
	states := network.routerReportedStates

	first := &CanaryValue{Seq: 1, Version: "v2.0.0", Capabilities: capabilities.NewMask(capabilities.RouterLinkGossip).Bytes()}
	states.canaryChanged("r1", nil, first)
	require.Equal(t, []string{"r1"}, states.takePending(), "a router's first canary is a report")

	sameReport := &CanaryValue{Seq: 2, Version: "v2.0.0", Capabilities: first.Capabilities}
	states.canaryChanged("r1", first, sameReport)
	require.Empty(t, states.takePending(), "a canary reporting the same version and capabilities")

	newVersion := &CanaryValue{Seq: 3, Version: "v2.1.0", Capabilities: first.Capabilities}
	states.canaryChanged("r1", sameReport, newVersion)
	require.Equal(t, []string{"r1"}, states.takePending())

	newCapabilities := &CanaryValue{Seq: 4, Version: "v2.1.0", Capabilities: capabilities.NewMask(capabilities.RouterMultiChannel).Bytes()}
	states.canaryChanged("r1", newVersion, newCapabilities)
	require.Equal(t, []string{"r1"}, states.takePending())
}

// Test_routerReportedStates_passStopsOnLosingLeadership covers leadership lost partway through a pass. A write
// sent after that is forwarded to the new leader, which has a view of its own.
func Test_routerReportedStates_passStopsOnLosingLeadership(t *testing.T) {
	_, network := newReportedStateTestNetwork(t)
	first := newReportedStateEdgeRouter(t, network)
	second := newReportedStateEdgeRouter(t, network)
	states := network.routerReportedStates

	connectReportingRouter(network, first, "v1.6.0")
	connectReportingRouter(network, second, "v1.6.0")

	// Leader for the start of the pass and its first write, then not.
	checks := 0
	states.isLeader = func() bool {
		checks++
		return checks <= 2
	}
	states.pass()

	written := 0
	for _, routerId := range []string{first, second} {
		edgeRouter, err := network.EdgeRouter.Read(routerId)
		require.NoError(t, err)
		if edgeRouter.Version != "" {
			written++
		}
	}
	require.Equal(t, 1, written)
}
