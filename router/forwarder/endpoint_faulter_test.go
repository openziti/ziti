package forwarder

import (
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// testCtrlChannel completes ctrlchan.CtrlChannel over BaseCtrlChannel and runs a goroutine in
// place of an underlay, so that sends waiting for a write actually complete. Setting writeErr
// makes those writes fail the way channel v5.0.27 reports it: NotifyErr, then NotifyAfterWrite.
type testCtrlChannel struct {
	*ctrlchan.BaseCtrlChannel
	writeErr atomic.Pointer[error]
	written  chan *ctrl_pb.Fault
	attempts atomic.Int32
	notifier *channel.CloseNotifier
}

func newTestCtrlChannel(t *testing.T) *testCtrlChannel {
	result := &testCtrlChannel{
		BaseCtrlChannel: ctrlchan.NewBaseCtrlChannel(),
		written:         make(chan *ctrl_pb.Fault, 1024),
		notifier:        channel.NewCloseNotifier(),
	}

	go result.runUnderlay()
	t.Cleanup(result.notifier.NotifyClosed)

	return result
}

func (self *testCtrlChannel) IsConnected() bool {
	return true
}

func (self *testCtrlChannel) failWrites(err error) {
	self.writeErr.Store(&err)
}

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
		self.attempts.Add(1)

		if writeErr := self.writeErr.Load(); writeErr != nil {
			// how a failed underlay write is reported once HandleTxFailed declines a retry
			listener.NotifyErr(*writeErr)
			listener.NotifyAfterWrite()
			continue
		}

		fault := &ctrl_pb.Fault{}
		if err = proto.Unmarshal(sendable.Msg().Body, fault); err == nil {
			self.written <- fault
		}
		listener.NotifyAfterWrite()
	}
}

// testControllers hands out one controller channel, or none when unavailable is set, standing in
// for a controller that is not connected.
type testControllers struct {
	env.MockNetworkControllers
	ctrlCh *testCtrlChannel
	// read by the sender goroutine while the test flips it
	unavailable atomic.Bool
}

func (self *testControllers) GetCtrlChannel(string) ctrlchan.CtrlChannel {
	if self.unavailable.Load() {
		return nil
	}
	return self.ctrlCh
}

func newTestFaulter(t *testing.T, retention time.Duration) (*Faulter, *testControllers) {
	registry := metrics.NewRegistry("test", nil)
	ctrls := &testControllers{ctrlCh: newTestCtrlChannel(t)}

	return &Faulter{
		ctrls:                  ctrls,
		closeNotify:            make(chan struct{}),
		endpointFaultSenders:   cmap.New[*endpointFaultSender](),
		endpointFaultRetention: retention,
		circuitFaults:          registry.Meter("faults.circuit"),
		endpointFaultsSent:     registry.Meter("faults.circuit.endpoint"),
		endpointFaultsExpired:  registry.Meter("faults.circuit.endpoint.expired"),
	}, ctrls
}

// takeFault waits for the next fault the underlay wrote.
func takeFault(req *require.Assertions, ctrls *testControllers) *ctrl_pb.Fault {
	select {
	case fault := <-ctrls.ctrlCh.written:
		return fault
	case <-time.After(5 * time.Second):
		req.Fail("no fault was written")
		return nil
	}
}

func pendingCount(f *Faulter, ctrlId string) int {
	sender, found := f.endpointFaultSenders.Get(ctrlId)
	if !found {
		return 0
	}
	sender.lock.Lock()
	defer sender.lock.Unlock()
	return len(sender.pending)
}

func Test_EndpointFault_Delivered(t *testing.T) {
	req := require.New(t)
	f, ctrls := newTestFaulter(t, time.Minute)

	f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

	fault := takeFault(req, ctrls)
	req.Equal("circuit-1", fault.Id)
	req.Equal(ctrl_pb.FaultSubject_IngressFault, fault.Subject)

	req.Eventually(func() bool {
		return pendingCount(f, "ctrl-1") == 0
	}, 5*time.Second, time.Millisecond, "a delivered fault must not stay pending")
}

// Test_EndpointFault_RetainedWhileControllerUnavailable is the point of the change: a fault that
// cannot be sent is kept rather than lost, since nothing else will report it again.
func Test_EndpointFault_RetainedWhileControllerUnavailable(t *testing.T) {
	req := require.New(t)
	f, ctrls := newTestFaulter(t, time.Minute)
	ctrls.unavailable.Store(true)

	f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

	req.Eventually(func() bool {
		return pendingCount(f, "ctrl-1") == 1
	}, 5*time.Second, time.Millisecond, "an unsendable fault must be retained")

	// the controller comes back and is announced; the retained fault goes out without waiting for
	// the sweep
	ctrls.unavailable.Store(false)
	sender, _ := f.endpointFaultSenders.Get("ctrl-1")
	sender.wake()

	fault := takeFault(req, ctrls)
	req.Equal("circuit-1", fault.Id)
}

func Test_EndpointFault_DuplicatesCollapse(t *testing.T) {
	req := require.New(t)
	f, ctrls := newTestFaulter(t, time.Minute)
	ctrls.unavailable.Store(true)

	for range 20 {
		f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)
	}

	req.Eventually(func() bool {
		return pendingCount(f, "ctrl-1") == 1
	}, 5*time.Second, time.Millisecond, "repeat reports of one fault must collapse")
}

