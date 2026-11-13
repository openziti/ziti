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
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/xt"
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

// selectTerminator runs path selection for serviceId from router and returns the id of the selected
// terminator. A selection failure is returned as a CircuitError.
func selectTerminator(network *Network, router *model.Router, serviceId string) (string, error) {
	svc, err := network.Service.Read(serviceId)
	if err != nil {
		return "", err
	}
	_, terminator, _, _, cerr := network.selectPath(newCircuitParams(svc, router), svc, "", logcontext.NewContext())
	if cerr != nil {
		return "", cerr
	}
	return terminator.GetId(), nil
}

// addPreferredTerminator creates a terminator with id for serviceId on routerId with required
// precedence, so path selection prefers it over default precedence terminators whatever their dynamic
// costs.
func addPreferredTerminator(network *Network, id, serviceId, routerId string) (*model.Terminator, error) {
	term := &model.Terminator{
		BaseEntity: models.BaseEntity{Id: id},
		Service:    serviceId,
		Router:     routerId,
		Address:    "ToDo",
		Precedence: xt.Precedences.Required,
	}
	return term, network.Terminator.Create(term, change.New())
}

func TestRouteSender_InvalidTerminatorPassedOverWhileDeleteRefused(t *testing.T) {
	ctx := model.NewTestContext(t)
	defer ctx.Cleanup()

	network, limiter, done := newUnusableTestNetwork(ctx)
	defer done()

	entityHelper := newTestEntityHelper(ctx, network)
	router := entityHelper.addTestRouter()
	svc := entityHelper.addTestService("svc")
	term, err := addPreferredTerminator(network, "preferred", svc.Id, router.Id)
	ctx.NoError(err)
	term.Binding = "edge"
	other := entityHelper.addTestTerminator(svc.Id, router.Id, "", false)

	limiter.busy.Store(true)
	cerr := sendFailedRouteStatus(ctx, network, router, term, ctrl_msg.ErrorTypeInvalidTerminator)
	ctx.Equal(CircuitFailureRouterErrInvalidTerminator, cerr.Cause())

	present, err := network.Terminator.IsEntityPresent(term.Id)
	ctx.NoError(err)
	ctx.True(present, "delete was refused, so the terminator is still stored")

	selected, err := selectTerminator(network, router, svc.Id)
	ctx.NoError(err)
	ctx.Equal(other.Id, selected, "path selection passes over the terminator reported invalid")

	// A later report, once the controller accepts writes, deletes it.
	limiter.busy.Store(false)
	cerr = sendFailedRouteStatus(ctx, network, router, term, ctrl_msg.ErrorTypeInvalidTerminator)
	ctx.Equal(CircuitFailureRouterErrInvalidTerminator, cerr.Cause())

	present, err = network.Terminator.IsEntityPresent(term.Id)
	ctx.NoError(err)
	ctx.False(present)

	// Delete listeners run after the commit, so the mark can outlive the stored terminator briefly.
	ctx.Eventually(func() bool {
		return !network.unusableTerminators.IsMarked(term.Id)
	}, 5*time.Second, 10*time.Millisecond, "applied delete clears the mark")
}

