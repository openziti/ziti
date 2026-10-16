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
	"sync"
	"testing"
	"time"

	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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
// on whether the destination is gone. A router in link gossip learns of deletions from the router data
// model, but only where its controllers are new enough to distribute routers there, so it is told too.
func TestShouldNotifyDestinationRemoved_EveryReporterIsTold(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	for _, tc := range []struct {
		id   string
		caps []capabilities.RouterCapability
	}{
		{id: "gossiping", caps: []capabilities.RouterCapability{capabilities.RouterLinkGossip}},
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

// TestReconcileGossipLinksForRouter_RefusedDeletedDestNotifiesOwner covers the same correction on the gossip
// path. A gossiping router sends no link reports at all, so without this the notice could never reach the
// routers that now make up most of a fleet. Reconcile runs on every connect, which is what makes it repeat.
func TestReconcileGossipLinksForRouter_RefusedDeletedDestNotifiesOwner(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)
	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "deleted-router", Iteration: 1}))

	network.ReconcileGossipLinksForRouter(dialer)

	_, held := network.Link.Get("l0")
	req.False(held, "a link to a router that does not exist must not be recorded")

	requireRemovalNotice(t, network, dialer.Id, "deleted-router",
		"refusing the gossip entry must tell its owner the destination is gone")
}

// TestSweepGossipLinks_AppliesEntriesHeldBackByCatchUp: a gossiped report is not held anywhere, so the
// store is what asks again. Without the sweep the link waits for its dialer to reconnect, which a gossiping
// router may not do for days.
func TestSweepGossipLinks_AppliesEntriesHeldBackByCatchUp(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)
	ctx.RaftNotCaughtUp = true

	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "arrives-later", LinkProtocol: "tls", Iteration: 1}))

	_, found := network.Link.Get("l0")
	req.False(found, "the link must not be built while its destination cannot be judged")
	req.Zero(network.Link.DeferredReportCount(), "and a gossiped report must not be held a second time")

	req.NotNil(newPersistedRouter(t, network, addr, "arrives-later"))
	ctx.RaftNotCaughtUp = false

	network.sweepGossipLinks()

	_, found = network.Link.Get("l0")
	req.True(found, "the sweep must re-ask the store once this controller has caught up")
}

// TestSweepGossipLinks_KeepsAskingWhileThisControllerIsBehind: the sweep takes the pending set before it
// applies anything, so an entry it still cannot judge has to put itself back. Otherwise the first pass
// while behind is also the last one, and the entry is never looked at again.
func TestSweepGossipLinks_KeepsAskingWhileThisControllerIsBehind(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)
	ctx.RaftNotCaughtUp = true

	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "arrives-later", LinkProtocol: "tls", Iteration: 1}))

	for i := 0; i < 3; i++ {
		network.sweepGossipLinks()
		_, found := network.Link.Get("l0")
		req.False(found, "nothing has changed yet, so the answer must not have")
	}

	req.NotNil(newPersistedRouter(t, network, addr, "arrives-later"))
	ctx.RaftNotCaughtUp = false

	network.sweepGossipLinks()

	_, found := network.Link.Get("l0")
	req.True(found, "a sweep that cannot judge an entry must leave itself owed another")
	req.Zero(network.GossipLinkSweepPendingCount(), "and must stop asking once it has an answer")
}

// linkEventCapture records the link events a network under test dispatches. The test dispatcher discards
// them, so a test that is about what is announced has to stand in front of it.
type linkEventCapture struct {
	event.Dispatcher
	lock   sync.Mutex
	events []*event.LinkEvent
}

func (self *linkEventCapture) AcceptLinkEvent(evt *event.LinkEvent) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.events = append(self.events, evt)
}

// typesFor returns the event types announced for linkId, in order.
func (self *linkEventCapture) typesFor(linkId string) []event.LinkEventType {
	self.lock.Lock()
	defer self.lock.Unlock()
	var result []event.LinkEventType
	for _, evt := range self.events {
		if evt.LinkId == linkId {
			result = append(result, evt.EventType)
		}
	}
	return result
}

