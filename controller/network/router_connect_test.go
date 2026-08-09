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
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/tcp"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/gossip"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/stretchr/testify/require"
)

// fakeCtrlChannel is a minimal ctrlchan.CtrlChannel double for connect/disconnect lifecycle tests.
// It embeds the interface (unimplemented methods panic if called), and implements only the close-related
// methods the reject/kick path exercises. onClose, if set, runs synchronously on the first Close to
// simulate the real channel close handler firing DisconnectRouter.
type fakeCtrlChannel struct {
	ctrlchan.CtrlChannel
	closed  atomic.Bool
	onClose func()
}

func (f *fakeCtrlChannel) Close() error {
	if f.closed.CompareAndSwap(false, true) && f.onClose != nil {
		f.onClose()
	}
	return nil
}

func (f *fakeCtrlChannel) IsClosed() bool { return f.closed.Load() }

func (f *fakeCtrlChannel) IsConnected() bool { return !f.closed.Load() }

func newConnectTestNetwork(t *testing.T) (*model.TestContext, *Network, transport.Address) {
	ctx := model.NewTestContext(t)
	t.Cleanup(ctx.Cleanup)

	config := newTestConfig(ctx)
	t.Cleanup(func() { close(config.closeNotify) })

	network, err := NewNetwork(config, ctx)
	require.NoError(t, err)

	addr, err := tcp.AddressParser{}.Parse("tcp:0.0.0.0:0")
	require.NoError(t, err)

	return ctx, network, addr
}

// TestConnectRouter_RejectsAndKicksWhenBusy: a connect into an occupied slot returns ErrConnectRejected,
// kicks the occupant (whose teardown clears the slot), and a subsequent redial then connects cleanly.
func TestConnectRouter_RejectsAndKicksWhenBusy(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	currentCh := &fakeCtrlChannel{}
	cur := model.NewRouterForTest("r1", "", addr, currentCh, 0, false)
	// Simulate the real close handler: closing the occupant runs its DisconnectRouter.
	currentCh.onClose = func() { network.DisconnectRouter(cur) }

	require.NoError(t, network.QueueRouterConnect(cur))
	require.Equal(t, cur, network.Router.GetConnected("r1"))

	newCh := &fakeCtrlChannel{}
	rNew := model.NewRouterForTest("r1", "", addr, newCh, 0, false)
	err := network.QueueRouterConnect(rNew)
	require.ErrorIs(t, err, ErrConnectRejected)
	require.True(t, IsConnectRejected(err))
	require.True(t, currentCh.IsClosed(), "occupant should have been kicked")
	require.Nil(t, network.Router.GetConnected("r1"), "kicked occupant's teardown should clear the slot")
	require.False(t, rNew.Connected.Load(), "rejected connect must not be registered")

	// Redial into the now-clear slot succeeds.
	rRedial := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.QueueRouterConnect(rRedial))
	require.Equal(t, rRedial, network.Router.GetConnected("r1"))
	require.True(t, rRedial.Connected.Load())
}

// TestConnectRouter_SetsUpWhenClear: a connect into an empty slot registers the router.
func TestConnectRouter_SetsUpWhenClear(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	r := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.QueueRouterConnect(r))
	require.Equal(t, r, network.Router.GetConnected("r1"))
	require.True(t, r.Connected.Load())
}

// TestDisconnectRouter_IgnoresStaleConnection: a disconnect for a superseded connection must not disturb
// the current one (the §9 race guard).
func TestDisconnectRouter_IgnoresStaleConnection(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	rNew := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.QueueRouterConnect(rNew))
	require.Equal(t, rNew, network.Router.GetConnected("r1"))

	// A stale disconnect for a different (old) instance of the same router id.
	rOld := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	network.DisconnectRouter(rOld)

	require.Equal(t, rNew, network.Router.GetConnected("r1"), "stale disconnect must not evict the current connection")
	require.True(t, rNew.Connected.Load())
}

