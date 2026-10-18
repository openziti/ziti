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
	"go.etcd.io/bbolt"
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

	// absenceLock guards unjudgedDestinations, which holds, per destination router id, when something naming
	// that router first arrived while this controller could not judge whether it exists.
	absenceLock          sync.Mutex
	unjudgedDestinations map[string]time.Time
}

// unjudgedDestinationRetention is how long after it is first noted a destination is remembered once settled,
// so that further reports naming it are refused on arrival rather than each waiting for a barrier of its own.
const unjudgedDestinationRetention = 10 * time.Minute

// NeedsReadBarrier reports whether a read barrier would let this controller judge a destination it holds
// something for: one has been noted, and the read barrier in hand was requested before it.
func (self *LinkManager) NeedsReadBarrier() bool {
	if self.env == nil {
		return false
	}
	_, requestedAt := self.env.GetReadBarrier()

	self.absenceLock.Lock()
	defer self.absenceLock.Unlock()
	for _, notedAt := range self.unjudgedDestinations {
		if !requestedAt.After(notedAt) {
			return true
		}
	}
	return false
}

// NoteUnjudgedDestination records that something naming routerId has arrived while this controller cannot
// judge whether that router exists. Only the first note counts, so it is safe to call on every arrival and
// every re-read: a router's create is committed before anything can name it, so a read barrier requested
// after the first arrival settles the router for every later one.
func (self *LinkManager) NoteUnjudgedDestination(routerId string) {
	now := time.Now()

	self.absenceLock.Lock()
	defer self.absenceLock.Unlock()
	if self.unjudgedDestinations == nil {
		self.unjudgedDestinations = map[string]time.Time{}
	}
	if _, found := self.unjudgedDestinations[routerId]; !found {
		self.unjudgedDestinations[routerId] = now
	}
}

// PruneUnjudgedDestinations forgets destinations noted more than ten minutes ago that a read barrier has since
// settled. Anything still naming one notes it again when next read, and waits for another barrier.
func (self *LinkManager) PruneUnjudgedDestinations() {
	if self.env == nil {
		return
	}

	self.absenceLock.Lock()
	var candidates []string
	for routerId, notedAt := range self.unjudgedDestinations {
		if time.Since(notedAt) > unjudgedDestinationRetention {
			candidates = append(candidates, routerId)
		}
	}
	self.absenceLock.Unlock()

	for _, routerId := range candidates {
		// Read outside the lock: CanJudgeAbsence takes it, and reads the env.
		if !self.CanJudgeAbsence(routerId) {
			continue
		}
		self.absenceLock.Lock()
		delete(self.unjudgedDestinations, routerId)
		self.absenceLock.Unlock()
	}
}

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
	if err != nil {
		if boltz.IsErrNotFoundErr(err) {
			return false
		}
		// Unreadable is not evidence the router is gone, and callers act on absence by taking links out of
		// service. Answering "exists" costs a report refused later rather than a live link removed now.
		linkLog.Warn("could not check whether a link destination exists, treating it as present",
			"destRouterId", routerId,
			"error", err)
		return true
	}
	return router != nil
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

// SweepUnresolvedDestinations returns the links that never resolved a destination and whose destination
// router does not exist, for the caller to remove with RemoveUnresolved. These are links recorded for a
// destination this controller could not yet judge (see RouterReportedLinkUnverified). Nothing resolved them,
// so none was ever selectable for a path, and removing one takes nothing out of service.
//
// A link whose destination did resolve is not returned, even if that router is now gone. The router delete
// retires it, and that path reroutes what was on it.
//
// A destination is judged once a read barrier requested after it was first noted has been applied up to (see
// CanJudgeAbsence):
//
//	the router dialed the destination, so its create was committed before anything naming it arrived
//	  <= the destination being first noted, on or after that arrival
//	  < the read barrier request, made after that
//	  <= the index the leader answered with
//	  <= what this controller has applied
//
// so the create would have been applied, and an absence now is a deletion. Router ids are not reused, so the
// same holds for every link naming the destination, whenever it arrived. A destination not yet covered is
// noted, so a barrier is asked for and a later sweep can judge it.
func (self *LinkManager) SweepUnresolvedDestinations() []*Link {
	if self.env == nil || !self.env.HasRaftLeader() {
		return nil
	}

	// Read from the table rather than the per-router index. A router delete drops its index after retiring
	// its links, so a link recorded between the two is in the table but not under its destination.
	var unresolved []*Link
	absent := map[string]bool{}
	for _, link := range self.linkTable.withoutDest() {
		gone, checked := absent[link.DstId]
		if !checked {
			gone = !self.destinationExists(link.DstId) && self.judgeAbsence(link.DstId)
			absent[link.DstId] = gone
		}
		if gone {
			unresolved = append(unresolved, link)
		}
	}

	return unresolved
}

