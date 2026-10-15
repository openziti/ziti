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
	"math"
	"sync"
	"time"

	"github.com/openziti/ziti/v2/common/concurrency"
	"github.com/openziti/ziti/v2/common/datastructures"
	"github.com/openziti/ziti/v2/common/logging"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/objectz"
	cmap "github.com/orcaman/concurrent-map/v2"
)

// linkLog is the logger for the controller's link manager. Its channel name is
// "controller.link".
var linkLog = logging.For("controller.link")

type LinkManager struct {
	linkTable      *linkTable
	linkLocks      *concurrency.StripedIdLocker
	initialLatency time.Duration
	models.BaseObjectStoreManager[*Link]
	// env is held rather than the router manager, which does not exist yet when this one is built. Nil in
	// tests that do not need router lookups.
	env Env
	// deferredReports holds reports refused only because this controller had not caught up, keyed by link
	// id. See RetryDeferredReports.
	deferredReports map[string]*deferredReport
	// deferredBarrier is the raft index the next evaluation pass waits for, taken at the end of the previous
	// pass. Shared rather than per report: passes are a minute apart and a report gets several, so the worst
	// case is that one deferred just after a barrier was taken spends its first pass looking at slightly old
	// state, which costs an attempt out of maxDeferredReportAttempts and is corrected on the next pass.
	//
	// Taken per pass rather than per deferral on purpose. A barrier that moved with every arrival would keep
	// receding while reports kept arriving, which is the starvation that made the plain caught-up check
	// unusable here.
	deferredBarrier     uint64
	deferredReportsLock sync.Mutex
}

// maxDeferredReports bounds the deferred set. A controller that never catches up is partitioned and cannot
// route anyway, and every router announces its full set again when it reconnects, so discarding the excess
// costs nothing that reconnecting will not restore.
const maxDeferredReports = 4096

// maxDeferredReportAttempts is how many barriers a report waits through before its destination is treated as
// genuinely absent. The commit index a follower knows trails the leader by up to one append, so reaching one
// barrier can still leave a just-committed create unseen; taking a fresh barrier each pass removes that.
const maxDeferredReportAttempts = 5

// DeferredLinkReportsGauge is how many link reports are held awaiting catch-up.
//
// Deliberately not under `link.`, which the metrics mappers read as a per-link namespace: a name there is
// split into a metric and a link id, and one with fewer than two dotted segments after the prefix panics
// the mapper rather than mapping oddly.
const DeferredLinkReportsGauge = "link_manager.deferred_reports"

// deferredReport is a link report held back because the destination was unknown while this controller was
// behind. The report is kept rather than the routers it named, since those are re-resolved on each retry.
type deferredReport struct {
	reportedLink *ctrl_pb.RouterLinks_RouterLink
	srcRouterId  string
	attempts     int

	// holds counts the times deferReport has held this report. A retry pass records the count it took the
	// report at and resolves it only while the count still matches, since handling a report can hand it
	// straight back and that fresh deferral has to outlive the pass that produced it.
	//
	// Read and written only under deferredReportsLock.
	holds uint64
}

// pendingReport is one held report as a retry pass took it, so the pass can act without holding
// deferredReportsLock and without reading a struct a concurrent report may be rewriting.
type pendingReport struct {
	linkId       string
	reportedLink *ctrl_pb.RouterLinks_RouterLink
	srcRouterId  string
	attempts     int
	holds        uint64
}

// deferOutcome says what deferReport did with a report, so a caller reporting on it can say which of the
// ways it was not held applies.
type deferOutcome int

const (
	// deferHeld means the report is held, and will be judged again once this controller has caught up.
	deferHeld deferOutcome = iota

	// deferNoSource means the report named no reporting router. Nothing re-resolves it, and nothing would
	// announce it again, so it is dropped.
	deferNoSource

	// deferSetFull means the held set is already at maxDeferredReports.
	deferSetFull
)

