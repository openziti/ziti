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

package xgress_edge

import (
	"testing"
	"time"

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

func Test_edgeTerminator_rateLimitSignaling(t *testing.T) {
	t.Run("establishment under threshold reports success", func(t *testing.T) {
		req := require.New(t)

		ctrl := &stubRateLimitControl{}
		term := &edgeTerminator{}
		term.replaceRateLimitCallback(ctrl)
		req.Equal(0, ctrl.backoff, "storing a control with no prior attempt must not signal backoff")

		term.resolveRateLimitCallback(establishmentTimeout - time.Second)

		req.Equal(1, ctrl.success)
		req.Equal(0, ctrl.backoff)
		req.Equal(0, ctrl.failed)
		req.Nil(term.GetAndClearRateLimitCallback(), "control must be cleared once resolved")
	})

	t.Run("establishment at or over threshold reports backoff", func(t *testing.T) {
		req := require.New(t)

		ctrl := &stubRateLimitControl{}
		term := &edgeTerminator{}
		term.replaceRateLimitCallback(ctrl)

		term.resolveRateLimitCallback(establishmentTimeout)

		req.Equal(0, ctrl.success)
		req.Equal(1, ctrl.backoff)
		req.Equal(0, ctrl.failed)
		req.Nil(term.GetAndClearRateLimitCallback(), "control must be cleared once resolved")
	})

	t.Run("well over threshold reports backoff", func(t *testing.T) {
		req := require.New(t)

		ctrl := &stubRateLimitControl{}
		term := &edgeTerminator{}
		term.replaceRateLimitCallback(ctrl)

		term.resolveRateLimitCallback(2 * establishmentTimeout)

		req.Equal(0, ctrl.success)
		req.Equal(1, ctrl.backoff)
	})

	t.Run("resolving with no outstanding control is a no-op", func(t *testing.T) {
		req := require.New(t)

		term := &edgeTerminator{}
		req.NotPanics(func() {
			term.resolveRateLimitCallback(time.Hour)
		})
	})

	t.Run("re-send resolves the prior control with backoff instead of orphaning it", func(t *testing.T) {
		req := require.New(t)

		prior := &stubRateLimitControl{}
		term := &edgeTerminator{}
		term.replaceRateLimitCallback(prior)
		req.Equal(0, prior.backoff)

		// A 30s re-send acquires a fresh control and supersedes the prior attempt's control.
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

func Test_edgeTerminator_retryBackoff(t *testing.T) {
	t.Run("inactive by default", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}
		req.False(term.retryBackoffActive())
	})

	t.Run("scheduling activates backoff and grows the ceiling to the cap", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}

		term.scheduleRetryBackoff()
		req.True(term.retryBackoffActive(), "backoff must be active right after scheduling")
		req.Equal(minRetryBackoff, term.retryBackoff, "first backoff ceiling must be the minimum")
		// equal-jitter: the delay lands in [ceiling/2, ceiling]
		req.WithinDuration(time.Now().Add(minRetryBackoff), term.retryAfter, minRetryBackoff/2+time.Second)

		term.scheduleRetryBackoff()
		req.Equal(2*minRetryBackoff, term.retryBackoff, "ceiling must double on the next rejection")

		for i := 0; i < 20; i++ {
			term.scheduleRetryBackoff()
		}
		req.Equal(maxRetryBackoff, term.retryBackoff, "ceiling must saturate at the maximum")
	})

	t.Run("clearing resets the backoff", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}

		term.scheduleRetryBackoff()
		req.True(term.retryBackoffActive())

		term.clearRetryBackoff()
		req.False(term.retryBackoffActive(), "cleared backoff must be inactive")
		req.Zero(term.retryBackoff, "cleared backoff ceiling must reset to zero")
	})
}

// newTestHostedServiceRegistry builds a registry with just the maps the event-loop handlers touch,
// without starting the run loop. It is enough to drive handleRemoveTerminatorsV2Response directly.
func newTestHostedServiceRegistry() *hostedServiceRegistry {
	return &hostedServiceRegistry{
		terminators:    cmap.New[*edgeTerminator](),
		deleteSet:      map[string]*edgeTerminator{},
		pendingRemoves: map[string]*pendingRemoveBatch{},
		triggerEvalC:   make(chan struct{}, 1),
	}
}