// CanJudgeAbsence reports whether routerId missing from the store can be taken as a router that does not
// exist: true once a read barrier requested after the router was noted (see NoteUnjudgedDestination) has
// been applied up to. See SweepUnresolvedDestinations for why that is enough. False for a router never noted.
// Always true without an env.
func (self *LinkManager) CanJudgeAbsence(routerId string) bool {
	if self.env == nil {
		return true
	}

	self.absenceLock.Lock()
	notedAt, found := self.unjudgedDestinations[routerId]
	self.absenceLock.Unlock()
	if !found {
		return false
	}

	// Both times are this controller's own, so the comparison is causal rather than a clock agreement. A
	// barrier never answered has a zero time, which is after nothing.
	index, requestedAt := self.env.GetReadBarrier()
	if !requestedAt.After(notedAt) {
		return false
	}

	return self.env.GetRaftIndex() >= index
}

// judgeAbsence reports whether routerId can be judged absent, noting it when it cannot so that a barrier is
// asked for.
func (self *LinkManager) judgeAbsence(routerId string) bool {
	if self.CanJudgeAbsence(routerId) {
		return true
	}
	self.NoteUnjudgedDestination(routerId)
	return false
}

// RemoveUnresolved removes link if it is still the link the sweep selected, still has no destination, and
// that destination still does not exist, reporting whether it did. The link never resolved a destination, so
// it carries nothing and there is nothing to reroute.
//
// Decided and removed under the link's stripe. The sweep scans the whole table before anything is removed,
// so a destination can arrive, or a newer iteration take the id, in between.
func (self *LinkManager) RemoveUnresolved(link *Link) bool {
	defer self.linkLocks.LockFor(link.Id)()

	current, found := self.Get(link.Id)
	if !found || current != link || current.Iteration != link.Iteration {
		return false
	}
	if link.GetDest() != nil || self.destinationExists(link.DstId) {
		return false
	}

	link.SetState(Failed)
	self.removeLocked(link)
	// Recording the link indexed it under its destination, and nothing else drops that index for a router
	// that does not exist.
	self.RouterDeleted(link.DstId)
	return true
}

// LinkReportOutcome says what RouterReportedLink did with a report, so a caller can tell a refusal that
// settles the report from one that only means "not yet".
type LinkReportOutcome int

const (
	// LinkReportCreated means the link was recorded, either as new or as a newer iteration replacing one.
	LinkReportCreated LinkReportOutcome = iota

	// LinkReportCurrent means a link at this iteration or newer is already held, and was left untouched.
	LinkReportCurrent

	// LinkReportDestinationUnknown means the destination could not be found, and this controller cannot yet
	// tell a router it has not caught up to from one that was deleted. Nothing is decided: the destination is
	// noted, and whoever holds the report asks again once a read barrier covers it.
	LinkReportDestinationUnknown

	// LinkReportRefused means the report was rejected because the reporting router is not the link's source.
	// Asking again changes nothing.
	LinkReportRefused

	// LinkReportDestinationAbsent means the destination is gone, and this controller has proved it. Final for
	// the report, and the reporting router is owed word that the destination is gone: it would not be naming
	// one otherwise. Distinct from LinkReportRefused so a caller acts on the proof only where there is one.
	LinkReportDestinationAbsent
)

// RouterReportedLink records a link a router has reported, returning the link and what became of the report.
// The link is nil for every outcome but LinkReportCreated and LinkReportCurrent, so a caller using the link
// checks it rather than the outcome.
//
// A destination that cannot be found is refused once its absence is proved (see CanJudgeAbsence), and
// otherwise reported as LinkReportDestinationUnknown without being held here. For reports kept elsewhere, as
// gossiped reports are kept by the gossip store; a report with no such backing is recorded by
// RouterReportedLinkUnverified instead.
func (self *LinkManager) RouterReportedLink(reportedLink *ctrl_pb.RouterLinks_RouterLink, src, dst *Router) (*Link, LinkReportOutcome) {
	// Striped by link id so concurrent reports for different links proceed in
	// parallel; reports for the same link still serialize. Replaces a single
	// global mutex that funneled every router's link reports through one lock.
	defer self.linkLocks.LockFor(reportedLink.Id)()
	return self.routerReportedLink(reportedLink, src, dst, false)
}

// RouterReportedLinkUnverified records a link report whose destination this controller may not have caught
// up to, returning the link and what became of the report.
//
// For reports from routers that do not gossip. Nothing else keeps such a report, so a destination that
// cannot be found and whose absence is not yet proved is recorded with the destination left unresolved, and
// SweepUnresolvedDestinations removes the link if that destination still does not exist once the question
// can be answered. A destination proved absent, including one deleted while the report is being published,
// is reported as LinkReportDestinationAbsent.
func (self *LinkManager) RouterReportedLinkUnverified(reportedLink *ctrl_pb.RouterLinks_RouterLink, src, dst *Router) (*Link, LinkReportOutcome) {
	defer self.linkLocks.LockFor(reportedLink.Id)()
	return self.routerReportedLink(reportedLink, src, dst, true)
}

