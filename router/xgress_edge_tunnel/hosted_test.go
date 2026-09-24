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

package xgress_edge_tunnel

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/v2/ziti"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xgress_common"
	"github.com/openziti/ziti/v2/tunnel"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

// countingCtrlChannel is a channel.Channel that records how many create requests were sent. Only
// the methods establishTerminator touches are implemented; the rest panic via the nil embedded
// interface if the test strays onto an unexpected path.
type countingCtrlChannel struct {
	channel.Channel
	sends atomic.Int32

	// afterSetSequence runs inside TrySend, right after the sequence is assigned, which is the
	// earliest point a response could be processed.
	afterSetSequence func(seq int32)

	// refuseQueue models a full send queue: the sequence is assigned, but the message never goes out.
	refuseQueue bool
}

func (self *countingCtrlChannel) Id() string { return "test-ctrl" }

// TrySend assigns a sequence as a real sender does, before deciding whether the message is queued, so
// tests can correlate a response to the attempt that produced it and can model a full send queue.
func (self *countingCtrlChannel) TrySend(s channel.Sendable) (bool, error) {
	s.SetSequence(self.sends.Add(1))
	if self.afterSetSequence != nil {
		self.afterSetSequence(s.Sequence())
	}
	return !self.refuseQueue, nil
}

// settleTestEnv supplies only the env surface establishTerminator uses: the router id, the model
// update channel, and a rate limiter that never limits.
type settleTestEnv struct {
	routerEnv.RouterEnv
	ctrlCh *countingCtrlChannel
}

func (self *settleTestEnv) GetRouterId() *identity.TokenId {
	return &identity.TokenId{Token: "test-router"}
}

func (self *settleTestEnv) GetNetworkControllers() routerEnv.NetworkControllers {
	return &routerEnv.MockNetworkControllers{Channel: self.ctrlCh}
}

func (self *settleTestEnv) GetCtrlRateLimiter() rate.AdaptiveRateLimitTracker {
	return rate.NoOpAdaptiveRateLimitTracker{}
}

// settleTestHostingContext supplies the three accessors establishTerminator reads to build the
// create request.
type settleTestHostingContext struct {
	tunnel.HostingContext
}

func (settleTestHostingContext) ServiceName() string                { return "test-service" }
func (settleTestHostingContext) ServiceId() string                  { return "test-service-id" }
func (settleTestHostingContext) ListenOptions() *ziti.ListenOptions { return &ziti.ListenOptions{} }

func newSettleTestRegistry() (*HostedServiceRegistry, *countingCtrlChannel) {
	ctrlCh := &countingCtrlChannel{}
	return &HostedServiceRegistry{
		establishSet: map[string]*tunnelTerminator{},
		env:          &settleTestEnv{ctrlCh: ctrlCh},
	}, ctrlCh
}

func newSettleTestTerminator(id string, createTime time.Time) *tunnelTerminator {
	terminator := &tunnelTerminator{
		id:         id,
		context:    settleTestHostingContext{},
		createTime: createTime,
	}
	terminator.state.Store(xgress_common.TerminatorStateEstablishing)
	return terminator
}

// TestSettleGateHoldsNewTerminator verifies that a terminator still inside its settle window is
// not sent to the controller, stays queued, and arms the re-check timer.
func TestSettleGateHoldsNewTerminator(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	terminator := newSettleTestTerminator("t1", time.Now())
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()

	req.Contains(registry.establishSet, terminator.id, "terminator inside its settle window must stay queued")
	req.Equal(int32(0), ctrlCh.sends.Load(), "no create should be sent inside the settle window")
	req.NotNil(registry.establishNotify, "a re-check must be scheduled so the terminator is retried after the window")
}

// TestSettleGateReleasesSettledTerminator verifies that once the settle window has passed the
// terminator is sent and removed from the queue.
func TestSettleGateReleasesSettledTerminator(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	terminator := newSettleTestTerminator("t1", time.Now().Add(-establishSettleTime-time.Second))
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()

	req.NotContains(registry.establishSet, terminator.id, "a settled terminator must be dequeued once attempted")
	req.Equal(int32(1), ctrlCh.sends.Load(), "a settled terminator must be sent to the controller")
}