func NewLinkManager(env Env) *LinkManager {
	initialLatency := config.DefaultOptionsInitialLinkLatency
	if env != nil {
		initialLatency = env.GetConfig().Network.InitialLinkLatency
	}

	result := &LinkManager{
		linkTable:      newLinkTable(),
		linkLocks:      concurrency.NewStripedIdLocker(256),
		initialLatency: initialLatency,
		env:            env,
	}

	if env != nil {
		if reg := env.GetMetricsRegistry(); reg != nil {
			reg.FuncGauge(DeferredLinkReportsGauge, func() int64 {
				return int64(result.DeferredReportCount())
			})
		}
	}

	result.InitStore(objectz.NewObjectStore[*Link](func() objectz.ObjectIterator[*Link] {
		return datastructures.IterateCMap[*Link](result.linkTable.links)
	}))

	result.GetStore().AddStringSymbol("id", func(entity *Link) *string {
		return &entity.Id
	})
	result.GetStore().AddStringSymbol("protocol", func(entity *Link) *string {
		return &entity.Protocol
	})
	result.GetStore().AddStringSymbol("dialAddress", func(entity *Link) *string {
		return &entity.DialAddress
	})
	result.GetStore().AddStringSymbol("sourceRouter", func(entity *Link) *string {
		return &entity.GetSrc().Id
	})
	result.GetStore().AddStringSymbol("destRouter", func(entity *Link) *string {
		return &entity.DstId
	})
	result.GetStore().AddInt64Symbol("cost", func(entity *Link) *int64 {
		val := entity.GetCost()
		return &val
	})
	result.GetStore().AddInt64Symbol("staticCost", func(entity *Link) *int64 {
		val := int64(entity.GetStaticCost())
		return &val
	})
	result.GetStore().AddInt64Symbol("destLatency", func(entity *Link) *int64 {
		val := entity.GetDstLatency()
		return &val
	})
	result.GetStore().AddInt64Symbol("sourceLatency", func(entity *Link) *int64 {
		val := entity.GetSrcLatency()
		return &val
	})
	result.GetStore().AddStringSymbol("state", func(entity *Link) *string {
		val := entity.CurrentState().Mode.String()
		return &val
	})
	result.GetStore().AddInt64Symbol("iteration", func(entity *Link) *int64 {
		val := int64(entity.Iteration)
		return &val
	})

	return result
}

func (self *LinkManager) BaseLoad(id string) (*Link, error) {
	entity, found := self.Get(id)
	if !found {
		return nil, boltz.NewNotFoundError("link", "id", id)
	}
	return entity, nil
}

// BuildRouterLinks points every link destined for router at it, and adds each to router's link set. Safe
// to call repeatedly; links already pointing at router are left alone.
func (self *LinkManager) BuildRouterLinks(router *Router) {
	for _, linkId := range self.linkTable.linkIdsForRouter(router.Id) {
		self.buildRouterLink(router, linkId)
	}
}

// buildRouterLink pairs one link with router, taking the link's stripe so the pairing cannot interleave
// with a removal. Without it a removal that reads no destination completes while this is still deciding,
// and the link lands in router's link set after the table has dropped it, where it is still routed over.
func (self *LinkManager) buildRouterLink(router *Router, linkId string) {
	defer self.linkLocks.LockFor(linkId)()

	// Resolved under the stripe: a replacement may have taken this id since the index was read, and it is
	// the link the table holds now that has to be paired.
	link, found := self.Get(linkId)
	if !found {
		return
	}
	src := link.GetSrc()
	if src == nil {
		return
	}
	if link.DstId == router.Id && link.PointDestAt(router) {
		router.routerLinks.Add(link, src.Id)
	}
	// Symmetric to the destination side: a link built from a gossiped entry can hold a database-loaded
	// source router, and neighbour and path queries need the connected object rather than that placeholder.
	// Src links are keyed by DstId.
	if src.Id == router.Id && src != router {
		link.Src.Store(router)
		router.routerLinks.Add(link, link.DstId)
	}
}

// Add publishes link to the link table, to its endpoint routers' link sets, and to the per-router index.
// Serialized against any other change to the same link id: the table and the index are written in separate
// steps, and an interleaved removal would leave the link indexed but no longer in the table.
func (self *LinkManager) Add(link *Link) {
	defer self.linkLocks.LockFor(link.Id)()
	self.addLocked(link)
}

