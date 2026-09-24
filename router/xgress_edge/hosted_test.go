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

	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	routerEnv "github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/state"
	"github.com/openziti/ziti/v2/router/xgress_common"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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

	// A reply to a superseded create, or a controller validation message, can arrive while a later
	// create is still unanswered. Either proves the terminator exists, so the state transition stands,
	// but neither may report the terminator idle: a delete sent then would race the live create.
	t.Run("an outstanding create keeps the operation marked active", func(t *testing.T) {
		req := require.New(t)

		registry := newMarkEstablishedTestRegistry()
		term, _ := newEstablishingTerminator()
		term.establishStart.Store(time.Now().Add(-time.Second))
		term.lastAttempt = time.Now().Add(-time.Second)
		term.noteCreateRequestSent("ctrl1", 7)

		(&markEstablishedEvent{terminator: term, reason: "test"}).handle(registry)

		req.Equal(xgress_common.TerminatorStateEstablished, term.state.Load())
		req.True(term.operationActive.Load(), "the unanswered create must still block a queued delete")
	})
}

// Test_edgeTerminator_createRequestCorrelation covers matching a create response to the attempt that
// produced it. Attempts overlap once one overruns EstablishmentTimeout and they all carry the same
// terminator id, so only the reply sequence tells them apart.
func Test_edgeTerminator_createRequestCorrelation(t *testing.T) {
	t.Run("nothing outstanding matches nothing", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}

		req.False(term.hasOutstandingCreate())
		req.False(term.resolveCreateRequest("ctrl1", 7, false))
	})

	t.Run("the outstanding attempt's reply matches and clears it", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}
		term.noteCreateRequestSent("ctrl1", 7)

		req.True(term.hasOutstandingCreate())
		req.True(term.resolveCreateRequest("ctrl1", 7, false))
		req.False(term.hasOutstandingCreate())
		req.False(term.resolveCreateRequest("ctrl1", 7, false), "an attempt resolves only once")
	})

	t.Run("a superseded attempt's reply leaves the outstanding attempt alone", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}
		term.noteCreateRequestSent("ctrl1", 7)
		term.noteCreateRequestSent("ctrl1", 9)

		req.False(term.resolveCreateRequest("ctrl1", 7, false))
		req.True(term.hasOutstandingCreate(), "the later attempt is still unanswered")
		req.True(term.resolveCreateRequest("ctrl1", 9, false))
	})

	// Sequences are per-channel, so the same number from another controller is a different request.
	t.Run("a matching sequence from another controller does not match", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}
		term.noteCreateRequestSent("ctrl1", 7)

		req.False(term.resolveCreateRequest("ctrl2", 7, false))
		req.True(term.hasOutstandingCreate())
	})

	// The confirmation is set here rather than by the caller so a retry cannot start between the match
	// and the store and end up vouched for by a response that answered the attempt before it.
	t.Run("a matching success confirms the create in the same step", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}
		term.noteCreateRequestSent("ctrl1", 7)

		req.True(term.resolveCreateRequest("ctrl1", 7, true))
		req.True(term.createConfirmed.Load())
	})

	t.Run("a matching failure does not confirm", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}
		term.noteCreateRequestSent("ctrl1", 7)

		req.True(term.resolveCreateRequest("ctrl1", 7, false))
		req.False(term.createConfirmed.Load())
	})

	t.Run("a superseded success does not confirm", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}
		term.noteCreateRequestSent("ctrl1", 7)
		term.noteCreateRequestSent("ctrl1", 9)

		req.False(term.resolveCreateRequest("ctrl1", 7, true))
		req.False(term.createConfirmed.Load(), "the create in flight is still unacknowledged")
	})

	t.Run("clearCreateRequest drops the attempt and the confirmation", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}
		term.noteCreateRequestSent("ctrl1", 7)
		term.createConfirmed.Store(true)

		term.clearCreateRequest()

		req.False(term.hasOutstandingCreate())
		req.False(term.createConfirmed.Load())
		req.False(term.resolveCreateRequest("ctrl1", 7, false),
			"a reply to the superseded attempt must not confirm the one that replaced it")
	})

	// replace() takes over the terminator id, so a reply still in flight answers the terminator that
	// inherited it.
	t.Run("replace inherits the outstanding attempt", func(t *testing.T) {
		req := require.New(t)
		replaced := &edgeTerminator{terminatorId: "t1"}
		replaced.noteCreateRequestSent("ctrl1", 7)

		replacement := &edgeTerminator{}
		replacement.replace(replaced)

		req.True(replacement.hasOutstandingCreate())
		req.True(replacement.resolveCreateRequest("ctrl1", 7, false))
	})

	// The sender assigns the sequence before the message can reach the tx goroutine, so recording it
	// here is what keeps a response from arriving before the attempt it answers is known.
	t.Run("the sendable records the attempt as the sequence is assigned", func(t *testing.T) {
		req := require.New(t)
		term := &edgeTerminator{}
		sendable := &createRequestSendable{
			Message:    channel.NewMessage(1, nil),
			terminator: term,
			ctrlId:     "ctrl1",
		}

		sendable.SetSequence(7)

		req.Equal(int32(7), sendable.Msg().Sequence(), "the message must still carry the sequence")
		req.True(term.resolveCreateRequest("ctrl1", 7, false),
			"the attempt must be recorded by the time the sender holds the sequence")
	})
}

