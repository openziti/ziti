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

package env

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"

	cmap "github.com/orcaman/concurrent-map/v2"

	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/stretchr/testify/require"
)

// Test_firstUnderlayHeaders verifies that the initial underlay is flagged as the first grouped
// connection, and that each call yields an independent map. If the flag leaked onto headers shared with
// additional underlays (e.g. ctrl.high), every additional underlay would look like a new channel and
// trip the controller's already-connected churn guard.
func Test_firstUnderlayHeaders(t *testing.T) {
	req := require.New(t)

	first := firstUnderlayHeaders()

	firstFlag, ok := first.GetBoolHeader(channel.IsFirstGroupConnection)
	req.True(ok, "initial underlay headers must contain the first-connection flag")
	req.True(firstFlag)

	grouped, _ := first.GetBoolHeader(channel.IsGroupedHeader)
	req.True(grouped, "initial underlay must be flagged as grouped")
	chType, _ := first.GetStringHeader(channel.TypeHeader)
	req.Equal(ctrlchan.ChannelTypeDefault, chType, "initial underlay must carry the default ctrl channel type")

	// Callers mutate the returned headers, so each call must hand back a distinct map; a shared map
	// would let the first-connection flag reach underlays that must not carry it.
	other := firstUnderlayHeaders()
	delete(other, channel.IsFirstGroupConnection)
	_, stillSet := first[channel.IsFirstGroupConnection]
	req.True(stillSet, "each call must return an independent header map")
}

// ctrlsTestChannel is a channel.Channel double reporting a fixed id and label. It embeds the interface, so
// any method these tests do not exercise panics rather than silently returning a zero value.
type ctrlsTestChannel struct {
	channel.Channel
	id     string
	label  string
	closed atomic.Bool
	// underlaysLost models a channel that has lost every underlay without having been closed, which is the
	// state Add treats as displaceable.
	underlaysLost atomic.Bool
}

func (self *ctrlsTestChannel) Id() string     { return self.id }
func (self *ctrlsTestChannel) Label() string  { return self.label }
func (self *ctrlsTestChannel) IsClosed() bool { return self.closed.Load() }
func (self *ctrlsTestChannel) Close() error   { self.closed.Store(true); return nil }
func (self *ctrlsTestChannel) IsConnected() bool {
	return !self.closed.Load() && !self.underlaysLost.Load()
}

// ctrlsTestCtrlChannel is a ctrlchan.CtrlChannel double wrapping a ctrlsTestChannel.
type ctrlsTestCtrlChannel struct {
	ctrlchan.CtrlChannel
	ch *ctrlsTestChannel
}

func (self *ctrlsTestCtrlChannel) GetChannel() channel.Channel { return self.ch }
func (self *ctrlsTestCtrlChannel) IsClosed() bool              { return self.ch.IsClosed() }
func (self *ctrlsTestCtrlChannel) IsConnected() bool           { return self.ch.IsConnected() }
func (self *ctrlsTestCtrlChannel) Close() error                { return self.ch.Close() }

// ctrlsTestUnderlay is a channel.Underlay double supplying only the hello headers Add reads.
type ctrlsTestUnderlay struct {
	channel.Underlay
	headers channel.Headers
}

func (self *ctrlsTestUnderlay) Headers() map[int32][]byte { return self.headers }

// newTestUnderlay supplies the version header Add requires of a controller hello.
func newTestUnderlay(t *testing.T) channel.Underlay {
	t.Helper()
	encoded, err := versions.StdVersionEncDec.Encode(&versions.VersionInfo{Version: "v1.0.0"})
	require.NoError(t, err)
	return &ctrlsTestUnderlay{headers: channel.Headers{channel.HelloVersionHeader: encoded}}
}

// newTestNetworkControllers builds a networkControllers with no DialEnv. Tests using it must not reach a
// path that dials, which for these tests means leaving controllerDetails empty so no reconnect is started.
func newTestNetworkControllers() *networkControllers {
	return &networkControllers{
		heartbeatOptions: NewDefaultHeartbeatOptions(),
		idsBeingDialed:   cmap.New[struct{}](),
	}
}