// TestSettleGateDoesNotDelayRequeuedTerminator verifies that the delay applies only to a
// terminator's first attempt. A retry reuses the terminator with its original createTime, so it
// is already past the window and must be sent again without waiting.
func TestSettleGateDoesNotDelayRequeuedTerminator(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	terminator := newSettleTestTerminator("t1", time.Now().Add(-establishSettleTime-time.Second))
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()
	req.Equal(int32(1), ctrlCh.sends.Load())

	// model a controller response that left the terminator establishing: it is requeued with the
	// same createTime and its in-flight flag cleared.
	terminator.operationActive.Store(false)
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()

	req.Equal(int32(2), ctrlCh.sends.Load(), "a requeued terminator must be retried without a second settle delay")
	req.NotContains(registry.establishSet, terminator.id)
}

// TestSettleGateIsPerTerminator verifies the gate is evaluated independently per terminator: a
// settled terminator is sent even while a newer one is still holding, and neither starves the
// other.
func TestSettleGateIsPerTerminator(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	settled := newSettleTestTerminator("settled", time.Now().Add(-establishSettleTime-time.Second))
	holding := newSettleTestTerminator("holding", time.Now())
	registry.establishSet[settled.id] = settled
	registry.establishSet[holding.id] = holding

	registry.evaluateEstablishQueue()

	req.Equal(int32(1), ctrlCh.sends.Load(), "only the settled terminator should be sent")
	req.NotContains(registry.establishSet, settled.id, "the settled terminator must be dequeued")
	req.Contains(registry.establishSet, holding.id, "the holding terminator must remain queued")
}

// stubRateLimitControl is a rate.RateLimitControl that records how each outcome was signaled, so
// tests can assert the router classified an establishment as a success or a backoff.
type stubRateLimitControl struct {
	success int
	backoff int
	failed  int
}

func (self *stubRateLimitControl) Success() { self.success++ }
func (self *stubRateLimitControl) Backoff() { self.backoff++ }
func (self *stubRateLimitControl) Failed()  { self.failed++ }

func Test_tunnelTerminator_rateLimitSignaling(t *testing.T) {
	t.Run("establishment under threshold reports success", func(t *testing.T) {
		req := require.New(t)

		ctrl := &stubRateLimitControl{}
		term := &tunnelTerminator{}
		term.replaceRateLimitCallback(ctrl)
		req.Equal(0, ctrl.backoff, "storing a control with no prior attempt must not signal backoff")

		term.resolveRateLimitCallback(xgress_common.EstablishmentTimeout - time.Second)

		req.Equal(1, ctrl.success)
		req.Equal(0, ctrl.backoff)
		req.Equal(0, ctrl.failed)
		req.Nil(term.GetAndClearRateLimitCallback(), "control must be cleared once resolved")
	})

	t.Run("establishment at or over threshold reports backoff", func(t *testing.T) {
		req := require.New(t)

		ctrl := &stubRateLimitControl{}
		term := &tunnelTerminator{}
		term.replaceRateLimitCallback(ctrl)

		term.resolveRateLimitCallback(xgress_common.EstablishmentTimeout)

		req.Equal(0, ctrl.success)
		req.Equal(1, ctrl.backoff)
		req.Equal(0, ctrl.failed)
		req.Nil(term.GetAndClearRateLimitCallback(), "control must be cleared once resolved")
	})

	t.Run("resolving with no outstanding control is a no-op", func(t *testing.T) {
		term := &tunnelTerminator{}
		require.NotPanics(t, func() {
			term.resolveRateLimitCallback(time.Hour)
		})
	})

	t.Run("re-send resolves the prior control with backoff instead of orphaning it", func(t *testing.T) {
		req := require.New(t)

		prior := &stubRateLimitControl{}
		term := &tunnelTerminator{}
		term.replaceRateLimitCallback(prior)
		req.Equal(0, prior.backoff)

		next := &stubRateLimitControl{}
		term.replaceRateLimitCallback(next)

		req.Equal(1, prior.backoff, "the superseded control must be resolved with backoff, not orphaned")
		req.Equal(0, prior.success)
		req.Equal(0, next.backoff, "the new control is still outstanding and must not be resolved yet")

		got := term.GetAndClearRateLimitCallback()
		req.NotNil(got)
		req.Same(next, got.(*stubRateLimitControl), "the new control must be the one now stored")
	})
}

