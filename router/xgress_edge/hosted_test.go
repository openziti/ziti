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

	"github.com/openziti/identity"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xgress_common"
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

		term.resolveRateLimitCallback(xgress_common.EstablishmentTimeout - time.Second)

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

		term.resolveRateLimitCallback(xgress_common.EstablishmentTimeout)

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

		term.resolveRateLimitCallback(2 * xgress_common.EstablishmentTimeout)

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

func Test_edgeTerminator_needsPostEstablishInspect(t *testing.T) {
	t.Run("recent establishment does not need a re-inspect", func(t *testing.T) {
		term := &edgeTerminator{supportsInspect: true}
		term.establishStart.Store(time.Now().Add(-time.Second))

		require.False(t, term.needsPostEstablishInspect())
	})

	t.Run("establishment past the threshold needs a re-inspect", func(t *testing.T) {
		term := &edgeTerminator{supportsInspect: true}
		term.establishStart.Store(time.Now().Add(-postEstablishInspectThreshold))

		require.True(t, term.needsPostEstablishInspect())
	})

	t.Run("no re-inspect when the sdk cannot be inspected", func(t *testing.T) {
		term := &edgeTerminator{supportsInspect: false}
		term.establishStart.Store(time.Now().Add(-time.Hour))

		require.False(t, term.needsPostEstablishInspect())
	})

	t.Run("unrecorded establishment start is treated as overdue", func(t *testing.T) {
		term := &edgeTerminator{supportsInspect: true}

		require.True(t, term.needsPostEstablishInspect())
	})

	// The sdk's bind deadline runs from its bind and is blind to the router re-sending the create,
	// so re-sends must not restart the clock the re-inspect is gated on. Timing this from
	// lastAttempt lets a stalled establishment look fresh and skips the re-inspect entirely.
	t.Run("create re-sends do not reset the establishment clock", func(t *testing.T) {
		req := require.New(t)

		bind := time.Now().Add(-70 * time.Second)
		term := &edgeTerminator{supportsInspect: true}
		term.establishStart.Store(bind)

		// re-sends land while the establishment is stalled, the most recent one 10s ago
		for _, resendAt := range []time.Time{bind.Add(30 * time.Second), bind.Add(60 * time.Second)} {
			term.lastAttempt = resendAt
		}

		req.Less(time.Since(term.lastAttempt), postEstablishInspectThreshold, "the last attempt looks fresh")
		req.True(term.needsPostEstablishInspect(), "the sdk has still been waiting since the bind")
	})
}

// markEstablishedTestEnv supplies the only env surface markEstablishedEvent.handle reads: the
// router id it logs. The rest of RouterEnv is left nil so an unexpected call panics rather than
// silently returning a zero value.
type markEstablishedTestEnv struct {
	routerEnv.RouterEnv
}

func (self *markEstablishedTestEnv) GetRouterId() *identity.TokenId {
	return &identity.TokenId{Token: "test-router"}
}

// newMarkEstablishedTestRegistry builds the minimum registry markEstablishedEvent.handle touches:
// the env it logs the router id from, and the set it queues post-establish inspects into.
func newMarkEstablishedTestRegistry() *hostedServiceRegistry {
	return &hostedServiceRegistry{
		env:                  &markEstablishedTestEnv{},
		postCreateInspectSet: map[string]*pendingPostCreateInspect{},
	}
}

// newEstablishingTerminator builds a terminator mid-establishment with an outstanding rate-limit
// control. Callers set the three clocks themselves, since which one drives which decision is the
// point of these tests.
func newEstablishingTerminator() (*edgeTerminator, *stubRateLimitControl) {
	terminator := &edgeTerminator{
		terminatorId:    "t1",
		supportsInspect: true,
		createTime:      time.Now(),
	}
	// updateState is a compare-and-swap, so the starting state has to be set explicitly: a
	// zero-value state fails the swap and falls through to the duplicate-notification path.
	terminator.state.Store(xgress_common.TerminatorStateEstablishing)
	terminator.operationActive.Store(true)

	ctrl := &stubRateLimitControl{}
	terminator.replaceRateLimitCallback(ctrl)
	return terminator, ctrl
}

