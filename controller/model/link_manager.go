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

func (self *LinkManager) ScanForDeadLinks() {
	var toRemove []*Link
	self.linkTable.links.IterCb(func(_ string, link *Link) {
		if !link.GetSrc().Connected.Load() {
			toRemove = append(toRemove, link)
		}
	})

	for _, link := range toRemove {
		self.Remove(link)
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

// RouterDeleted drops the deleted router's link index, unless the id has since come back.
//
// A router id can be reused: fabric router ids are the enrollment certificate's common name, so re-adding
// a router from the same cert reuses its id. Nothing orders this call against that, since store commit
// handlers run after bolt has released the writer lock, so it can execute long after the id was recreated,
// connected, and reported links. Dropping a live router's index is invisible in the link table and surfaces
// only when a per-router query fails to find links it should have.
//
// Re-reading the store closes that rather than narrowing it, because the read happens while the index map's
// shard is held. Indexing a link takes the same shard, so an entry for a live router cannot appear without
// its create having committed first, which this read would then see. Anything indexed while the store says
// the id is absent belongs to the deleted router.
func (self *LinkManager) RouterDeleted(routerId string) {
	self.linkTable.dropRouterIndexIf(routerId, func() bool {
		return !self.routerExists(routerId)
	})
}

// routerExists reports whether a router with this id is in the store. A nil env, which only happens in
// tests that do not build one, reports absent so cleanup still runs.
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

// Remove retracts link from the link table, its endpoint routers' link sets, and the per-router index.
// Serialized against any other change to the same link id, for the reason given on Add.
func (self *LinkManager) Remove(link *Link) {
	defer self.linkLocks.LockFor(link.Id)()
	self.removeLocked(link)
}

// removeLocked retracts a link. The caller must hold the link's stripe.
func (self *LinkManager) removeLocked(link *Link) {
	if self.linkTable.remove(link) {
		link.GetSrc().routerLinks.Remove(link, link.DstId)
		if dest := link.GetDest(); dest != nil {
			dest.routerLinks.Remove(link, link.GetSrc().Id)
		}
	}
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

// dropRouterIndexIf forgets a router's link index when shouldDrop agrees. shouldDrop runs with the index
// map's shard held, so it must not index a link, and callers relying on that exclusion should say so.
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