// addLocked publishes a link. The caller must hold the link's stripe.
func (self *LinkManager) addLocked(link *Link) {
	self.linkTable.add(link)
	link.GetSrc().routerLinks.Add(link, link.DstId)
	if dest := link.GetDest(); dest != nil {
		dest.routerLinks.Add(link, link.GetSrc().Id)
	}
}

func (self *LinkManager) has(link *Link) bool {
	return self.linkTable.has(link)
}

// destinationExists reports whether routerId names a router that still exists. The router cache answers
// most calls without a bolt transaction; a miss falls through to the store.
//
// True when there is no one to ask, so a link report is never refused on the strength of a missing lookup.
func (self *LinkManager) destinationExists(routerId string) bool {
	if self.env == nil {
		return true
	}
	router, err := self.env.GetManagers().Router.Read(routerId)
	return err == nil && router != nil
}

// isRaftCaughtUp reports whether this controller has applied everything the cluster is known to have
// committed. True when there is no env, which is the case in tests that do not exercise catch-up.
func (self *LinkManager) isRaftCaughtUp() bool {
	if self.env == nil {
		return true
	}
	return self.env.IsRaftCaughtUp()
}

// raftCommitIndex returns the barrier to wait for, or zero when there is no env, which is the case in tests
// that do not exercise catch-up. A zero barrier is immediately satisfied.
func (self *LinkManager) raftCommitIndex() uint64 {
	if self.env == nil {
		return 0
	}
	return self.env.GetRaftCommitIndex()
}

// deferReport holds a report whose destination is unknown while this controller is behind, so it can be
// re-evaluated once caught up, and reports whether it was held. Re-reporting the same link replaces the
// held copy and keeps its attempt count, since it is the same question being asked again.
//
// A report that is not held is discarded, so a caller that logs the deferral has to ask rather than assume.
func (self *LinkManager) deferReport(reportedLink *ctrl_pb.RouterLinks_RouterLink, src *Router) deferOutcome {
	if src == nil {
		return deferNoSource
	}

	self.deferredReportsLock.Lock()
	defer self.deferredReportsLock.Unlock()

	if self.deferredReports == nil {
		self.deferredReports = map[string]*deferredReport{}
	}

	if existing, found := self.deferredReports[reportedLink.Id]; found {
		// Same question asked again. The original barrier already covers the destination's creation, and it
		// is the earlier of the two, so keeping it resolves the report sooner.
		existing.reportedLink = reportedLink
		existing.srcRouterId = src.Id
		existing.holds++
		return deferHeld
	}

	if len(self.deferredReports) >= maxDeferredReports {
		return deferSetFull
	}

	self.deferredReports[reportedLink.Id] = &deferredReport{
		reportedLink: reportedLink,
		srcRouterId:  src.Id,
		holds:        1,
	}
	return deferHeld
}

