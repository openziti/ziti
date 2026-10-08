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
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/openziti/ziti/v2/common/logcontext"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
	"github.com/stretchr/testify/require"
)

// busyLimiter refuses every command while busy is set, the way a controller under raft
// backpressure does.
type busyLimiter struct {
	busy atomic.Bool
}

func (self *busyLimiter) RunRateLimited(f func() error) error {
	if self.busy.Load() {
		return errors.New("controller busy")
	}
	return f()
}

func (self *busyLimiter) GetQueueFillPct() float64 { return 0 }

func newUnusableTestNetwork(ctx *model.TestContext) (*Network, *busyLimiter, func()) {
	limiter := &busyLimiter{}
	ctx.GetCommandDispatcher().(*command.LocalDispatcher).Limiter = limiter

	config := newTestConfig(ctx)
	network, err := NewNetwork(config, ctx)
	ctx.NoError(err)
	return network, limiter, func() { close(config.closeNotify) }
}

func sendFailedRouteStatus(ctx *model.TestContext, network *Network, router *model.Router, term *model.Terminator, errCode byte) CircuitError {
	rs := network.newRouteSender("circuit")
	defer network.removeRouteSender(rs)

	status := &RouteStatus{
		Router:    router,
		ErrorCode: &errCode,
		Attempt:   1,
		Err:       "terminator test failure",
	}
	path := &model.Path{Nodes: []*model.Router{router}}
	_, _, cerr := rs.handleRouteSend(1, path, xt_smartrouting.NewFactory().NewStrategy(), status, term, pfxlog.ChannelLogger("test"))
	ctx.Error(cerr)
	return cerr
}

func selectOnlyTerminator(ctx *model.TestContext, network *Network, router *model.Router, serviceId string) CircuitError {
	svc, err := network.Service.Read(serviceId)
	ctx.NoError(err)
	_, _, _, _, cerr := network.selectPath(newCircuitParams(svc, router), svc, "", logcontext.NewContext())
	return cerr
}

func TestRouteSender_InvalidTerminatorSkippedWhileDeleteRefused(t *testing.T) {
	ctx := model.NewTestContext(t)
	defer ctx.Cleanup()

	network, limiter, done := newUnusableTestNetwork(ctx)
	defer done()

	entityHelper := newTestEntityHelper(ctx, network)
	router := entityHelper.addTestRouter()
	svc := entityHelper.addTestService("svc")
	term := entityHelper.addTestTerminator(svc.Id, router.Id, "", false)
	term.Binding = "edge"

	limiter.busy.Store(true)
	cerr := sendFailedRouteStatus(ctx, network, router, term, ctrl_msg.ErrorTypeInvalidTerminator)
	ctx.Equal(CircuitFailureRouterErrInvalidTerminator, cerr.Cause())

	present, err := network.Terminator.IsEntityPresent(term.Id)
	ctx.NoError(err)
	ctx.True(present, "delete was refused, so the terminator is still stored")

	cerr = selectOnlyTerminator(ctx, network, router, svc.Id)
	ctx.Error(cerr, "the only terminator was reported invalid, so nothing is selectable")
	ctx.Equal(CircuitFailureNoOnlineTerminators, cerr.Cause())

	// A later report, once the controller accepts writes, deletes it.
	limiter.busy.Store(false)
	sendFailedRouteStatus(ctx, network, router, term, ctrl_msg.ErrorTypeInvalidTerminator)

	present, err = network.Terminator.IsEntityPresent(term.Id)
	ctx.NoError(err)
	ctx.False(present)

	// Delete listeners run after the commit, so the mark can outlive the stored terminator briefly.
	ctx.Eventually(func() bool {
		return !network.unusableTerminators.IsMarked(term.Id)
	}, 5*time.Second, 10*time.Millisecond, "applied delete clears the mark")
}

