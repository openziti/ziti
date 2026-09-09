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

package forwarder

import (
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func Test_ReportEndpointFault(t *testing.T) {
	t.Run("a reported fault is delivered", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, ctrls := newTestFaulter(t, time.Minute)

		// act
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

		// assert
		fault := takeFault(t, ctrls)
		req.Equal("circuit-1", fault.Id)
		req.Equal(ctrl_pb.FaultSubject_IngressFault, fault.Subject)
		requirePendingCount(t, f, "ctrl-1", 0, "a delivered fault must not stay pending")
	})

	// Nothing else reports an endpoint that has gone away, so a fault that can't be sent is kept
	// rather than lost.
	t.Run("a fault is retained while its controller is unavailable", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, ctrls := newTestFaulter(t, time.Minute)
		ctrls.unavailable.Store(true)

		// act
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

		// assert
		select {
		case <-ctrls.lookups:
		case <-time.After(5 * time.Second):
			req.FailNow("the sender never looked for its controller")
		}
		req.Equal(1, pendingCount(f, "ctrl-1"), "an unsendable fault must be retained")

		t.Run("and sent once the controller is back", func(t *testing.T) {
			req := require.New(t)

			// arrange
			ctrls.unavailable.Store(false)
			sender, found := f.endpointFaultSenders.Get("ctrl-1")
			req.True(found)

			// act: the controller is announced, which wakes its sender rather than waiting for the sweep
			sender.wake()

			// assert
			fault := takeFault(t, ctrls)
			req.Equal("circuit-1", fault.Id)
		})
	})

	t.Run("repeat reports of one fault collapse", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, ctrls := newTestFaulter(t, time.Minute)
		ctrls.unavailable.Store(true)

		// act
		for range 20 {
			f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)
		}

		// assert
		req.Equal(1, pendingCount(f, "ctrl-1"), "repeat reports of one fault must collapse")
	})

	// A router holding both ends of a circuit reports two distinct faults for it, and neither may
	// displace the other.
	t.Run("ingress and egress faults for one circuit are distinct", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, ctrls := newTestFaulter(t, time.Minute)
		ctrls.unavailable.Store(true)

		// act
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_EgressFault)

		// assert
		req.Equal(2, pendingCount(f, "ctrl-1"), "ingress and egress faults for one circuit are distinct")
	})

	// A fault outlives the circuit it names, so a controller that stays unreachable would otherwise
	// accumulate one entry per teardown for the length of the outage.
	t.Run("a fault held past retention is given up", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, ctrls := newTestFaulter(t, time.Millisecond)
		f.endpointFaultSweepInterval = time.Millisecond
		ctrls.unavailable.Store(true)

		// act
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

		// assert
		requireMeterCount(t, f.endpointFaultsExpired, 1, "giving up on a fault must be metered")
		req.Equal(0, pendingCount(f, "ctrl-1"), "a fault held past retention must be given up")
	})

	// Every report wakes the sender, so expiring on wakes would rescan the whole set once per fault
	// reported during an outage.
	t.Run("waking the sender does not expire faults ahead of the sweep", func(t *testing.T) {
		req := require.New(t)

		// arrange: retention is short, but the sweep is far off
		f, ctrls := newTestFaulter(t, time.Millisecond)
		ctrls.unavailable.Store(true)
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)
		sender, found := f.endpointFaultSenders.Get("ctrl-1")
		req.True(found)

		// act: each time the sender goes round looking for its controller, wake it again, for well
		// past the retention window
		deadline := time.Now().Add(100 * time.Millisecond)
		for time.Now().Before(deadline) {
			select {
			case <-ctrls.lookups:
			case <-time.After(5 * time.Second):
				req.FailNow("the sender stopped going round when woken")
			}
			sender.wake()
		}

		// assert
		req.Equal(1, pendingCount(f, "ctrl-1"), "a wake must not expire faults ahead of the sweep")
	})

	t.Run("reporting does not block on an unavailable controller", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, ctrls := newTestFaulter(t, time.Minute)
		ctrls.unavailable.Store(true)
		done := make(chan struct{})

		// act
		go reportFaults(f, 500, done)

		// assert
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			req.Fail("reporting a fault blocked on an unavailable controller")
		}
	})

	t.Run("a fault with no controller is dropped", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, _ := newTestFaulter(t, time.Minute)

		// act
		f.ReportEndpointFault("circuit-1", "", ctrl_pb.FaultSubject_IngressFault)

		// assert
		req.Zero(f.endpointFaultSenders.Count(), "with nothing to send to, no sender is created")
	})

	// A controller that has left the cluster has no circuits left to clean up.
	t.Run("removing a controller abandons its pending faults", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, ctrls := newTestFaulter(t, time.Minute)
		ctrls.unavailable.Store(true)
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)
		sender, found := f.endpointFaultSenders.Get("ctrl-1")
		req.True(found)

		// act
		f.notifyOfCtrlEvent(env.CtrlEvent{
			Type:       env.ControllerRemoved,
			Controller: &idOnlyController{ch: &idOnlyChannel{id: "ctrl-1"}},
		})

		// assert: the sender is gone, and stays quiet even if the controller comes back and it is woken
		req.Zero(f.endpointFaultSenders.Count(), "a removed controller's sender must be dropped")
		ctrls.unavailable.Store(false)
		sender.wake()
		select {
		case fault := <-ctrls.ctrlCh.written:
			req.Failf("an abandoned fault was sent", "circuit %s", fault.Id)
		case <-time.After(100 * time.Millisecond):
		}
	})

	t.Run("shutting a sender down twice is harmless", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, ctrls := newTestFaulter(t, time.Minute)
		ctrls.unavailable.Store(true)
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)
		sender, found := f.endpointFaultSenders.Get("ctrl-1")
		req.True(found)
		sender.shutdown()

		// act and assert
		req.NotPanics(sender.shutdown, "a second shutdown must do nothing")
	})

	// The sender declines to retry a failed write, so treating queue acceptance as delivery would
	// drop the fault while reporting success.
	t.Run("a fault whose write fails is retained", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, ctrls := newTestFaulter(t, time.Minute)
		ctrls.ctrlCh.failWrites(errors.New("underlay write failed"))

		// act
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

		// assert: what is checked is the state after a failed write, not before one
		select {
		case <-ctrls.ctrlCh.failed:
		case <-time.After(5 * time.Second):
			req.FailNow("the fault was never written")
		}
		req.Equal(1, pendingCount(f, "ctrl-1"), "a fault whose write failed must be retained")
		req.Zero(f.endpointFaultsSent.Count(), "a failed write is not a delivery")

		t.Run("and sent once writes recover", func(t *testing.T) {
			req := require.New(t)

			// arrange
			ctrls.ctrlCh.succeedWrites()
			sender, found := f.endpointFaultSenders.Get("ctrl-1")
			req.True(found)

			// act
			sender.wake()

			// assert
			fault := takeFault(t, ctrls)
			req.Equal("circuit-1", fault.Id)
			requirePendingCount(t, f, "ctrl-1", 0, "a delivered fault must not stay pending")
			req.Equal(int64(1), f.endpointFaultsSent.Count(), "the delivery must be metered")
		})
	})

	// The sender both delivers and expires, so a send that waited indefinitely would stop expiry
	// while new faults kept arriving.
	t.Run("retention runs while the controller takes nothing", func(t *testing.T) {
		req := require.New(t)

		// arrange
		f, ctrls := newTestFaulter(t, time.Millisecond)
		f.endpointFaultSweepInterval = time.Millisecond
		ctrls.ctrlCh.failWrites(errors.New("underlay write failed"))

		// act
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

		// assert
		requireMeterCount(t, f.endpointFaultsExpired, 1, "retention must run even while sends are failing")
		req.Equal(0, pendingCount(f, "ctrl-1"))
		req.Zero(f.endpointFaultsSent.Count(), "the fault must leave by expiring, not by being counted as sent")
	})
}

