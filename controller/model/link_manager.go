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
)

// linkLog is the logger for the controller's link manager. Its channel name is
// "controller.link".
var linkLog = logging.For("controller.link")

type LinkManager struct {
	linkTable      *linkTable
	linkLocks      *concurrency.StripedIdLocker
	initialLatency time.Duration
	models.BaseObjectStoreManager[*Link]
	// env resolves a connected router when repairing a link's destination. Read at call time: the router
	// manager does not exist yet when this one is built. Nil in tests that do not need it.
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
		return &entity.Src.Id
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

func (self *LinkManager) BuildRouterLinks(router *Router) {
	// Only this router's links, not the whole table: this runs for every connect, and in a full mesh a
	// scan would cost the square of the router count.
	for _, link := range self.linkTable.forRouter(router.Id) {
		// PointDestAt rather than a plain store: a report can already have pointed this link at the connected
		// router (see repairDest), and indexing it again would double-count it, since the index is a slice.
		if link.DstId == router.Id && link.PointDestAt(router) {
			router.routerLinks.Add(link, link.Src.Id)
		}
	}
}

// Add publishes a link, serialized against any other change to the same link id.
//
// The link table and its per-router index are separate structures written in sequence, so without that
// serialization a removal can complete between the two writes: it finds the link in the table, removes it,
// finds nothing yet in the index, and the add then indexes a link the table no longer holds. That entry is
// never cleaned up, and per-router queries hand out a removed link indefinitely.
func (self *LinkManager) Add(link *Link) {
	defer self.linkLocks.LockFor(link.Id)()
	self.addLocked(link)
}

// addLocked publishes a link. The caller must hold the link's stripe.
func (self *LinkManager) addLocked(link *Link) {
	self.linkTable.add(link)
	link.Src.routerLinks.Add(link, link.DstId)
	if dest := link.GetDest(); dest != nil {
		dest.routerLinks.Add(link, link.Src.Id)
	}
}

func (self *LinkManager) has(link *Link) bool {
	return self.linkTable.has(link)
}

func (self *LinkManager) ScanForDeadLinks() {
	var toRemove []*Link
	self.linkTable.links.IterCb(func(_ string, link *Link) {
		if !link.Src.Connected.Load() {
			toRemove = append(toRemove, link)
		}
	})

	for _, link := range toRemove {
		self.Remove(link)
	}
}

// repairDest points a link at its destination router, and indexes it there, when the router is connected but
// the link does not know it.
//
// The two paths that establish that pairing can both miss it. A report resolves the destination before the
// link is in the table, and BuildRouterLinks repairs links already in the table when a router connects, so a
// router connecting between those two moments is seen by neither: the resolve found it absent, and the repair
// did not yet find the link.
//
// Re-reading here closes that window rather than narrowing it, but only because Network.ConnectRouter
// registers the router before building its links. A router whose build ran too early is therefore already in
// the connected map by the time the link lands in the table. Reverse that ordering and this repair can also
// miss, leaving the link with no destination and nothing left to notice.
//
// Left unrepaired the link is skipped by ConnectedNeighborsOfRouter, so it carries no adjacency and cannot be
// routed over, while every operator-facing view still reports it healthy.
func (self *LinkManager) repairDest(link *Link) {
	if self.env == nil || link.Src == nil {
		return
	}
	dst := self.env.GetManagers().Router.GetConnected(link.DstId)
	if dst == nil {
		return
	}
	if link.PointDestAt(dst) {
		dst.routerLinks.Add(link, link.Src.Id)
		linkLog.Info("repaired link destination for a router that connected while its link was being reported",
			"linkId", link.Id,
			"linkSrcRouterId", link.Src.Id,
			"linkDestRouterId", link.DstId)
	}
}

func (self *LinkManager) RouterReportedLink(reportedLink *ctrl_pb.RouterLinks_RouterLink, src, dst *Router) (*Link, bool) {
	// Striped by link id so concurrent reports for different links proceed in
	// parallel; reports for the same link still serialize. Replaces a single
	// global mutex that funneled every router's link reports through one lock.
	defer self.linkLocks.LockFor(reportedLink.Id)()

	link, _ := self.Get(reportedLink.Id)
	if link != nil && link.Iteration >= reportedLink.Iteration {
		// The destination is repaired here though, since a link already holding none is one that both paths
		// that set it have missed, and a later report is the only thing left that will notice.
		if link.GetDest() == nil {
			self.repairDest(link)
		}
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
	link.Src = src
	link.Dst.Store(dst)
	link.DstId = reportedLink.DestRouterId
	link.SetState(Connected)
	link.SetConnsState(reportedLink.ConnState)
	self.addLocked(link)

	// The destination was resolved by the caller before the link existed. Ask again now that it does: see
	// repairDest for why that is the moment the answer can differ.
	if dst == nil {
		self.repairDest(link)
	}

	return link, true
}

func (self *LinkManager) Get(linkId string) (*Link, bool) {
	return self.linkTable.get(linkId)
}

func (self *LinkManager) All() []*Link {
	return self.linkTable.all()
}

func (self *LinkManager) IterateLinks() <-chan cmap.Tuple[string, *Link] {
	return self.linkTable.links.IterBuffered()
}

func (self *LinkManager) GetLinkMap() map[string]*Link {
	linkMap := make(map[string]*Link)
	self.linkTable.links.IterCb(func(key string, link *Link) {
		linkMap[key] = link
	})
	return linkMap
}

// Remove retracts a link, serialized against any other change to the same link id. See Add for why the
// table and its index must not be updated concurrently for one link.
func (self *LinkManager) Remove(link *Link) {
	defer self.linkLocks.LockFor(link.Id)()
	self.removeLocked(link)
}

// removeLocked retracts a link. The caller must hold the link's stripe.
func (self *LinkManager) removeLocked(link *Link) {
	if self.linkTable.remove(link) {
		link.Src.routerLinks.Remove(link, link.DstId)
		if dest := link.GetDest(); dest != nil {
			dest.routerLinks.Remove(link, link.Src.Id)
		}
	}
}

func (self *LinkManager) ConnectedNeighborsOfRouter(router *Router) []*Router {
	neighborMap := make(map[string]*Router)

	links := router.routerLinks.GetLinks()
	for _, link := range links {
		dstRouter := link.GetDest()
		if dstRouter != nil && dstRouter.Connected.Load() && link.IsUsable() {
			if link.Src.Id != router.Id {
				neighborMap[link.Src.Id] = link.Src
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
			} else if link.Src.Id == b.Id {
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
	// byRouter indexes every link under both of its endpoint router ids, so per-router work does not have
	// to scan the whole table. A router's Link objects outlive the Router objects that reference it, and
	// a reconnect replaces the object but never the id, which is what makes an id-keyed index stable.
	byRouter cmap.ConcurrentMap[string, cmap.ConcurrentMap[string, *Link]]
}

func newLinkTable() *linkTable {
	return &linkTable{
		links:    cmap.New[*Link](),
		byRouter: cmap.New[cmap.ConcurrentMap[string, *Link]](),
	}
}

// endpointIds returns the router ids a link should be indexed under, without duplicates.
func (lt *linkTable) endpointIds(link *Link) []string {
	ids := make([]string, 0, 2)
	if link.Src != nil {
		ids = append(ids, link.Src.Id)
	}
	if link.DstId != "" && (len(ids) == 0 || link.DstId != ids[0]) {
		ids = append(ids, link.DstId)
	}
	return ids
}

// indexFor returns the link index for a router id, creating it if absent. Indexes are never removed once
// created: dropping an empty one could discard a link another goroutine had already fetched the index to
// record, and the number of routers is bounded anyway.
func (lt *linkTable) indexFor(routerId string) cmap.ConcurrentMap[string, *Link] {
	if index, found := lt.byRouter.Get(routerId); found {
		return index
	}
	return lt.byRouter.Upsert(routerId, cmap.New[*Link](),
		func(exists bool, existing, created cmap.ConcurrentMap[string, *Link]) cmap.ConcurrentMap[string, *Link] {
			if exists {
				return existing
			}
			return created
		})
}

// forRouter returns the links with the given router at either end.
func (lt *linkTable) forRouter(routerId string) []*Link {
	index, found := lt.byRouter.Get(routerId)
	if !found {
		return nil
	}
	result := make([]*Link, 0, index.Count())
	index.IterCb(func(linkId string, link *Link) {
		// The table is authoritative; the index is a cache written in a separate step. Identity rather than
		// presence also covers a replacement that has taken the table slot but not yet reindexed.
		if current, ok := lt.links.Get(linkId); ok && current == link {
			result = append(result, link)
		}
	})
	return result
}

func (lt *linkTable) add(link *Link) {
	lt.links.Set(link.Id, link)
	for _, routerId := range lt.endpointIds(link) {
		lt.indexFor(routerId).Set(link.Id, link)
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
		for _, routerId := range lt.endpointIds(link) {
			if index, found := lt.byRouter.Get(routerId); found {
				// Only if the index still refers to this link; a replacement can have indexed itself since
				// the primary removal.
				index.RemoveCb(link.Id, func(_ string, indexed *Link, _ bool) bool {
					return indexed == link
				})
			}
		}
	}
	return removed
}