func TestRouteSender_UnusableTerminatorSkippedButNotDeleted(t *testing.T) {
	ctx := model.NewTestContext(t)
	defer ctx.Cleanup()

	network, _, done := newUnusableTestNetwork(ctx)
	defer done()

	entityHelper := newTestEntityHelper(ctx, network)
	router := entityHelper.addTestRouter()
	svc := entityHelper.addTestService("svc")
	term := entityHelper.addTestTerminator(svc.Id, router.Id, "", false)

	cerr := sendFailedRouteStatus(ctx, network, router, term, ctrl_msg.ErrorTypeUnusableTerminator)
	ctx.Equal(CircuitFailureRouterErrUnusableTerminator, cerr.Cause())

	present, err := network.Terminator.IsEntityPresent(term.Id)
	ctx.NoError(err)
	ctx.True(present, "an unusable terminator is not deleted")

	cerr = selectOnlyTerminator(ctx, network, router, svc.Id)
	ctx.Error(cerr)
	ctx.Equal(CircuitFailureNoOnlineTerminators, cerr.Cause())

	network.TerminatorEstablished(term.Id)
	ctx.NoError(selectOnlyTerminator(ctx, network, router, svc.Id), "an established terminator is selectable again")
}

func TestUnusableTerminators_UnmarkedOnCreateAndDelete(t *testing.T) {
	ctx := model.NewTestContext(t)
	defer ctx.Cleanup()

	network, _, done := newUnusableTestNetwork(ctx)
	defer done()

	entityHelper := newTestEntityHelper(ctx, network)
	router := entityHelper.addTestRouter()
	svc := entityHelper.addTestService("svc")

	// addTestTerminator numbers terminators from zero.
	network.unusableTerminators.Mark("terminator-#0")
	term := entityHelper.addTestTerminator(svc.Id, router.Id, "", false)
	ctx.Equal("terminator-#0", term.Id)
	ctx.Eventually(func() bool {
		return !network.unusableTerminators.IsMarked(term.Id)
	}, 5*time.Second, 10*time.Millisecond, "creating the terminator clears the mark")

	network.unusableTerminators.Mark(term.Id)
	ctx.NoError(network.Terminator.Delete(term.Id, change.New()))
	ctx.Eventually(func() bool {
		return !network.unusableTerminators.IsMarked(term.Id)
	}, 5*time.Second, 10*time.Millisecond, "deleting the terminator clears the mark")
}

func TestSelectPath_SkipsUnusableTerminators(t *testing.T) {
	ctx := model.NewTestContext(t)
	defer ctx.Cleanup()

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	network, err := NewNetwork(config, ctx)
	ctx.NoError(err)

	entityHelper := newTestEntityHelper(ctx, network)
	router := entityHelper.addTestRouter()
	svc := entityHelper.addTestService("svc")
	t0 := entityHelper.addTestTerminator(svc.Id, router.Id, "", false)
	t1 := entityHelper.addTestTerminator(svc.Id, router.Id, "", false)

	svc, err = network.Service.Read(svc.Id)
	ctx.NoError(err)
	ctx.Len(svc.Terminators, 2)

	selectTerminator := func() (string, CircuitError) {
		_, terminator, _, _, cerr := network.selectPath(newCircuitParams(svc, router), svc, "", logcontext.NewContext())
		if cerr != nil {
			return "", cerr
		}
		return terminator.GetId(), nil
	}

	network.unusableTerminators.Mark(t0.Id)
	for range 10 {
		id, cerr := selectTerminator()
		ctx.NoError(cerr)
		ctx.Equal(t1.Id, id)
	}

	network.unusableTerminators.Mark(t1.Id)
	_, cerr := selectTerminator()
	ctx.Error(cerr)
	ctx.Equal(CircuitFailureNoOnlineTerminators, cerr.Cause())

	network.unusableTerminators.Unmark(t0.Id)
	id, cerr := selectTerminator()
	ctx.NoError(cerr)
	ctx.Equal(t0.Id, id)
}

func TestUnusableTerminators_Expiry(t *testing.T) {
	req := require.New(t)
	set := newUnusableTerminators(20 * time.Millisecond)

	set.Mark("t0")
	req.True(set.IsMarked("t0"))
	req.False(set.IsMarked("t1"))

	req.Eventually(func() bool { return !set.IsMarked("t0") }, time.Second, 5*time.Millisecond)
	req.Equal(1, set.expiries.Count(), "expired entry lingers until a sweep")

	set.Mark("t1")
	req.False(set.expiries.Has("t0"), "marking sweeps expired entries")
	req.True(set.IsMarked("t1"))
}
