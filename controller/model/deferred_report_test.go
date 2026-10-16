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
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/stretchr/testify/require"
)

// deferReportForTest stands in for a report arriving from a router, without needing a real one.
func deferReportForTest(mgr *LinkManager, linkId, destId, srcId string) deferOutcome {
	return mgr.deferReport(&ctrl_pb.RouterLinks_RouterLink{
		Id:           linkId,
		DestRouterId: destId,
		LinkProtocol: "tls",
		Iteration:    1,
	}, &Router{BaseEntity: models.BaseEntity{Id: srcId}})
}

// Test_DeferReport_ReplacesRatherThanDuplicating: a router re-reports the same link on every notification
// cycle while a controller is behind, and each arrival is the same question, not a new one.
func Test_DeferReport_ReplacesRatherThanDuplicating(t *testing.T) {
	req := require.New(t)
	mgr := &LinkManager{}

	deferReportForTest(mgr, "l1", "unknown-dest", "src")
	deferReportForTest(mgr, "l1", "unknown-dest", "src")
	deferReportForTest(mgr, "l2", "unknown-dest", "src")

	req.Equal(2, mgr.DeferredReportCount())
}

// Test_DeferReport_IsBounded: a controller that never catches up is partitioned and cannot route, and every
// router announces its whole set again on reconnect, so the held set must not grow without limit.
func Test_DeferReport_IsBounded(t *testing.T) {
	req := require.New(t)
	mgr := &LinkManager{}

	for i := 0; i < maxDeferredReports+50; i++ {
		deferReportForTest(mgr, fmt.Sprintf("link-%d", i), "unknown-dest", "src")
	}

	req.Equal(maxDeferredReports, mgr.DeferredReportCount())
}

// Test_RetryDeferredReports_WaitsForCatchUp: retrying while still behind would reach the same wrong answer,
// so nothing is decided until the store is current.
func Test_RetryDeferredReports_WaitsForCatchUp(t *testing.T) {
	req := require.New(t)
	mgr := &LinkManager{}

	deferReportForTest(mgr, "l1", "unknown-dest", "src")
	mgr.RetryDeferredReports()

	req.Equal(1, mgr.DeferredReportCount(), "a held report must survive a retry that runs with no env")
}

// Test_DeferReport_SaysWhetherItHeldTheReport: a report that is not held is discarded, and the caller logs
// what happened. Reporting the intent rather than the outcome left a controller shedding reports saying it
// had deferred every one of them, which is the opposite of what an operator needs at that moment.
func Test_DeferReport_SaysWhetherItHeldTheReport(t *testing.T) {
	req := require.New(t)
	mgr := &LinkManager{}

	req.Equal(deferHeld, deferReportForTest(mgr, "l1", "unknown-dest", "src"))
	req.Equal(deferHeld, deferReportForTest(mgr, "l1", "unknown-dest", "src"), "the same link re-reported is still held")

	outcome := mgr.deferReport(&ctrl_pb.RouterLinks_RouterLink{Id: "l2", DestRouterId: "unknown-dest"}, nil)
	req.Equal(deferNoSource, outcome, "a report with no reporting router cannot be resolved later")
	req.Equal(1, mgr.DeferredReportCount())

	for i := 0; i < maxDeferredReports; i++ {
		deferReportForTest(mgr, fmt.Sprintf("fill-%d", i), "unknown-dest", "src")
	}
	req.Equal(maxDeferredReports, mgr.DeferredReportCount())

	req.Equal(deferSetFull, deferReportForTest(mgr, "overflow", "unknown-dest", "src"))
	req.Equal(deferHeld, deferReportForTest(mgr, "fill-0", "unknown-dest", "src"),
		"a report already held is replaced rather than refused, even with no room for a new one")
}

// Test_InspectDeferredReports_RendersTheHeldSet: the held set is otherwise invisible, so a controller
// holding thousands of reports and discarding the rest reads the same as one holding none.
func Test_InspectDeferredReports_RendersTheHeldSet(t *testing.T) {
	req := require.New(t)
	mgr := &LinkManager{}

	req.Empty(mgr.InspectDeferredReports().Reports, "an empty set renders as no reports, not as nil entries")

	deferReportForTest(mgr, "l1", "dest-1", "src-1")
	deferReportForTest(mgr, "l2", "dest-2", "src-2")

	result := mgr.InspectDeferredReports()
	req.Equal(2, result.Count)
	req.Equal(maxDeferredReports, result.Max)
	req.Equal(maxDeferredReportAttempts, result.MaxAttempts)
	req.Len(result.Reports, 2)

	byLink := map[string]DeferredLinkReport{}
	for _, r := range result.Reports {
		byLink[r.LinkId] = r
	}
	req.Equal("dest-1", byLink["l1"].DestRouterId)
	req.Equal("src-1", byLink["l1"].SrcRouterId)
	req.Equal("src-2", byLink["l2"].SrcRouterId)
	req.Zero(byLink["l1"].Attempts, "a report that has not been judged yet has made no attempts")
}