// newTestNetworkCtrl returns a registration and the test channel backing it, so tests can drive the
// channel into the states the usability check distinguishes.
func newTestNetworkCtrl(nc *networkControllers, ctrlId string, label string) (*networkCtrl, *ctrlsTestChannel) {
	ch := &ctrlsTestChannel{id: ctrlId, label: label}
	return newNetworkCtrl(&ctrlsTestCtrlChannel{ch: ch}, "tls:"+ctrlId+":6262", nc.heartbeatOptions), ch
}

// collectCtrlEvents registers a listener and returns a function draining the events seen so far. Listeners
// are notified on their own goroutine, so the buffer carries them back to the test.
func collectCtrlEvents(nc *networkControllers) func() []CtrlEvent {
	received := make(chan CtrlEvent, 16)
	nc.AddChangeListener(CtrlEventListenerFunc(func(event CtrlEvent) {
		received <- event
	}))

	return func() []CtrlEvent {
		var result []CtrlEvent
		for {
			select {
			case e := <-received:
				result = append(result, e)
			default:
				return result
			}
		}
	}
}

// TestHandleChannelClose_SupersededLeavesCurrentRegistered guards against a router going permanently
// invisible to a controller. Overlapping channels to one controller share its id, so a close that gives up
// the registration by id alone can delete the live channel's entry. Nothing re-registers an already
// established channel, and the reconnect path treats a present entry as connected, so the router would
// neither notice nor redial.
func TestHandleChannelClose_SupersededLeavesCurrentRegistered(t *testing.T) {
	nc := newTestNetworkControllers()
	drain := collectCtrlEvents(nc)

	superseded, _ := newTestNetworkCtrl(nc, "ctrl1", "old")
	current, _ := newTestNetworkCtrl(nc, "ctrl1", "current")

	nc.ctrls.Put("ctrl1", current)

	// The superseded channel's close lands after its replacement has registered.
	nc.handleChannelClose("ctrl1", superseded, 0)

	require.Same(t, current, nc.ctrls.Get("ctrl1"),
		"a superseded channel's close must not unregister the channel that replaced it")

	require.Never(t, func() bool {
		return len(drain()) > 0
	}, 100*time.Millisecond, 10*time.Millisecond,
		"a superseded channel's close must not report the current channel as disconnected")
}

// TestHandleChannelClose_CurrentUnregisters covers the ordinary case: the registered channel closing gives
// up its registration and reports the disconnect.
func TestHandleChannelClose_CurrentUnregisters(t *testing.T) {
	nc := newTestNetworkControllers()
	drain := collectCtrlEvents(nc)

	current, _ := newTestNetworkCtrl(nc, "ctrl1", "current")
	nc.ctrls.Put("ctrl1", current)

	nc.handleChannelClose("ctrl1", current, 0)

	require.Nil(t, nc.ctrls.Get("ctrl1"), "the registered channel closing must unregister the controller")

	var events []CtrlEvent
	require.Eventually(t, func() bool {
		events = append(events, drain()...)
		return len(events) > 0
	}, time.Second, 10*time.Millisecond, "expected a controller change event")

	require.Len(t, events, 1)
	require.Equal(t, ControllerDisconnected, events[0].Type)
	require.Same(t, current, events[0].Controller)
}

// TestHandleChannelClose_AlreadyUnregistered: closeAndRemoveById drops the entry before closing the
// channel, so the close handler that follows must not report a second disconnect.
func TestHandleChannelClose_AlreadyUnregistered(t *testing.T) {
	nc := newTestNetworkControllers()
	drain := collectCtrlEvents(nc)

	current, _ := newTestNetworkCtrl(nc, "ctrl1", "current")

	nc.handleChannelClose("ctrl1", current, 0)

	require.Never(t, func() bool {
		return len(drain()) > 0
	}, 100*time.Millisecond, 10*time.Millisecond,
		"closing a channel that is already unregistered must not report a disconnect")
}