// TestEstablishQueueResolvesSupersededRateLimitControl verifies the re-send path resolves the
// stalled attempt's control rather than dropping it, which would hold its slot in the limiter
// window until the limiter's own expiry reclaimed it.
func TestEstablishQueueResolvesSupersededRateLimitControl(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	terminator := newSettleTestTerminator("t1", time.Now().Add(-establishSettleTime-time.Second))
	prior := &stubRateLimitControl{}
	terminator.rateLimitCallback = prior
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()

	req.Equal(int32(1), ctrlCh.sends.Load(), "the terminator must be re-sent")
	req.Equal(1, prior.backoff, "the superseded control must be resolved with backoff, not orphaned")
	req.Equal(0, prior.success)
}

// Test_tunnelTerminator_createRequestCorrelation covers matching a create response to the attempt
// that produced it. Attempts overlap once one overruns EstablishmentTimeout and they all carry the
// same terminator id, so only the reply sequence tells them apart.
func Test_tunnelTerminator_createRequestCorrelation(t *testing.T) {
	t.Run("nothing outstanding matches nothing", func(t *testing.T) {
		req := require.New(t)
		term := &tunnelTerminator{}

		req.False(term.hasOutstandingCreate())
		req.False(term.resolveCreateRequest("ctrl1", 7))
	})

	t.Run("the outstanding attempt's reply matches and clears it", func(t *testing.T) {
		req := require.New(t)
		term := &tunnelTerminator{}
		term.noteCreateRequestSent("ctrl1", 7)

		req.True(term.hasOutstandingCreate())
		req.True(term.resolveCreateRequest("ctrl1", 7))
		req.False(term.hasOutstandingCreate())
		req.False(term.resolveCreateRequest("ctrl1", 7), "an attempt resolves only once")
	})

	t.Run("a superseded attempt's reply leaves the outstanding attempt alone", func(t *testing.T) {
		req := require.New(t)
		term := &tunnelTerminator{}
		term.noteCreateRequestSent("ctrl1", 7)
		term.noteCreateRequestSent("ctrl1", 9)

		req.False(term.resolveCreateRequest("ctrl1", 7))
		req.True(term.hasOutstandingCreate(), "the later attempt is still unanswered")
		req.True(term.resolveCreateRequest("ctrl1", 9))
	})

	// Sequences are per-channel, so the same number from another controller is a different request.
	t.Run("a matching sequence from another controller does not match", func(t *testing.T) {
		req := require.New(t)
		term := &tunnelTerminator{}
		term.noteCreateRequestSent("ctrl1", 7)

		req.False(term.resolveCreateRequest("ctrl2", 7))
		req.True(term.hasOutstandingCreate())
	})

	t.Run("endOperationIfIdle waits for the outstanding attempt", func(t *testing.T) {
		req := require.New(t)
		term := &tunnelTerminator{}
		term.operationActive.Store(true)
		term.noteCreateRequestSent("ctrl1", 7)

		term.endOperationIfIdle()
		req.True(term.operationActive.Load(), "the unanswered create must still block a queued delete")

		req.True(term.resolveCreateRequest("ctrl1", 7))
		term.endOperationIfIdle()
		req.False(term.operationActive.Load())
	})
}