// TestDisconnectRouter_CurrentTearsDown: a disconnect for the current connection clears it.
func TestDisconnectRouter_CurrentTearsDown(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	r := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.QueueRouterConnect(r))
	require.Equal(t, r, network.Router.GetConnected("r1"))

	network.DisconnectRouter(r)
	require.Nil(t, network.Router.GetConnected("r1"))
	require.False(t, r.Connected.Load())
}

// TestReject_DisplacesOccupantThatCannotTearItselfDown is the regression guard against a permanent
// reject loop. A connection whose channel closes without its close handler running can never remove
// itself from the connected map, so a reject that merely closed the channel would leave the slot occupied
// forever and every redial would be rejected against a slot nothing can free. The reject must displace
// the occupant itself, so the next redial finds the slot clear.
func TestReject_DisplacesOccupantThatCannotTearItselfDown(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	// No onClose: the occupant's channel goes closed without any teardown running, modeling both a
	// channel already closed before the reject and one whose close handler has already run.
	currentCh := &fakeCtrlChannel{}
	cur := model.NewRouterForTest("r1", "", addr, currentCh, 0, false)
	require.NoError(t, network.QueueRouterConnect(cur))
	require.Equal(t, cur, network.Router.GetConnected("r1"))
	currentCh.closed.Store(true)

	rNew := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.ErrorIs(t, network.QueueRouterConnect(rNew), ErrConnectRejected)
	require.Nil(t, network.Router.GetConnected("r1"), "reject must displace an occupant that cannot tear itself down")
	require.False(t, cur.Connected.Load())
	require.False(t, rNew.Connected.Load(), "rejected connect must not be registered")

	// The redial therefore makes progress instead of bouncing off the slot forever.
	rRedial := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.QueueRouterConnect(rRedial))
	require.Equal(t, rRedial, network.Router.GetConnected("r1"))
	require.True(t, rRedial.Connected.Load())
}

// TestConnectRouter_RefusesAlreadyClosedChannel: a connect whose channel is already closed must be
// refused rather than registered. Registering it would put a connection in the connected map that no
// disconnect can ever remove, since its close handler has already run, wedging the router out of this
// controller permanently.
func TestConnectRouter_RefusesAlreadyClosedChannel(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	deadCh := &fakeCtrlChannel{}
	deadCh.closed.Store(true)
	r := model.NewRouterForTest("r1", "", addr, deadCh, 0, false)

	err := network.QueueRouterConnect(r)
	require.ErrorIs(t, err, ErrConnectChannelClosed)
	require.True(t, IsConnectRejected(err), "an already-closed channel is a refusal the router redials after")
	require.Nil(t, network.Router.GetConnected("r1"), "a closed connection must not occupy the slot")
	require.False(t, r.Connected.Load())

	// A subsequent healthy connect is unaffected.
	rOk := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.QueueRouterConnect(rOk))
	require.Equal(t, rOk, network.Router.GetConnected("r1"))
}

// connectSetupSignal records which routers' deferred connect setup has finished. The presence callback runs
// at the end of that setup, so it is the signal a test can wait on instead of racing it. Counting callbacks
// is not enough here: several routers connect, and a count cannot say which one finished.
type connectSetupSignal struct{ done sync.Map }

func (self *connectSetupSignal) RouterConnected(r *model.Router)  { self.done.Store(r.Id, true) }
func (self *connectSetupSignal) RouterDisconnected(*model.Router) {}
func (self *connectSetupSignal) has(id string) bool               { _, ok := self.done.Load(id); return ok }

// countingPresenceHandler counts RouterConnected callbacks. It does not opt into synchronous invocation,
// so ConnectRouter is what calls it, which makes the count a witness for the deferred setup having run.
type countingPresenceHandler struct {
	connected atomic.Int32
}

func (self *countingPresenceHandler) RouterConnected(*model.Router)    { self.connected.Add(1) }
func (self *countingPresenceHandler) RouterDisconnected(*model.Router) {}