// TestReconcileGossipLinksForRouter_DoesNotReannounceLinksItFindsUnchanged: the connect-time walk asks
// about every entry the router published, and in the ordinary case every one of them is already current.
// Announcing those puts the router's whole set on the event stream on each connect, as news, when nothing
// has changed.
func TestReconcileGossipLinksForRouter_DoesNotReannounceLinksItFindsUnchanged(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)
	events := &linkEventCapture{Dispatcher: network.eventDispatcher}
	network.eventDispatcher = events

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)
	peer := newPersistedRouter(t, network, addr, "peer")
	req.NotNil(peer)

	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: peer.Id, LinkProtocol: "tls", Iteration: 1}))
	req.Equal([]event.LinkEventType{event.LinkFromRouterNew}, events.typesFor("l0"),
		"the entry arriving is news and must be announced")

	network.ReconcileGossipLinksForRouter(dialer)
	network.ReconcileGossipLinksForRouter(dialer)

	req.Equal([]event.LinkEventType{event.LinkFromRouterNew}, events.typesFor("l0"),
		"a walk that finds the link already current has nothing to announce")
}

// TestSweepGossipLinks_AsksOnlyAboutEntriesItCouldNotJudge: one unknown destination must not put the whole
// store through the sweep. The sweep runs every cycle for as long as catch-up lasts, and a full-store pass
// would re-apply every established link each time, at exactly the moment this controller is already behind.
func TestSweepGossipLinks_AsksOnlyAboutEntriesItCouldNotJudge(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)
	events := &linkEventCapture{Dispatcher: network.eventDispatcher}
	network.eventDispatcher = events

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)
	peer := newPersistedRouter(t, network, addr, "peer")
	req.NotNil(peer)

	req.NoError(network.LinkGossipType.Set(LinkGossipKey("established", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "established", DestRouterId: peer.Id, LinkProtocol: "tls", Iteration: 1}))
	link, found := network.Link.Get("established")
	req.True(found)

	// Taken out of the link manager but left in the store, so that a pass over the whole store would put it
	// back and a pass over the pending entries alone would not. The sweep is not a store-to-manager
	// reconciler: entries whose notification was missed are the connect-time walk's business, and doing
	// that work every cycle is the cost this is about.
	req.True(network.Link.Remove(link))

	ctx.RaftNotCaughtUp = true
	req.NoError(network.LinkGossipType.Set(LinkGossipKey("pending", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "pending", DestRouterId: "arrives-later", LinkProtocol: "tls", Iteration: 1}))

	req.Equal(1, network.GossipLinkSweepPendingCount(),
		"only the entry that could not be judged is owed another look")

	before := events.typesFor("established")
	network.sweepGossipLinks()

	_, found = network.Link.Get("established")
	req.False(found, "the sweep must ask only about the entries it could not judge")
	req.Equal(before, events.typesFor("established"), "and announce nothing for the rest")
	req.Equal(1, network.GossipLinkSweepPendingCount(), "the pending entry is still pending")
}

// TestSweepGossipLinks_AppliesUnderTheStoresDeliveryLock: the sweep reads an entry and acts on it, and the
// two have to be one step. If the store can deliver for that key in between, a tombstone's callback removes
// the link and the sweep then rebuilds it from a value the store has already retired.
func TestSweepGossipLinks_AppliesUnderTheStoresDeliveryLock(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)
	ctx.RaftNotCaughtUp = true

	key := LinkGossipKey("l0", 1)
	req.NoError(network.LinkGossipType.Set(key, dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "arrives-later", LinkProtocol: "tls", Iteration: 1}))
	req.Equal(1, network.GossipLinkSweepPendingCount())

	req.NotNil(newPersistedRouter(t, network, addr, "arrives-later"))
	ctx.RaftNotCaughtUp = false

	// Holding the key's delivery lock stands in for a removal being delivered for it: what matters is that
	// the sweep cannot read and apply across that, whichever of the two is inside.
	inside := make(chan struct{})
	release := make(chan struct{})
	held := make(chan struct{})
	go func() {
		defer close(held)
		network.LinkGossipType.WithEntry(dialer.Id, key, func(_ *ctrl_pb.RouterLinks_RouterLink, _ bool) {
			close(inside)
			<-release
		})
	}()
	<-inside

	swept := make(chan struct{})
	go func() {
		defer close(swept)
		network.sweepGossipLinks()
	}()

	require.Never(t, func() bool {
		_, found := network.Link.Get("l0")
		return found
	}, 100*time.Millisecond, 10*time.Millisecond,
		"the sweep must not apply an entry while the store's delivery for its key is held")

	close(release)
	<-held
	<-swept

	_, found := network.Link.Get("l0")
	req.True(found, "and must apply it once the key is free")
}

