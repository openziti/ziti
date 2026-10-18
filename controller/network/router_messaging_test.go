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
	ctx, network, addr := newConnectTestNetwork(t)

	// Telling a router its destination is gone takes evidence that the store holds what the cluster
	// committed; see TestShouldNotifyDestinationRemoved_NeedsProvedAbsence.
	network.Link.NoteUnjudgedDestination("never-existed")
	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())

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
func TestNotifyExistingLink_SweptDeletedDestNotifiesReporter(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

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

	_, recorded := network.Link.Get("l0")
	req.True(recorded, "admission records rather than judging, so the sweep can judge with evidence")

	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())
	network.sweepUnresolvedLinkDestinations()

	_, recorded = network.Link.Get("l0")
	req.False(recorded, "and the sweep removes a link to a router that does not exist")

	requireRemovalNotice(t, network, src.Id, "deleted-router",
		"sweeping the link must tell the reporting router the destination is gone")
}

// TestNotifyExistingLink_ReDialsOfADeletedDestinationSettle: a router that never learned its destination was
// deleted keeps re-dialing it and reports every new iteration, faster than a barrier round trip. Each report is
// something this controller cannot judge on sight, and with one clock across all of them no barrier ever
// covered the latest, so the sweep never ran and the notice that would stop the re-dials was never sent.
func TestNotifyExistingLink_ReDialsOfADeletedDestinationSettle(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	src := model.NewRouterForTest("legacy", "", addr, &fakeCtrlChannel{}, 0, false)
	src.VersionInfo = &versions.VersionInfo{Version: "v2.0.5"}
	req.NoError(network.Router.Create(src, change.New()))
	network.Router.MarkConnected(src)

	report := func(iteration uint32) *ctrl_pb.RouterLinks_RouterLink {
		return &ctrl_pb.RouterLinks_RouterLink{
			Id: "l0", DestRouterId: "deleted-router", LinkProtocol: "tls", Iteration: iteration,
		}
	}

	network.NotifyExistingLink(src, report(1))
	for iteration := uint32(2); iteration <= 4; iteration++ {
		network.requestReadBarrier()
		network.NotifyExistingLink(src, report(iteration))
	}

	network.sweepUnresolvedLinkDestinations()

	_, recorded := network.Link.Get("l0")
	req.False(recorded, "the destination was settled by the first barrier, whatever arrived after it")
	requireRemovalNotice(t, network, src.Id, "deleted-router",
		"and the dialer is told, which is what stops the re-dials")
}

// TestReconcileGossipLinksForRouter_RefusedDeletedDestNotifiesOwner covers the same correction on the gossip
// path. A gossiping router sends no link reports at all, so without this the notice could never reach the
// routers that now make up most of a fleet. Reconcile runs on every connect, which is what makes it repeat.
func TestReconcileGossipLinksForRouter_RefusedDeletedDestNotifiesOwner(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)
	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "deleted-router", Iteration: 1}))

	// Reconciliation is a first sight of the entry, whatever walk found it, so it holds rather than judges:
	// every barrier in hand was taken before it arrived.
	network.ReconcileGossipLinksForRouter(dialer)

	_, held := network.Link.Get("l0")
	req.False(held, "a link to a router that does not exist must not be recorded")
	req.Equal(1, network.GossipLinkSweepPendingCount(), "and must be held for the sweep rather than refused")
	requireNoRemovalNotice(t, network, dialer.Id, "deleted-router",
		"its owner must not be told on a barrier older than the entry")

	// The sweep reads it back, now with an arrival for a barrier to postdate.
	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())
	network.sweepGossipLinks()

	requireRemovalNotice(t, network, dialer.Id, "deleted-router",
		"refusing the gossip entry must tell its owner the destination is gone")
}

// TestSweepGossipLinks_AppliesEntriesHeldBackByCatchUp: a gossiped report is not held anywhere, so the
// store is what asks again. Without the sweep the link waits for its dialer to reconnect, which a gossiping
// router may not do for days.
func TestSweepGossipLinks_AppliesEntriesHeldBackByCatchUp(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)

	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "arrives-later", LinkProtocol: "tls", Iteration: 1}))

	_, found := network.Link.Get("l0")
	req.False(found, "the link must not be built while its destination cannot be judged")

	req.NotNil(newPersistedRouter(t, network, addr, "arrives-later"))

	network.sweepGossipLinks()

	_, found = network.Link.Get("l0")
	req.True(found, "the sweep must re-ask the store once this controller has caught up")
}