// TestRemoveIfCurrent_LeavesOtherControllers guards the key comparison: giving up one controller's
// registration must not touch another's.
func TestRemoveIfCurrent_LeavesOtherControllers(t *testing.T) {
	nc := newTestNetworkControllers()

	ctrl1, _ := newTestNetworkCtrl(nc, "ctrl1", "ctrl1")
	ctrl2, _ := newTestNetworkCtrl(nc, "ctrl2", "ctrl2")
	nc.ctrls.Put("ctrl1", ctrl1)
	nc.ctrls.Put("ctrl2", ctrl2)

	require.True(t, nc.removeIfCurrent("ctrl1", ctrl1))

	require.Nil(t, nc.ctrls.Get("ctrl1"))
	require.Same(t, ctrl2, nc.ctrls.Get("ctrl2"))

	require.False(t, nc.removeIfCurrent("ctrl1", ctrl1), "removing an absent registration must report no match")
}

// TestIsUsable covers what counts as being connected to a controller. A registration is not evidence of
// connectivity on its own: the channel behind it may have been closed, or may have lost every underlay
// without closing, which is the state a control channel is left in when its group dies but nothing detects
// it.
func TestIsUsable(t *testing.T) {
	nc := newTestNetworkControllers()

	require.False(t, isUsable(nil), "an absent registration is not usable")

	healthy, _ := newTestNetworkCtrl(nc, "ctrl1", "healthy")
	require.True(t, isUsable(healthy))

	closed, closedCh := newTestNetworkCtrl(nc, "ctrl2", "closed")
	closedCh.closed.Store(true)
	require.False(t, isUsable(closed), "a registration whose channel is closed is not usable")

	underlayless, underlaylessCh := newTestNetworkCtrl(nc, "ctrl3", "no-underlays")
	underlaylessCh.underlaysLost.Store(true)
	require.False(t, isUsable(underlayless), "a registration with no underlays left is not usable")
}

// TestUpdateControllerDetails_ReconnectsWhenRegistrationUnusable: a registration left behind by a channel
// that died must not suppress the reconnect. Treating presence as connectivity is what leaves a router
// permanently invisible to a controller, since nothing else in this path would notice.
func TestUpdateControllerDetails_ReconnectsWhenRegistrationUnusable(t *testing.T) {
	nc := newTestNetworkControllers()

	stale, staleCh := newTestNetworkCtrl(nc, "ctrl1", "stale")
	staleCh.underlaysLost.Store(true)
	nc.ctrls.Put("ctrl1", stale)

	// The detail carries no endpoints, so the reconnect gives up before reaching the network.
	require.True(t, nc.UpdateControllerDetails([]*ctrl_pb.CtrlDetail{{Id: "ctrl1"}}),
		"an unusable registration must not suppress the reconnect")
}

// TestUpdateControllerDetails_LeavesUsableRegistrationAlone is the other half: a working channel must not
// be redialed on every controller detail update.
func TestUpdateControllerDetails_LeavesUsableRegistrationAlone(t *testing.T) {
	nc := newTestNetworkControllers()

	current, _ := newTestNetworkCtrl(nc, "ctrl1", "current")
	nc.ctrls.Put("ctrl1", current)

	require.False(t, nc.UpdateControllerDetails([]*ctrl_pb.CtrlDetail{{Id: "ctrl1"}}),
		"a usable registration must not be redialed")
	require.Same(t, current, nc.ctrls.Get("ctrl1"))
}

// TestAdd_RejectsDuplicateOfUsableChannel: one usable channel per controller. The duplicate is refused and
// the established channel is left alone.
func TestAdd_RejectsDuplicateOfUsableChannel(t *testing.T) {
	nc := newTestNetworkControllers()
	underlay := newTestUnderlay(t)

	established := &ctrlsTestChannel{id: "ctrl1", label: "established"}
	first, err := nc.Add("tls:ctrl1:6262", &ctrlsTestCtrlChannel{ch: established}, established, underlay)
	require.NoError(t, err)

	duplicate := &ctrlsTestChannel{id: "ctrl1", label: "duplicate"}
	second, err := nc.Add("tls:ctrl1:6262", &ctrlsTestCtrlChannel{ch: duplicate}, duplicate, underlay)
	require.Error(t, err)
	require.Nil(t, second)

	require.Same(t, first, nc.ctrls.Get("ctrl1"), "the established channel must keep its registration")
	require.False(t, established.IsClosed(), "the established channel must not be closed to admit a duplicate")
}

