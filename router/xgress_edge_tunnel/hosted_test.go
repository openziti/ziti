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
	"testing"
	"time"

	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/identity"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xgress_common"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

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

// markEstablishedTestEnv supplies the only env surface markEstablishedEvent.handle reads: the
// router id it logs.
type markEstablishedTestEnv struct {
	routerEnv.RouterEnv
}

func (self *markEstablishedTestEnv) GetRouterId() *identity.TokenId {
	return &identity.TokenId{Token: "test-router"}
}

// Test_markEstablishedEvent_handle_outstandingCreate covers the case a create response cannot settle:
// a success for a superseded attempt proves the terminator exists, so the state transition stands, but
// the create that replaced it is still unanswered and a delete sent now would race it.
func Test_markEstablishedEvent_handle_outstandingCreate(t *testing.T) {
	req := require.New(t)

	registry := &HostedServiceRegistry{env: &markEstablishedTestEnv{}}
	terminator := &tunnelTerminator{id: "t1", createTime: time.Now()}
	terminator.state.Store(xgress_common.TerminatorStateEstablishing)
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