func Test_markEstablishedEvent_handle(t *testing.T) {
	// The two clocks disagree here, which is the whole point: the create that completed was fast,
	// so it is not congestion, but the sdk has been waiting since its bind and may already have
	// given up on the listener. Timing both from lastAttempt loses the second half.
	t.Run("slow bind with a fresh attempt reports success and re-inspects", func(t *testing.T) {
		req := require.New(t)

		registry := newMarkEstablishedTestRegistry()
		term, ctrl := newEstablishingTerminator()
		term.establishStart.Store(time.Now().Add(-70 * time.Second))
		term.lastAttempt = time.Now().Add(-10 * time.Second)

		(&markEstablishedEvent{terminator: term, reason: "test"}).handle(registry)

		req.Equal(1, ctrl.success, "the create that completed was fast, so it is not congestion")
		req.Equal(0, ctrl.backoff)
		req.Contains(registry.postCreateInspectSet, term.terminatorId,
			"the sdk has been waiting since the bind, so its listener must be re-inspected")
		req.Equal(xgress_common.TerminatorStateEstablished, term.state.Load())
		req.False(term.operationActive.Load())
	})

	t.Run("fast establishment reports success and does not re-inspect", func(t *testing.T) {
		req := require.New(t)

		registry := newMarkEstablishedTestRegistry()
		term, ctrl := newEstablishingTerminator()
		term.establishStart.Store(time.Now().Add(-time.Second))
		term.lastAttempt = time.Now().Add(-time.Second)

		(&markEstablishedEvent{terminator: term, reason: "test"}).handle(registry)

		req.Equal(1, ctrl.success)
		req.Equal(0, ctrl.backoff)
		req.Empty(registry.postCreateInspectSet, "a prompt establishment needs no re-inspect")
		req.Equal(xgress_common.TerminatorStateEstablished, term.state.Load())
		req.False(term.operationActive.Load())
	})

	// lastAttempt is always at or after establishStart, so a slow attempt implies a slow bind:
	// both signals fire together.
	t.Run("slow attempt reports backoff and re-inspects", func(t *testing.T) {
		req := require.New(t)

		registry := newMarkEstablishedTestRegistry()
		term, ctrl := newEstablishingTerminator()
		term.establishStart.Store(time.Now().Add(-70 * time.Second))
		term.lastAttempt = time.Now().Add(-40 * time.Second)

		(&markEstablishedEvent{terminator: term, reason: "test"}).handle(registry)

		req.Equal(0, ctrl.success)
		req.Equal(1, ctrl.backoff, "a create that took this long is congestion")
		req.Contains(registry.postCreateInspectSet, term.terminatorId)
	})

	// A reconnect re-establishes a terminator that has been up for hours. createTime is ancient,
	// but the sdk is not waiting on a bind, so this must not read as a stalled establishment.
	t.Run("reconnect re-establishment reports success and does not re-inspect", func(t *testing.T) {
		req := require.New(t)

		registry := newMarkEstablishedTestRegistry()
		term, ctrl := newEstablishingTerminator()
		term.createTime = time.Now().Add(-6 * time.Hour)
		term.establishStart.Store(time.Now().Add(-time.Second))
		term.lastAttempt = time.Now().Add(-time.Second)

		(&markEstablishedEvent{terminator: term, reason: "reconnecting"}).handle(registry)

		req.Equal(1, ctrl.success, "a reconnect must not back off during recovery")
		req.Equal(0, ctrl.backoff)
		req.Empty(registry.postCreateInspectSet, "the sdk is not waiting on a bind after a reconnect")
	})

	t.Run("duplicate notification does not re-inspect", func(t *testing.T) {
		req := require.New(t)

		registry := newMarkEstablishedTestRegistry()
		term, _ := newEstablishingTerminator()
		term.state.Store(xgress_common.TerminatorStateEstablished)
		term.establishStart.Store(time.Now().Add(-70 * time.Second))
		term.lastAttempt = time.Now().Add(-10 * time.Second)

		(&markEstablishedEvent{terminator: term, reason: "test"}).handle(registry)

		req.Empty(registry.postCreateInspectSet,
			"a repeat notification must not queue an inspect off a stale establishment clock")
		req.False(term.operationActive.Load(), "the in-flight flag must be cleared either way")
	})
}