// TestApplyGossipLinkEntries_SkipsAnEntryRetiredSinceTheWalk: a store walk names entries rather than
// carrying what they said, because applying one reaches the link manager and so has to happen after the
// walk. In that gap the dialer can retire the link, and rebuilding it from what the walk saw would undo a
// tombstone that has already been applied.
func TestApplyGossipLinkEntries_SkipsAnEntryRetiredSinceTheWalk(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)
	peer := newPersistedRouter(t, network, addr, "peer")
	req.NotNil(peer)

	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: peer.Id, LinkProtocol: "tls", Iteration: 1}))

	refs := network.collectGossipLinkRefsForOwner(dialer.Id)
	req.Len(refs, 1)

	network.LinkGossipType.Delete(LinkGossipKey("l0", 1), dialer.Id)
	_, found := network.Link.Get("l0")
	req.False(found, "the tombstone must have removed the link before the walk's references are used")

	applied, _ := network.applyGossipLinkEntries(refs)

	req.Zero(applied)
	_, found = network.Link.Get("l0")
	req.False(found, "a walk must not rebuild a link whose entry was retired since it ran")
}

// TestSweepGossipLinks_SettlesAGenuinelyDeletedDestination is what bounds the sweep in place of an attempt
// count. Once this controller is current, the entry is refused rather than left pending, its owner is told
// the destination is gone, and the owner retires the entry out of the store.
func TestSweepGossipLinks_SettlesAGenuinelyDeletedDestination(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)

	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "deleted-router", LinkProtocol: "tls", Iteration: 1}))

	_, found := network.Link.Get("l0")
	req.False(found, "a link to a router that does not exist must not be recorded")
	req.Zero(network.GossipLinkSweepPendingCount(),
		"a refusal is an answer, so it must not leave the entry pending")

	requireRemovalNotice(t, network, dialer.Id, "deleted-router",
		"refusing the gossip entry must tell its owner the destination is gone")
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

// countingRaftIndex returns a GetRaftIndex that advances on every read, so a test can tell which of the
// two reads in computePeerState a change was stamped from.
func countingRaftIndex() func() uint64 {
	var n uint64
	return func() uint64 {
		n++
		return n
	}
}

// syncOnce drives one syncStates pass on a standalone RouterMessaging, so the assertions are not racing the
// Network's own event loop, and returns the batches that reached each router.
func syncOnce(t *testing.T, ctx *model.TestContext, network *Network, targets map[string]*fakeCtrlChannel, changed ...string) map[string]*ctrl_pb.PeerStateChanges {
	t.Helper()
	rm := NewRouterMessaging(ctx, network.RouterMessaging.routerCommPool)

	for routerId := range targets {
		updates := &routerUpdates{changedRouters: map[string]struct{}{}}
		for _, c := range changed {
			updates.changedRouters[c] = struct{}{}
		}
		rm.routerUpdates[routerId] = updates
	}

	rm.syncStates()

	result := map[string]*ctrl_pb.PeerStateChanges{}
	require.Eventually(t, func() bool {
		for routerId, ch := range targets {
			sent := ch.sentMessages()
			if len(sent) == 0 {
				return false
			}
			changes := &ctrl_pb.PeerStateChanges{}
			if err := proto.Unmarshal(sent[0].Body, changes); err != nil {
				return false
			}
			result[routerId] = changes
		}
		return true
	}, 2*time.Second, 10*time.Millisecond, "every target must receive its batch")

	return result
}

// connectTarget registers a router that exists and is connected, as one that has come through the connect
// path is, and returns the channel peer state arrives on.
func connectTarget(t *testing.T, network *Network, addr transport.Address, id string) *fakeCtrlChannel {
	t.Helper()
	require.NoError(t, network.Router.Create(model.NewRouterForTest(id, "", addr, nil, 0, false), change.New()))
	ch := &fakeCtrlChannel{}
	network.Router.MarkConnected(model.NewRouterForTest(id, "", addr, ch, 0, false))
	return ch
}

// TestSyncStates_RemovalIsStampedFromTheLaterRead: a removal has to be as strong as it can truthfully be, or
// another controller's stale healthy report outranks it and the router resurrects the destination. Reading
// the index after the existence check is what makes it so, and stays true because router ids are not reused.
func TestSyncStates_RemovalIsStampedFromTheLaterRead(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)
	ctx.RaftIndexF = countingRaftIndex()

	targets := map[string]*fakeCtrlChannel{"r1": connectTarget(t, network, addr, "r1")}
	batches := syncOnce(t, ctx, network, targets, "gone")

	changes := batches["r1"].Changes
	req.Len(changes, 1)
	req.Equal(ctrl_pb.PeerState_Removed, changes[0].State)
	req.Equal(uint64(2), changes[0].RaftIndex,
		"a removal must carry the index read after the existence check, not the one read before it")
}