// TestSweepGossipLinks_KeepsAskingWhileThisControllerIsBehind: the sweep takes the pending set before it
// applies anything, so an entry it still cannot judge has to put itself back. Otherwise the first pass
// while behind is also the last one, and the entry is never looked at again.
func TestSweepGossipLinks_KeepsAskingWhileThisControllerIsBehind(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)

	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "arrives-later", LinkProtocol: "tls", Iteration: 1}))

	for i := 0; i < 3; i++ {
		network.sweepGossipLinks()
		_, found := network.Link.Get("l0")
		req.False(found, "nothing has changed yet, so the answer must not have")
	}

	req.NotNil(newPersistedRouter(t, network, addr, "arrives-later"))

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
	_, network, addr := newConnectTestNetwork(t)
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
	_, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)

	key := LinkGossipKey("l0", 1)
	req.NoError(network.LinkGossipType.Set(key, dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "arrives-later", LinkProtocol: "tls", Iteration: 1}))
	req.Equal(1, network.GossipLinkSweepPendingCount())

	req.NotNil(newPersistedRouter(t, network, addr, "arrives-later"))

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

	applied, _ := network.applyGossipLinkEntries(refs, linkEntryReconciled)

	req.Zero(applied)
	_, found = network.Link.Get("l0")
	req.False(found, "a walk must not rebuild a link whose entry was retired since it ran")
}

// TestSweepGossipLinks_SettlesAGenuinelyDeletedDestination is what bounds the sweep in place of an attempt
// count. Once this controller is current, the entry is refused rather than left pending, its owner is told
// the destination is gone, and the owner retires the entry out of the store.
func TestSweepGossipLinks_SettlesAGenuinelyDeletedDestination(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)

	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "deleted-router", LinkProtocol: "tls", Iteration: 1}))

	_, found := network.Link.Get("l0")
	req.False(found, "a link to a router that does not exist must not be recorded")
	req.Equal(1, network.GossipLinkSweepPendingCount(),
		"and must wait rather than be refused on a view that may be behind the cluster")

	// Refusing answers the owner with a removal notice, so it takes evidence that the store holds what the
	// cluster committed. Once there is some, the sweep settles the entry.
	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())
	network.sweepGossipLinks()

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

// requireNoRemovalNotice is the negative of requireRemovalNotice. Settled over a window rather than once,
// since the notice it is ruling out would be queued asynchronously.
func requireNoRemovalNotice(t *testing.T, network *Network, notifyRouterId, destRouterId, msg string) {
	t.Helper()
	require.Never(t, func() bool {
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
	}, 500*time.Millisecond, 25*time.Millisecond, msg)
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
	req.Equal(uint64(2), changes[0].StoreIndex,
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
	req.Equal(uint64(1), changes[0].StoreIndex,
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
	req.Equal(batches["r1"].Changes[0].StoreIndex, batches["r2"].Changes[0].StoreIndex,
		"both recipients must hear the same statement, computed once")
}

// TestNotifyExistingLink_BehindControllerRecordsRatherThanDropping is the failure a soak run reproduced: a
// follower that has not yet applied a router's creation cannot tell it from one that was deleted, and
// refusing the report loses the link for good, because the reporting router has already marked it notified
// and only re-announces on reconnect. One controller ended 125 links short of its peers that way.
func TestNotifyExistingLink_BehindControllerRecordsRatherThanDropping(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	src := newReportingRouter(t, network, addr, "reporter")

	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id:           "l0",
		DestRouterId: "not-applied-yet",
		LinkProtocol: "tls",
		DialAddress:  "tcp:localhost:1234",
		Iteration:    1,
	})

	link, recorded := network.Link.Get("l0")
	req.True(recorded, "the report has no other copy, so the link table has to keep it")
	req.Nil(link.GetDest(), "with its destination unresolved until this controller catches up")

	// Once the destination is applied and the controller has caught up, the sweep leaves it alone.
	dest := newPersistedRouter(t, network, addr, "not-applied-yet")
	req.NotNil(dest)
	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())

	network.sweepUnresolvedLinkDestinations()

	_, recorded = network.Link.Get("l0")
	req.True(recorded, "a destination that turned out to exist keeps its link")
}