// TestQueueRouterConnect_RunsSetupAsynchronously: the connect decision and registration are synchronous,
// but the remaining setup is handed to a semaphore-bounded goroutine, so binding does not wait on it. The
// listener holds the channel group's create reservation until the bind returns, and the group's other
// underlays wait on that, which is why this work must not run inline.
func TestQueueRouterConnect_RunsSetupAsynchronously(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	handler := &countingPresenceHandler{}
	network.AddRouterPresenceHandler(handler)

	r := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.QueueRouterConnect(r))

	// Registration is synchronous, so it has already happened.
	require.Equal(t, r, network.Router.GetConnected("r1"))
	require.True(t, r.Connected.Load())

	// The deferred setup runs on its own goroutine.
	require.Eventually(t, func() bool { return handler.connected.Load() == 1 }, 5*time.Second, 5*time.Millisecond,
		"the deferred connect setup should run")
}

// TestQueueRouterConnect_ReleasesConnectSlots is the leak guard for the semaphore bounding connect setup.
// Every connect must hand its slot back: a slot that is not released is gone for the controller's lifetime,
// and once enough have leaked no router can complete connect setup at all. Reclaiming the full capacity
// afterwards is what proves none were kept.
func TestQueueRouterConnect_ReleasesConnectSlots(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	handler := &countingPresenceHandler{}
	network.AddRouterPresenceHandler(handler)

	const connects = 25
	for i := 0; i < connects; i++ {
		r := model.NewRouterForTest(fmt.Sprintf("r%d", i), "", addr, &fakeCtrlChannel{}, 0, false)
		require.NoError(t, network.QueueRouterConnect(r))
	}

	require.Eventually(t, func() bool { return handler.connected.Load() == connects }, 10*time.Second, 5*time.Millisecond,
		"every connect's deferred setup should run")

	// The slot is released after the setup finishes, so this settles just after the count above.
	capacity := int(network.config.GetOptions().RouterConnectConcurrency)
	require.Eventually(t, func() bool {
		held := 0
		for i := 0; i < capacity; i++ {
			if network.routerConnectSem.TryAcquire() {
				held++
			}
		}
		for i := 0; i < held; i++ {
			network.routerConnectSem.Release()
		}
		return held == capacity
	}, 10*time.Second, 10*time.Millisecond, "every connect slot should be released")
}

// TestConnectDisconnectRace exercises concurrent connect/disconnect for one router id under the race
// detector, asserting the map never ends holding a disconnected router.
func TestConnectDisconnectRace(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(connect bool) {
			defer wg.Done()
			if connect {
				r := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
				_ = network.QueueRouterConnect(r)
			} else if cur := network.Router.GetConnected("r1"); cur != nil {
				network.DisconnectRouter(cur)
			}
		}(i%2 == 0)
	}
	wg.Wait()

	if cur := network.Router.GetConnected("r1"); cur != nil {
		require.True(t, cur.Connected.Load(), "map must not hold a disconnected router")
	}
}

// fakeConnectChannel is a channel.Channel double reporting a fixed router id and a real
// ListenerCtrlChannel as its senders, which is what the accept path records on the router. It embeds the
// interface, so any method these tests do not exercise panics rather than returning a zero value.
type fakeConnectChannel struct {
	channel.Channel
	id      string
	senders channel.Senders
}

func (f *fakeConnectChannel) Id() string                  { return f.id }
func (f *fakeConnectChannel) GetSenders() channel.Senders { return f.senders }
func (f *fakeConnectChannel) SetLogicalName(string)       {}

func newFakeConnectChannel(routerId string) *fakeConnectChannel {
	return &fakeConnectChannel{id: routerId, senders: ctrlchan.NewListenerCtrlChannel()}
}

// newPersistedRouter stores a router so the connect path can load it. Create publishes the instance it was
// given into the router cache, which is the instance a connection must not be handed.
func newPersistedRouter(t *testing.T, network *Network, addr transport.Address, id string) *model.Router {
	t.Helper()
	router := model.NewRouterForTest(id, "", addr, nil, 0, false)
	require.NoError(t, network.Router.Create(router, change.New()))
	return router
}

