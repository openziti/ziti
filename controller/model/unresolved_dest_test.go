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

package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/stretchr/testify/require"
)

// unresolvedDestTestCtx builds a manager with a connected source router, and returns a report naming a
// destination that does not exist in the store.
func unresolvedDestTestCtx(t *testing.T) (*TestContext, *LinkManager, *Router, func(linkId, destId string, iteration uint32) *ctrl_pb.RouterLinks_RouterLink) {
	t.Helper()
	req := require.New(t)
	ctx := NewTestContext(t)

	lm := ctx.GetManagers().Link
	src := NewRouter("src", "", "", 0, true)
	req.NoError(ctx.GetManagers().Router.Create(src, change.New()))
	ctx.GetManagers().Router.MarkConnected(src)

	report := func(linkId, destId string, iteration uint32) *ctrl_pb.RouterLinks_RouterLink {
		return &ctrl_pb.RouterLinks_RouterLink{
			Id:           linkId,
			DestRouterId: destId,
			LinkProtocol: "tls",
			DialAddress:  "tcp:localhost:1234",
			Iteration:    iteration,
		}
	}
	return ctx, lm, src, report
}

// Test_RouterReportedLinkUnverified_RecordsADestinationNotYetCaughtUpTo: a report from a router that does
// not gossip has no other copy, so a destination this controller has not applied yet cannot be refused
// without losing the link. It is recorded with the destination unresolved, and the sweep judges it later.
func Test_RouterReportedLinkUnverified_RecordsADestinationNotYetCaughtUpTo(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	link, outcome := lm.RouterReportedLinkUnverified(report("l1", "not-applied-yet", 1), src, nil)
	req.Equal(LinkReportCreated, outcome, "the report must be recorded rather than held or refused")
	req.NotNil(link)
	req.Nil(link.GetDest(), "with the destination left unresolved")
	req.Equal("not-applied-yet", link.DstId)

	_, found := lm.Get("l1")
	req.True(found, "the link table is the only copy of the report, so it has to hold it")
}

// Test_RouterReportedLinkUnverified_RecordsADestinationNamedForTheFirstTime: an absence is not judged on the
// first report naming a destination. The only evidence that the store holds the destination's create is a
// barrier requested after something named it, and nothing had. Refusing answers the router with a
// destination-removed notice, so being wrong tears down a live link.
func Test_RouterReportedLinkUnverified_RecordsADestinationNamedForTheFirstTime(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	link, outcome := lm.RouterReportedLinkUnverified(report("l1", "never-existed", 1), src, nil)
	req.Equal(LinkReportCreated, outcome, "admission records; the sweep is what judges")
	req.NotNil(link)

	// And the sweep settles it, once it has evidence.
	settled(ctx)
	req.Len(lm.SweepUnresolvedDestinations(), 1)
}

// Test_RouterReportedLink_LeavesTheGossipPathUnjudged: a gossiped report is kept by the gossip store and
// re-walked from there, so the destination it cannot judge is reported as such rather than recorded. Only
// the path with no other copy records an unverified destination.
func Test_RouterReportedLink_LeavesTheGossipPathUnjudged(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	link, outcome := lm.RouterReportedLink(report("gossiped", "not-applied-yet", 1), src, nil)
	req.Equal(LinkReportDestinationUnknown, outcome)
	req.Nil(link)

	_, found := lm.Get("gossiped")
	req.False(found, "the gossip store holds the entry; recording it here would be a second copy")
}

// settled supplies a read barrier covering every link recorded so far, as the leader would answer one.
func settled(ctx *TestContext) {
	ctx.SetReadBarrier(ctx.GetRaftIndex(), time.Now())
}