// TestEstablishQueueRecordsTheAttempt verifies the create send records what a response must match,
// and that a re-send supersedes the attempt before it.
func TestEstablishQueueRecordsTheAttempt(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	terminator := newSettleTestTerminator("t1", time.Now().Add(-establishSettleTime-time.Second))
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()
	req.Equal(int32(1), ctrlCh.sends.Load())
	req.True(terminator.hasOutstandingCreate(), "the create just sent is awaiting a response")

	// A re-send after the establishment timeout leaves the first attempt unanswered.
	terminator.operationActive.Store(false)
	registry.establishSet[terminator.id] = terminator
	registry.evaluateEstablishQueue()
	req.Equal(int32(2), ctrlCh.sends.Load())

	req.False(terminator.resolveCreateRequest(ctrlCh.Id(), 1),
		"a reply to the superseded attempt must not resolve the one in flight")
	req.True(terminator.resolveCreateRequest(ctrlCh.Id(), 2))
}

// TestEstablishQueueRecordsTheAttemptBeforeItCanBeAnswered covers the ordering the correlation rests
// on. A response can be processed as soon as the message is queued, so an attempt recorded after the
// send returns can be answered before it is known, and the reply reads as superseded.
func TestEstablishQueueRecordsTheAttemptBeforeItCanBeAnswered(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	terminator := newSettleTestTerminator("t1", time.Now().Add(-establishSettleTime-time.Second))
	registry.establishSet[terminator.id] = terminator

	var matchedDuringSend bool
	ctrlCh.afterSetSequence = func(seq int32) {
		matchedDuringSend = terminator.resolveCreateRequest(ctrlCh.Id(), seq)
	}

	registry.evaluateEstablishQueue()

	req.True(matchedDuringSend, "the attempt must be recorded before the message can be answered")
}

// TestEstablishTerminatorRollsBackAnUnsentAttempt covers the cost of recording that early: the
// sequence is assigned even when the send queue is full, so an attempt that never went out has to be
// withdrawn, or the delete queue waits out the establishment timeout for a create that does not exist.
func TestEstablishTerminatorRollsBackAnUnsentAttempt(t *testing.T) {
	req := require.New(t)

	registry, ctrlCh := newSettleTestRegistry()
	ctrlCh.refuseQueue = true
	terminator := newSettleTestTerminator("t1", time.Now().Add(-establishSettleTime-time.Second))
	registry.establishSet[terminator.id] = terminator

	registry.evaluateEstablishQueue()

	req.Equal(int32(1), ctrlCh.sends.Load(), "the send was attempted")
	req.False(terminator.hasOutstandingCreate(), "an attempt that never went out must not be awaited")
}

// Test_markEstablishedEvent_handle_outstandingCreate covers the case a create response cannot settle:
// a success for a superseded attempt proves the terminator exists, so the state transition stands, but
// the create that replaced it is still unanswered and a delete sent now would race it.
func Test_markEstablishedEvent_handle_outstandingCreate(t *testing.T) {
	req := require.New(t)

	registry, _ := newSettleTestRegistry()
	terminator := newSettleTestTerminator("t1", time.Now())
	terminator.lastAttempt = time.Now()
	terminator.operationActive.Store(true)
	terminator.noteCreateRequestSent("test-ctrl", 7)

	(&markEstablishedEvent{terminator: terminator, reason: "test"}).handle(registry)

	req.Equal(xgress_common.TerminatorStateEstablished, terminator.state.Load())
	req.True(terminator.operationActive.Load(), "the unanswered create must still block a queued delete")
}

// deleteQueueTestEnv supplies the only env surface evaluateDeleteQueue reaches once a terminator is
// batched: a rate limiter that reports itself full, which requeues the batch without needing a
// control channel. That is enough to tell "was batched" from "was skipped".
type deleteQueueTestEnv struct {
	routerEnv.RouterEnv
}

func (self *deleteQueueTestEnv) GetCtrlRateLimiter() rate.AdaptiveRateLimitTracker {
	return fullRateLimiter{}
}

type fullRateLimiter struct {
	rate.AdaptiveRateLimitTracker
}