// Test_RetryDeferredReports_KeepsAReportDeferredWhileItRuns: a pass resolves every report it handled by
// deleting it, which is wrong for one that was deferred again while the pass was running. The motivating
// case is the destination going between the pass finding it and RouterReportedLink looking again, which
// hands the report straight back to deferReport; that window cannot be opened from outside, so the guard is
// exercised here through the other path into it, a router re-announcing the link mid-pass. Either way the
// fresh deferral has to outlive the pass, since nothing would say the link again.
func Test_RetryDeferredReports_KeepsAReportDeferredWhileItRuns(t *testing.T) {
	req := require.New(t)
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	lm := ctx.GetManagers().Link
	routers := ctx.GetManagers().Router

	src := NewRouter("src", "", "", 0, true)
	req.NoError(routers.Create(src, change.New()))
	routers.MarkConnected(src)

	dst := NewRouter("dst", "", "", 0, true)
	req.NoError(routers.Create(dst, change.New()))

	report := &ctrl_pb.RouterLinks_RouterLink{
		Id:           "l1",
		DestRouterId: dst.Id,
		LinkProtocol: "tls",
		DialAddress:  "tcp:localhost:1234",
		Iteration:    1,
	}
	req.Equal(deferHeld, lm.deferReport(report, src))

	// RouterReportedLink takes the link's stripe before doing anything else, so holding it parks the pass
	// between taking the report and resolving it.
	unlock := lm.linkLocks.LockFor(report.Id)

	done := make(chan struct{})
	go func() {
		defer close(done)
		lm.RetryDeferredReports()
	}()

	// The pass counts itself against the report while it takes it, under the lock, so an attempt recorded
	// means the report has been taken and the pass can get no further than the stripe.
	req.Eventually(func() bool {
		held := lm.InspectDeferredReports().Reports
		return len(held) == 1 && held[0].Attempts == 1
	}, time.Second, time.Millisecond, "the pass must take the report before the link is re-reported")

	req.Equal(deferHeld, lm.deferReport(report, src), "the reporting router announces the link again")

	unlock()
	<-done

	_, applied := lm.Get(report.Id)
	req.True(applied, "the pass must still apply the report it took")
	req.Equal(1, lm.DeferredReportCount(),
		"a report deferred again while the pass ran must survive it, or nothing says the link again")
}

// Test_RouterReportedLink_DefersOnlyForReportsWithNoOtherCopy: a report that cannot be judged yet is held
// only when nothing else is keeping it. A gossiped report is already in the gossip store and is re-read from
// there, so holding it here would be a second copy to keep in step with the first. A report from a router
// that does not gossip has no such backing and is lost if it is not held.
func Test_RouterReportedLink_DefersOnlyForReportsWithNoOtherCopy(t *testing.T) {
	req := require.New(t)
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	// Behind rather than caught up, which is what makes a destination unfindable mean "not yet" rather than
	// "deleted".
	ctx.RaftNotCaughtUp = true

	lm := ctx.GetManagers().Link
	routers := ctx.GetManagers().Router

	src := NewRouter("src", "", "", 0, true)
	req.NoError(routers.Create(src, change.New()))
	routers.MarkConnected(src)

	report := func(linkId string) *ctrl_pb.RouterLinks_RouterLink {
		return &ctrl_pb.RouterLinks_RouterLink{
			Id:           linkId,
			DestRouterId: "not-applied-here-yet",
			LinkProtocol: "tls",
			DialAddress:  "tcp:localhost:1234",
			Iteration:    1,
		}
	}

	link, outcome := lm.RouterReportedLink(report("gossiped"), src, nil)
	req.Nil(link)
	req.Equal(LinkReportDestinationUnknown, outcome,
		"a destination this controller has not applied yet is not an answer either way")
	req.Zero(lm.DeferredReportCount(),
		"a gossiped report is kept by the gossip store and must not be held a second time here")

	link, outcome = lm.RouterReportedLinkDeferrable(report("reported"), src, nil)
	req.Nil(link)
	req.Equal(LinkReportDestinationUnknown, outcome)
	req.Equal(1, lm.DeferredReportCount(), "a report with no other copy must be held")
}