// RetryDeferredReports re-evaluates reports held back because their destination was unknown while this
// controller was behind. A report is only judged once this controller has applied the index it was holding
// for, at which point failing to find the destination is a real answer rather than a symptom of lag.
//
// Each pass that still cannot find the destination takes a fresh barrier, so the next pass judges against
// newer state. After maxDeferredReportAttempts the destination is treated as genuinely gone and the report
// is dropped.
//
// Safe to call on a timer from a single goroutine; concurrent calls are serialized by the lock but gain
// nothing.
func (self *LinkManager) RetryDeferredReports() {
	if self.env == nil {
		return
	}

	// With no leader this controller is not receiving commits at all, so its view is frozen rather than
	// behind. Judging an absence now would mistake isolation for deletion, and waiting for an index it will
	// never be told about would never end.
	if !self.env.HasRaftLeader() {
		return
	}

	self.deferredReportsLock.Lock()
	barrier := self.deferredBarrier
	empty := len(self.deferredReports) == 0
	self.deferredReportsLock.Unlock()

	if empty {
		return
	}

	// Still short of what the cluster had committed when the last pass ran, so nothing here could tell us
	// anything new yet. Checked before this pass is counted against any report, so a pass that judges
	// nothing does not spend an attempt.
	if self.env.GetRaftIndex() < barrier {
		return
	}

	// Copy what each held report says, and count this pass against it, under the lock. A held report is
	// mutated in place by any concurrent re-report of the same link, so the pass works from a snapshot
	// rather than reading the shared struct while it runs.
	self.deferredReportsLock.Lock()
	pending := make([]pendingReport, 0, len(self.deferredReports))
	for id, report := range self.deferredReports {
		report.attempts++
		pending = append(pending, pendingReport{
			linkId:       id,
			reportedLink: report.reportedLink,
			srcRouterId:  report.srcRouterId,
			attempts:     report.attempts,
			holds:        report.holds,
		})
	}
	self.deferredReportsLock.Unlock()

	routers := self.env.GetManagers().Router
	var resolved []pendingReport

	for _, report := range pending {
		src := routers.GetConnected(report.srcRouterId)
		if src == nil {
			// The reporting router has gone. It announces its whole set again when it returns, so there is
			// nothing left here to hold.
			resolved = append(resolved, report)
			continue
		}

		dst := routers.GetConnected(report.reportedLink.DestRouterId)
		if dst != nil || self.destinationExists(report.reportedLink.DestRouterId) {
			self.RouterReportedLink(report.reportedLink, src, dst)
			resolved = append(resolved, report)
			linkLog.Info("applied deferred link report once its destination was known",
				"linkId", report.linkId,
				"destRouterId", report.reportedLink.DestRouterId)
			continue
		}

		if report.attempts >= maxDeferredReportAttempts {
			resolved = append(resolved, report)
			linkLog.Warn("dropping link report naming a destination router that does not exist",
				"linkId", report.linkId,
				"destRouterId", report.reportedLink.DestRouterId,
				"attempts", report.attempts)
		}
	}

	self.deferredReportsLock.Lock()
	for _, report := range resolved {
		// The destination can go between this pass finding it and RouterReportedLink looking again, which
		// hands the report back to deferReport. Resolving it here would undo a deferral the link manager
		// has just decided on, and nothing would say the link again.
		if held, found := self.deferredReports[report.linkId]; found && held.holds != report.holds {
			continue
		}
		delete(self.deferredReports, report.linkId)
	}
	// Anything still held is judged next pass against everything committed as of now.
	self.deferredBarrier = self.raftCommitIndex()
	self.deferredReportsLock.Unlock()
}

// DeferredReportCount returns how many reports are held awaiting catch-up.
func (self *LinkManager) DeferredReportCount() int {
	self.deferredReportsLock.Lock()
	defer self.deferredReportsLock.Unlock()
	return len(self.deferredReports)
}

// DeferredLinkReport is one held report, as rendered for inspection.
type DeferredLinkReport struct {
	LinkId       string `json:"linkId"`
	SrcRouterId  string `json:"srcRouterId"`
	DestRouterId string `json:"destRouterId"`
	Attempts     int    `json:"attempts"`
}

// DeferredLinkReports is what this controller is holding back for want of catching up.
type DeferredLinkReports struct {
	Count int `json:"count"`
	// Max is the point past which further reports are discarded rather than held.
	Max int `json:"max"`
	// MaxAttempts is how many evaluation passes a report survives before its destination is treated as gone.
	MaxAttempts int `json:"maxAttempts"`
	// Barrier is the raft index the next pass waits to reach before it judges anything. A barrier above the
	// applied index is why a non-empty set can sit untouched.
	Barrier uint64 `json:"barrier"`
	// AppliedIndex is what this controller has applied, for reading against Barrier.
	AppliedIndex uint64               `json:"appliedIndex"`
	Reports      []DeferredLinkReport `json:"reports"`
}

// InspectDeferredReports renders the held reports and the state that decides when they are next judged.
func (self *LinkManager) InspectDeferredReports() *DeferredLinkReports {
	self.deferredReportsLock.Lock()
	result := &DeferredLinkReports{
		Count:       len(self.deferredReports),
		Max:         maxDeferredReports,
		MaxAttempts: maxDeferredReportAttempts,
		Barrier:     self.deferredBarrier,
		Reports:     make([]DeferredLinkReport, 0, len(self.deferredReports)),
	}
	for _, report := range self.deferredReports {
		result.Reports = append(result.Reports, DeferredLinkReport{
			LinkId:       report.reportedLink.Id,
			SrcRouterId:  report.srcRouterId,
			DestRouterId: report.reportedLink.DestRouterId,
			Attempts:     report.attempts,
		})
	}
	self.deferredReportsLock.Unlock()

	// Read outside the lock: it reaches raft, and nothing here orders against it.
	if self.env != nil {
		result.AppliedIndex = self.env.GetRaftIndex()
	}
	return result
}