// takeFault returns the next fault the underlay wrote, failing the test if none is written within
// five seconds.
func takeFault(t *testing.T, ctrls *testControllers) *ctrl_pb.Fault {
	t.Helper()
	select {
	case fault := <-ctrls.ctrlCh.written:
		return fault
	case <-time.After(5 * time.Second):
		require.FailNow(t, "no fault was written")
		return nil
	}
}

// requirePendingCount fails the test unless f's sender for ctrlId holds want faults within five
// seconds.
func requirePendingCount(t *testing.T, f *Faulter, ctrlId string, want int, msg string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return pendingCount(f, ctrlId) == want
	}, 5*time.Second, time.Millisecond, msg)
}

// requireMeterCount fails the test unless meter counts want within five seconds.
func requireMeterCount(t *testing.T, meter metrics.Meter, want int64, msg string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return meter.Count() == want
	}, 5*time.Second, time.Millisecond, msg)
}

// pendingCount returns the number of faults f's sender for ctrlId holds, or zero if there is no
// such sender.
func pendingCount(f *Faulter, ctrlId string) int {
	sender, found := f.endpointFaultSenders.Get(ctrlId)
	if !found {
		return 0
	}
	sender.lock.Lock()
	defer sender.lock.Unlock()
	return len(sender.pending)
}

// reportFaults reports an ingress fault to ctrl-1 for each of count circuits, then closes done.
func reportFaults(f *Faulter, count int, done chan<- struct{}) {
	defer close(done)
	for i := range count {
		f.ReportEndpointFault("circuit-"+strconv.Itoa(i), "ctrl-1", ctrl_pb.FaultSubject_IngressFault)
	}
}