// Test_SweepUnresolvedDestinations_RemovesADestinationThatNeverArrives: the sweep is what settles a link
// recorded while this controller was behind. Once it can judge and the destination still is not there, the
// link is handed back for the caller to fault.
func Test_SweepUnresolvedDestinations_RemovesADestinationThatNeverArrives(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	_, outcome := lm.RouterReportedLinkUnverified(report("l1", "never-arrives", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	settled(ctx)

	unresolved := lm.SweepUnresolvedDestinations()
	req.Len(unresolved, 1, "the sweep must hand back what it judged, for the caller to fault")
	req.Equal("l1", unresolved[0].Id)
}

// Test_SweepUnresolvedDestinations_KeepsADestinationThatArrives: catching up is the ordinary outcome. A link
// whose destination turned out to exist must survive, whether or not that router is connected here.
func Test_SweepUnresolvedDestinations_KeepsADestinationThatArrives(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	_, outcome := lm.RouterReportedLinkUnverified(report("l1", "arrives-later", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	dst := NewRouter("arrives-later", "", "", 0, true)
	req.NoError(ctx.GetManagers().Router.Create(dst, change.New()))
	settled(ctx)

	req.Empty(lm.SweepUnresolvedDestinations(),
		"a destination that exists keeps its link, connected here or not")
	_, found := lm.Get("l1")
	req.True(found)
}

// Test_SweepUnresolvedDestinations_WaitsForABarrier is the condition that covers a follower
// replaying a backlog: between batches it can have applied everything it holds while the leader has
// committed far beyond it, so its own caught-up answer proves nothing. Only a commit index the leader
// reported after the link was recorded does.
func Test_SweepUnresolvedDestinations_WaitsForABarrier(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	_, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	req.Empty(lm.SweepUnresolvedDestinations(),
		"a follower that has heard nothing from the leader cannot judge an absence")

	// The leader answers where the cluster is, to a request sent after the link was recorded, and this
	// controller has applied that far.
	ctx.RaftIndex = 100
	ctx.SetReadBarrier(100, time.Now())

	req.Len(lm.SweepUnresolvedDestinations(), 1, "a report covering the link is what makes it judgeable")
}

// Test_SweepUnresolvedDestinations_WaitsToApplyTheBarrierIndex: hearing where the cluster is says
// nothing until this controller has applied that far.
func Test_SweepUnresolvedDestinations_WaitsToApplyTheBarrierIndex(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	_, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	ctx.RaftIndex = 50
	ctx.SetReadBarrier(100, time.Now())

	for i := 0; i < 3; i++ {
		req.Empty(lm.SweepUnresolvedDestinations(),
			"the create being asked about may sit between what is applied and what the leader reported")
	}

	ctx.RaftIndex = 100
	req.Len(lm.SweepUnresolvedDestinations(), 1)
}

// Test_SweepUnresolvedDestinations_IgnoresABarrierPredatingTheLink: a commit index the leader
// reported before the link arrived cannot speak for it. The destination was dialed, so its create was
// committed before the link was recorded, but that is no help if the report is older still.
func Test_SweepUnresolvedDestinations_IgnoresABarrierPredatingTheLink(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	ctx.RaftIndex = 100
	// A barrier whose request went out before the link was recorded.
	ctx.SetReadBarrier(100, time.Now())

	_, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	for i := 0; i < 3; i++ {
		req.Empty(lm.SweepUnresolvedDestinations(),
			"a report taken before the link arrived must not be read as covering it")
	}

	ctx.SetReadBarrier(100, time.Now())
	req.Len(lm.SweepUnresolvedDestinations(), 1, "a fresh round trip covering the link settles it")
}

// Test_SweepUnresolvedDestinations_IsolatedControllerJudgesNothing: with no leader this controller receives
// no commits, so its view is frozen rather than behind. Judging an absence then mistakes isolation for
// deletion, which is the failure recording an unverified destination exists to prevent.
func Test_SweepUnresolvedDestinations_IsolatedControllerJudgesNothing(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	_, outcome := lm.RouterReportedLinkUnverified(report("l1", "unreachable-truth", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	ctx.RaftLeaderless = true
	settled(ctx)

	for i := 0; i < 5; i++ {
		req.Empty(lm.SweepUnresolvedDestinations(),
			"an isolated controller must not conclude anything about a destination it cannot hear about")
	}

	_, found := lm.Get("l1")
	req.True(found)
}

// Test_SweepUnresolvedDestinations_LeavesTheSourceEndAlone: the index holds both of a link's routers, so a
// sweep driven off it sees the source as well. A link whose source is gone belongs to the deleted-router
// cleanup, which has a dialer to answer; claiming it here would race that.
func Test_SweepUnresolvedDestinations_LeavesTheSourceEndAlone(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	dst := NewRouter("dst", "", "", 0, true)
	req.NoError(ctx.GetManagers().Router.Create(dst, change.New()))

	_, outcome := lm.RouterReportedLinkUnverified(report("l1", dst.Id, 1), src, dst)
	req.Equal(LinkReportCreated, outcome)

	req.NoError(ctx.GetManagers().Router.Delete(src.Id, change.New()))
	settled(ctx)

	req.Empty(lm.SweepUnresolvedDestinations(), "the destination still exists, so this sweep has nothing to do")
	_, found := lm.Get("l1")
	req.True(found)
}

// Test_RemoveUnresolved_SparesADestinationThatArrived: the sweep scans the whole table before anything is
// removed, so a destination can be created in between. Removing then would drop a link that is now good.
func Test_RemoveUnresolved_SparesADestinationThatArrived(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	link, outcome := lm.RouterReportedLinkUnverified(report("l1", "arrives-mid-sweep", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)
	settled(ctx)

	req.Len(lm.SweepUnresolvedDestinations(), 1, "selected while the destination was still absent")

	// The create lands between the scan and the removal.
	dst := NewRouter("arrives-mid-sweep", "", "", 0, true)
	req.NoError(ctx.GetManagers().Router.Create(dst, change.New()))

	req.False(lm.RemoveUnresolved(link), "a destination that arrived before the removal must spare the link")
	_, stillThere := lm.Get("l1")
	req.True(stillThere)
}

// Test_RemoveUnresolved_SparesALinkReplacedSinceTheScan: a link id is reused across its dialer's re-dials,
// so a newer iteration under the same id is a different link from the one the scan picked.
func Test_RemoveUnresolved_SparesALinkReplacedSinceTheScan(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	stale, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	// The dialer re-dials and reports the same id at a higher iteration.
	_, outcome = lm.RouterReportedLinkUnverified(report("l1", "absent", 2), src, nil)
	req.Equal(LinkReportCreated, outcome)
	settled(ctx)

	req.False(lm.RemoveUnresolved(stale), "the iteration the scan picked is no longer the link at that id")

	current, found := lm.Get("l1")
	req.True(found, "and the live one must be untouched")
	req.True(lm.RemoveUnresolved(current), "the current iteration is still removable on its own terms")
}

// Test_SweepUnresolvedDestinations_LeavesAResolvedLinkToTheDelete: a link whose destination resolved could
// have been in a path, so when that router goes it is the router delete that retires it, and that path
// reroutes what was on it. The sweep reaching it first would remove and unindex it, and the delete would then
// find nothing to retire or reroute.
func Test_SweepUnresolvedDestinations_LeavesAResolvedLinkToTheDelete(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	gone := NewRouter("resolved-then-deleted", "", "", 0, true)
	req.NoError(ctx.GetManagers().Router.Create(gone, change.New()))
	link, outcome := lm.RouterReportedLinkUnverified(report("l1", gone.Id, 1), src, gone)
	req.Equal(LinkReportCreated, outcome)
	req.NotNil(link.GetDest(), "the destination resolved")

	// Deleted after the link was published, and before the delete's own cleanup has run, which this
	// manager has no network to run.
	req.NoError(ctx.GetManagers().Router.Delete(gone.Id, change.New()))

	settled(ctx)

	req.Empty(lm.SweepUnresolvedDestinations(), "a resolved link is the delete's to retire, not the sweep's")
	req.False(lm.RemoveUnresolved(link), "and must not be removable through the sweep's path either")

	_, found := lm.Get("l1")
	req.True(found)
}

// Test_DestinationExists_TreatsAnUnreadableStoreAsPresent: the sweep and the removal notice act on absence,
// so a read that failed must not read as a deletion. Answering "present" costs a link kept a while longer;
// answering "absent" tells a router to drop a destination that may be fine.
func Test_DestinationExists_TreatsAnUnreadableStoreAsPresent(t *testing.T) {
	req := require.New(t)
	ctx, lm, _, _ := unresolvedDestTestCtx(t)

	// Closing the store is the cheapest way to make a read fail rather than report a miss.
	ctx.Cleanup()

	req.True(lm.destinationExists("any-router"),
		"an unreadable store is not evidence the destination is gone")
}

// Test_FaultedWhileCatchingUp_RemovesTheLink is the resurrection a mixed-version soak turned up, stated as
// the property that stops it: a link reported while this controller was behind is in the table like any
// other, so the fault that follows it has something to remove. Holding the report instead left the fault
// with nothing to act on and recreated the link once the hold was released.
func Test_FaultedWhileCatchingUp_RemovesTheLink(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	link, outcome := lm.RouterReportedLinkUnverified(report("l1", "not-applied-yet", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	req.True(lm.Remove(link), "the fault path must find a link to remove")

	settled(ctx)
	lm.SweepUnresolvedDestinations()

	_, found := lm.Get("l1")
	req.False(found, "a faulted link must not come back when this controller catches up")
}

// Test_RouterReportedLinkUnverified_IgnoresABarrierPredatingTheDestination: a barrier requested before
// anything named the destination cannot speak for it, since the destination's create may have been committed
// after the barrier. Refusing would answer the router with a destination-removed notice and tear down a valid
// link.
func Test_RouterReportedLinkUnverified_IgnoresABarrierPredatingTheDestination(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	ctx.RaftIndex = 100

	// A barrier covering everything recorded so far, and this controller current by its own reckoning.
	ctx.SetReadBarrier(100, time.Now())

	link, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome,
		"the report must be recorded rather than refused on a barrier older than it")
	req.NotNil(link)
}

// Test_CanJudgeAbsence_IsKeptPerDestination: a destination is settled by a barrier requested after it was
// first named, whatever else arrives afterwards. A single clock across destinations never settles anything
// while arrivals keep coming, which a router re-dialing a deleted destination keeps doing, and the sweep
// that would answer it never runs.
func Test_CanJudgeAbsence_IsKeptPerDestination(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	ctx.RaftIndex = 100

	settledLink, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	for i := 0; i < 3; i++ {
		ctx.SetReadBarrier(100, time.Now())

		// Something naming another destination arrives after every barrier.
		uncovered, outcome := lm.RouterReportedLinkUnverified(report(fmt.Sprintf("other-%d", i), fmt.Sprintf("other-dest-%d", i), 1), src, nil)
		req.Equal(LinkReportCreated, outcome)

		req.True(lm.CanJudgeAbsence("absent"), "an arrival naming another destination says nothing about this one")
		swept := lm.SweepUnresolvedDestinations()
		req.Contains(swept, settledLink, "so the sweep settles it")
		req.NotContains(swept, uncovered, "and leaves the destination the barrier does not cover")
	}
}

// Test_CanJudgeAbsence_CoversEverythingNamingTheDestination: a router's create is committed before anything
// can name it, so once the first report naming it is covered, so is every later one, and a later report is
// refused as it arrives rather than recorded to wait for a barrier of its own.
func Test_CanJudgeAbsence_CoversEverythingNamingTheDestination(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	_, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)
	settled(ctx)

	link, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 2), src, nil)
	req.Nil(link)
	req.Equal(LinkReportDestinationAbsent, outcome, "a re-dial naming a settled destination is refused")

	link, outcome = lm.RouterReportedLink(report("gossiped", "absent", 1), src, nil)
	req.Nil(link)
	req.Equal(LinkReportDestinationAbsent, outcome, "on the gossip path as well, even on first sight")
}

// Test_NoteUnjudgedDestination_KeepsTheFirst: a destination named again while a barrier is on its way must
// not move past that barrier. Each later report would push it out of reach of the one in flight, and a router
// re-dialing faster than the round trip would keep it there.
func Test_NoteUnjudgedDestination_KeepsTheFirst(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	_, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	requestedAt := time.Now()
	_, outcome = lm.RouterReportedLinkUnverified(report("l1", "absent", 2), src, nil)
	req.Equal(LinkReportCreated, outcome, "named again before the barrier is answered")

	ctx.SetReadBarrier(ctx.GetRaftIndex(), requestedAt)
	req.True(lm.CanJudgeAbsence("absent"), "the barrier was requested after the first note, which is what counts")
}

// Test_RouterReportedLink_NotesWhatItCannotJudge: the gossip path holds nothing itself, so a destination it
// cannot judge has to be noted for a barrier to be asked for at all.
func Test_RouterReportedLink_NotesWhatItCannotJudge(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	_, outcome := lm.RouterReportedLink(report("gossiped", "absent", 1), src, nil)
	req.Equal(LinkReportDestinationUnknown, outcome)
	req.True(lm.NeedsReadBarrier())

	settled(ctx)
	_, outcome = lm.RouterReportedLink(report("gossiped", "absent", 1), src, nil)
	req.Equal(LinkReportDestinationAbsent, outcome)
}

// Test_PruneUnjudgedDestinations_ForgetsOnlyWhatIsSettled: the record of when a destination was first named
// is what lets a barrier settle it, so it is kept until a barrier has, and for a while after that so that
// reports still in flight are refused on arrival.
func Test_PruneUnjudgedDestinations_ForgetsOnlyWhatIsSettled(t *testing.T) {
	req := require.New(t)
	ctx, lm, _, _ := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	now := time.Now()
	lm.unjudgedDestinations = map[string]time.Time{
		"settled-long-ago": now.Add(-unjudgedDestinationRetention - 2*time.Minute),
		"unsettled":        now.Add(-unjudgedDestinationRetention - time.Minute),
		"settled-recently": now.Add(-time.Second),
	}
	// Requested between the first two notes, so it covers only the first.
	ctx.SetReadBarrier(0, now.Add(-unjudgedDestinationRetention-90*time.Second))

	lm.PruneUnjudgedDestinations()
	req.NotContains(lm.unjudgedDestinations, "settled-long-ago")
	req.Contains(lm.unjudgedDestinations, "unsettled",
		"no barrier covers it, so it is still owed one however long it has waited")
	req.Contains(lm.unjudgedDestinations, "settled-recently")

	settled(ctx)
	lm.PruneUnjudgedDestinations()
	req.NotContains(lm.unjudgedDestinations, "unsettled", "covered now, and old enough to forget")
	req.Contains(lm.unjudgedDestinations, "settled-recently", "covered, but recent enough to keep")
}

// Test_SweepUnresolvedDestinations_NotesAForgottenDestination: a destination forgotten while something still
// names it is noted again by the sweep, so it waits for another barrier rather than for ever.
func Test_SweepUnresolvedDestinations_NotesAForgottenDestination(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	link, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)
	settled(ctx)

	lm.absenceLock.Lock()
	delete(lm.unjudgedDestinations, "absent")
	lm.absenceLock.Unlock()

	req.Empty(lm.SweepUnresolvedDestinations(), "nothing settles a destination that is not noted")
	req.True(lm.NeedsReadBarrier(), "so the sweep notes it")

	settled(ctx)
	req.Equal([]*Link{link}, lm.SweepUnresolvedDestinations())
}

// Test_NeedsReadBarrier_OnlyWhenItWouldSaySomethingNew keeps the round trip rare: it is the cost of this
// design, so it is paid only by a controller actually holding something it cannot judge.
func Test_NeedsReadBarrier_OnlyWhenItWouldSaySomethingNew(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	req.False(lm.NeedsReadBarrier(), "nothing has been recorded that needs judging")

	_, outcome := lm.RouterReportedLinkUnverified(report("l1", "absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	req.True(lm.NeedsReadBarrier(), "now there is something no barrier in hand covers")

	ctx.SetReadBarrier(100, time.Now())
	req.False(lm.NeedsReadBarrier(), "and the answer covers it, so asking again would say nothing new")

	_, outcome = lm.RouterReportedLinkUnverified(report("l2", "also-absent", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)
	req.True(lm.NeedsReadBarrier(), "a destination named for the first time needs one of its own")
}

// Test_RouterReportedLink_DestinationDeletedWhilePublishing: a router delete retires its links by scanning the
// table after the delete commits, so a report holding the destination's connection from before the delete can
// publish after that scan, and nothing else would retire the link. Seen and then gone is a deletion, so it is
// settled on the spot rather than left for the sweep.
func Test_RouterReportedLink_DestinationDeletedWhilePublishing(t *testing.T) {
	entryPoints := map[string]func(*LinkManager) func(*ctrl_pb.RouterLinks_RouterLink, *Router, *Router) (*Link, LinkReportOutcome){
		"judged": func(lm *LinkManager) func(*ctrl_pb.RouterLinks_RouterLink, *Router, *Router) (*Link, LinkReportOutcome) {
			return lm.RouterReportedLink
		},
		"unverified": func(lm *LinkManager) func(*ctrl_pb.RouterLinks_RouterLink, *Router, *Router) (*Link, LinkReportOutcome) {
			return lm.RouterReportedLinkUnverified
		},
	}

	for name, entryPoint := range entryPoints {
		t.Run(name, func(t *testing.T) {
			req := require.New(t)
			ctx, lm, src, report := unresolvedDestTestCtx(t)
			defer ctx.Cleanup()

			dst := NewRouter("doomed", "", "", 0, true)
			req.NoError(ctx.GetManagers().Router.Create(dst, change.New()))
			req.NoError(ctx.GetManagers().Router.Delete(dst.Id, change.New()))

			link, outcome := entryPoint(lm)(report("l1", dst.Id, 1), src, dst)
			req.Nil(link)
			req.Equal(LinkReportDestinationAbsent, outcome)

			_, found := lm.Get("l1")
			req.False(found, "the link must not outlive the delete that should have retired it")
			req.False(lm.linkTable.byRouter.Has(dst.Id), "nor leave the deleted router's index behind")
			req.False(lm.NeedsReadBarrier(), "proved by the store, so nothing is left for the sweep")
		})
	}
}

// Test_SweepUnresolvedDestinations_FindsALinkRecordedAroundTheDelete: a router delete retires its links and
// then drops its index, so a link recorded between the two is in the table but not under its destination. The
// sweep is the only thing left that will retire it.
func Test_SweepUnresolvedDestinations_FindsALinkRecordedAroundTheDelete(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	link, outcome := lm.RouterReportedLinkUnverified(report("l1", "deleted", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)

	lm.RouterDeleted("deleted")
	req.False(lm.linkTable.byRouter.Has("deleted"), "the delete's index drop landed after the link was recorded")

	settled(ctx)
	req.Equal([]*Link{link}, lm.SweepUnresolvedDestinations())
	req.True(lm.RemoveUnresolved(link))
}

// Test_RemoveUnresolved_DropsTheAbsentDestinationsIndex: recording a link indexes it under a destination
// that, once the sweep removes it, is known not to exist and has no delete coming to drop that index.
func Test_RemoveUnresolved_DropsTheAbsentDestinationsIndex(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	link, outcome := lm.RouterReportedLinkUnverified(report("l1", "never-existed", 1), src, nil)
	req.Equal(LinkReportCreated, outcome)
	req.True(lm.linkTable.byRouter.Has("never-existed"))

	settled(ctx)
	req.Equal([]*Link{link}, lm.SweepUnresolvedDestinations())
	req.True(lm.RemoveUnresolved(link))

	req.False(lm.linkTable.byRouter.Has("never-existed"), "an index for a router that does not exist is never dropped otherwise")
	req.True(lm.linkTable.byRouter.Has(src.Id), "the source's index is its own router's to drop")
}

// Test_LinksWithEndpoint_FindsLinksWhoseIndexWasDropped: anything that finds a router gone may drop its index,
// a link report included, and can do so before the router's delete handler has retired its links. The handler
// still has to find them.
func Test_LinksWithEndpoint_FindsLinksWhoseIndexWasDropped(t *testing.T) {
	req := require.New(t)
	ctx, lm, src, report := unresolvedDestTestCtx(t)
	defer ctx.Cleanup()

	dst := NewRouter("deleted", "", "", 0, true)
	req.NoError(ctx.GetManagers().Router.Create(dst, change.New()))
	link, outcome := lm.RouterReportedLinkUnverified(report("l1", dst.Id, 1), src, dst)
	req.Equal(LinkReportCreated, outcome)

	req.NoError(ctx.GetManagers().Router.Delete(dst.Id, change.New()))
	lm.RouterDeleted(dst.Id)
	req.Empty(lm.LinksForRouter(dst.Id), "the index is gone")

	req.Equal([]*Link{link}, lm.LinksWithEndpoint(dst.Id), "but the link is still found by its destination")
	req.Equal([]*Link{link}, lm.LinksWithEndpoint(src.Id), "and by its source")
	req.Empty(lm.LinksWithEndpoint("bystander"))
}