// TestAdd_DuplicateErrorIsRetryable: losing a dial race is an ordinary race, not a permanent condition. A
// permanent refusal ends the retry loop for that controller for the life of the process, so if the winner
// later turns out to be unusable there is nothing left to redial.
func TestAdd_DuplicateErrorIsRetryable(t *testing.T) {
	nc := newTestNetworkControllers()
	underlay := newTestUnderlay(t)

	established := &ctrlsTestChannel{id: "ctrl1", label: "established"}
	_, err := nc.Add("tls:ctrl1:6262", &ctrlsTestCtrlChannel{ch: established}, established, underlay)
	require.NoError(t, err)

	duplicate := &ctrlsTestChannel{id: "ctrl1", label: "duplicate"}
	_, err = nc.Add("tls:ctrl1:6262", &ctrlsTestCtrlChannel{ch: duplicate}, duplicate, underlay)
	require.Error(t, err)

	var permanent *backoff.PermanentError
	require.False(t, errors.As(err, &permanent), "a lost dial race must not stop the retry loop")

	var dup *errDuplicateChannel
	require.True(t, errors.As(err, &dup), "the refusal must identify itself as a duplicate")
	require.Equal(t, "ctrl1", dup.ctrlId,
		"the refusal must carry the controller id, which a dial started from a bare endpoint has no other way to learn")
}

// TestDuplicateResolved covers how the retry loop reacts to losing a race. The id checks at the top of a
// retry cannot help a dial started from a bare endpoint, since it has no controller id to check with, so
// the refusal itself is what ends the retries.
func TestDuplicateResolved(t *testing.T) {
	nc := newTestNetworkControllers()

	winner, winnerCh := newTestNetworkCtrl(nc, "ctrl1", "winner")
	nc.ctrls.Put("ctrl1", winner)

	// Wrapped the way connectToController wraps a dial failure.
	refusal := fmt.Errorf("error connecting ctrl (%w)", &errDuplicateChannel{ctrlId: "ctrl1"})

	ctrlId, ok := nc.duplicateResolved(refusal)
	require.True(t, ok, "a refusal by a usably connected controller leaves this dial nothing to do")
	require.Equal(t, "ctrl1", ctrlId)

	// The channel that won the race then dies, so the work is this dial's after all.
	winnerCh.closed.Store(true)
	_, ok = nc.duplicateResolved(refusal)
	require.False(t, ok, "a refusal by a controller that is no longer usable must not end the retries")

	_, ok = nc.duplicateResolved(errors.New("connection refused"))
	require.False(t, ok, "an unrelated failure must not be read as a lost race")
}

// TestAdd_DisplacesUnusableChannel: a registration whose channel has no underlays left is not a reason to
// refuse a new one. The new channel takes the registration over and the old one is closed.
func TestAdd_DisplacesUnusableChannel(t *testing.T) {
	nc := newTestNetworkControllers()
	underlay := newTestUnderlay(t)

	stale := &ctrlsTestChannel{id: "ctrl1", label: "stale"}
	staleCtrl, err := nc.Add("tls:ctrl1:6262", &ctrlsTestCtrlChannel{ch: stale}, stale, underlay)
	require.NoError(t, err)

	// Simulate the channel losing its underlays without having been closed.
	stale.underlaysLost.Store(true)

	replacement := &ctrlsTestChannel{id: "ctrl1", label: "replacement"}
	replacementCtrl, err := nc.Add("tls:ctrl1:6262", &ctrlsTestCtrlChannel{ch: replacement}, replacement, underlay)
	require.NoError(t, err)
	require.NotSame(t, staleCtrl, replacementCtrl)

	require.Same(t, replacementCtrl, nc.ctrls.Get("ctrl1"), "the new channel must hold the registration")
	require.True(t, stale.IsClosed(), "the displaced channel must be closed")
}