// newTestFaulter returns a Faulter that keeps faults for retention, delivering them to a single
// testCtrlChannel through the returned testControllers. It sweeps at the production interval;
// tests that need expiry to run promptly shorten endpointFaultSweepInterval before reporting.
func newTestFaulter(t *testing.T, retention time.Duration) (*Faulter, *testControllers) {
	t.Helper()

	registry := metrics.NewRegistry("test", nil)
	ctrls := &testControllers{
		ctrlCh:  newTestCtrlChannel(t),
		lookups: make(chan struct{}, 1024),
	}

	return &Faulter{
		ctrls:                      ctrls,
		closeNotify:                make(chan struct{}),
		endpointFaultSenders:       cmap.New[*endpointFaultSender](),
		endpointFaultRetention:     retention,
		endpointFaultSweepInterval: endpointFaultSweepInterval,
		circuitFaults:              registry.Meter("faults.circuit"),
		endpointFaultsSent:         registry.Meter("faults.circuit.endpoint"),
		endpointFaultsExpired:      registry.Meter("faults.circuit.endpoint.expired"),
	}, ctrls
}

// testControllers hands out one controller channel, or none when unavailable is set, standing in
// for a controller that is not connected. Each lookup of a controller channel signals lookups,
// dropping the signal if lookups is full.
type testControllers struct {
	env.MockNetworkControllers
	ctrlCh  *testCtrlChannel
	lookups chan struct{}
	// read by the sender goroutine while the test flips it
	unavailable atomic.Bool
}

func (self *testControllers) GetCtrlChannel(string) ctrlchan.CtrlChannel {
	select {
	case self.lookups <- struct{}{}:
	default:
	}
	if self.unavailable.Load() {
		return nil
	}
	return self.ctrlCh
}

// testCtrlChannel completes ctrlchan.CtrlChannel over BaseCtrlChannel and runs a goroutine in
// place of an underlay, so that sends waiting for a write actually complete. Successful writes are
// delivered on written. After failWrites, writes fail the way the channel reports a failed write,
// NotifyErr then NotifyAfterWrite, and each failure is signalled on failed, dropping the signal if
// failed is full.
type testCtrlChannel struct {
	*ctrlchan.BaseCtrlChannel
	writeErr atomic.Pointer[error]
	written  chan *ctrl_pb.Fault
	failed   chan struct{}
	notifier *channel.CloseNotifier
}

// newTestCtrlChannel returns a testCtrlChannel whose underlay goroutine stops when the test ends.
func newTestCtrlChannel(t *testing.T) *testCtrlChannel {
	t.Helper()

	result := &testCtrlChannel{
		BaseCtrlChannel: ctrlchan.NewBaseCtrlChannel(),
		written:         make(chan *ctrl_pb.Fault, 1024),
		failed:          make(chan struct{}, 1024),
		notifier:        channel.NewCloseNotifier(),
	}

	go result.runUnderlay()
	t.Cleanup(result.notifier.NotifyClosed)

	return result
}

func (self *testCtrlChannel) IsConnected() bool {
	return true
}

func (self *testCtrlChannel) Reconnect() error {
	return nil
}

// failWrites makes later writes fail with err.
func (self *testCtrlChannel) failWrites(err error) {
	self.writeErr.Store(&err)
}

// succeedWrites makes later writes succeed.
func (self *testCtrlChannel) succeedWrites() {
	self.writeErr.Store(nil)
}

func (self *testCtrlChannel) runUnderlay() {
	for {
		sendable, err := self.GetNextMsgDefault(self.notifier)
		if err != nil || sendable == nil {
			return
		}

		listener := sendable.SendListener()
		listener.NotifyBeforeWrite()

		if writeErr := self.writeErr.Load(); writeErr != nil {
			// how a failed underlay write is reported once HandleTxFailed declines a retry
			listener.NotifyErr(*writeErr)
			listener.NotifyAfterWrite()
			select {
			case self.failed <- struct{}{}:
			default:
			}
			continue
		}

		fault := &ctrl_pb.Fault{}
		if err = proto.Unmarshal(sendable.Msg().Body, fault); err == nil {
			self.written <- fault
		}
		listener.NotifyAfterWrite()
	}
}

// idOnlyController is a NetworkController that serves only its channel, which is all the faulter
// reads from a controller event. Any other method panics through the nil embedded interface.
type idOnlyController struct {
	env.NetworkController
	ch channel.Channel
}

func (self *idOnlyController) Channel() channel.Channel {
	return self.ch
}

// idOnlyChannel is a channel.Channel that serves only its id. Any other method panics through the
// nil embedded interface.
type idOnlyChannel struct {
	channel.Channel
	id string
}

func (self *idOnlyChannel) Id() string {
	return self.id
}
