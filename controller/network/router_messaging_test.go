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
	"testing"
	"time"

	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

// TestSyncStates_DiscardsUpdatesForClosedChannel covers the retry behaviour for peer state changes bound
// for a router whose control channel has closed while it is still in the connected map. Sending on a
// closed channel fails immediately instead of blocking, and a failed send is retried as soon as the event
// loop turns, so retaining the updates spins the loop and floods the log. Reconnecting resyncs everything,
// so the pending changes are discarded instead. Updates for a router with a live channel must still be
// retained.
func TestSyncStates_DiscardsUpdatesForClosedChannel(t *testing.T) {
	ctx, network, addr := newConnectTestNetwork(t)

	// A standalone instance, so syncStates can be driven directly; the Network's own RouterMessaging runs
	// an event loop goroutine that would race these map accesses.
	rm := NewRouterMessaging(ctx, network.RouterMessaging.routerCommPool)

	deadCh := &fakeCtrlChannel{}
	dead := model.NewRouterForTest("r1", "", addr, deadCh, 0, false)
	network.Router.MarkConnected(dead)
	// The channel closes without any teardown running, so it stays in the connected map.
	deadCh.closed.Store(true)
	rm.routerUpdates["r1"] = &routerUpdates{changedRouters: map[string]struct{}{"other": {}}}

	// A live router with a send already in flight, which syncStates skips without queueing another.
	liveCh := &fakeCtrlChannel{}
	live := model.NewRouterForTest("r2", "", addr, liveCh, 0, false)
	network.Router.MarkConnected(live)
	liveUpdates := &routerUpdates{changedRouters: map[string]struct{}{"other": {}}}
	liveUpdates.sendInProgress.Store(true)
	rm.routerUpdates["r2"] = liveUpdates

	rm.syncStates()

	require.NotContains(t, rm.routerUpdates, "r1", "updates for a closed channel must be discarded, not retried")
	require.Contains(t, rm.routerUpdates, "r2", "updates for a live channel must be retained")
}

// TestRouterRemovalNotice_QueuesStateForOneRouter: the notice names its audience, unlike a routerChanged
// event, which fans out to everyone. syncStates then computes the state, so the message itself is whatever
// is true at send time.
func TestRouterRemovalNotice_QueuesStateForOneRouter(t *testing.T) {
	req := require.New(t)
	ctx, network, _ := newConnectTestNetwork(t)
	rm := NewRouterMessaging(ctx, network.RouterMessaging.routerCommPool)

	(&routerRemovalNoticeEvent{notifyRouterId: "r1", routerId: "gone"}).handle(rm)

	req.Contains(rm.routerUpdates, "r1")
	req.Contains(rm.routerUpdates["r1"].changedRouters, "gone")
	req.NotContains(rm.routerUpdates, "gone", "only the router that needs telling is queued")
}

// newReportingRouter builds a connected router, standing in for the one whose link report names a
// destination that is gone. caps is what it advertised on connect; none means it advertised nothing.
func newReportingRouter(t *testing.T, network *Network, addr transport.Address, id string, caps ...capabilities.RouterCapability) *model.Router {
	t.Helper()
	r := model.NewRouterForTest(id, "", addr, &fakeCtrlChannel{}, 0, false)
	if len(caps) > 0 {
		r.Capabilities = capabilities.NewMask(caps...)
	}
	network.Router.MarkConnected(r)
	return r
}

// TestShouldNotifyDestinationRemoved_EveryReporterIsTold: what the reporting router supports has no bearing
// on whether the destination is gone. Every router that names it is told, whatever it advertised.
func TestShouldNotifyDestinationRemoved_EveryReporterIsTold(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	for _, tc := range []struct {
		id   string
		caps []capabilities.RouterCapability
	}{
		{id: "other-caps", caps: []capabilities.RouterCapability{capabilities.RouterDataModel}},
		{id: "no-caps"},
	} {
		req.True(network.shouldNotifyDestinationRemoved(
			newReportingRouter(t, network, addr, tc.id, tc.caps...), "never-existed", nil),
			"router %v named a destination that does not exist and must be told it is gone", tc.id)
	}
}

