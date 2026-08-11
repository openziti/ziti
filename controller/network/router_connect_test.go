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

	"github.com/openziti/channel/v5"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/tcp"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/change"
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

	require.NoError(t, network.ConnectRouter(cur))
	require.Equal(t, cur, network.Router.GetConnected("r1"))

	newCh := &fakeCtrlChannel{}
	rNew := model.NewRouterForTest("r1", "", addr, newCh, 0, false)
	err := network.ConnectRouter(rNew)
	require.ErrorIs(t, err, ErrConnectRejected)
	require.True(t, IsConnectRejected(err))
	require.True(t, currentCh.IsClosed(), "occupant should have been kicked")
	require.Nil(t, network.Router.GetConnected("r1"), "kicked occupant's teardown should clear the slot")
	require.False(t, rNew.Connected.Load(), "rejected connect must not be registered")

	// Redial into the now-clear slot succeeds.
	rRedial := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(rRedial))
	require.Equal(t, rRedial, network.Router.GetConnected("r1"))
	require.True(t, rRedial.Connected.Load())
}

// TestConnectRouter_SetsUpWhenClear: a connect into an empty slot registers the router.
func TestConnectRouter_SetsUpWhenClear(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	r := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(r))
	require.Equal(t, r, network.Router.GetConnected("r1"))
	require.True(t, r.Connected.Load())
}

// TestDisconnectRouter_IgnoresStaleConnection: a disconnect for a superseded connection must not disturb
// the current one (the §9 race guard).
func TestDisconnectRouter_IgnoresStaleConnection(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	rNew := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(rNew))
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
	require.NoError(t, network.ConnectRouter(r))
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
	require.NoError(t, network.ConnectRouter(cur))
	require.Equal(t, cur, network.Router.GetConnected("r1"))
	currentCh.closed.Store(true)

	rNew := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.ErrorIs(t, network.ConnectRouter(rNew), ErrConnectRejected)
	require.Nil(t, network.Router.GetConnected("r1"), "reject must displace an occupant that cannot tear itself down")
	require.False(t, cur.Connected.Load())
	require.False(t, rNew.Connected.Load(), "rejected connect must not be registered")

	// The redial therefore makes progress instead of bouncing off the slot forever.
	rRedial := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(rRedial))
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

	err := network.ConnectRouter(r)
	require.ErrorIs(t, err, ErrConnectChannelClosed)
	require.True(t, IsConnectRejected(err), "an already-closed channel is a refusal the router redials after")
	require.Nil(t, network.Router.GetConnected("r1"), "a closed connection must not occupy the slot")
	require.False(t, r.Connected.Load())

	// A subsequent healthy connect is unaffected.
	rOk := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(rOk))
	require.Equal(t, rOk, network.Router.GetConnected("r1"))
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
				_ = network.ConnectRouter(r)
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
		require.NoError(t, network.ConnectRouter(r))

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
			require.NotEqual(t, "r1", l.Src.Id,
				"iteration %d: a link published by a router that has been disconnected must not survive", i)
		}
	}
}

// TestRouterReportedLink_RepairsDestConnectedMidReport drives the interleave that leaves a link with no
// destination router. Both paths that establish the pairing miss it: the report resolves the destination
// before the link is in the table, and the destination's own link build runs before the link lands there. The
// link is then skipped by path computation while every operator-facing view still calls it healthy.
//
// The destination connects through ConnectRouter rather than by calling its steps here, so the test cannot
// disagree with production about the order they run in. That order is load-bearing: the repair re-reads the
// connected map, so it only helps if registration precedes the link build.
func TestRouterReportedLink_RepairsDestConnectedMidReport(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	src := model.NewRouterForTest("r0", "", addr, &fakeCtrlChannel{}, 0, false)
	dst := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(src))

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
	// not in the table yet.
	require.NoError(t, network.ConnectRouter(dst))
	require.Empty(t, dst.GetLinks(), "the destination's link build must have found nothing")

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

	// Indexed on the destination exactly once. The index is a slice that does not deduplicate, and the
	// destination's own link build is entitled to run again.
	require.Len(t, dst.GetLinks(), 1)
	network.Link.BuildRouterLinks(dst)
	require.Len(t, dst.GetLinks(), 1, "a second link build must not index the link again")
}

// TestConnectRouter_RepairsLinkAlreadyMissingItsDest covers the other half of the pairing: a link already in
// the table holding no destination must be repaired when that destination connects. Together with the report
// side above, whichever of the two arrives second fixes what the first missed.
func TestConnectRouter_RepairsLinkAlreadyMissingItsDest(t *testing.T) {
	_, network, addr := newConnectTestNetwork(t)

	src := model.NewRouterForTest("r0", "", addr, &fakeCtrlChannel{}, 0, false)
	dst := model.NewRouterForTest("r1", "", addr, &fakeCtrlChannel{}, 0, false)
	require.NoError(t, network.ConnectRouter(src))

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

	require.NoError(t, network.ConnectRouter(dst))

	require.Same(t, dst, link.GetDest(), "connecting the destination must repair the link")
	require.Len(t, dst.GetLinks(), 1, "and index it on the destination")

	neighbors := network.Link.ConnectedNeighborsOfRouter(src)
	require.Len(t, neighbors, 1, "the link must carry adjacency once repaired")
	require.Equal(t, dst.Id, neighbors[0].Id)
}