// ScanForDeadLinks removes links whose dialer is no longer connected to this controller, skipping any the
// keep predicate claims.
//
// Sound only for links the dialer re-announces on reconnect, which is what keep is for. A router that
// reports links the pre-gossip way sends its whole dialed set every time it reconnects, so dropping its
// links when it goes away costs nothing: it says them all again when it returns, and while it is away this
// controller cannot route through it in any case. A router that gossips deliberately does not re-announce,
// leaving that to anti-entropy, so dropping its links would strand them until a digest exchange noticed.
//
// The dialer is the only end considered. A link whose far end is gone is still the dialer's to retire, and
// it does so by faulting it. A link always has a source: every path that builds one stores a router, and
// disconnect clears the router's link set rather than the link's source.
//
// Every candidate is re-tested before it is removed. Walking the table and removing from it are separate
// passes, and on a large table the gap between them is wide enough for a dialer to reconnect and re-announce
// its links. Acting on the first pass's answer then removes a link the controller has just been told about,
// and a router that reports the pre-gossip way announces its set only on reconnect, so it does not say so
// again until it next connects.
func (self *LinkManager) ScanForDeadLinks(keep func(*Link) bool) {
	dead := func(link *Link) bool {
		if link.GetSrc().Connected.Load() {
			return false
		}
		return keep == nil || !keep(link)
	}

	var toRemove []*Link
	self.linkTable.links.IterCb(func(_ string, link *Link) {
		if dead(link) {
			toRemove = append(toRemove, link)
		}
	})

	for _, link := range toRemove {
		if !dead(link) {
			linkLog.Info("skipping retirement of a link whose dialer returned mid-scan",
				"linkId", link.Id,
				"iteration", link.Iteration,
				"srcRouterId", link.GetSrc().Id,
				"destRouterId", link.DstId)
			continue
		}

		src := link.GetSrc()

		// Whether the id is connected here under a different object is the distinction that matters when
		// this fires: a router that is genuinely away, versus a link still pointing at a connection that has
		// been replaced. The second is a stale pointer rather than a dead dialer, and retiring on it drops a
		// live router's link. The two want different fixes, and nothing else records which one happened.
		supersededSrc := false
		if self.env != nil {
			if current := self.env.GetManagers().Router.GetConnected(src.Id); current != nil && current != src {
				supersededSrc = true
			}
		}

		linkLog.Info("retiring link whose dialer is not connected here",
			"linkId", link.Id,
			"iteration", link.Iteration,
			"srcRouterId", src.Id,
			"destRouterId", link.DstId,
			"supersededSrc", supersededSrc,
			"removed", self.Remove(link))
	}
}

// repairDest points link at whichever router is currently connected for its destination id, and adds it to
// that router's link set. It covers a link recorded while that router was not connected, and one still
// pointing at a connection that has since been replaced. No-op if the router is absent or the link already
// points at it, so it is safe to call on every report. The link must already be in the link table.
func (self *LinkManager) repairDest(link *Link) {
	if self.env == nil || link.GetSrc() == nil {
		return
	}
	dst := self.env.GetManagers().Router.GetConnected(link.DstId)
	if dst == nil {
		return
	}
	if link.PointDestAt(dst) {
		dst.routerLinks.Add(link, link.GetSrc().Id)
		linkLog.Info("repaired link destination",
			"linkId", link.Id,
			"linkSrcRouterId", link.GetSrc().Id,
			"linkDestRouterId", link.DstId)
	}
}