// TestNotifyExistingLink_BehindControllerStaysQuiet guards the hazard the other way. The destination-removed
// notice tears links down at the router, so a controller that simply has not caught up must not send it. A
// leader can be behind too, in the window after it wins an election and before it finishes applying.
func TestNotifyExistingLink_BehindControllerStaysQuiet(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)
	src := newReportingRouter(t, network, addr, "reporter")

	require.False(t, network.shouldNotifyDestinationRemoved(src, "not-applied-yet", nil),
		"a controller that is still catching up must not announce a router as removed")
}

// TestSweepUnresolvedLinkDestinations_DropsAGenuinelyAbsentDestination: recording an unverified destination
// must not become keeping it for ever. Once the controller is current and the destination is still absent,
// the link goes, which is the behaviour the refusal existed for.
func TestSweepUnresolvedLinkDestinations_DropsAGenuinelyAbsentDestination(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	src := newReportingRouter(t, network, addr, "reporter")

	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id: "l0", DestRouterId: "never-existed", LinkProtocol: "tls", Iteration: 1,
	})
	_, recorded := network.Link.Get("l0")
	req.True(recorded)

	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())
	network.sweepUnresolvedLinkDestinations()

	_, recorded = network.Link.Get("l0")
	req.False(recorded, "a destination that is still absent once caught up is gone")
}

// TestSweepUnresolvedLinkDestinations_IsolatedControllerDropsNothing: with no leader this controller
// receives no commits, so its view is frozen rather than behind. Judging an absence then mistakes isolation
// for deletion, which is the failure recording an unverified destination exists to prevent.
func TestSweepUnresolvedLinkDestinations_IsolatedControllerDropsNothing(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	src := newReportingRouter(t, network, addr, "reporter")

	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id: "l0", DestRouterId: "unreachable-truth", LinkProtocol: "tls", Iteration: 1,
	})
	_, recorded := network.Link.Get("l0")
	req.True(recorded)

	ctx.RaftLeaderless = true
	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())
	for i := 0; i < 3; i++ {
		network.sweepUnresolvedLinkDestinations()
	}

	_, recorded = network.Link.Get("l0")
	req.True(recorded,
		"an isolated controller must not conclude anything about a destination it cannot hear about")
}

// TestRequestReadBarrier_OnlyWhileHoldingSomethingUnjudged: the round trip is the cost of judging a recorded
// absence, so it is paid only while there is one to judge, and the answer is what lets the sweep act.
func TestRequestReadBarrier_OnlyWhileHoldingSomethingUnjudged(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	network.requestReadBarrier()
	req.Zero(ctx.ReadBarrierRequests(), "nothing is held that needs judging")

	src := newReportingRouter(t, network, addr, "reporter")
	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id: "l0", DestRouterId: "never-existed", LinkProtocol: "tls", Iteration: 1,
	})

	ctx.RaftIndex = 100
	network.requestReadBarrier()
	req.Equal(1, ctx.ReadBarrierRequests())
	req.False(network.Link.NeedsReadBarrier(), "the answer covers what is held")

	network.requestReadBarrier()
	req.Equal(1, ctx.ReadBarrierRequests(), "so asking again would say nothing new")

	network.sweepUnresolvedLinkDestinations()
	_, recorded := network.Link.Get("l0")
	req.False(recorded, "and the sweep can judge once it has it")
}