func newDeletingTerminator(id string) *edgeTerminator {
	term := &edgeTerminator{terminatorId: id}
	term.operationActive.Store(true)
	term.setState(xgress_common.TerminatorStateDeleting, "test setup")
	return term
}

func Test_hostedServiceRegistry_handleRemoveTerminatorsV2Response(t *testing.T) {
	t.Run("success removes the terminator and reports success", func(t *testing.T) {
		req := require.New(t)

		reg := newTestHostedServiceRegistry()
		ctrl := &stubRateLimitControl{}
		term := newDeletingTerminator("t1")
		reg.terminators.Set(term.terminatorId, term)
		reg.pendingRemoves["req1"] = &pendingRemoveBatch{rateLimitCtrl: ctrl, terminators: []*edgeTerminator{term}}

		reg.handleRemoveTerminatorsV2Response(&removeTerminatorsV2ResponseEvent{requestId: "req1", success: true})

		req.Equal(1, ctrl.success)
		req.Equal(0, ctrl.backoff)
		req.Equal(0, ctrl.failed)
		_, found := reg.terminators.Get("t1")
		req.False(found, "terminator must be removed from the router set on success")
		req.False(term.operationActive.Load(), "operationActive must be cleared on success")
		req.NotContains(reg.pendingRemoves, "req1", "the pending batch must be cleared")
		req.Empty(reg.deleteSet, "a successfully removed terminator must not be requeued")
	})

	t.Run("rate limited backs off and requeues for retry", func(t *testing.T) {
		req := require.New(t)

		reg := newTestHostedServiceRegistry()
		ctrl := &stubRateLimitControl{}
		term := newDeletingTerminator("t2")
		reg.terminators.Set(term.terminatorId, term)
		reg.pendingRemoves["req2"] = &pendingRemoveBatch{rateLimitCtrl: ctrl, terminators: []*edgeTerminator{term}}

		reg.handleRemoveTerminatorsV2Response(&removeTerminatorsV2ResponseEvent{requestId: "req2", wasRateLimited: true, msg: "server too busy"})

		req.Equal(0, ctrl.success)
		req.Equal(1, ctrl.backoff, "a rate-limited removal must signal congestion via backoff")
		req.Equal(0, ctrl.failed)
		_, found := reg.terminators.Get("t2")
		req.True(found, "terminator must remain in the router set for retry")
		req.Contains(reg.deleteSet, "t2", "terminator must be requeued for deletion")
		req.False(term.operationActive.Load())
		req.NotContains(reg.pendingRemoves, "req2")
		req.True(term.retryBackoffActive(), "a rejected removal must be paced with a retry backoff, not retried immediately")
	})

	t.Run("generic failure reports failed and requeues for retry", func(t *testing.T) {
		req := require.New(t)

		reg := newTestHostedServiceRegistry()
		ctrl := &stubRateLimitControl{}
		term := newDeletingTerminator("t3")
		reg.terminators.Set(term.terminatorId, term)
		reg.pendingRemoves["req3"] = &pendingRemoveBatch{rateLimitCtrl: ctrl, terminators: []*edgeTerminator{term}}

		reg.handleRemoveTerminatorsV2Response(&removeTerminatorsV2ResponseEvent{requestId: "req3", msg: "boom"})

		req.Equal(0, ctrl.success)
		req.Equal(0, ctrl.backoff, "a non-rate-limited failure must not move the window")
		req.Equal(1, ctrl.failed)
		req.Contains(reg.deleteSet, "t3", "terminator must be requeued for deletion")
		req.NotContains(reg.pendingRemoves, "req3")
		req.True(term.retryBackoffActive(), "a failed removal must be paced with a retry backoff, not retried immediately")
	})

	t.Run("response for an unknown request id is a no-op", func(t *testing.T) {
		req := require.New(t)

		reg := newTestHostedServiceRegistry()
		req.NotPanics(func() {
			reg.handleRemoveTerminatorsV2Response(&removeTerminatorsV2ResponseEvent{requestId: "gone", success: true})
		})
		req.Empty(reg.deleteSet)
	})
}