// TestNewCtrlChanRouter_InstanceIsNotShared is the guard on the assumption every currency check in the
// connect and disconnect paths rests on: each connection gets its own Router instance. Two connections
// sharing one instance are indistinguishable to those checks, so a connect into an occupied slot is not
// rejected and the first connection's teardown dismantles the second's registration, leaving a live
// control channel whose router is not registered and can never re-register.
func TestNewCtrlChanRouter_InstanceIsNotShared(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	cached := newPersistedRouter(t, network, addr, "r1")

	first, err := network.NewCtrlChanRouter(newFakeConnectChannel("r1"))
	require.NoError(t, err)
	second, err := network.NewCtrlChanRouter(newFakeConnectChannel("r1"))
	require.NoError(t, err)

	require.NotSame(t, first, second, "each connection must get its own router instance")
	require.NotSame(t, cached, first, "a connection must not be handed the cached instance")
	require.NotSame(t, cached, second, "a connection must not be handed the cached instance")

	// The connection's instance must not become the cached one either, or the next connection would be
	// handed it.
	readBack, err := network.Router.Read("r1")
	require.NoError(t, err)
	require.NotSame(t, first, readBack, "a connection's instance must stay out of the router cache")
	require.NotSame(t, second, readBack, "a connection's instance must stay out of the router cache")
}

// TestNewCtrlChanRouter_ConcurrentConnectsAreNotShared covers the way instances came to be shared: the
// load was an eviction followed by a read-through read, so two connects racing could both evict and the
// later read could then hit the instance the earlier one had just published.
func TestNewCtrlChanRouter_ConcurrentConnectsAreNotShared(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)
	newPersistedRouter(t, network, addr, "r1")

	const connects = 16
	start := make(chan struct{})
	results := make([]*model.Router, connects)
	var wg sync.WaitGroup

	for i := 0; i < connects; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			r, err := network.NewCtrlChanRouter(newFakeConnectChannel("r1"))
			if err == nil {
				results[idx] = r
			}
		}(i)
	}

	close(start)
	wg.Wait()

	seen := map[*model.Router]int{}
	for idx, r := range results {
		require.NotNil(t, r, "connect %d failed to load a router", idx)
		seen[r]++
	}
	require.Len(t, seen, connects, "every concurrent connect must get its own router instance")
}

// TestNewCtrlChanRouter_RecordsTheChannel: the instance arrives carrying its connection, so no caller has
// to remember to attach it, and the channel it carries is the one it was built for.
func TestNewCtrlChanRouter_RecordsTheChannel(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)
	newPersistedRouter(t, network, addr, "r1")

	ch := newFakeConnectChannel("r1")
	r, err := network.NewCtrlChanRouter(ch)
	require.NoError(t, err)

	require.NotNil(t, r.Control, "the connection's channel must be recorded")
	require.Same(t, ch, r.Control.GetChannel(), "the recorded channel must be the one the instance was built for")
	require.False(t, r.ConnectTime.IsZero(), "connect time must be recorded")
	require.False(t, r.Connected.Load(), "loading a router must not register it as connected")
}

// TestNewCtrlChanRouter_UnknownRouter: a channel from a router the controller has no record of is refused
// rather than yielding an empty instance.
func TestNewCtrlChanRouter_UnknownRouter(t *testing.T) {
	_, network, _ := newConnectTestNetwork(t)

	r, err := network.NewCtrlChanRouter(newFakeConnectChannel("nope"))
	require.Error(t, err)
	require.Nil(t, r)
}