func (fullRateLimiter) IsRateLimited() bool { return true }

func newDeleteQueueTestRegistry() *HostedServiceRegistry {
	return &HostedServiceRegistry{
		env:         &deleteQueueTestEnv{},
		terminators: cmap.New[*tunnelTerminator](),
		deleteSet:   map[string]*tunnelTerminator{},
	}
}

func newDeletingTerminator(id string) *tunnelTerminator {
	terminator := &tunnelTerminator{id: id}
	terminator.setState(xgress_common.TerminatorStateDeleting, "test setup")
	return terminator
}

// TestDeleteQueueWaitsForInFlightOperation covers the sequencing a delete depends on. A delete sent
// while a create is still in flight can reach a controller that has not applied the create, which
// reads the id as absent.
func TestDeleteQueueWaitsForInFlightOperation(t *testing.T) {
	t.Run("a terminator with an operation in flight stays queued", func(t *testing.T) {
		req := require.New(t)

		registry := newDeleteQueueTestRegistry()
		terminator := newDeletingTerminator("t1")
		terminator.operationActive.Store(true)
		terminator.lastAttempt = time.Now()
		registry.deleteSet[terminator.id] = terminator

		registry.evaluateDeleteQueue()

		req.Contains(registry.deleteSet, "t1", "the delete must stay queued, not be dropped until the retry scan")
		req.True(terminator.operationActive.Load(), "the in-flight operation must be left alone")
	})

	// The wait is bounded: an operation that never completes must not block the delete forever.
	t.Run("an operation past the timeout is presumed stuck and the delete proceeds", func(t *testing.T) {
		req := require.New(t)

		registry := newDeleteQueueTestRegistry()
		terminator := newDeletingTerminator("t1")
		terminator.operationActive.Store(true)
		terminator.lastAttempt = time.Now().Add(-2 * xgress_common.EstablishmentTimeout)
		registry.deleteSet[terminator.id] = terminator

		registry.evaluateDeleteQueue()

		req.False(terminator.operationActive.Load(), "a stuck operation must be abandoned so the delete can go")
	})

	t.Run("a terminator with nothing in flight is batched", func(t *testing.T) {
		req := require.New(t)

		registry := newDeleteQueueTestRegistry()
		terminator := newDeletingTerminator("t1")
		registry.deleteSet[terminator.id] = terminator

		registry.evaluateDeleteQueue()

		// The stub limiter reports full, so a batched terminator is requeued rather than sent. Either
		// way it left the queue and came back, which the skip path never does.
		req.Contains(registry.deleteSet, "t1")
		req.False(terminator.operationActive.Load())
	})
}

// TestQueueRemoveTerminatorInFlightHandling pins which enqueue paths may clear the in-flight marker.
// Clearing it when queuing a delete is what let a delete be sent while a create was still outstanding;
// clearing it when requeuing after a failed remove is correct, because the operation that set it has
// ended.
func TestQueueRemoveTerminatorInFlightHandling(t *testing.T) {
	t.Run("queuing a delete leaves an in-flight operation marked", func(t *testing.T) {
		req := require.New(t)

		registry := newDeleteQueueTestRegistry()
		terminator := newDeletingTerminator("t1")
		terminator.operationActive.Store(true)

		registry.queueRemoveTerminatorUnchecked(terminator, "test")

		req.Contains(registry.deleteSet, "t1")
		req.True(terminator.operationActive.Load(),
			"a create may still be outstanding; the delete queue waits for it rather than racing it")
	})

	t.Run("requeuing after a failed remove clears it", func(t *testing.T) {
		req := require.New(t)

		registry := newDeleteQueueTestRegistry()
		terminator := newDeletingTerminator("t1")
		terminator.operationActive.Store(true)

		registry.requeueRemoveTerminatorSync(terminator)

		req.Contains(registry.deleteSet, "t1")
		req.False(terminator.operationActive.Load(),
			"the remove that set the marker has ended, so the retry must not wait on it")
	})
}