// Test_EndpointFault_SubjectIsPartOfTheKey covers a router holding both ends of a circuit: those
// are two distinct faults and neither may displace the other.
func Test_EndpointFault_SubjectIsPartOfTheKey(t *testing.T) {
	req := require.New(t)
	f, ctrls := newTestFaulter(t, time.Minute)
	ctrls.unavailable.Store(true)

	f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)
	f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_EgressFault)

	req.Eventually(func() bool {
		return pendingCount(f, "ctrl-1") == 2
	}, 5*time.Second, time.Millisecond, "ingress and egress faults for one circuit are distinct")
}

// Test_EndpointFault_ExpiresAfterRetention covers the memory bound: a fault outlives the circuit
// it names, so a controller that stays unreachable would otherwise accumulate one entry per
// teardown for the length of the outage.
func Test_EndpointFault_ExpiresAfterRetention(t *testing.T) {
	req := require.New(t)
	f, ctrls := newTestFaulter(t, time.Millisecond)
	ctrls.unavailable.Store(true)

	f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

	sender, found := f.endpointFaultSenders.Get("ctrl-1")
	req.True(found)

	req.Eventually(func() bool {
		sender.wake()
		return pendingCount(f, "ctrl-1") == 0
	}, 5*time.Second, time.Millisecond, "a fault held past retention must be given up")
}

func Test_EndpointFault_ReportDoesNotBlockOnUnavailableController(t *testing.T) {
	req := require.New(t)
	f, ctrls := newTestFaulter(t, time.Minute)
	ctrls.unavailable.Store(true)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 500 {
			f.ReportEndpointFault(circuitId(i), "ctrl-1", ctrl_pb.FaultSubject_IngressFault)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		req.Fail("reporting a fault blocked on an unavailable controller")
	}
}

func Test_EndpointFault_NoControllerIsReported(t *testing.T) {
	req := require.New(t)
	f, _ := newTestFaulter(t, time.Minute)

	// nothing to send to and nothing to retry against; must not create a sender
	f.ReportEndpointFault("circuit-1", "", ctrl_pb.FaultSubject_IngressFault)

	req.Equal(0, f.endpointFaultSenders.Count())
}

func Test_EndpointFault_ShutdownAbandonsPending(t *testing.T) {
	req := require.New(t)
	f, ctrls := newTestFaulter(t, time.Minute)
	ctrls.unavailable.Store(true)

	f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

	sender, found := f.endpointFaultSenders.Get("ctrl-1")
	req.True(found)

	// a controller removed from the cluster has no circuits left to clean up
	f.endpointFaultSenders.Remove("ctrl-1")
	sender.shutdown()
	sender.shutdown() // idempotent

	req.Equal(0, f.endpointFaultSenders.Count())
}

func circuitId(i int) string {
	return "circuit-" + strconv.Itoa(i)
}

// Test_EndpointFault_RetainedWhenWriteFails covers the difference between the queue accepting a
// fault and the fault reaching the wire. The sender declines to retry a failed write, so treating
// queue acceptance as delivery drops the fault while reporting success.
func Test_EndpointFault_RetainedWhenWriteFails(t *testing.T) {
	req := require.New(t)
	f, ctrls := newTestFaulter(t, time.Minute)
	ctrls.ctrlCh.failWrites(errors.New("underlay write failed"))

	f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

	// wait for the write to have been attempted and failed, so that what is asserted below is the
	// state after the attempt rather than the state before it
	req.Eventually(func() bool {
		return ctrls.ctrlCh.attempts.Load() > 0
	}, 5*time.Second, time.Millisecond, "the fault was never written")

	req.Equal(1, pendingCount(f, "ctrl-1"), "a fault whose write failed must be retained")
	req.Equal(int64(0), f.endpointFaultsSent.Count(), "a failed write is not a delivery")

	// writes recover, and the retained fault goes out
	ctrls.ctrlCh.succeedWrites()
	sender, found := f.endpointFaultSenders.Get("ctrl-1")
	req.True(found)
	sender.wake()

	fault := takeFault(req, ctrls)
	req.Equal("circuit-1", fault.Id)

	req.Eventually(func() bool {
		return pendingCount(f, "ctrl-1") == 0
	}, 5*time.Second, time.Millisecond)
	req.Equal(int64(1), f.endpointFaultsSent.Count())
}

// Test_EndpointFault_ExpiresWhileControllerNotDraining covers retention continuing to run while a
// controller takes nothing. The sender both delivers and expires, so a send that waited
// indefinitely would stop expiry while new faults kept arriving.
func Test_EndpointFault_ExpiresWhileControllerNotDraining(t *testing.T) {
	req := require.New(t)
	f, ctrls := newTestFaulter(t, time.Millisecond)
	ctrls.ctrlCh.failWrites(errors.New("underlay write failed"))

	f.ReportEndpointFault("circuit-1", "ctrl-1", ctrl_pb.FaultSubject_IngressFault)

	sender, found := f.endpointFaultSenders.Get("ctrl-1")
	req.True(found)

	// the fault must leave by expiring, not by being counted as sent
	req.Eventually(func() bool {
		sender.wake()
		return f.endpointFaultsExpired.Count() > 0
	}, 10*time.Second, 5*time.Millisecond, "retention must run even while sends are failing")

	req.Equal(int64(0), f.endpointFaultsSent.Count())
	req.Equal(0, pendingCount(f, "ctrl-1"))
}