// TestNotifyExistingLink_RaceDisconnect is the invariant that link publication and disconnect teardown
// cannot interleave: once a router is disconnected, no link may remain naming it as its source.
//
// Checking currency and then publishing without holding the router's connect stripe is a check-then-act.
// A report can find the connection current, and by the time it reaches the link manager the teardown has
// already taken its snapshot of the router's links and cleared them, so the link is recreated after
// everything that would have removed it has run. It is then invisible to the router (its index was
// cleared) while still in the controller's link table with a disconnected source, and a reconnect
// reporting the same iteration can adopt that stale source rather than rebuilding the link.
//
// Run under -race, and repeated, since the window is small.
func TestNotifyExistingLink_RaceDisconnect(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	// The destination is left unconnected: only the source router's disconnect can recreate a stale link,
	// so connecting a second router would add the peer-state sync to the race for no extra coverage.
	for i := 0; i < 200; i++ {
		r := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
		require.NoError(t, network.QueueRouterConnect(r))

		reported := &ctrl_pb.RouterLinks_RouterLink{
			Id:           fmt.Sprintf("l-%d", i),
			DestRouterId: "r2",
			LinkProtocol: "tls",
			DialAddress:  "tcp:localhost:1234",
			Iteration:    1,
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			network.NotifyExistingLink(r, reported)
		}()
		go func() {
			defer wg.Done()
			network.DisconnectRouter(r)
		}()
		wg.Wait()

		require.Nil(t, network.Router.GetConnected("r1"), "the router must be disconnected")
		for _, l := range network.Link.All() {
			require.NotEqual(t, "r1", l.GetSrc().Id,
				"iteration %d: a link published by a router that has been disconnected must not survive", i)
		}
	}
}

// TestLinkGossipListener_EntryRemovedRequiresEndpoint guards the eviction path. Entry ownership is validated
// at ingress, so a tombstone's owner really is the router that published it, but nothing stops a router
// publishing one keyed to a link between two others. The tombstone reaches every controller, so an
// unauthorized one would be propagated as an authoritative eviction of a link the publisher has no part in.
func TestLinkGossipListener_EntryRemovedRequiresEndpoint(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)
	listener := &linkGossipListener{network: network}

	src := model.NewRouterForTest("r0", "", addr, &fakeCtrlChannel{}, 0, false)
	dst := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)

	newLink := func(t *testing.T) *model.Link {
		t.Helper()
		link, created := network.Link.RouterReportedLink(&ctrl_pb.RouterLinks_RouterLink{
			Id:           "l0",
			DestRouterId: dst.Id,
			LinkProtocol: "tls",
			DialAddress:  "tcp:localhost:1234",
			Iteration:    1,
		}, src, dst)
		require.True(t, created)
		require.NotNil(t, link)
		return link
	}

	t.Run("a tombstone from a bystander is ignored", func(t *testing.T) {
		link := newLink(t)
		listener.EntryRemoved(LinkGossipKey("l0", 1), "r2", 1, gossip.OriginGossip)

		held, ok := network.Link.Get("l0")
		require.True(t, ok, "the link must survive a tombstone from a router that is not an endpoint")
		require.Same(t, link, held)
		network.Link.Remove(held)
	})

	t.Run("a tombstone from the source removes the link", func(t *testing.T) {
		newLink(t)
		listener.EntryRemoved(LinkGossipKey("l0", 1), src.Id, 1, gossip.OriginGossip)

		_, ok := network.Link.Get("l0")
		require.False(t, ok, "the source router may retire its own link")
	})

	// Either endpoint may report a link gone. The far end sees the same failure and is not always able to
	// wait for the dialer to notice first.
	t.Run("a tombstone from the destination removes the link", func(t *testing.T) {
		newLink(t)
		listener.EntryRemoved(LinkGossipKey("l0", 1), dst.Id, 1, gossip.OriginGossip)

		_, ok := network.Link.Get("l0")
		require.False(t, ok, "the destination router may retire a link it terminates")
	})
}

// TestNotifyExistingLink_RefusedReportRaisesNoEvent covers the legacy link-report path against a refused
// report. RouterReportedLink returns no link when the reporting router is not the link's source, and the
// link event carries fields read straight off that link, so raising one anyway dereferences nil and takes the
// controller down. Only a crafted report reaches this, since a router only ever reports links it dialed, but
// a check that refuses bad input must not turn it into a crash.
func TestNotifyExistingLink_RefusedReportRaisesNoEvent(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	src := model.NewRouterForTest("r0", "", addr, &fakeCtrlChannel{}, 0, false)
	dst := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	impostor := model.NewRouterForTest("r2", "", addr, &fakeCtrlChannel{}, 0, false)

	report := func(iteration uint32) *ctrl_pb.RouterLinks_RouterLink {
		return &ctrl_pb.RouterLinks_RouterLink{
			Id:           "l0",
			DestRouterId: dst.Id,
			LinkProtocol: "tls",
			DialAddress:  "tcp:localhost:1234",
			Iteration:    iteration,
		}
	}

	link, created := network.Link.RouterReportedLink(report(1), src, dst)
	require.True(t, created)
	require.NotNil(t, link)

	// The reporting router has to be the current connection to get as far as the report.
	require.NoError(t, network.QueueRouterConnect(impostor))

	network.NotifyExistingLink(impostor, report(2))

	held, ok := network.Link.Get("l0")
	require.True(t, ok, "the real link must survive the refused report")
	require.Same(t, link, held)
	require.Equal(t, "r0", held.GetSrc().Id)
}

