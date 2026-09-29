/*
	(c) Copyright NetFoundry Inc.

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

package link

import (
	"container/heap"
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/idgen"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xlink"
	"github.com/stretchr/testify/require"
)

type testEnv struct {
	metricsRegistry metrics.UsageRegistry
	closeNotify     chan struct{}
	ctrls           env.NetworkControllers
	rateLimiterPool goroutines.Pool
}

func (self *testEnv) GetRouterId() *identity.TokenId {
	return &identity.TokenId{
		Token: "test",
	}
}

func (self *testEnv) GetNetworkControllers() env.NetworkControllers {
	return self.ctrls
}

func (self *testEnv) GetXlinkDialers() []xlink.Dialer {
	panic("implement me")
}

func (self *testEnv) GetCloseNotify() <-chan struct{} {
	return self.closeNotify
}

func (self *testEnv) GetLinkDialerPool() goroutines.Pool {
	panic("implement me")
}

func (self *testEnv) GetRateLimiterPool() goroutines.Pool {
	return self.rateLimiterPool
}

func (self *testEnv) GetMetricsRegistry() metrics.UsageRegistry {
	return self.metricsRegistry
}

type testLink struct {
	id     string
	key    string
	destId string
}

func (self *testLink) GetDestinationType() string {
	return "link"
}

func (self *testLink) Id() string {
	return self.id
}

func (self *testLink) SendPayload(payload *xgress.Payload, timeout time.Duration, payloadType xgress.PayloadType) error {
	panic("implement me")
}

func (self *testLink) SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error {
	panic("implement me")
}

func (self *testLink) SendControl(control *xgress.Control) error {
	panic("implement me")
}

func (self *testLink) InspectCircuit(circuitDetail *xgress.CircuitInspectDetail) {
	panic("implement me")
}

func (self *testLink) Key() string {
	return self.key
}

func (self *testLink) Init(metricsRegistry metrics.Registry) error {
	panic("implement me")
}

func (self *testLink) Close() error {
	panic("implement me")
}

func (self *testLink) CloseNotified() error {
	panic("implement me")
}

func (self *testLink) DestinationId() string {
	return self.destId
}

func (self *testLink) DestVersion() string {
	panic("implement me")
}

func (self *testLink) LinkProtocol() string {
	return "tls"
}

func (self *testLink) DialAddress() string {
	panic("implement me")
}

func (self *testLink) CloseOnce(f func()) {
	panic("implement me")
}

func (self *testLink) IsClosed() bool {
	panic("implement me")
}

func (self *testLink) InspectLink() *inspect.LinkInspectDetail {
	panic("implement me")
}

func (self *testLink) GetLinkConnState() *ctrl_pb.LinkConnState {
	panic("implement me")
}

func (self *testLink) IsDialed() bool {
	panic("implement me")
}

func (self *testLink) Iteration() uint32 {
	panic("implement me")
}

func (self *testLink) AreFaultsSent() bool {
	panic("implement me")
}

func (self *testLink) DuplicatesRejected() uint32 {
	panic("implement me")
}

func newTestLink(reg *linkRegistryImpl) *testLink {
	linkId := idgen.MustNewUUIDString()
	destId := idgen.MustNewUUIDString()
	linkKey := reg.GetLinkKey("", "tls", destId, "")
	return &testLink{
		id:     linkId,
		key:    linkKey,
		destId: destId,
	}
}

func newTestEnv() *testEnv {
	closeNotify := make(chan struct{})

	registryConfig := metrics.DefaultUsageRegistryConfig("test", closeNotify)
	metricsRegistry := metrics.NewUsageRegistry(registryConfig)

	ctrls := env.NewNetworkControllers(time.Second, func(address transport.Address, bindHandler channel.BindHandler) error {
		return errors.New("implement me")
	}, env.NewDefaultHeartbeatOptions())
	pool, err := goroutines.NewPool(goroutines.PoolConfig{
		QueueSize:      32,
		MinWorkers:     0,
		MaxWorkers:     2,
		IdleTime:       10 * time.Second,
		CloseNotify:    closeNotify,
		PanicHandler:   func(err interface{}) {},
		WorkerFunction: func(_ uint32, f func()) { f() },
	})
	if err != nil {
		panic(err)
	}
	return &testEnv{
		metricsRegistry: metricsRegistry,
		closeNotify:     closeNotify,
		ctrls:           ctrls,
		rateLimiterPool: pool,
	}

}

func Test_gcLinkMetrics(t *testing.T) {
	routerEnv := newTestEnv()
	defer close(routerEnv.closeNotify)

	reg := NewLinkRegistry(routerEnv).(*linkRegistryImpl)
	orphaned := reg.getOrphanedLinkMetrics()

	req := require.New(t)
	req.Equal(0, len(orphaned))

	linkId := idgen.MustNewUUIDString()
	registry := routerEnv.metricsRegistry

	linkMetric := func(linkId, name string) string {
		return "link." + linkId + "." + name
	}

	addLinkMetrics := func(linkId string) map[string]metrics.Metric {
		result := map[string]metrics.Metric{}
		addHist := func(name string) {
			result[linkMetric(linkId, name)] = registry.Histogram(linkMetric(linkId, name))
		}

		addMeter := func(name string) {
			result[linkMetric(linkId, name)] = registry.Meter(linkMetric(linkId, name))
		}

		addHist("latency")
		addHist("queue_time")
		addMeter("tx.bytesrate")
		addMeter("tx.msgrate")
		addHist("tx.msgsize")
		addMeter("rx.bytesrate")
		addMeter("rx.msgrate")
		addHist("rx.msgsize")

		metricId := "link.dropped_msgs:" + linkId
		result[metricId] = registry.Meter(metricId)

		return result
	}

	checkLinkMetrics := func(linkId string, m map[string]metrics.Metric, contains bool) {
		checkMetric := func(name string) {
			metricId := linkMetric(linkId, name)
			if contains {
				req.True(m[metricId] != nil, "missing metric %s", metricId)
			} else {
				req.False(m[metricId] != nil, "should not have metric %s", metricId)
			}
		}
		checkPostFixMetric := func(name string) {
			metricId := "link." + name + ":" + linkId
			if contains {
				req.True(m[metricId] != nil, "missing metric %s", metricId)
			} else {
				req.False(m[metricId] != nil, "should not have metric %s", metricId)
			}
		}
		checkMetric("latency")
		checkMetric("queue_time")
		checkMetric("tx.bytesrate")
		checkMetric("tx.msgrate")
		checkMetric("tx.msgsize")
		checkMetric("rx.bytesrate")
		checkMetric("rx.msgrate")
		checkMetric("rx.msgsize")
		checkPostFixMetric("dropped_msgs")
	}

	checkLinkMetricsContains := func(linkId string, m map[string]metrics.Metric) {
		checkLinkMetrics(linkId, m, true)
	}

	checkLinkMetricsDoesntHave := func(linkId string, m map[string]metrics.Metric) {
		checkLinkMetrics(linkId, m, false)
	}

	getRegistryMetrics := func() map[string]metrics.Metric {
		result := map[string]metrics.Metric{}
		registry.EachMetric(func(name string, metric metrics.Metric) {
			result[name] = metric
		})
		return result
	}

	l := addLinkMetrics(linkId)
	registry.Histogram("unrelated.to.links.hist")
	registry.Meter("unrelated.to.links.meter")

	orphaned = reg.getOrphanedLinkMetrics()
	req.Equal(len(l), len(orphaned))
	checkLinkMetricsContains(linkId, orphaned)

	orphaned = reg.gcLinkMetrics(nil)
	req.Equal(len(l), len(orphaned))
	checkLinkMetricsContains(linkId, orphaned)

	orphaned = reg.gcLinkMetrics(orphaned)
	req.Equal(0, len(orphaned))
	checkLinkMetricsDoesntHave(linkId, getRegistryMetrics())

	req.Equal(2, len(getRegistryMetrics()))

	linkId2 := idgen.MustNewUUIDString()
	link3 := newTestLink(reg)
	link4 := newTestLink(reg)
	linkId5 := idgen.MustNewUUIDString()

	reg.linkByIdMap[link3.id] = link3
	reg.linkMap[link3.Key()] = link4

	dest := newLinkDest(link4.DestinationId())
	reg.destinations[link4.DestinationId()] = dest
	dest.linkMap[link4.key] = &linkState{
		linkKey: link4.key,
		linkId:  link4.id,
		status:  StatusPending,
		dest:    dest,
	}

	addLinkMetrics(linkId2)
	addLinkMetrics(link3.id)
	addLinkMetrics(link4.id)
	addLinkMetrics(linkId5)

	req.Equal(9*4+2, len(getRegistryMetrics()))

	orphaned = reg.gcLinkMetrics(nil)
	req.Equal(18, len(orphaned))
	checkLinkMetricsContains(linkId2, orphaned)
	checkLinkMetricsContains(linkId5, orphaned)
	checkLinkMetricsDoesntHave(link3.id, orphaned)
	checkLinkMetricsDoesntHave(link4.id, orphaned)

	req.Equal(9*4+2, len(getRegistryMetrics()))

	orphaned = reg.gcLinkMetrics(orphaned)
	req.Equal(0, len(orphaned))
	req.Equal(9*2+2, len(getRegistryMetrics()))

	checkLinkMetricsContains(link3.id, getRegistryMetrics())
	checkLinkMetricsContains(link4.id, getRegistryMetrics())
	checkLinkMetricsDoesntHave(linkId2, getRegistryMetrics())
	checkLinkMetricsDoesntHave(linkId5, getRegistryMetrics())
}

// flakySendChannel is a channel.Channel double whose sends fail a set number of times before succeeding. It
// embeds the interface, so any method these tests do not exercise panics rather than returning a zero value.
type flakySendChannel struct {
	channel.Channel
	failures    atomic.Int32
	sends       atomic.Int32
	closed      atomic.Bool
	closeNotify chan struct{}
}

func newFlakySendChannel(failures int) *flakySendChannel {
	result := &flakySendChannel{closeNotify: make(chan struct{})}
	result.failures.Store(int32(failures))
	return result
}

func (self *flakySendChannel) Id() string     { return "ctrl1" }
func (self *flakySendChannel) Label() string  { return "ctrl1" }
func (self *flakySendChannel) IsClosed() bool { return self.closed.Load() }

func (self *flakySendChannel) CloseNotify() <-chan struct{} { return self.closeNotify }
func (self *flakySendChannel) Underlay() channel.Underlay   { return headerlessUnderlay{} }

func (self *flakySendChannel) Send(s channel.Sendable) error {
	if self.sends.Add(1) <= self.failures.Load() {
		return errors.New("timeout waiting for space in send queue")
	}
	// What the tx loop does once the message is actually written.
	s.SendListener().NotifyAfterWrite()
	return nil
}

// newReconnectTestRegistry builds a registry with no links and no event loop. An empty link set is a case
// worth covering rather than avoiding: the reconnect announcement is the only message that prunes, so a
// router with nothing to report still has to send one to clear stale controller state. Tests drive the
// loop's passes by hand.
func newReconnectTestRegistry(t *testing.T) (*linkRegistryImpl, *testEnv) {
	t.Helper()
	routerEnv := newTestEnv()
	t.Cleanup(func() { close(routerEnv.closeNotify) })
	return &linkRegistryImpl{
		env:              routerEnv,
		ctrls:            routerEnv.ctrls,
		destinations:     map[string]*linkDest{},
		linkMap:          map[string]xlink.Xlink{},
		events:           make(chan event, 16),
		triggerNotifyC:   make(chan struct{}, 1),
		ctrlSynchronizer: newCtrlSynchronizer(),
		// Production's pacing is not this test's concern; the timeout has to stay reachable, though, since it
		// is what distinguishes a discarded message from a delivered one.
		fullRefreshSendTimeout: 20 * time.Millisecond,
	}, routerEnv
}

// testCtrls is an env.NetworkControllers double exposing a fixed controller set.
type testCtrls struct {
	env.NetworkControllers
	all map[string]env.NetworkController
}

func (self *testCtrls) GetAll() map[string]env.NetworkController { return self.all }

// testCtrl is an env.NetworkController double over a test channel.
type testCtrl struct {
	env.NetworkController
	ch           channel.Channel
	connected    atomic.Bool
	sinceContact time.Duration
}

func (self *testCtrl) IsConnected() bool { return self.connected.Load() }

// Channel wraps the test channel so its underlay reports this controller's connectivity, which is how the
// incremental senders decide whether a controller is connected.
func (self *testCtrl) Channel() channel.Channel {
	return &testCtrlChannel{Channel: self.ch, ctrl: self}
}
func (self *testCtrl) TimeSinceLastContact() time.Duration { return self.sinceContact }

// withCtrl points the registry at a single connected controller reached over ch.
func withCtrl(reg *linkRegistryImpl, ctrlId string, ch channel.Channel) *testCtrl {
	ctrl := &testCtrl{ch: ch}
	ctrl.connected.Store(true)
	reg.ctrls = &testCtrls{all: map[string]env.NetworkController{ctrlId: ctrl}}
	return ctrl
}

// tickRefresh runs one loop pass of the refresh sender and waits for the attempt it queued, if any, to finish.
func tickRefresh(t *testing.T, reg *linkRegistryImpl, ctrlId string) {
	t.Helper()
	reg.notifyControllersOfRefresh()
	require.Eventually(t, func() bool { return !reg.ctrlSynchronizer.isRefreshInFlight(ctrlId) }, time.Second, time.Millisecond)
}

// markRefreshed records a written refresh for ctrlId without sending one.
func markRefreshed(t *testing.T, reg *linkRegistryImpl, ctrlId string) {
	t.Helper()
	gen, ok := reg.ctrlSynchronizer.beginRefresh(ctrlId)
	require.True(t, ok)
	require.True(t, reg.ctrlSynchronizer.markRefreshWritten(ctrlId, gen))
}

// Test_NotifyOfReconnect_RetriesUntilTheAnnouncementLands covers the retry. A router announces its full link
// set once per controller reconnect and nothing re-asks, so an announcement lost to send-queue back-pressure
// leaves that controller unable to route over the router's links, and unable to prune the ones it should have
// dropped, until the next reconnect. Once written, the controller is synced.
func Test_NotifyOfReconnect_RetriesUntilTheAnnouncementLands(t *testing.T) {
	reg, _ := newReconnectTestRegistry(t)
	ch := newFlakySendChannel(2)
	withCtrl(reg, "ctrl1", ch)

	reg.NotifyOfReconnect(ch)
	require.False(t, reg.ctrlSynchronizer.isSynced("ctrl1"))

	for i := 0; i < 3; i++ {
		tickRefresh(t, reg, "ctrl1")
	}

	require.Equal(t, int32(3), ch.sends.Load(), "the announcement should have been retried until it reached the wire")
	require.True(t, reg.ctrlSynchronizer.isSynced("ctrl1"))

	tickRefresh(t, reg, "ctrl1")
	require.Equal(t, int32(3), ch.sends.Load(), "a written announcement is not sent again")
}

// Test_NotifyOfReconnect_KeepsRetryingWhileTheDebtStands: the retry is not bounded by a count. A channel that
// never drains is retried every pass, with the controller held unsynced, for as long as it is connected.
func Test_NotifyOfReconnect_KeepsRetryingWhileTheDebtStands(t *testing.T) {
	reg, _ := newReconnectTestRegistry(t)
	ch := newFlakySendChannel(math.MaxInt32)
	ctrl := withCtrl(reg, "ctrl1", ch)

	reg.NotifyOfReconnect(ch)
	for i := 0; i < 6; i++ {
		tickRefresh(t, reg, "ctrl1")
	}
	require.Equal(t, int32(6), ch.sends.Load(), "the announcement should be retried on every pass")
	require.False(t, reg.ctrlSynchronizer.isSynced("ctrl1"), "the controller stays unsynced until the announcement lands")

	ctrl.connected.Store(false)
	tickRefresh(t, reg, "ctrl1")
	require.Equal(t, int32(6), ch.sends.Load(), "a disconnected controller is left owed rather than retried")
	require.False(t, reg.ctrlSynchronizer.isSynced("ctrl1"))
}

// acceptThenDiscardChannel models what a real channel does to a message queued behind a backlog: the send is
// accepted, and the message is dropped when its deadline expires before the queue drains. Nothing calls back,
// because a plain send's listener ignores the error.
type acceptThenDiscardChannel struct {
	channel.Channel
	sends       atomic.Int32
	closeNotify chan struct{}
}

func (self *acceptThenDiscardChannel) Id() string                   { return "ctrl1" }
func (self *acceptThenDiscardChannel) Label() string                { return "ctrl1" }
func (self *acceptThenDiscardChannel) IsClosed() bool               { return false }
func (self *acceptThenDiscardChannel) CloseNotify() <-chan struct{} { return self.closeNotify }
func (self *acceptThenDiscardChannel) Underlay() channel.Underlay   { return headerlessUnderlay{} }
func (self *acceptThenDiscardChannel) Send(channel.Sendable) error {
	self.sends.Add(1)
	return nil // queued, and never written
}

// Test_NotifyOfReconnect_TreatsADiscardedAnnouncementAsFailure is the guard on how the announcement is sent.
// A plain Send returns once the message is queued, and the deadline stays live afterwards, so the tx loop can
// drop it with nobody told: a plain send's listener ignores the error. Reporting that as success marks the
// links announced and leaves the controller permanently without them.
func Test_NotifyOfReconnect_TreatsADiscardedAnnouncementAsFailure(t *testing.T) {
	reg, _ := newReconnectTestRegistry(t)
	ch := &acceptThenDiscardChannel{closeNotify: make(chan struct{})}
	withCtrl(reg, "ctrl1", ch)

	reg.NotifyOfReconnect(ch)
	for i := 0; i < 3; i++ {
		tickRefresh(t, reg, "ctrl1")
	}

	require.Equal(t, int32(3), ch.sends.Load(),
		"an announcement accepted into the queue but never written must count as a failure and be retried")
	require.False(t, reg.ctrlSynchronizer.isSynced("ctrl1"))
}

// Test_NotifyOfReconnect_OneAnnouncementInFlightPerController: a pass does not queue a second announcement
// for a controller whose previous one has not finished.
func Test_NotifyOfReconnect_OneAnnouncementInFlightPerController(t *testing.T) {
	reg, _ := newReconnectTestRegistry(t)
	ch := newFlakySendChannel(0)
	withCtrl(reg, "ctrl1", ch)

	reg.NotifyOfReconnect(ch)
	_, ok := reg.ctrlSynchronizer.beginRefresh("ctrl1") // an announcement is in flight
	require.True(t, ok)

	reg.notifyControllersOfRefresh()
	require.Equal(t, int32(0), ch.sends.Load(), "no second announcement while one is in flight")

	reg.ctrlSynchronizer.markRefreshFailed("ctrl1")
	tickRefresh(t, reg, "ctrl1")
	require.Equal(t, int32(1), ch.sends.Load())
	require.True(t, reg.ctrlSynchronizer.isSynced("ctrl1"))
}

// Test_NotifyOfReconnect_ReconnectDuringAnnouncementLeavesItOwed: an announcement built before a further
// reconnect describes a channel state since replaced, so landing it does not sync the controller; the next
// pass announces again.
func Test_NotifyOfReconnect_ReconnectDuringAnnouncementLeavesItOwed(t *testing.T) {
	reg, _ := newReconnectTestRegistry(t)
	ch := newFlakySendChannel(0)
	withCtrl(reg, "ctrl1", ch)

	reg.NotifyOfReconnect(ch)
	gen, ok := reg.ctrlSynchronizer.beginRefresh("ctrl1")
	require.True(t, ok)
	reg.NotifyOfReconnect(ch) // reconnect while the announcement for gen is in flight

	reg.sendFullRefresh("ctrl1", ch, gen)
	require.Equal(t, int32(1), ch.sends.Load())
	require.False(t, reg.ctrlSynchronizer.isSynced("ctrl1"), "the stale announcement does not satisfy the later reconnect")

	tickRefresh(t, reg, "ctrl1")
	require.Equal(t, int32(2), ch.sends.Load(), "one more announcement follows the later reconnect")
	require.True(t, reg.ctrlSynchronizer.isSynced("ctrl1"))
}

// Test_sendNewLinks_HeldUntilTheReconnectAnnouncementIsWritten covers the gate. A new-link report that reaches
// a controller ahead of the full refresh is pruned by it, and a fault that arrives ahead of it re-adds a link
// the refresh omitted, so incremental reports to an unsynced controller stay pending rather than being sent
// or marked done.
func Test_sendNewLinks_HeldUntilTheReconnectAnnouncementIsWritten(t *testing.T) {
	reg, _ := newReconnectTestRegistry(t)
	ch := newFlakySendChannel(0)
	withCtrl(reg, "ctrl1", ch)
	link := &reportTestLink{id: "l1", destId: "peer"}
	report := []stateAndLink{{state: &linkState{linkId: link.Id()}, link: link}}

	reg.ctrlSynchronizer.markReconnected("ctrl1")
	reg.sendNewLinks(report)
	require.Equal(t, int32(0), ch.sends.Load(), "reports to an unsynced controller are held")
	require.Empty(t, reg.events, "a held report is not marked as notified")

	markRefreshed(t, reg, "ctrl1")
	reg.sendNewLinks(report)
	require.Equal(t, int32(1), ch.sends.Load(), "reports flow once the announcement is written")
	require.Len(t, reg.events, 1, "a delivered report is marked as notified")
}

// Test_sendLinkFaults_HeldUntilTheReconnectAnnouncementIsWritten is the fault half of the gate.
func Test_sendLinkFaults_HeldUntilTheReconnectAnnouncementIsWritten(t *testing.T) {
	reg, _ := newReconnectTestRegistry(t)
	ch := newFlakySendChannel(0)
	withCtrl(reg, "ctrl1", ch)
	faults := []stateAndFaults{{state: &linkState{}, faults: []linkFault{{linkId: "l1", iteration: 1}}}}

	reg.ctrlSynchronizer.markReconnected("ctrl1")
	reg.sendLinkFaults(faults)
	require.Equal(t, int32(0), ch.sends.Load())
	require.Empty(t, reg.events)

	markRefreshed(t, reg, "ctrl1")
	reg.sendLinkFaults(faults)
	require.Equal(t, int32(1), ch.sends.Load())
	require.Len(t, reg.events, 1)
}

// Test_linkReports_DisconnectedUnsyncedController covers a controller that owes a refresh but is not
// connected. The sync gate applies only to connected controllers, so it keeps the handling it always had:
// new-link reports skip it, since the refresh on reconnect covers them, and faults are held through a brief
// outage and retired for it after a long one. Holding either for the whole outage would re-send them to the
// healthy controllers on every tick.
func Test_linkReports_DisconnectedUnsyncedController(t *testing.T) {
	newReg := func(sinceContact time.Duration) (*linkRegistryImpl, *flakySendChannel) {
		reg, _ := newReconnectTestRegistry(t)
		ch := newFlakySendChannel(0)
		ctrl := withCtrl(reg, "ctrl1", ch)
		ctrl.connected.Store(false)
		ctrl.sinceContact = sinceContact
		reg.ctrlSynchronizer.markReconnected("ctrl1")
		return reg, ch
	}

	t.Run("new link reports are retired", func(t *testing.T) {
		reg, ch := newReg(time.Hour)
		link := &reportTestLink{id: "l1", destId: "peer"}
		reg.sendNewLinks([]stateAndLink{{state: &linkState{linkId: link.Id()}, link: link}})
		require.Equal(t, int32(0), ch.sends.Load())
		require.Len(t, reg.events, 1, "the report is marked notified")
	})

	t.Run("faults are held through a brief outage", func(t *testing.T) {
		reg, ch := newReg(time.Second)
		reg.sendLinkFaults([]stateAndFaults{{state: &linkState{}, faults: []linkFault{{linkId: "l1", iteration: 1}}}})
		require.Equal(t, int32(0), ch.sends.Load())
		require.Empty(t, reg.events)
	})

	t.Run("faults are retired after a long outage", func(t *testing.T) {
		reg, ch := newReg(time.Hour)
		reg.sendLinkFaults([]stateAndFaults{{state: &linkState{}, faults: []linkFault{{linkId: "l1", iteration: 1}}}})
		require.Equal(t, int32(0), ch.sends.Load())
		require.Len(t, reg.events, 1, "the fault is marked notified")
	})
}

// reportTestLink is an xlink.Xlink double carrying only what a new-link report reads.
type reportTestLink struct {
	xlink.Xlink
	id     string
	destId string
}

func (self *reportTestLink) Id() string                               { return self.id }
func (self *reportTestLink) DestinationId() string                    { return self.destId }
func (self *reportTestLink) LinkProtocol() string                     { return "tls" }
func (self *reportTestLink) DialAddress() string                      { return "tls:peer:6000" }
func (self *reportTestLink) Iteration() uint32                        { return 1 }
func (self *reportTestLink) IsDialed() bool                           { return true }
func (self *reportTestLink) GetLinkConnState() *ctrl_pb.LinkConnState { return nil }

// headerlessUnderlay is a channel.Underlay double with no hello headers, as from a controller advertising no
// capabilities.
type headerlessUnderlay struct {
	channel.Underlay
}

func (headerlessUnderlay) Headers() map[int32][]byte { return nil }

// testCtrlChannel is a channel whose underlay reports its test controller's connectivity.
type testCtrlChannel struct {
	channel.Channel
	ctrl *testCtrl
}

func (self *testCtrlChannel) Underlay() channel.Underlay {
	return connectivityUnderlay{connected: self.ctrl.connected.Load()}
}

// connectivityUnderlay is a headerless channel.Underlay double reporting a fixed connectivity.
type connectivityUnderlay struct {
	headerlessUnderlay
	connected bool
}

func (self connectivityUnderlay) IsConnected() bool { return self.connected }

// unaccountedTestLink is an xlink.Xlink double for a link the registry may have no state for.
type unaccountedTestLink struct {
	xlink.Xlink
	id     string
	destId string
	dialed bool
	closed bool
}

func (self *unaccountedTestLink) Id() string            { return self.id }
func (self *unaccountedTestLink) Key() string           { return "key-" + self.id }
func (self *unaccountedTestLink) DestinationId() string { return self.destId }
func (self *unaccountedTestLink) IsDialed() bool        { return self.dialed }
func (self *unaccountedTestLink) IsClosed() bool        { return self.closed }
func (self *unaccountedTestLink) Close() error          { self.closed = true; return nil }

// Test_LinkRegistry_UnaccountedDialedLinkIsClosed: a dial that completes after its destination has been
// removed cannot be recorded. Only warning left the link up and unreported, while the far end kept it and
// used its id to refuse the dialer's later attempts for the same pair.
func Test_LinkRegistry_UnaccountedDialedLinkIsClosed(t *testing.T) {
	req := require.New(t)
	reg := &linkRegistryImpl{destinations: map[string]*linkDest{}}

	link := &unaccountedTestLink{id: "orphan", dialed: true, destId: "removed-dest"}
	(&updateLinkStatusForLink{link: link, status: StatusEstablished}).Handle(reg)
	req.True(link.closed, "a dialed link with no destination to belong to must be closed, not just logged")

	dest := newLinkDest("known-dest")
	reg.destinations["known-dest"] = dest
	link = &unaccountedTestLink{id: "stateless", dialed: true, destId: "known-dest"}
	(&updateLinkStatusForLink{link: link, status: StatusEstablished}).Handle(reg)
	req.True(link.closed, "a dialed link with no dial state must be closed, not just logged")
}

// Test_LinkRegistry_UnaccountedAcceptedLinkIsLeftOpen: the listener side keeps no state for a link it
// accepted, so absence there is normal and says nothing about whether the link is wanted.
func Test_LinkRegistry_UnaccountedAcceptedLinkIsLeftOpen(t *testing.T) {
	req := require.New(t)
	reg := &linkRegistryImpl{destinations: map[string]*linkDest{}}

	link := &unaccountedTestLink{id: "accepted", dialed: false, destId: "no-state-here"}
	(&updateLinkStatusForLink{link: link, status: StatusEstablished}).Handle(reg)

	req.False(link.closed, "an accepted link must not be closed for having no dial state")
}

// Test_LinkRegistry_RemovedDestIsNotDialed: removing a destination leaves its linkMap intact, so a state
// still sitting in the dial queue looked live and got dialed anyway. That dial reaches whoever now answers
// the address, and its result has no state to be recorded against.
func Test_LinkRegistry_RemovedDestIsNotDialed(t *testing.T) {
	req := require.New(t)
	tenv := newTestEnv()
	defer close(tenv.closeNotify)

	reg := &linkRegistryImpl{
		linkMap:        map[string]xlink.Xlink{},
		linkByIdMap:    map[string]xlink.Xlink{},
		ctrls:          tenv.ctrls,
		env:            tenv,
		destinations:   map[string]*linkDest{},
		linkStateQueue: &linkStateHeap{},
		events:         make(chan event, 16),
		triggerNotifyC: make(chan struct{}, 1),
	}
	dest := newLinkDest("doomed-dest")
	reg.destinations[dest.id] = dest
	state := &linkState{
		linkKey:      "k",
		linkId:       "l1",
		status:       StatusPending,
		dest:         dest,
		listener:     &ctrl_pb.Listener{Address: "tls:peer:6000", Protocol: "tls"},
		allowedDials: -1,
	}
	dest.linkMap["k"] = state

	(&removeLinkDest{id: dest.id}).Handle(reg)

	// Queue it as the only due entry: scheduled to dial, with its destination removed out from under it.
	state.nextDial = time.Now().Add(-time.Minute)
	*reg.linkStateQueue = linkStateHeap{}
	heap.Push(reg.linkStateQueue, state)
	req.NotPanics(reg.evaluateLinkStateQueue)

	req.Equal(StatusDestRemoved, state.status, "a removed destination's link state must be left alone, not dialed")
}