// TestSyncStates_HealthyIsStampedFromTheEarlierRead is the other half of the bias. Existence is not
// permanent, so a report that a router is up must not claim to describe a store newer than the one it
// actually observed, or it outranks a removal that really is newer.
func TestSyncStates_HealthyIsStampedFromTheEarlierRead(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)
	ctx.RaftIndexF = countingRaftIndex()

	live := connectTarget(t, network, addr, "live")
	req.NotNil(live)

	targets := map[string]*fakeCtrlChannel{"r1": connectTarget(t, network, addr, "r1")}
	batches := syncOnce(t, ctx, network, targets, "live")

	changes := batches["r1"].Changes
	req.Len(changes, 1)
	req.Equal(ctrl_pb.PeerState_Healthy, changes[0].State)
	req.Equal(uint64(1), changes[0].RaftIndex,
		"a healthy report must carry the index read before the observation")
}

// TestSyncStates_DeletedRouterStillConnectedIsReportedRemoved: removal from the connected map waits on the
// control channel's close handler, so a deleted router lingers there. Settling existence from the store
// first is what stops it being reported healthy, and reporting healthy would be unfixable by ordering: the
// controller has applied the delete, so it would stamp that healthy state above the removal.
func TestSyncStates_DeletedRouterStillConnectedIsReportedRemoved(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	// In the connected map but absent from the store, which is a router deleted since it connected.
	network.Router.MarkConnected(model.NewRouterForTest("deleted", "", addr, &fakeCtrlChannel{}, 0, false))

	targets := map[string]*fakeCtrlChannel{"r1": connectTarget(t, network, addr, "r1")}
	batches := syncOnce(t, ctx, network, targets, "deleted")

	changes := batches["r1"].Changes
	req.Len(changes, 1)
	req.Equal(ctrl_pb.PeerState_Removed, changes[0].State,
		"a router the store no longer holds must be reported removed, whatever the connected map says")
}

// TestSyncStates_OneStatementPerRouterPerPass: the state for a router is settled once per pass and reused,
// so two routers being told about the same one cannot be told different things, and a delete landing partway
// through the pass cannot split the fleet's view.
func TestSyncStates_OneStatementPerRouterPerPass(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)
	ctx.RaftIndexF = countingRaftIndex()

	targets := map[string]*fakeCtrlChannel{
		"r1": connectTarget(t, network, addr, "r1"),
		"r2": connectTarget(t, network, addr, "r2"),
	}
	batches := syncOnce(t, ctx, network, targets, "gone")

	req.Len(batches["r1"].Changes, 1)
	req.Len(batches["r2"].Changes, 1)
	req.Equal(batches["r1"].Changes[0].RaftIndex, batches["r2"].Changes[0].RaftIndex,
		"both recipients must hear the same statement, computed once")
}

// TestNotifyExistingLink_BehindControllerDefersRatherThanDropping is the failure a soak run reproduced: a
// follower that has not yet applied a router's creation cannot tell it from one that was deleted, and
// refusing the report loses the link for good, because the reporting router has already marked it notified
// and only re-announces on reconnect. One controller ended 125 links short of its peers that way.
func TestNotifyExistingLink_BehindControllerDefersRatherThanDropping(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	src := newReportingRouter(t, network, addr, "reporter")
	ctx.RaftNotCaughtUp = true

	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id:           "l0",
		DestRouterId: "not-applied-yet",
		LinkProtocol: "tls",
		DialAddress:  "tcp:localhost:1234",
		Iteration:    1,
	})

	_, held := network.Link.Get("l0")
	req.False(held, "the link must not be recorded while the destination is unknown")
	req.Equal(1, network.Link.DeferredReportCount(), "but the report must be held, not discarded")

	// Once the destination is applied and the controller has caught up, the held report is applied.
	dest := newPersistedRouter(t, network, addr, "not-applied-yet")
	req.NotNil(dest)
	ctx.RaftNotCaughtUp = false

	network.Link.RetryDeferredReports()

	_, held = network.Link.Get("l0")
	req.True(held, "the link must be recorded once its destination is known")
	req.Zero(network.Link.DeferredReportCount())
}