// TestRouterReportedLink_RepairsDestConnectedMidReport drives the interleave where both paths that pair a
// link with its destination miss: the report resolves the destination before the link is in the table, and
// the destination's own link build runs before the link lands there. The destination connects through
// ConnectRouter rather than by calling its steps here, since the order of those steps is load-bearing.
func TestRouterReportedLink_RepairsDestConnectedMidReport(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	src := model.NewRouterForTest("r0", "", addr, &fakeCtrlChannel{}, 0, false)
	dst := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.QueueRouterConnect(src))
	network.Router.MarkConnected(src)

	report := &ctrl_pb.RouterLinks_RouterLink{
		Id:           "l0",
		DestRouterId: dst.Id,
		LinkProtocol: "tls",
		DialAddress:  "tcp:localhost:1234",
		Iteration:    1,
	}

	// The reporting path resolves the destination first, and finds it absent.
	resolved := network.Router.GetConnected(dst.Id)
	require.Nil(t, resolved)

	// The destination connects in the gap. Its link build finds nothing, because the link being reported is
	// not in the table yet. Connect setup is deferred to its own goroutine, so wait for it rather than
	// racing it: the presence callback runs at the end of that setup.
	setupDone := &connectSetupSignal{}
	network.AddRouterPresenceHandler(setupDone)
	require.NoError(t, network.QueueRouterConnect(dst))
	require.Eventually(t, func() bool { return setupDone.has(dst.Id) }, 5*time.Second, 5*time.Millisecond,
		"the destination's connect setup should run")
	require.Empty(t, dst.GetLinks(), "the destination's link build must have found nothing")
	// The destination connects in the gap. It registers, then builds its links and finds none, because the
	// link being reported is not in the table yet.
	network.Router.MarkConnected(dst)
	network.Link.BuildRouterLinks(dst)

	// Only now does the report land, still carrying the resolution that was accurate when it was taken.
	link, created := network.Link.RouterReportedLink(report, src, resolved)
	require.True(t, created)
	require.NotNil(t, link)

	require.Same(t, dst, link.GetDest(),
		"the link must be pointed at the destination router that connected while the report was in flight")

	// The consequence that matters: path computation can see the adjacency.
	neighbors := network.Link.ConnectedNeighborsOfRouter(src)
	require.Len(t, neighbors, 1, "the link must carry adjacency")
	require.Equal(t, dst.Id, neighbors[0].Id)

	// Exactly once: the router's link set does not deduplicate, and its link build may run again.
	require.Len(t, dst.GetLinks(), 1)
	network.Link.BuildRouterLinks(dst)
	require.Len(t, dst.GetLinks(), 1, "a second link build must not index the link again")
}

// TestRouterDelete_DropsTheLinkIndex covers the wiring rather than the drop itself: a delete has to
// actually reach the cleanup. Awaited, since the store dispatches the delete listener on its own goroutine.
func TestRouterDelete_DropsTheLinkIndex(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	gone := newPersistedRouter(t, network, addr, "r0")
	peer := newPersistedRouter(t, network, addr, "r1")
	network.Link.Add(model.NewTestLink("l0", gone, peer))

	require.Len(t, network.Link.LinksForRouter(gone.Id), 1)
	require.Len(t, network.Link.LinksForRouter(peer.Id), 1)

	require.NoError(t, network.Router.Delete(gone.Id, change.New()))

	require.Eventually(t, func() bool { return len(network.Link.LinksForRouter(gone.Id)) == 0 },
		2*time.Second, 10*time.Millisecond, "deleting a router must drop its link index")
	require.Len(t, network.Link.LinksForRouter(peer.Id), 1, "the other endpoint's index must be untouched")
}