// TestAdd_ConcurrentForOneControllerRegistersOne is the check-then-put race. Initial endpoint dials carry
// no controller id, so idsBeingDialed cannot keep two dials to one controller apart; without the
// registration decision being atomic, both see no entry, both register, and whichever loses is left with a
// live channel that nothing tracks or reconnects.
func TestAdd_ConcurrentForOneControllerRegistersOne(t *testing.T) {
	for i := 0; i < 50; i++ {
		nc := newTestNetworkControllers()
		underlay := newTestUnderlay(t)

		const dials = 8
		start := make(chan struct{})
		var wg sync.WaitGroup
		var registered atomic.Int32
		results := make([]NetworkController, dials)

		for j := 0; j < dials; j++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				ch := &ctrlsTestChannel{id: "ctrl1", label: fmt.Sprintf("dial-%d", idx)}
				<-start
				ctrl, err := nc.Add("tls:ctrl1:6262", &ctrlsTestCtrlChannel{ch: ch}, ch, underlay)
				if err == nil {
					results[idx] = ctrl
					registered.Add(1)
				}
			}(j)
		}

		close(start)
		wg.Wait()

		require.Equal(t, int32(1), registered.Load(),
			"exactly one concurrent dial to a controller may take the registration")

		var winner NetworkController
		for _, ctrl := range results {
			if ctrl != nil {
				winner = ctrl
			}
		}
		require.Same(t, winner, nc.ctrls.Get("ctrl1"), "the registration must belong to the dial that took it")
	}
}

// TestEverConnected_NotSetByRegistrationAlone guards the difference between registering a control channel
// and having established one. Add runs in the first of several bind handlers, and a later one can still
// refuse the channel, after which it is closed and unregistered. If registering were enough to count, a
// router that only ever reached a controller it cannot talk to, an older one lacking a required capability
// for instance, would pass its startup check and retry forever instead of exiting.
func TestEverConnected_NotSetByRegistrationAlone(t *testing.T) {
	nc := newTestNetworkControllers()
	underlay := newTestUnderlay(t)

	require.False(t, nc.EverConnected(), "a router that has not connected yet must report so")

	ch := &ctrlsTestChannel{id: "ctrl1", label: "first"}
	ctrl, err := nc.Add("tls:ctrl1:6262", &ctrlsTestCtrlChannel{ch: ch}, ch, underlay)
	require.NoError(t, err)
	require.False(t, nc.EverConnected(),
		"a registration is not an established channel; the bind can still be refused after Add returns")

	// Losing it without the bind ever completing must leave the router still having never connected, so
	// the startup check can do its job.
	nc.handleChannelClose("ctrl1", ctrl, 0)
	require.False(t, nc.EverConnected(), "a channel that was never established must not count as one")
}

// TestEverConnected_StaysTrueAcrossLosingEveryController is the distinction the startup check depends on.
// The check fires once, so asking whether a controller is reachable at that instant kills a router that
// happens to be between control channels, which is routine rather than a failure to start. Asking whether
// one was ever reached answers the question the check is actually for.
func TestEverConnected_StaysTrueAcrossLosingEveryController(t *testing.T) {
	nc := newTestNetworkControllers()
	underlay := newTestUnderlay(t)

	ch := &ctrlsTestChannel{id: "ctrl1", label: "first"}
	ctrl, err := nc.Add("tls:ctrl1:6262", &ctrlsTestCtrlChannel{ch: ch}, ch, underlay)
	require.NoError(t, err)

	// What the dial and accept paths call once the whole bind has succeeded.
	nc.MarkChannelEstablished()
	require.True(t, nc.EverConnected())

	// Lose it, leaving no controllers at all, which is what the instant-count check used to trip on.
	nc.handleChannelClose("ctrl1", ctrl, 0)
	require.Empty(t, nc.GetAll(), "no controllers should remain")
	require.True(t, nc.EverConnected(),
		"having reached a controller once must not be forgotten when the connection is lost")
}