func TestRouteSender_UnusableTerminatorPassedOverButNotDeleted(t *testing.T) {
	ctx := model.NewTestContext(t)
	defer ctx.Cleanup()

	network, _, done := newUnusableTestNetwork(ctx)
	defer done()

	entityHelper := newTestEntityHelper(ctx, network)
	router := entityHelper.addTestRouter()
	svc := entityHelper.addTestService("svc")
	term, err := addPreferredTerminator(network, "preferred", svc.Id, router.Id)
	ctx.NoError(err)
	other := entityHelper.addTestTerminator(svc.Id, router.Id, "", false)

	cerr := sendFailedRouteStatus(ctx, network, router, term, ctrl_msg.ErrorTypeUnusableTerminator)
	ctx.Equal(CircuitFailureRouterErrUnusableTerminator, cerr.Cause())

	present, err := network.Terminator.IsEntityPresent(term.Id)
	ctx.NoError(err)
	ctx.True(present, "an unusable terminator is not deleted")

	selected, err := selectTerminator(network, router, svc.Id)
	ctx.NoError(err)
	ctx.Equal(other.Id, selected, "path selection passes over the terminator reported unusable")

	network.TerminatorEstablished(term.Id)
	ctx.False(network.unusableTerminators.IsMarked(term.Id), "establishing the terminator clears the mark")
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

func TestSelectPath_MarkedTerminators(t *testing.T) {
	t.Run("an unmarked terminator is selected over a marked one", func(t *testing.T) {
		// Arrange
		ctx := model.NewTestContext(t)
		defer ctx.Cleanup()
		network, _, done := newUnusableTestNetwork(ctx)
		defer done()

		entityHelper := newTestEntityHelper(ctx, network)
		router := entityHelper.addTestRouter()
		svc := entityHelper.addTestService("svc")
		marked, err := addPreferredTerminator(network, "preferred", svc.Id, router.Id)
		require.NoError(t, err)
		unmarked := entityHelper.addTestTerminator(svc.Id, router.Id, "", false)
		network.unusableTerminators.Mark(marked.Id)

		// Act
		selected, err := selectTerminator(network, router, svc.Id)

		// Assert
		require.NoError(t, err)
		require.Equal(t, unmarked.Id, selected, "a marked terminator isn't a candidate while an unmarked one is")
	})

	t.Run("the only terminator is selected while marked", func(t *testing.T) {
		// Arrange
		ctx := model.NewTestContext(t)
		defer ctx.Cleanup()
		network, _, done := newUnusableTestNetwork(ctx)
		defer done()

		entityHelper := newTestEntityHelper(ctx, network)
		router := entityHelper.addTestRouter()
		svc := entityHelper.addTestService("svc")
		marked := entityHelper.addTestTerminator(svc.Id, router.Id, "", false)
		network.unusableTerminators.Mark(marked.Id)

		// Act
		selected, err := selectTerminator(network, router, svc.Id)

		// Assert
		require.NoError(t, err, "a mark doesn't refuse the dial")
		require.Equal(t, marked.Id, selected)
	})

	t.Run("a marked terminator is selected when the unmarked one's router is offline", func(t *testing.T) {
		// Arrange
		ctx := model.NewTestContext(t)
		defer ctx.Cleanup()
		network, _, done := newUnusableTestNetwork(ctx)
		defer done()

		entityHelper := newTestEntityHelper(ctx, network)
		router := entityHelper.addTestRouter()
		offlineRouter := entityHelper.addTestRouter()
		network.Router.MarkDisconnected(offlineRouter)
		svc := entityHelper.addTestService("svc")
		marked := entityHelper.addTestTerminator(svc.Id, router.Id, "", false)
		entityHelper.addTestTerminator(svc.Id, offlineRouter.Id, "", false)
		network.unusableTerminators.Mark(marked.Id)

		// Act
		selected, err := selectTerminator(network, router, svc.Id)

		// Assert
		require.NoError(t, err)
		require.Equal(t, marked.Id, selected)
	})

	t.Run("a marked terminator on an offline router fails as no online terminators", func(t *testing.T) {
		// Arrange
		ctx := model.NewTestContext(t)
		defer ctx.Cleanup()
		network, _, done := newUnusableTestNetwork(ctx)
		defer done()

		entityHelper := newTestEntityHelper(ctx, network)
		router := entityHelper.addTestRouter()
		offlineRouter := entityHelper.addTestRouter()
		network.Router.MarkDisconnected(offlineRouter)
		svc := entityHelper.addTestService("svc")
		marked := entityHelper.addTestTerminator(svc.Id, offlineRouter.Id, "", false)
		network.unusableTerminators.Mark(marked.Id)

		// Act
		_, err := selectTerminator(network, router, svc.Id)

		// Assert
		var cerr CircuitError
		require.ErrorAs(t, err, &cerr)
		require.Equal(t, CircuitFailureNoOnlineTerminators, cerr.Cause())
	})
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