// TestRouterDelete_KeepsTheIndexOfAReusedId covers the delete cleanup running late against an id that has
// come back. Fabric router ids are the enrollment certificate's common name, so re-adding a router from the
// same cert reuses its id, and nothing orders the cleanup against that: store commit handlers run after
// bolt has released the writer lock. Dropping the live router's index here is invisible in the link table
// and shows up only when a per-router query misses links it should have.
//
// The late callback is invoked directly rather than by pausing the real one, since the guard is a store read
// taken while the index map's shard is held, so the observable property is the same either way.
func TestRouterDelete_KeepsTheIndexOfAReusedId(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	peer := newPersistedRouter(t, network, addr, "r1")

	original := newPersistedRouter(t, network, addr, "r0")
	require.NoError(t, network.Router.Delete(original.Id, change.New()))

	// The same id comes back and its links are reported, all before the earlier delete's cleanup runs.
	reused := newPersistedRouter(t, network, addr, "r0")
	network.Link.Add(model.NewTestLink("l0", reused, peer))
	require.Len(t, network.Link.LinksForRouter(reused.Id), 1)

	network.Link.RouterDeleted(original.Id)

	require.Len(t, network.Link.LinksForRouter(reused.Id), 1,
		"a delete that lands after the id was recreated must not drop the live router's index")
	require.Len(t, network.Link.LinksForRouter(peer.Id), 1)
}

// TestRouterReportedLink_RepairsDestDisplacedMidReport is the same interleave with a stale destination
// rather than an absent one. The report resolves the destination under the source's connect stripe, not the
// destination's, so that router can be replaced before the report lands. A link left pointing at the
// displaced instance reads as healthy everywhere while carrying no adjacency, since the instance it names
// is no longer connected.
func TestRouterReportedLink_RepairsDestDisplacedMidReport(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	setupDone := &connectSetupSignal{}
	network.AddRouterPresenceHandler(setupDone)
	connect := func(r *model.Router) {
		t.Helper()
		require.NoError(t, network.QueueRouterConnect(r))
		require.Eventually(t, func() bool { return setupDone.has(r.Id) }, 5*time.Second, 5*time.Millisecond,
			"connect setup should run for %v", r.Id)
	}

	src := model.NewRouterForTest("r0", "", addr, &fakeCtrlChannel{}, 0, false)
	firstDst := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	connect(src)
	connect(firstDst)

	// The reporting path resolves the destination, and gets the connection that is current right then.
	resolved := network.Router.GetConnected(firstDst.Id)
	require.Same(t, firstDst, resolved)

	// That connection is replaced before the report lands.
	network.DisconnectRouter(firstDst)
	secondDst := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	setupDone.done.Delete(secondDst.Id)
	connect(secondDst)
	require.Empty(t, secondDst.GetLinks(), "the replacement's link build must have found nothing")

	report := &ctrl_pb.RouterLinks_RouterLink{
		Id:           "l0",
		DestRouterId: firstDst.Id,
		LinkProtocol: "tls",
		DialAddress:  "tcp:localhost:1234",
		Iteration:    1,
	}
	link, created := network.Link.RouterReportedLink(report, src, resolved)
	require.True(t, created)

	require.Same(t, secondDst, link.GetDest(),
		"the link must be pointed at the connection that replaced the one the report resolved")

	neighbors := network.Link.ConnectedNeighborsOfRouter(src)
	require.Len(t, neighbors, 1, "the link must carry adjacency")
	require.Same(t, secondDst, neighbors[0])

	require.Len(t, secondDst.GetLinks(), 1, "the link must be indexed on the current connection exactly once")
}