// TestNotifyExistingLink_BehindControllerStaysQuiet guards the hazard the other way. The destination-removed
// notice tears links down at the router, so a controller that simply has not caught up must not send it. A
// leader can be behind too, in the window after it wins an election and before it finishes applying.
func TestNotifyExistingLink_BehindControllerStaysQuiet(t *testing.T) {
	ctx, network, addr := newConnectTestNetwork(t)
	src := newReportingRouter(t, network, addr, "reporter")

	ctx.RaftNotCaughtUp = true

	require.False(t, network.shouldNotifyDestinationRemoved(src, "not-applied-yet", nil),
		"a controller that is still catching up must not announce a router as removed")
}

// TestRetryDeferredReports_DropsAGenuinelyAbsentDestination: deferring must not become permanent holding. Once the
// controller is current and the destination is still absent after the retries, the report is dropped, which
// is the behaviour the refusal existed for.
func TestRetryDeferredReports_DropsAGenuinelyAbsentDestination(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	src := newReportingRouter(t, network, addr, "reporter")
	ctx.RaftNotCaughtUp = true

	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id: "l0", DestRouterId: "never-existed", LinkProtocol: "tls", Iteration: 1,
	})
	req.Equal(1, network.Link.DeferredReportCount())

	ctx.RaftNotCaughtUp = false
	for i := 0; i < 6; i++ {
		network.Link.RetryDeferredReports()
	}

	req.Zero(network.Link.DeferredReportCount(), "a destination that is still absent once caught up is gone")
	_, held := network.Link.Get("l0")
	req.False(held)
}

// TestRetryDeferredReports_AdvancesWhileTheClusterKeepsCommitting is the liveness property the per-pass
// barrier exists for. A busy cluster commits continuously, so "have I applied everything committed" can be
// false every time it is asked, and a retry gated on that would hold reports for ever. The barrier is taken
// once per pass instead, so it is a fixed target and is always reached.
func TestRetryDeferredReports_AdvancesWhileTheClusterKeepsCommitting(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	src := newReportingRouter(t, network, addr, "reporter")

	ctx.RaftNotCaughtUp = true
	ctx.RaftCommitIndex = 100
	ctx.RaftIndex = 50

	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id: "l0", DestRouterId: "arrives-later", LinkProtocol: "tls", Iteration: 1,
	})
	req.Equal(1, network.Link.DeferredReportCount())

	// The destination lands, and the cluster keeps committing, so this controller is still not caught up in
	// the global sense. The held report must be judged anyway.
	dest := newPersistedRouter(t, network, addr, "arrives-later")
	req.NotNil(dest)
	ctx.RaftIndex = 120
	ctx.RaftCommitIndex = 500

	network.Link.RetryDeferredReports()

	_, held := network.Link.Get("l0")
	req.True(held, "a pass past its barrier must run even while the cluster keeps committing")
	req.Zero(network.Link.DeferredReportCount())
}

// TestRetryDeferredReports_HoldsUntilTheBarrierIsReached: judging before the pass barrier would reach the
// same wrong answer that deferring exists to avoid.
func TestRetryDeferredReports_HoldsUntilTheBarrierIsReached(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	src := newReportingRouter(t, network, addr, "reporter")
	ctx.RaftNotCaughtUp = true
	ctx.RaftCommitIndex = 100
	ctx.RaftIndex = 50

	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id: "l0", DestRouterId: "never-existed", LinkProtocol: "tls", Iteration: 1,
	})

	// First pass runs against a zero barrier and burns one attempt, then sets the barrier to 100.
	network.Link.RetryDeferredReports()
	req.Equal(1, network.Link.DeferredReportCount())

	// Applied is still short of that barrier, so no further pass may consume an attempt or drop it.
	for i := 0; i < 10; i++ {
		network.Link.RetryDeferredReports()
	}
	req.Equal(1, network.Link.DeferredReportCount(), "a pass short of the barrier must leave reports alone")
}

// TestRetryDeferredReports_IsolatedControllerDropsNothing: with no leader this controller receives no
// commits, so its view is frozen rather than behind. Judging an absence then mistakes isolation for
// deletion, which is the failure deferring exists to prevent.
func TestRetryDeferredReports_IsolatedControllerDropsNothing(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	src := newReportingRouter(t, network, addr, "reporter")
	ctx.RaftNotCaughtUp = true

	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id: "l0", DestRouterId: "unreachable-truth", LinkProtocol: "tls", Iteration: 1,
	})
	req.Equal(1, network.Link.DeferredReportCount())

	ctx.RaftLeaderless = true
	for i := 0; i < 10; i++ {
		network.Link.RetryDeferredReports()
	}

	req.Equal(1, network.Link.DeferredReportCount(),
		"an isolated controller must not conclude anything about a destination it cannot hear about")
}