// routerReportedLink is the body of the entry points, with the link's stripe held. recordUnjudged keeps a
// link whose destination cannot be judged yet, rather than reporting it as unknown.
func (self *LinkManager) routerReportedLink(reportedLink *ctrl_pb.RouterLinks_RouterLink, src, dst *Router, recordUnjudged bool) (*Link, LinkReportOutcome) {

	// A nil dst means the destination is not connected here, which is ordinary: the link is recorded and
	// repaired when that router connects. It also covers a router that has been deleted, and that link can
	// never be repaired, since a recreated router is assigned a new id. Left alone it is a link to an entity
	// that does not exist: it counts, it validates, it offers itself for routing, and it cannot even be
	// rendered by the API, which fails the whole listing rather than one row.
	//
	// The routers that dialed it do not know the router is gone and re-announce the link on every reconnect,
	// so refusing here is what stops it coming back.
	destinationSeen := dst != nil || self.destinationExists(reportedLink.DestRouterId)
	if !destinationSeen {
		if self.CanJudgeAbsence(reportedLink.DestRouterId) {
			linkLog.Warn("ignoring link report naming a destination router that no longer exists",
				"linkId", reportedLink.Id,
				"destRouterId", reportedLink.DestRouterId)
			return nil, LinkReportDestinationAbsent
		}

		// A destination this controller has not caught up to looks exactly like one that was deleted, and
		// nothing in hand tells them apart yet. Noted, so a barrier is asked for.
		self.NoteUnjudgedDestination(reportedLink.DestRouterId)

		if !recordUnjudged {
			return nil, LinkReportDestinationUnknown
		}

		// The link table is the only copy of a report from a router that does not gossip, so keeping the
		// link is what keeps the report. SweepUnresolvedDestinations judges it once a barrier covers the
		// destination, and answers the dialer if it turns out to be gone.
		linkLog.Info("recording link report naming a destination this controller cannot yet judge",
			"linkId", reportedLink.Id,
			"destRouterId", reportedLink.DestRouterId)
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
			return nil, LinkReportRefused
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
		return link, LinkReportCurrent
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

	// A router delete retires its links by scanning the table after the delete commits, so a link published
	// after that scan is retired by nothing. Reading the store after publishing closes the gap: either the
	// read precedes the commit and the scan finds the link, or the read sees the delete. A destination seen
	// here and now absent was deleted, so no barrier is needed to say so.
	if destinationSeen && self.env != nil && !self.routerExists(link.DstId) {
		self.removeLocked(link)
		// Publishing may have recreated the deleted router's index after the delete dropped it.
		self.RouterDeleted(link.DstId)
		linkLog.Info("removed link published while its destination router was being deleted",
			"linkId", link.Id,
			"destRouterId", link.DstId)
		return nil, LinkReportDestinationAbsent
	}

	return link, LinkReportCreated
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
	self.linkTable.dropRouterIndexIf(routerId, func() bool {
		return !self.routerExists(routerId)
	})
}

// routerExists reports whether routerId is still in the store. An unreadable store answers true, since that
// is not evidence the router is gone. A manager built without an env answers false.
func (self *LinkManager) routerExists(routerId string) bool {
	if self.env == nil {
		return false
	}
	found := false
	if err := self.env.GetDb().View(func(tx *bbolt.Tx) error {
		found = self.env.GetStores().Router.IsEntityPresent(tx, routerId)
		return nil
	}); err != nil {
		// Unreadable is not evidence the router is gone, and keeping an index costs only its entry.
		linkLog.Warn("could not check whether router still exists, keeping its link index",
			"routerId", routerId,
			"error", err)
		return true
	}
	return found
}

// LinksForRouter returns the links with routerId at either end, from the per-router index. Per-router work
// must use this rather than filtering the whole table, which is quadratic in the router count. The index is
// dropped once its router no longer exists, so work on a router that may be gone uses LinksWithEndpoint.
func (self *LinkManager) LinksForRouter(routerId string) []*Link {
	return self.linkTable.forRouter(routerId)
}

// LinksWithEndpoint returns the links with routerId at either end by scanning the link table, so it finds
// them whether or not routerId's index is still held. For a router that may no longer exist; per-router work
// on a live router uses LinksForRouter.
func (self *LinkManager) LinksWithEndpoint(routerId string) []*Link {
	return self.linkTable.withEndpoint(routerId)
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

// dropRouterIndexIf forgets a router's link index when shouldDrop says to. shouldDrop runs while the
// index map's shard is held, so the decision cannot race a concurrent add under the same id.
func (lt *linkTable) dropRouterIndexIf(routerId string, shouldDrop func() bool) {
	lt.byRouter.RemoveCb(routerId, func(_ string, _ *concurrency.LockedSet[string], _ bool) bool {
		return shouldDrop()
	})
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

// withEndpoint returns the links with routerId at either end.
func (lt *linkTable) withEndpoint(routerId string) []*Link {
	var links []*Link
	lt.links.IterCb(func(_ string, link *Link) {
		if link.HasEndpoint(routerId) {
			links = append(links, link)
		}
	})
	return links
}

// withoutDest returns the links whose destination has not resolved.
func (lt *linkTable) withoutDest() []*Link {
	var links []*Link
	lt.links.IterCb(func(_ string, link *Link) {
		if link.GetDest() == nil {
			links = append(links, link)
		}
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