// stubCtrlCh answers only Id(). Every other method dispatches through the nil embedded interface and
// panics, so the test fails loudly if the handler grows a dependency this fixture does not model.
type stubCtrlCh struct {
	channel.Channel
	id string
}

func (self *stubCtrlCh) Id() string { return self.id }

// createResponseTestEnv adds the close-notify channel queue() selects on to the router id the
// mark-established path logs.
type createResponseTestEnv struct {
	markEstablishedTestEnv
}

func (self *createResponseTestEnv) GetCloseNotify() <-chan struct{} { return nil }

func newCreateResponseTestRegistry() *hostedServiceRegistry {
	return &hostedServiceRegistry{
		env:                  &createResponseTestEnv{},
		terminators:          cmap.New[*edgeTerminator](),
		postCreateInspectSet: map[string]*pendingPostCreateInspect{},
		events:               make(chan terminatorEvent, 4),
		triggerEvalC:         make(chan struct{}, 1),
	}
}

func createTerminatorResponse(terminatorId string, result edge_ctrl_pb.CreateTerminatorResult, replyFor int32) *channel.Message {
	body, err := proto.Marshal(&edge_ctrl_pb.CreateTerminatorV2Response{
		TerminatorId: terminatorId,
		Result:       result,
	})
	if err != nil {
		panic(err)
	}
	msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_CreateTerminatorV2ResponseType), body)
	msg.PutUint32Header(channel.ReplyForHeader, uint32(replyFor))
	return msg
}