// RouterReportedLink records a link a router has reported, returning the link and whether this report created
// it. There are three outcomes, and callers have to tell them apart:
//
//	(link, true)   the link was created, or an older iteration of it was replaced
//	(link, false)  the report matched a link already held, which is left untouched
//	(nil, false)   the report was refused, because the reporting router is not the link's source
//
// A nil link is the case worth naming: every caller has to check for it before using the result, since the
// refusal means there is no link to speak of rather than one that happens to be unchanged.
func (self *LinkManager) RouterReportedLink(reportedLink *ctrl_pb.RouterLinks_RouterLink, src, dst *Router) (*Link, bool) {
	// Striped by link id so concurrent reports for different links proceed in
	// parallel; reports for the same link still serialize. Replaces a single
	// global mutex that funneled every router's link reports through one lock.
	defer self.linkLocks.LockFor(reportedLink.Id)()

	// A nil dst means the destination is not connected here, which is ordinary: the link is recorded and
	// repaired when that router connects. It also covers a router that has been deleted, and that link can
	// never be repaired, since a recreated router is assigned a new id. Left alone it is a link to an entity
	// that does not exist: it counts, it validates, it offers itself for routing, and it cannot even be
	// rendered by the API, which fails the whole listing rather than one row.
	//
	// The routers that dialed it do not know the router is gone and re-announce the link on every reconnect,
	// so refusing here is what stops it coming back.
	if dst == nil && !self.destinationExists(reportedLink.DestRouterId) {
		// A destination this controller has not caught up to looks exactly like one that was deleted, and
		// the two want opposite answers. Refusing while behind silently drops a link to a router that does
		// exist, and the reporting router considers it delivered, so nothing ever says it again.
		if !self.isRaftCaughtUp() {
			log := linkLog.With(
				"linkId", reportedLink.Id,
				"destRouterId", reportedLink.DestRouterId)

			switch self.deferReport(reportedLink, src) {
			case deferHeld:
				log.Info("deferring link report naming an unknown destination until this controller catches up")
			case deferNoSource:
				log.Warn("dropping link report naming an unknown destination, it names no reporting router")
			case deferSetFull:
				log.Warn("dropping link report naming an unknown destination, no room to hold it",
					"deferredReports", maxDeferredReports)
			}
			return nil, false
		}

		linkLog.Warn("ignoring link report naming a destination router that no longer exists",
			"linkId", reportedLink.Id,
			"destRouterId", reportedLink.DestRouterId)
		return nil, false
	}

	link, _ := self.Get(reportedLink.Id)

	// Only the router that dialed a link may report on it. Link ids are generated by the dialing router and
	// reused across its re-dials, with the iteration distinguishing attempts, so a report naming an existing
	// link id from a different router is not a re-dial. Allowing it would let any router take over a link
	// between two others by reporting a higher iteration: the replacement below would drop the real link and
	// install one whose source is the impostor. Every caller passes the reporting router as src, so comparing
	// against it is comparing against who is making the claim.
	if link != nil {
		if existingSrc := link.GetSrc(); existingSrc != nil && src != nil && existingSrc.Id != src.Id {
			linkLog.Warn("ignoring link report from a router that is not the link's source",
				"routerId", src.Id,
				"linkId", reportedLink.Id,
				"linkSrcRouterId", existingSrc.Id,
				"iteration", reportedLink.Iteration,
				"linkIteration", link.Iteration)
			return nil, false
		}
	}

	if link != nil && link.Iteration >= reportedLink.Iteration {
		// A link built from a gossiped entry can hold a database-loaded source router. Repointing that is
		// BuildRouterLinks' job at connect time, not this path's.
		//
		// The destination is repaired here, since it can be left pointing at nothing or at a connection that
		// has since been replaced. Both are invisible in the table, so a later report is the only thing that
		// will notice.
		self.repairDest(link)
		return link, false
	}

	// remove the older link before adding the new one
	if link != nil {
		log := linkLog.With(
			"routerId", src.Id,
			"linkId", reportedLink.Id,
			"destRouterId", reportedLink.DestRouterId,
			"iteration", reportedLink.Iteration)

		oldIteration := link.Iteration
		self.removeLocked(link)
		log.Info("replaced link with newer iteration",
			"oldIteration", oldIteration,
			"newIteration", reportedLink.Iteration)
	}

	link = newLink(reportedLink.Id, reportedLink.LinkProtocol, reportedLink.DialAddress, self.initialLatency)
	link.Iteration = reportedLink.Iteration
	link.Src.Store(src)
	link.Dst.Store(dst)
	link.DstId = reportedLink.DestRouterId
	link.SetState(Connected)
	link.SetConnsState(reportedLink.ConnState)
	self.addLocked(link)

	// dst was resolved before the link was in the table, and under the source's connect stripe rather than
	// the destination's, so the destination may have connected or been replaced since.
	self.repairDest(link)

	return link, true
}