// TestRequestReadBarrier_UnansweredJudgesNothing: a leader that cannot reach quorum cannot take a barrier and
// sends nothing. Without an answer there is no evidence, and nothing may be swept.
func TestRequestReadBarrier_UnansweredJudgesNothing(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	src := newReportingRouter(t, network, addr, "reporter")
	network.NotifyExistingLink(src, &ctrl_pb.RouterLinks_RouterLink{
		Id: "l0", DestRouterId: "never-existed", LinkProtocol: "tls", Iteration: 1,
	})

	ctx.ReadBarrierUnanswered = true
	network.requestReadBarrier()
	req.Equal(1, ctx.ReadBarrierRequests())
	req.True(network.Link.NeedsReadBarrier(), "an unanswered request is no evidence at all")

	network.sweepUnresolvedLinkDestinations()
	_, recorded := network.Link.Get("l0")
	req.True(recorded, "so the link stays until the question can be answered")
}

// TestSweepGossipLinks_UnprovenAbsenceStaysPending is why the gossip path waits for the same evidence the
// sweep does. A follower can have applied every commit it knows of while the leader has already committed
// the destination's create, and a gossip entry can arrive in that window. Refusing it there is final: only
// an unjudged entry is put back for the sweep, so applying the create afterwards never revisits the link,
// and the owner has been told its destination is gone.
func TestSweepGossipLinks_UnprovenAbsenceStaysPending(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)

	// No barrier has been taken, so this controller cannot yet say the destination is absent.
	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "arrives-later", LinkProtocol: "tls", Iteration: 1}))

	req.Equal(1, network.GossipLinkSweepPendingCount(), "the entry must be held for the sweep, not refused")
	requireNoRemovalNotice(t, network, dialer.Id, "arrives-later",
		"and its owner must not be told a destination is gone on a view that may be behind")

	// The create this controller was behind on lands, and the sweep builds the link from the entry it kept.
	req.NotNil(newPersistedRouter(t, network, addr, "arrives-later"))
	network.sweepGossipLinks()

	_, found := network.Link.Get("l0")
	req.True(found, "the entry must still be there to build from once the destination is known")
}

// TestApplyGossipLinkEntry_IgnoresABarrierPredatingTheDestination: a barrier settles a destination only if it
// was requested after something named that destination. Judging on an older one reads a destination that
// simply has not reached this store as deleted, and the refusal is final, since only an unjudged entry is put
// back for the sweep.
func TestApplyGossipLinkEntry_IgnoresABarrierPredatingTheDestination(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)

	// A barrier requested before anything named the destination.
	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())

	// Now an entry arrives, after that barrier was taken.
	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), dialer.Id,
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "arrives-later", LinkProtocol: "tls", Iteration: 1}))

	req.Equal(1, network.GossipLinkSweepPendingCount(),
		"an entry the barrier predates must be held, not judged against it")
	requireNoRemovalNotice(t, network, dialer.Id, "arrives-later",
		"and its owner must not be told its destination is gone")

	// The create this controller was behind on lands, and the entry it kept builds the link.
	req.NotNil(newPersistedRouter(t, network, addr, "arrives-later"))
	network.sweepGossipLinks()

	_, found := network.Link.Get("l0")
	req.True(found, "the entry must still be there to build from")
}

// TestApplyGossipLinkEntry_ReconciledEntryGetsABarrier: an entry whose owner was unknown when it was
// delivered is refused then, so it is never held. The connect-time walk is its first sight, and it still
// has to note the destination, or nothing ever asks for the barrier that settles it and it stays pending for
// good.
func TestApplyGossipLinkEntry_ReconciledEntryGetsABarrier(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	// Delivered while its owner is unknown here, which is refused outright rather than held.
	req.NoError(network.LinkGossipType.Set(LinkGossipKey("l0", 1), "dialer",
		&ctrl_pb.RouterLinks_RouterLink{Id: "l0", DestRouterId: "deleted-router", LinkProtocol: "tls", Iteration: 1}))
	req.Zero(network.GossipLinkSweepPendingCount(), "an entry with no known owner is not held")
	req.False(network.Link.NeedsReadBarrier(), "so nothing has asked for a barrier yet")

	// The owner connects, and the walk picks up what delivery skipped.
	dialer := newReportingRouter(t, network, addr, "dialer", capabilities.RouterLinkGossip)
	network.ReconcileGossipLinksForRouter(dialer)

	req.Equal(1, network.GossipLinkSweepPendingCount(), "the walk must hold it")
	req.True(network.Link.NeedsReadBarrier(),
		"and must note the destination, or nothing ever asks for the barrier that settles it")
}