// Test_hostedServiceRegistry_HandleCreateTerminatorResponse covers what a create response is allowed
// to conclude. The response names only the terminator id, which every attempt for that terminator
// shares, so a late success can arrive for an attempt that has already been superseded.
func Test_hostedServiceRegistry_HandleCreateTerminatorResponse(t *testing.T) {
	t.Run("a reply to the outstanding create confirms it and ends the operation", func(t *testing.T) {
		req := require.New(t)

		reg := newCreateResponseTestRegistry()
		term, _ := newEstablishingTerminator()
		term.lastAttempt = time.Now()
		term.establishStart.Store(time.Now())
		term.noteCreateRequestSent("ctrl1", 7)
		reg.terminators.Set(term.terminatorId, term)

		reg.HandleCreateTerminatorResponse(createTerminatorResponse(term.terminatorId, edge_ctrl_pb.CreateTerminatorResult_Success, 7), &stubCtrlCh{id: "ctrl1"})

		req.True(term.createConfirmed.Load())
		req.False(term.hasOutstandingCreate())

		(<-reg.events).handle(reg)
		req.False(term.operationActive.Load())
	})

	// A success for a superseded attempt says nothing about the create now in flight. Vouching for it
	// would let the controller's confirmed-absent fast path skip a delete the create then undoes, and
	// reporting the terminator idle would let that delete go out while the create is still unapplied.
	t.Run("a reply to a superseded create leaves the one in flight outstanding", func(t *testing.T) {
		req := require.New(t)

		reg := newCreateResponseTestRegistry()
		term, _ := newEstablishingTerminator()
		term.lastAttempt = time.Now()
		term.establishStart.Store(time.Now())
		term.noteCreateRequestSent("ctrl1", 7)
		term.noteCreateRequestSent("ctrl1", 9)
		reg.terminators.Set(term.terminatorId, term)

		reg.HandleCreateTerminatorResponse(createTerminatorResponse(term.terminatorId, edge_ctrl_pb.CreateTerminatorResult_Success, 7), &stubCtrlCh{id: "ctrl1"})

		req.False(term.createConfirmed.Load(), "the create in flight is still unacknowledged")
		req.True(term.hasOutstandingCreate())

		(<-reg.events).handle(reg)
		req.True(term.operationActive.Load(), "a queued delete must keep waiting for the live create")
	})

	// A rejection of a superseded attempt is no more conclusive than a success for one, and this path
	// can go on to close the terminator, which queues a delete.
	t.Run("a rejection of a superseded create leaves the one in flight outstanding", func(t *testing.T) {
		req := require.New(t)

		reg := newCreateResponseTestRegistry()
		term, _ := newEstablishingTerminator()
		term.lastAttempt = time.Now()
		term.noteCreateRequestSent("ctrl1", 7)
		term.noteCreateRequestSent("ctrl1", 9)
		reg.terminators.Set(term.terminatorId, term)

		msg := createTerminatorResponse(term.terminatorId, edge_ctrl_pb.CreateTerminatorResult_FailedInvalidSession, 7)
		reg.HandleCreateTerminatorResponse(msg, &stubCtrlCh{id: "ctrl1"})

		req.True(term.operationActive.Load(), "a queued delete must keep waiting for the live create")
		req.True(term.hasOutstandingCreate())
	})

	t.Run("a rejection of the outstanding create ends the operation", func(t *testing.T) {
		req := require.New(t)

		reg := newCreateResponseTestRegistry()
		term, _ := newEstablishingTerminator()
		term.lastAttempt = time.Now()
		term.noteCreateRequestSent("ctrl1", 7)
		reg.terminators.Set(term.terminatorId, term)

		msg := createTerminatorResponse(term.terminatorId, edge_ctrl_pb.CreateTerminatorResult_FailedInvalidSession, 7)
		reg.HandleCreateTerminatorResponse(msg, &stubCtrlCh{id: "ctrl1"})

		req.False(term.operationActive.Load(), "nothing is in flight, so the delete queue must not wait")
		req.False(term.hasOutstandingCreate())
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
	// serviceSessionToken is dereferenced by the delete queue's logging, so it must be present even
	// though these tests do not exercise sessions.
	term := &edgeTerminator{
		terminatorId:        id,
		serviceSessionToken: &state.ServiceSessionToken{Claims: &common.ServiceAccessClaims{}},
	}
	term.operationActive.Store(true)
	term.setState(xgress_common.TerminatorStateDeleting, "test setup")
	return term
}

func Test_hostedServiceRegistry_handleRemoveTerminatorsV2Response(t *testing.T) {
	// A removal that eventually succeeds is still congestion if it took too long. The establish path
	// classifies its own latency on the same threshold, and both resolve into one limiter, so
	// reporting success regardless would grow the window during the overload that caused the delay.
	t.Run("a slow success reports backoff", func(t *testing.T) {
		req := require.New(t)

		reg := newTestHostedServiceRegistry()
		ctrl := &stubRateLimitControl{}
		term := newDeletingTerminator("t1")
		reg.terminators.Set(term.terminatorId, term)
		reg.pendingRemoves["slow"] = &pendingRemoveBatch{
			rateLimitCtrl: ctrl,
			terminators:   []*edgeTerminator{term},
			queuedAt:      time.Now().Add(-xgress_common.EstablishmentTimeout),
		}

		reg.handleRemoveTerminatorsV2Response(&removeTerminatorsV2ResponseEvent{requestId: "slow", success: true})

		req.Equal(1, ctrl.backoff, "a removal past the threshold is congestion, however it ended")
		req.Equal(0, ctrl.success)
		_, found := reg.terminators.Get("t1")
		req.False(found, "the removal still succeeded, so the terminator must still be dropped")
	})

	t.Run("success removes the terminator and reports success", func(t *testing.T) {
		req := require.New(t)

		reg := newTestHostedServiceRegistry()
		ctrl := &stubRateLimitControl{}
		term := newDeletingTerminator("t1")
		reg.terminators.Set(term.terminatorId, term)
		reg.pendingRemoves["req1"] = &pendingRemoveBatch{rateLimitCtrl: ctrl, terminators: []*edgeTerminator{term}, queuedAt: time.Now()}

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
		reg.pendingRemoves["req2"] = &pendingRemoveBatch{rateLimitCtrl: ctrl, terminators: []*edgeTerminator{term}, queuedAt: time.Now()}

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
		reg.pendingRemoves["req3"] = &pendingRemoveBatch{rateLimitCtrl: ctrl, terminators: []*edgeTerminator{term}, queuedAt: time.Now()}

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

	t.Run("scanForRetries purges pending remove batches whose response was lost", func(t *testing.T) {
		req := require.New(t)

		reg := newTestHostedServiceRegistry()
		reg.pendingRemoves["fresh"] = &pendingRemoveBatch{queuedAt: time.Now()}
		reg.pendingRemoves["stale"] = &pendingRemoveBatch{queuedAt: time.Now().Add(-3 * xgress_common.EstablishmentTimeout)}

		reg.scanForRetries()

		req.Contains(reg.pendingRemoves, "fresh", "a recent pending batch must be retained")
		req.NotContains(reg.pendingRemoves, "stale", "a pending batch past the expiry must be purged so it can't leak")
	})
}

func Test_edgeTerminator_replaceCopiesCreateConfirmed(t *testing.T) {
	req := require.New(t)

	other := &edgeTerminator{terminatorId: "t1"}
	other.createConfirmed.Store(true)

	replacement := &edgeTerminator{}
	replacement.replace(other)

	req.True(replacement.createConfirmed.Load(), "replace must carry over the adopted terminator's create confirmation")
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

func newDeleteQueueTestRegistry() *hostedServiceRegistry {
	return &hostedServiceRegistry{
		env:            &deleteQueueTestEnv{},
		terminators:    cmap.New[*edgeTerminator](),
		deleteSet:      map[string]*edgeTerminator{},
		pendingRemoves: map[string]*pendingRemoveBatch{},
		triggerEvalC:   make(chan struct{}, 1),
	}
}

// Test_hostedServiceRegistry_evaluateDeleteQueue_waitsForInFlightOperation covers the sequencing the
// controller's confirmed-absent fast path depends on. A delete sent while a create is still in flight
// can reach a controller that has not applied the create, which reads the id as absent; skipping there
// drops a delete for a terminator that is about to exist.
func Test_hostedServiceRegistry_evaluateDeleteQueue_waitsForInFlightOperation(t *testing.T) {
	t.Run("a terminator with an operation in flight stays queued", func(t *testing.T) {
		req := require.New(t)

		reg := newDeleteQueueTestRegistry()
		term := newDeletingTerminator("t1")
		term.operationActive.Store(true)
		term.lastAttempt = time.Now()
		reg.deleteSet[term.terminatorId] = term

		reg.evaluateDeleteQueue()

		req.Contains(reg.deleteSet, "t1", "the delete must stay queued, not be dropped until the retry scan")
		req.True(term.operationActive.Load(), "the in-flight operation must be left alone")
	})

	// The wait is bounded: an operation that never completes must not block the delete forever.
	t.Run("an operation past the timeout is presumed stuck and the delete proceeds", func(t *testing.T) {
		req := require.New(t)

		reg := newDeleteQueueTestRegistry()
		term := newDeletingTerminator("t1")
		term.operationActive.Store(true)
		term.lastAttempt = time.Now().Add(-2 * xgress_common.EstablishmentTimeout)
		reg.deleteSet[term.terminatorId] = term

		reg.evaluateDeleteQueue()

		req.False(term.operationActive.Load(), "a stuck operation must be abandoned so the delete can go")
		req.False(term.createConfirmed.Load(),
			"an abandoned create leaves the id unconfirmed, so the controller orders the delete")
	})

	t.Run("a terminator with nothing in flight is batched", func(t *testing.T) {
		req := require.New(t)

		reg := newDeleteQueueTestRegistry()
		term := newDeletingTerminator("t1")
		reg.deleteSet[term.terminatorId] = term

		reg.evaluateDeleteQueue()

		// The stub limiter reports full, so a batched terminator is requeued rather than sent. Either
		// way it left the queue and came back, which the skip path never does.
		req.Contains(reg.deleteSet, "t1")
		req.False(term.operationActive.Load())
	})
}

// Test_hostedServiceRegistry_queueRemoveTerminator_inFlightHandling pins which enqueue paths may
// clear the in-flight marker. Clearing it when queuing a delete is what let a delete be sent while a
// create was still outstanding; clearing it when requeuing after a failed remove is correct, because
// the operation that set it has ended.
func Test_hostedServiceRegistry_queueRemoveTerminator_inFlightHandling(t *testing.T) {
	t.Run("queuing a delete leaves an in-flight operation marked", func(t *testing.T) {
		req := require.New(t)

		reg := newDeleteQueueTestRegistry()
		term := newDeletingTerminator("t1")
		term.operationActive.Store(true)

		reg.queueRemoveTerminatorUnchecked(term, "test")

		req.Contains(reg.deleteSet, "t1")
		req.True(term.operationActive.Load(),
			"a create may still be outstanding; the delete queue waits for it rather than racing it")
	})

	t.Run("requeuing after a failed remove clears it", func(t *testing.T) {
		req := require.New(t)

		reg := newDeleteQueueTestRegistry()
		term := newDeletingTerminator("t1")
		term.operationActive.Store(true)

		reg.requeueRemoveTerminatorSync(term)

		req.Contains(reg.deleteSet, "t1")
		req.False(term.operationActive.Load(),
			"the remove that set the marker has ended, so the retry must not wait on it")
	})
}