// TestNotifyOfConnectivityChange_ReconnectReportsBothHalves guards the reconnect half of a
// grouped control channel losing every underlay and getting one back. The registration
// survives that, so nothing else reports it: a reconnect that re-offers state without
// emitting ControllerReconnected leaves every listener keyed on connectivity believing the
// controller is still gone.
func TestNotifyOfConnectivityChange_ReconnectReportsBothHalves(t *testing.T) {
	nc := newTestNetworkControllers()
	drain := collectCtrlEvents(nc)

	ctrl, _ := newTestNetworkCtrl(nc, "ctrl1", "current")
	nc.ctrls.Put("ctrl1", ctrl)

	var wasDisconnected atomic.Bool
	var reconnectNotifications int
	notifyReconnect := func() { reconnectNotifications++ }

	// Last underlay lost, then one back.
	nc.notifyOfConnectivityChange("ctrl1", &wasDisconnected, 0, notifyReconnect)
	nc.notifyOfConnectivityChange("ctrl1", &wasDisconnected, 1, notifyReconnect)

	var events []CtrlEvent
	require.Eventually(t, func() bool {
		events = append(events, drain()...)
		return len(events) >= 2
	}, time.Second, 10*time.Millisecond, "expected a disconnect and a reconnect event, got %v", events)

	require.Len(t, events, 2)
	require.Equal(t, ControllerDisconnected, events[0].Type)
	require.Equal(t, ControllerReconnected, events[1].Type,
		"regaining an underlay must report the controller as reconnected")
	require.Same(t, ctrl, events[1].Controller)
	require.Equal(t, 1, reconnectNotifications, "the reconnect must also re-offer state to the controller")
}

// TestNotifyOfConnectivityChange_ReportsEachEdgeOnce covers underlays coming and going
// without connectivity changing. Only the edges are transitions; every other change is
// noise that listeners must not see as a reconnect.
func TestNotifyOfConnectivityChange_ReportsEachEdgeOnce(t *testing.T) {
	nc := newTestNetworkControllers()
	drain := collectCtrlEvents(nc)

	ctrl, _ := newTestNetworkCtrl(nc, "ctrl1", "current")
	nc.ctrls.Put("ctrl1", ctrl)

	var wasDisconnected atomic.Bool
	var reconnectNotifications int
	notifyReconnect := func() { reconnectNotifications++ }

	// A second underlay arriving and leaving while one remains is not a transition.
	nc.notifyOfConnectivityChange("ctrl1", &wasDisconnected, 2, notifyReconnect)
	nc.notifyOfConnectivityChange("ctrl1", &wasDisconnected, 1, notifyReconnect)

	require.Never(t, func() bool { return len(drain()) > 0 }, 100*time.Millisecond, 10*time.Millisecond,
		"underlay churn that never cost connectivity must not be reported")
	require.Zero(t, reconnectNotifications)

	// Two reports of the count reaching zero are one disconnect.
	nc.notifyOfConnectivityChange("ctrl1", &wasDisconnected, 0, notifyReconnect)
	nc.notifyOfConnectivityChange("ctrl1", &wasDisconnected, 0, notifyReconnect)

	var events []CtrlEvent
	require.Eventually(t, func() bool {
		events = append(events, drain()...)
		return len(events) >= 1
	}, time.Second, 10*time.Millisecond, "expected a disconnect event")

	// Same for the count coming back.
	nc.notifyOfConnectivityChange("ctrl1", &wasDisconnected, 1, notifyReconnect)
	nc.notifyOfConnectivityChange("ctrl1", &wasDisconnected, 2, notifyReconnect)

	require.Eventually(t, func() bool {
		events = append(events, drain()...)
		return len(events) >= 2
	}, time.Second, 10*time.Millisecond, "expected a reconnect event")

	require.Never(t, func() bool {
		events = append(events, drain()...)
		return len(events) > 2
	}, 100*time.Millisecond, 10*time.Millisecond, "each edge must be reported once, got %v", events)

	require.Equal(t, ControllerDisconnected, events[0].Type)
	require.Equal(t, ControllerReconnected, events[1].Type)
	require.Equal(t, 1, reconnectNotifications)
}