// TestShouldNotifyDestinationRemoved_NeedsProvedAbsence: the notice tears links down at the router, so it
// takes proof the destination is gone rather than a position in the cluster. Gating it on leadership
// stranded the correction whenever the dialer was connected only to a follower: that follower settles the
// entry, stops sweeping it, and nothing else is placed to tell the router. Being the caught-up leader is
// not proof either, since raft advances its applied index before the store has the entries.
func TestShouldNotifyDestinationRemoved_NeedsProvedAbsence(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	reporter := newReportingRouter(t, network, addr, "reporter")

	req.False(network.shouldNotifyDestinationRemoved(reporter, "never-existed", nil),
		"with nothing proving the store is current, a missing destination is not an answer")

	// A report naming it has been seen, and a barrier requested since.
	network.Link.NoteUnjudgedDestination("never-existed")
	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())

	req.True(network.shouldNotifyDestinationRemoved(reporter, "never-existed", nil),
		"with a barrier covering it, whoever holds the report may say the destination is gone")
}

// TestNotifyProvedDestinationRemoved_DoesNotAskAgain: once an absence is proved and acted on, the notice
// follows from the proof. Whether an absence can be judged rests on a record of when the destination was
// first named, which is forgotten some time after it is settled, so asking again can come back false, and the
// entry or link that would have raised the notice again is already gone.
func TestNotifyProvedDestinationRemoved_DoesNotAskAgain(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	reporter := newReportingRouter(t, network, addr, "reporter")
	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())

	// Nothing records the destination here, as after it has been forgotten.
	req.False(network.shouldNotifyDestinationRemoved(reporter, "never-existed", nil),
		"a caller that has not proved the absence is rightly turned away")

	network.notifyProvedDestinationRemoved(reporter, "never-existed")

	requireRemovalNotice(t, network, reporter.Id, "never-existed",
		"a caller that already proved it must still get the notice out")
	req.Zero(network.PendingRemovalNoticeCount())
}

// TestNotifyProvedDestinationRemoved_KeepsItWhenTheStoreCannotSay: the notice re-reads the destination
// before saying it is gone, and a read that fails says nothing either way. The caller has already let go of
// the entry or link that would raise it again, so the notice is kept and retried rather than dropped.
func TestNotifyProvedDestinationRemoved_KeepsItWhenTheStoreCannotSay(t *testing.T) {
	req := require.New(t)
	ctx, network, addr := newConnectTestNetwork(t)

	reporter := newReportingRouter(t, network, addr, "reporter")

	// Closing the store is the cheapest way to make a read fail rather than report a miss.
	req.NoError(ctx.GetDb().Close())

	network.notifyProvedDestinationRemoved(reporter, "never-existed")

	req.Equal(1, network.PendingRemovalNoticeCount(), "a notice that could not be decided must be kept")
}

// TestRetryRemovalNotices_SendsWhatWasKept: a kept notice goes out once the destination can be read.
func TestRetryRemovalNotices_SendsWhatWasKept(t *testing.T) {
	req := require.New(t)
	_, network, addr := newConnectTestNetwork(t)

	reporter := newReportingRouter(t, network, addr, "reporter")
	network.keepRemovalNoticePending(reporter.Id, "never-existed")

	network.retryRemovalNotices()

	requireRemovalNotice(t, network, reporter.Id, "never-existed", "the kept notice must be sent on retry")
	req.Zero(network.PendingRemovalNoticeCount(), "and no longer held once sent")
}

// TestRetryRemovalNotices_DropsADisconnectedRouter: a router no longer connected here is not this
// controller's to tell. It re-reports its links on reconnecting, which raises the question afresh.
func TestRetryRemovalNotices_DropsADisconnectedRouter(t *testing.T) {
	req := require.New(t)
	_, network, _ := newConnectTestNetwork(t)

	network.keepRemovalNoticePending("not-connected-here", "never-existed")
	network.retryRemovalNotices()

	req.Zero(network.PendingRemovalNoticeCount(), "a router that is not connected here must not be held for")
}