// TestRouterReportedLink_RepairsDestOnALaterReport: a link already pointing at a displaced connection is
// repaired by the next report for it, which is the only thing left that will look. The report carries no
// new iteration, so the link is not rebuilt; the repair has to happen on the already-known path.
func TestRouterReportedLink_RepairsDestOnALaterReport(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	setupDone := &connectSetupSignal{}
	network.AddRouterPresenceHandler(setupDone)
	connect := func(r *model.Router) {
		t.Helper()
		require.NoError(t, network.QueueRouterConnect(r))
		require.Eventually(t, func() bool { return setupDone.has(r.Id) }, 5*time.Second, 5*time.Millisecond,
			"connect setup should run for %v", r.Id)
	}

	src := model.NewRouterForTest("r0", "", addr, &fakeCtrlChannel{}, 0, false)
	firstDst := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	connect(src)
	connect(firstDst)

	report := &ctrl_pb.RouterLinks_RouterLink{
		Id:           "l0",
		DestRouterId: firstDst.Id,
		LinkProtocol: "tls",
		DialAddress:  "tcp:localhost:1234",
		Iteration:    1,
	}
	link, created := network.Link.RouterReportedLink(report, src, firstDst)
	require.True(t, created)
	require.Same(t, firstDst, link.GetDest())

	// The destination is replaced. Its link build repairs links already in the table, so drive the case it
	// cannot reach by putting the link back on the stale instance behind its back.
	network.DisconnectRouter(firstDst)
	secondDst := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	setupDone.done.Delete(secondDst.Id)
	connect(secondDst)
	require.True(t, link.PointDestAt(firstDst), "putting the link back on the displaced instance")

	again, created := network.Link.RouterReportedLink(report, src, firstDst)
	require.False(t, created, "a report at the same iteration must not rebuild the link")
	require.Same(t, link, again)
	require.Same(t, secondDst, link.GetDest(), "a later report must repair a displaced destination")
}

// TestConnectRouter_RepairsLinkAlreadyMissingItsDest covers the other half of the pairing: a link already in
// the table with no destination is repaired when that destination connects.
func TestConnectRouter_RepairsLinkAlreadyMissingItsDest(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	src := model.NewRouterForTest("r0", "", addr, &fakeCtrlChannel{}, 0, false)
	dst := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.QueueRouterConnect(src))

	// A link recorded while its destination was not connected, so it points at nothing.
	report := &ctrl_pb.RouterLinks_RouterLink{
		Id:           "l0",
		DestRouterId: dst.Id,
		LinkProtocol: "tls",
		DialAddress:  "tcp:localhost:1234",
		Iteration:    1,
	}
	link, created := network.Link.RouterReportedLink(report, src, nil)
	require.True(t, created)
	require.Nil(t, link.GetDest(), "the destination was not connected when the link was recorded")

	require.NoError(t, network.QueueRouterConnect(dst))

	// The repair runs in the deferred connect setup, not in QueueRouterConnect.
	require.Eventually(t, func() bool { return link.GetDest() == dst }, 5*time.Second, 5*time.Millisecond,
		"connecting the destination must repair the link")
	require.Len(t, dst.GetLinks(), 1, "and index it on the destination")

	neighbors := network.Link.ConnectedNeighborsOfRouter(src)
	require.Len(t, neighbors, 1, "the link must carry adjacency once repaired")
	require.Equal(t, dst.Id, neighbors[0].Id)
}

// TestIsCurrentConnection covers the predicate the deferred gossip paths use to decide whether a message is
// still worth applying. Each of them queues work capturing the connection its message arrived on, and that
// connection can be given up while the work waits, at which point the entries describe a router lifetime that
// may be over and a restart's epoch cleanup may already have discarded.
func TestIsCurrentConnection(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	r := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)

	require.False(t, network.IsCurrentConnection(r), "a connection that was never registered is not current")

	require.NoError(t, network.QueueRouterConnect(r))
	require.True(t, network.IsCurrentConnection(r))

	// A second connection for the same router is refused rather than taking over, and the occupant is
	// displaced, so neither is current once that has run.
	other := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.False(t, network.IsCurrentConnection(other),
		"a different connection for the same router id is not current")

	network.DisconnectRouter(r)
	require.False(t, network.IsCurrentConnection(r), "a connection that has been given up is not current")
}