func (self *LinkManager) Get(linkId string) (*Link, bool) {
	return self.linkTable.get(linkId)
}

func (self *LinkManager) All() []*Link {
	return self.linkTable.all()
}

// RouterDeleted drops the deleted router's link index. A router id is only ever added to the index, so
// without this every router that has ever had a link is held for the controller's lifetime.
func (self *LinkManager) RouterDeleted(routerId string) {
	self.linkTable.dropRouterIndex(routerId)
}

// LinksForRouter returns the links with routerId at either end. Per-router work must use this rather than
// filtering the whole table, which is quadratic in the router count.
func (self *LinkManager) LinksForRouter(routerId string) []*Link {
	return self.linkTable.forRouter(routerId)
}

func (self *LinkManager) GetLinkMap() map[string]*Link {
	linkMap := make(map[string]*Link)
	self.linkTable.links.IterCb(func(key string, link *Link) {
		linkMap[key] = link
	})
	return linkMap
}

// Remove retracts link from the link table, its endpoint routers' link sets, and the per-router index,
// reporting whether it did. Removal is conditional on link still being the current incarnation: it does
// nothing and returns false when a newer iteration has taken its place, so a caller holding an older
// reference can tell it lost the race rather than assume it won.
// Serialized against any other change to the same link id, for the reason given on Add.
func (self *LinkManager) Remove(link *Link) bool {
	defer self.linkLocks.LockFor(link.Id)()
	return self.removeLocked(link)
}

// removeLocked retracts a link, reporting whether it did. The caller must hold the link's stripe.
func (self *LinkManager) removeLocked(link *Link) bool {
	if !self.linkTable.remove(link) {
		return false
	}
	link.GetSrc().routerLinks.Remove(link, link.DstId)
	if dest := link.GetDest(); dest != nil {
		dest.routerLinks.Remove(link, link.GetSrc().Id)
	}
	return true
}

func (self *LinkManager) ConnectedNeighborsOfRouter(router *Router) []*Router {
	neighborMap := make(map[string]*Router)

	links := router.routerLinks.GetLinks()
	for _, link := range links {
		dstRouter := link.GetDest()
		if dstRouter != nil && dstRouter.Connected.Load() && link.IsUsable() {
			if link.GetSrc().Id != router.Id {
				neighborMap[link.GetSrc().Id] = link.GetSrc()
			}
			if link.DstId != router.Id {
				neighborMap[link.DstId] = dstRouter
			}
		}
	}

	neighbors := make([]*Router, 0)
	for _, r := range neighborMap {
		neighbors = append(neighbors, r)
	}
	return neighbors
}

func (self *LinkManager) LeastExpensiveLink(a, b *Router) (*Link, bool) {
	var selected *Link
	var cost int64 = math.MaxInt64

	linksByRouter := a.routerLinks.GetLinksByRouter()
	links := linksByRouter[b.Id]
	for _, link := range links {
		if link.IsUsable() {
			linkCost := link.GetCost()
			if link.DstId == b.Id {
				if linkCost < cost {
					selected = link
					cost = linkCost
				}
			} else if link.GetSrc().Id == b.Id {
				if linkCost < cost {
					selected = link
					cost = linkCost
				}
			}
		}
	}

	if selected != nil {
		return selected, true
	}

	return nil, false
}

func (self *LinkManager) LinksInMode(mode LinkMode) []*Link {
	return self.linkTable.allInMode(mode)
}

/*
 * linkTable
 */