// TestShouldNotifyDestinationRemoved_UnconnectedReporterIsSkipped: a gossip entry names its dialer whether or
// not that router is connected here, and a router this controller does not hold is one it cannot tell.
func TestShouldNotifyDestinationRemoved_UnconnectedReporterIsSkipped(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)
	src := newPersistedRouter(t, network, addr, "connected-elsewhere")

	require.False(t, network.shouldNotifyDestinationRemoved(src, "never-existed", nil))
}

// TestShouldNotifyDestinationRemoved_ExistingDestIsNotAnnounced guards the mirror-image failure: telling a
// router to drop a destination that is fine strands it just as permanently.
func TestShouldNotifyDestinationRemoved_ExistingDestIsNotAnnounced(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)
	src := newReportingRouter(t, network, addr, "old")
	live := newPersistedRouter(t, network, addr, "live")

	req.False(network.shouldNotifyDestinationRemoved(src, live.Id, nil),
		"a destination that exists but is not connected here must not be announced as removed")
}

// TestShouldNotifyDestinationRemoved_ConnectedDestIsNotAnnounced: a destination connected here is proof it
// exists, and costs no lookup.
func TestShouldNotifyDestinationRemoved_ConnectedDestIsNotAnnounced(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)
	src := newReportingRouter(t, network, addr, "old")
	dst := model.NewRouterForTest("dst", "", addr, &fakeCtrlChannel{}, 0, false)

	require.False(t, network.shouldNotifyDestinationRemoved(src, dst.Id, dst))
}

// TestShouldNotifyDestinationRemoved_NonLeaderStaysQuiet: the notice tears links down and is unsolicited,
// so it is the leader's to send. A controller behind on a create would otherwise announce a live router as
// removed, which nothing would correct.
func TestShouldNotifyDestinationRemoved_NonLeaderStaysQuiet(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)
	src := newReportingRouter(t, network, addr, "old")

	// Raft enabled and this controller not the leader.
	testConfigOf(t, network).raftEnabled = true

	require.False(t, network.shouldNotifyDestinationRemoved(src, "never-existed", nil))
}

// TestNotifyExistingLink_RefusedDeletedDestNotifiesReporter covers the wiring: a refused report has to
// actually produce the notice, since the refusal on its own leaves the router dialing forever.
func TestNotifyExistingLink_RefusedDeletedDestNotifiesReporter(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	src := model.NewRouterForTest("old", "", addr, &fakeCtrlChannel{}, 0, false)
	src.VersionInfo = &versions.VersionInfo{Version: "v1.6.20"}
	req.NoError(network.Router.Create(src, change.New()))
	network.Router.MarkConnected(src)

	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id:           "l0",
		DestRouterId: "deleted-router",
		LinkProtocol: "tls",
		DialAddress:  "tcp:localhost:1234",
		Iteration:    1,
	})

	_, held := network.Link.Get("l0")
	req.False(held, "a link to a router that does not exist must not be recorded")

	requireRemovalNotice(t, network, src.Id, "deleted-router",
		"refusing the report must tell the reporting router the destination is gone")
}

// requireRemovalNotice waits for a peer-state removal of destRouterId to be queued for notifyRouterId. The
// messaging event loop consumes the event, so the effect is read back through Inspect rather than off the
// queue.
func requireRemovalNotice(t *testing.T, network *Network, notifyRouterId, destRouterId, msg string) {
	t.Helper()
	require.Eventually(t, func() bool {
		state, err := network.RouterMessaging.Inspect()
		if err != nil {
			return false
		}
		for _, update := range state.RouterUpdates {
			if update.Router.Id != notifyRouterId {
				continue
			}
			for _, changed := range update.ChangedRouters {
				if changed.Id == destRouterId {
					return true
				}
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, msg)
}