type linkTable struct {
	links cmap.ConcurrentMap[string, *Link]
	// byRouter indexes router id -> link ids, so per-router work does not have to scan the whole table.
	// It answers which links touch a router; links is what a link id currently resolves to.
	byRouter cmap.ConcurrentMap[string, *concurrency.LockedSet[string]]
}

func newLinkTable() *linkTable {
	return &linkTable{
		links:    cmap.New[*Link](),
		byRouter: cmap.New[*concurrency.LockedSet[string]](),
	}
}

// endpointIds returns the router ids a link should be indexed under, without duplicates.
func (lt *linkTable) endpointIds(link *Link) []string {
	ids := make([]string, 0, 2)
	if src := link.GetSrc(); src != nil {
		ids = append(ids, src.Id)
	}
	if link.DstId != "" && (len(ids) == 0 || link.DstId != ids[0]) {
		ids = append(ids, link.DstId)
	}
	return ids
}

// indexFor returns the link index for a router id, creating it if absent. Indexes are dropped only when
// their router is deleted; dropping one otherwise, an empty one included, could discard a link another
// goroutine had already fetched the index to record.
func (lt *linkTable) indexFor(routerId string) *concurrency.LockedSet[string] {
	if index, found := lt.byRouter.Get(routerId); found {
		return index
	}
	return lt.byRouter.Upsert(routerId, concurrency.NewLockedSet[string](),
		func(exists bool, existing, created *concurrency.LockedSet[string]) *concurrency.LockedSet[string] {
			if exists {
				return existing
			}
			return created
		})
}

// dropRouterIndex forgets a router's link index.
func (lt *linkTable) dropRouterIndex(routerId string) {
	lt.byRouter.Remove(routerId)
}

// linkIdsForRouter returns the ids indexed under the given router. Ids the table no longer holds are
// included; callers resolving them get nothing back for those.
func (lt *linkTable) linkIdsForRouter(routerId string) []string {
	index, found := lt.byRouter.Get(routerId)
	if !found {
		return nil
	}
	return index.Values()
}

// forRouter returns the links with the given router at either end.
func (lt *linkTable) forRouter(routerId string) []*Link {
	ids := lt.linkIdsForRouter(routerId)
	result := make([]*Link, 0, len(ids))
	for _, linkId := range ids {
		if link, ok := lt.links.Get(linkId); ok {
			result = append(result, link)
		}
	}
	return result
}

func (lt *linkTable) add(link *Link) {
	lt.links.Set(link.Id, link)
	for _, routerId := range lt.endpointIds(link) {
		lt.indexFor(routerId).Add(link.Id)
	}
}

func (lt *linkTable) get(linkId string) (*Link, bool) {
	return lt.links.Get(linkId)
}

func (lt *linkTable) has(link *Link) bool {
	if _, found := lt.links.Get(link.Id); found {
		return true
	}
	return false
}

func (lt *linkTable) all() []*Link {
	links := make([]*Link, 0, lt.links.Count())
	lt.links.IterCb(func(_ string, link *Link) {
		links = append(links, link)
	})
	return links
}

func (lt *linkTable) allInMode(mode LinkMode) []*Link {
	links := make([]*Link, 0)
	lt.links.IterCb(func(_ string, link *Link) {
		if link.CurrentState().Mode == mode {
			links = append(links, link)
		}
	})
	return links
}

func (lt *linkTable) remove(link *Link) bool {
	removed := lt.links.RemoveCb(link.Id, func(key string, v *Link, exists bool) bool {
		return v != nil && v.Iteration == link.Iteration
	})
	if removed {
		lt.unindex(link)
	}
	return removed
}

// unindex drops link's id from its endpoint indexes, unless the table holds a link under that id again.
//
// A replacement can take the table slot and index itself between a removal's table write and this call, and
// dropping the id then would leave a live link that per-router queries never find. A stale id is the safe
// direction to err in, since forRouter resolves ids against the table and skips the ones it no longer holds.
func (lt *linkTable) unindex(link *Link) {
	for _, routerId := range lt.endpointIds(link) {
		if index, found := lt.byRouter.Get(routerId); found {
			index.RemoveIf(link.Id, func() bool {
				_, live := lt.links.Get(link.Id)
				return !live
			})
		}
	}
}
